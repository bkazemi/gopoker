package net

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type Room struct {
	name string

	clients *Clients

	table        *poker.Table
	tableAdminID string

	creatorToken string

	isLocked atomic.Bool
	mtx      sync.Mutex
}

func NewRoom(name string, table *poker.Table, creatorToken string) *Room {
	return &Room{
		name: name,

		clients: NewClients(),

		creatorToken: creatorToken,

		table: table,
	}
}

func (room *Room) Table() *poker.Table {
	return room.table.PublicInfo()
}

func (room *Room) Lock() {
	room.mtx.Lock()
	room.isLocked.Store(true)
}

func (room *Room) TryLock() bool {
	if room.mtx.TryLock() {
		room.isLocked.Store(true)

		return true
	}

	return false
}

func (room *Room) Unlock() {
	room.isLocked.Store(false)
	room.mtx.Unlock()
}

func (room *Room) IsLocked() bool {
	return room.isLocked.Load()
}

func (room *Room) sendResponseToAll(netData *NetData, except *Client) {
	if netData != nil && netData.room == nil {
		netData.room = room
	}

	for _, client := range room.clients.All() {
		if except == nil || client.ID != except.ID {
			netData.SendTo(client)
		}
	}
}

func (room *Room) getPlayerClient(player *poker.Player) *Client {
	if player == nil {
		log.Error().Str("room", room.name).Msg("player is nil")
		return nil
	}

	if client, ok := room.clients.ByPlayer(player); ok {
		return client
	}
	log.Warn().Str("room", room.name).Str("player", player.Name).Msg("player not found in playerClientMap")

	for _, client := range room.clients.All() {
		if client.Player != nil && client.Player.Name == player.Name {
			return client
		}
	}
	log.Warn().Str("room", room.name).Str("player", player.Name).Msg("player not found in connClientMap")

	return nil
}

func (room *Room) getPlayerConn(player *poker.Player) *websocket.Conn {
	if player == nil {
		log.Error().Str("room", room.name).Msg("player is nil")
		return nil
	}

	if client, ok := room.clients.ByPlayer(player); ok {
		return client.conn
	}
	log.Warn().Str("room", room.name).Str("player", player.Name).Msg("player not found in playerClientMap")

	for _, client := range room.clients.All() {
		if client.Player != nil && client.Player.Name == player.Name {
			return client.conn
		}
	}
	log.Warn().Str("room", room.name).Str("player", player.Name).Msg("player not found in connClientMap")

	return nil
}

func (room *Room) removeClient(client *Client) {
	room.table.Mtx().Lock()
	defer room.table.Mtx().Unlock()

	if client == nil {
		log.Error().Str("room", room.name).Msg("client is nil")
		return
	}

	room.clients.Remove(client)

	// NOTE: connections that don't become clients (e.g. in the case of a lock)
	//       never increment NumConnected
	room.table.NumConnected--

	netData := &NetData{
		Client:   client,
		Response: NetDataClientExited,
		Table:    room.Table(),
	}

	room.sendResponseToAll(netData, nil)
}

func (room *Room) removePlayer(client *Client, calledFromClientExit bool, movedToSpectator bool) {
	reset := false         // XXX race condition guard
	noPlayersLeft := false // XXX race condition guard

	room.table.Mtx().Lock()
	defer func() {
		log.Debug().Str("room", room.name).Msg("cleanup defer called")
		if reset {
			if calledFromClientExit || movedToSpectator {
				room.Lock()
			}

			if noPlayersLeft {
				log.Debug().Str("room", room.name).Msg("no players left, resetting")
				room.table.Reset(nil)
				room.sendReset(nil)
			} else if !calledFromClientExit && !movedToSpectator {
				log.Debug().Str("room", room.name).Msg("!calledFromClientExit, returning")
				return
			} else if room.table.State == poker.TableStateNotStarted {
				log.Debug().Str("room", room.name).Msg("state == TableStateNotStarted")
			} else {
				// XXX: if a player who hasn't bet preflop is
				//      the last player left he receives the mainpot chips.
				//      if he's a blind he should also get (only) his blind chips back.
				log.Debug().Str("room", room.name).Msg("state != (rndovr || gameovr)")

				room.table.FinishRound()
				room.table.State = poker.TableStateGameOver
				room.gameOver()
			}

			if calledFromClientExit || movedToSpectator {
				room.Unlock()
			}
		} else if room.table.State == poker.TableStateDoneBetting ||
			room.table.State == poker.TableStateRoundOver {
			if calledFromClientExit || movedToSpectator {
				room.Lock()
			}

			log.Debug().Str("room", room.name).Msg("defer postPlayerAction")
			room.postPlayerAction(nil, &NetData{})

			if calledFromClientExit || movedToSpectator {
				room.Unlock()
			}
		}
	}()
	defer room.table.Mtx().Unlock()

	table := room.table

	if player := client.Player; player != nil { // else client was a spectator
		playerName := player.Name
		hadDefaultName := client.Name == player.DefaultName()

		log.Debug().Str("room", room.name).Str("player", playerName).Msg("removing player")

		table.ActivePlayers().RemovePlayer(player)
		table.CurPlayers().RemovePlayer(player)

		table.NumPlayers--

		// use pointer identity — player and Dealer/SB/BB.Player point into
		// the same fixed seat pool, so address comparison is authoritative
		// and doesn't depend on name state (which Clear() resets)
		if table.Dealer != nil && player == table.Dealer.Player {
			table.Dealer = nil
		}
		if table.SmallBlind != nil && player == table.SmallBlind.Player {
			table.SmallBlind = nil
		}
		if table.BigBlind != nil && player == table.BigBlind.Player {
			table.BigBlind = nil
		}

		// wipe cards before building the notification — publicClientInfo
		// delegates to PublicPlayerInfo which skips redaction during
		// showdown, but a departing player's cards should never be broadcast
		player.NewCards()

		netData := &NetData{
			Client:   room.publicClientInfo(client),
			Response: NetDataPlayerLeft,
			Table:    room.Table(),
		}
		exceptClient := client
		if movedToSpectator {
			exceptClient = nil
		}
		room.sendResponseToAll(netData, exceptClient)

		// Sever client<->player link and clear the seat.
		room.clients.ClearPlayer(client)
		client.Player = nil
		player.Clear()

		if client.ID == room.tableAdminID {
			if table.ActivePlayers().Len == 0 {
				room.makeAdmin(nil)
			} else {
				activePlayerHeadClient := room.getPlayerClient(table.ActivePlayers().Head.Player)
				room.makeAdmin(activePlayerHeadClient)
			}
		}

		if hadDefaultName {
			// client had an empty/invalid name prior to becoming a player
			client.SetName("")
		}

		if table.NumPlayers < 2 {
			reset = true
			if table.NumPlayers == 0 {
				noPlayersLeft = true
				room.tableAdminID = ""
			}
			return
		}
	}
}

func (room *Room) sendPlayerTurn(client *Client) {
	if room.table.CurPlayer() == nil {
		log.Warn().Str("room", room.name).Msg("curPlayer is nil")
		return
	}

	curPlayer := room.table.CurPlayer().Player
	curPlayerClient := room.getPlayerClient(curPlayer)
	if curPlayerClient == nil {
		panic(fmt.Sprintf("Room.sendPlayerTurn(): {%s}: BUG: %s not found in connClientMap\n",
			room.name, curPlayer.Name))
	}

	netData := &NetData{
		room:     room,
		Client:   curPlayerClient,
		Response: NetDataPlayerTurn,
	}

	//netData.Client.Player.Action.Action = NetDataPlayerTurn

	netData.SendTo(client)

	if room.table.InBettingState() {
		room.sendPlayerHead(client, false)
	} else {
		room.sendPlayerHead(nil, true)
	}
}

func (room *Room) sendPlayerTurnToAll() {
	if room.table.CurPlayer() == nil {
		log.Warn().Str("room", room.name).Msg("curPlayer is nil")
		return
	}

	curPlayer := room.table.CurPlayer().Player
	curPlayerClient := room.getPlayerClient(curPlayer)
	if curPlayerClient == nil {
		panic(fmt.Sprintf("Room.sendPlayerTurnToAll(): {%s}: BUG: curPlayer <%s> not found in any maps\n",
			room.name, curPlayer.Name))
	}

	netData := &NetData{
		Client:   room.publicClientInfo(curPlayerClient),
		Response: NetDataPlayerTurn,
	}

	//netData.Client.Player.Action.Action = NetDataPlayerTurn

	room.sendResponseToAll(netData, nil)

	if room.table.InBettingState() {
		room.sendPlayerHead(nil, false)
	} else {
		room.sendPlayerHead(nil, true)
	}
}

// XXX: this response gets sent too often
func (room *Room) sendPlayerHead(client *Client, clear bool) {
	if clear {
		log.Debug().Str("room", room.name).Msg("sending clear player head")
		netData := &NetData{
			Response: NetDataPlayerHead,
		}
		if client == nil {
			room.sendResponseToAll(netData, nil)
		} else {
			netData.SendTo(client)
		}

		return
	}

	playerHeadNode := room.table.CurPlayers().Head
	curPlayerNode := room.table.CurPlayer()
	if playerHeadNode != nil && curPlayerNode != nil &&
		playerHeadNode.Player.Name != curPlayerNode.Player.Name {
		playerHead := playerHeadNode.Player
		playerHeadClient := room.getPlayerClient(playerHead)
		if playerHeadClient == nil {
			log.Warn().Str("room", room.name).Str("player", playerHead.Name).Msg("playerHead client not found, clearing")
			room.sendPlayerHead(client, true)
			return
		}

		netData := &NetData{
			Client:   room.publicClientInfo(playerHeadClient),
			Response: NetDataPlayerHead,
		}
		if client == nil {
			room.sendResponseToAll(netData, nil)
		} else {
			netData.SendTo(client)
		}
	}
}

func (room *Room) sendPlayerActionToAll(player *poker.Player, client *Client) {
	log.Debug().Str("room", room.name).Str("player", player.Name).Str("action", player.ActionToString()).Msg("sending player action")

	var c *Client
	if client == nil {
		c = room.getPlayerClient(player)
	} else {
		c = client
	}

	netData := &NetData{
		Client:   room.publicClientInfo(c),
		Response: NetDataPlayerAction,
		Table:    room.Table(),
	}

	room.sendResponseToAll(netData, c)

	if client != nil { // client is nil for blind auto allin corner case
		netData.Client.Player = player
		netData.SendTo(c)
	}
}

func (room *Room) sendDeals() {
	netData := &NetData{
		room:     room,
		Response: NetDataDeal,
		Table:    room.Table(),
	}

	for _, player := range room.table.CurPlayers().ToPlayerArray() {
		netData.Client = room.getPlayerClient(player)

		netData.Send()
	}
}

func (room *Room) sendHands() {
	netData := &NetData{
		room:     room,
		Response: NetDataShowHand,
		Table:    room.Table(),
	}

	for _, player := range room.table.GetNonFoldedPlayers() {
		client := room.getPlayerClient(player)
		//assert(client != nil, "Room.sendHands(): player not in playerMap")
		netData.Client = room.publicClientInfo(client)

		room.sendResponseToAll(netData, client)
	}
}

// NOTE: hand is currently computed on client side
func (room *Room) sendCurHands() {
	netData := &NetData{
		room:     room,
		Response: NetDataCurHand,
		Table:    room.Table(),
	}

	for _, client := range room.clients.Players() {
		poker.AssembleBestHand(true, room.table, client.Player)

		netData.Client = client
		netData.Msg = client.Player.PreHand().RankName()
		netData.Send()
	}
}

func (room *Room) sendActivePlayers(client *Client) {
	if client == nil {
		log.Warn().Str("room", room.name).Msg("client is nil")
		return
	}

	netData := &NetData{
		room:     room,
		Response: NetDataCurPlayers,
		Table:    room.Table(),
	}

	for _, player := range room.table.ActivePlayers().ToPlayerArray() {
		netData.Client = room.publicClientInfo(room.getPlayerClient(player))
		netData.SendTo(client)
	}
}

func (room *Room) sendAllPlayerInfo(client *Client, isCurPlayers bool, sendToSelf bool) {
	netData := &NetData{
		room:     room,
		Response: NetDataUpdatePlayer,
	}

	var players []*poker.Player
	if isCurPlayers {
		players = room.table.CurPlayers().ToPlayerArray()
	} else {
		// we use this instead of ActivePlayers because we need to preserve insertion order
		//
		// TODO: save table pos in Player instead
		players = room.table.GetOccupiedSeats()
	}

	for _, player := range players {
		playerClient := room.getPlayerClient(player)

		netData.Client = playerClient

		if client != nil {
			if !sendToSelf && playerClient.ID == client.ID {
				continue
			} else if playerClient.ID != client.ID {
				netData.Client = room.publicClientInfo(playerClient)
			}
			netData.SendTo(client)
		} else {
			netData.Send()
			netData.Client = room.publicClientInfo(playerClient)
			room.sendResponseToAll(netData, playerClient)
		}
	}
}

func (room *Room) sendTable(client *Client) {
	netData := &NetData{
		Client:   client,
		Response: NetDataUpdateTable,
		Table:    room.Table(),
	}

	if client == nil {
		room.sendResponseToAll(netData, nil)
	} else {
		netData.Send()
	}
}

func (room *Room) sendReset(winner *Client) {
	room.sendResponseToAll(&NetData{
		Client:   winner,
		Response: NetDataReset,
		Table:    room.Table(),
	}, nil)
}

func (room *Room) removeEliminatedPlayers() {
	netData := &NetData{Response: NetDataEliminated}

	for _, player := range room.table.GetEliminatedPlayers() {
		client := room.getPlayerClient(player)
		netData.Client = client
		netData.Response = NetDataEliminated
		netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", client.Player.Name,
			netData.Client.ID[:7])

		room.removePlayer(client, false, false)
		room.sendResponseToAll(netData, nil)
	}
}

func (room *Room) sendLock(conn *websocket.Conn, connType string) {
	log.Warn().Str("room", room.name).Str("lock", room.table.TableLockToString()).Msgf("locked out %p", conn)

	netData := &NetData{
		room:     room,
		Client:   &Client{conn: conn, connType: connType},
		Response: NetDataTableLocked,
		Msg:      fmt.Sprintf("table lock: %s", room.table.TableLockToString()),
	}

	netData.Send()

	time.Sleep(1 * time.Second)
}

func (room *Room) sendBadAuth(conn *websocket.Conn, connType string) {
	log.Warn().Str("room", room.name).Msgf("bad authentication from %p", conn)

	netData := &NetData{
		room:     room,
		Client:   &Client{conn: conn, connType: connType},
		Response: NetDataBadAuth,
		Msg:      "your password was incorrect",
	}

	netData.Send()

	time.Sleep(1 * time.Second)
}

func (room *Room) getRoomSettings() *RoomSettings {
	return &RoomSettings{
		RoomName: room.name,
		NumSeats: room.table.NumSeats,
		Lock:     room.table.Lock,
		Password: room.table.Password,
	}
}

func (room *Room) makeAdmin(client *Client) {
	if client == nil {
		log.Debug().Str("room", room.name).Msg("client is nil, unsetting tableAdmin")
		room.tableAdminID = ""
		return
	} else {
		log.Info().Str("room", room.name).Str("client", client.FullName(false)).Msg("making table admin")
		room.tableAdminID = client.ID
	}

	netData := &NetData{
		room:         room,
		Client:       client,
		RoomSettings: room.getRoomSettings(),
		Response:     NetDataMakeAdmin,
		Table:        room.Table(),
	}

	netData.Send()
}

func (room *Room) newRound() {
	room.table.NewRound()
	room.table.NextTableAction()
	room.checkBlindsAutoAllIn()
	room.sendDeals()
	room.sendCurHands()

	// XXX
	room.table.Mtx().Lock()
	realRoundState := room.table.State
	room.table.State = poker.TableStateNewRound
	room.sendAllPlayerInfo(nil, true, false)
	room.sendPlayerTurnToAll()
	room.table.State = realRoundState
	room.table.Mtx().Unlock()

	room.sendTable(nil)
}

// NOTE: called w/ room lock acquired in handleAsyncRequest()
func (room *Room) roundOver() {
	if room.table.State == poker.TableStateReset ||
		room.table.State == poker.TableStateNewRound {
		//room.mtx.Unlock()
		return
	}

	room.table.FinishRound()
	room.sendHands()

	netData := &NetData{
		Response: NetDataRoundOver,
		Table:    room.Table(),
		Msg:      room.table.WinInfo,
	}

	for i, sidePot := range room.table.SidePots().GetAllPots() {
		netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
	}

	room.sendResponseToAll(netData, nil)
	room.sendAllPlayerInfo(nil, false, true)

	room.removeEliminatedPlayers()

	if room.table.State == poker.TableStateGameOver {
		time.Sleep(5 * time.Second)
		room.gameOver()

		return
	}

	time.Sleep(5 * time.Second)
	room.newRound()
}

func (room *Room) gameOver() {
	log.Info().Str("room", room.name).Str("winner", room.table.Winners[0].Name).Msg("game over")
	winner := room.table.Winners[0]

	netData := &NetData{
		Response: NetDataServerMsg,
		Msg:      "game over, " + winner.Name + " wins",
	}

	room.sendResponseToAll(netData, nil)

	room.table.Reset(winner) // make a new game while keeping winner connected

	winnerClient := room.getPlayerClient(winner)
	if winnerClient == nil {
		log.Warn().Str("room", room.name).Str("winner", winner.Name).Msg("winner not found in any maps")
		room.makeAdmin(nil)
		room.sendReset(nil)
		return
	}

	if winnerClient.ID != room.tableAdminID {
		room.makeAdmin(winnerClient)
		room.sendPlayerTurnToAll()
	}
	room.sendReset(winnerClient)
}

// XXX: need to add to sidepots
func (room *Room) checkBlindsAutoAllIn() {
	if room.table.SmallBlind.Player.Action.Action == playerState.AllIn {
		log.Debug().Str("room", room.name).Str("player", room.table.SmallBlind.Player.Name).Msg("smallblind forced all-in")

		if room.table.CurPlayer().Player.Name == room.table.SmallBlind.Player.Name {
			// because blind is curPlayer SetNextPlayerTurn() will remove the blind
			// from the list for us
			room.table.SetNextPlayerTurn()
		} else {
			room.table.CurPlayers().RemovePlayer(room.table.SmallBlind.Player)
		}

		room.sendPlayerActionToAll(room.table.SmallBlind.Player, nil)
	}
	if room.table.BigBlind.Player.Action.Action == playerState.AllIn {
		log.Debug().Str("room", room.name).Str("player", room.table.BigBlind.Player.Name).Msg("bigblind forced all-in")

		if room.table.CurPlayer().Player.Name == room.table.BigBlind.Player.Name {
			// because blind is curPlayer SetNextPlayerTurn() will remove the blind
			// from the list for us
			room.table.SetNextPlayerTurn()
		} else {
			room.table.CurPlayers().RemovePlayer(room.table.BigBlind.Player)
		}

		room.sendPlayerActionToAll(room.table.BigBlind.Player, nil)
	}
}

func (room *Room) postBetting(player *poker.Player, netData *NetData, client *Client) {
	if player != nil {
		room.sendPlayerActionToAll(player, client)
		time.Sleep(2 * time.Second)
		room.sendPlayerTurnToAll()
	}

	log.Debug().Msg("done betting")

	if room.table.BettingIsImpossible() {
		log.Debug().Msg("no more betting possible this round")

		tmpReq := netData.Request
		tmpClient := netData.Client

		netData.Request = 0
		netData.Table = room.table
		netData.Client = nil

		tableState := room.table.State
		room.table.State = poker.TableStateShowHands
		room.sendHands()
		room.table.State = tableState

		for room.table.State != poker.TableStateRoundOver {
			room.table.NextCommunityAction()
			netData.Response = commState2NetDataResponse(room)

			room.sendResponseToAll(netData, nil)
			room.sendCurHands()

			time.Sleep(2500 * time.Millisecond)
		}

		netData.Request = tmpReq
		netData.Client = tmpClient
	} else {
		room.table.NextCommunityAction()
	}

	if room.table.State == poker.TableStateRoundOver {
		room.roundOver()

		//if room.table.State == poker.TableStateGameOver {
		//  ;
		//}
	} else { // new community card(s)
		netData.Response = commState2NetDataResponse(room)
		netData.Table = room.table
		if client != nil {
			netData.Client.Player = nil
		}

		room.sendResponseToAll(netData, nil)

		room.table.Bet = 0
		room.table.SetBetter(nil)

		for _, player := range room.table.CurPlayers().ToPlayerArray() {
			log.Debug().Str("player", player.Name).Msg("clearing action")
			player.Action.Clear()
		}

		room.sendAllPlayerInfo(nil, true, true)
		room.table.ReorderPlayers()
		room.sendPlayerTurnToAll()
		room.sendPlayerHead(nil, true)
		room.sendCurHands()
	}
}

func (room *Room) postPlayerAction(client *Client, netData *NetData) {
	var player *poker.Player
	if client != nil {
		player = client.Player
	}

	if room.table.State == poker.TableStateDoneBetting {
		room.postBetting(player, netData, client)
	} else if room.table.State == poker.TableStateRoundOver {
		// all other players folded before all comm cards were dealt
		// TODO: check for this state in a better fashion
		room.table.FinishRound()
		log.Debug().Int("numWinners", len(room.table.Winners)).Str("winner", room.table.Winners[0].Name).Msg("wins by folds")

		netData.Response = NetDataRoundOver
		netData.Table = room.table
		netData.Msg = room.table.Winners[0].Name + " wins by folds"
		if netData.HasClient() { // XXX ?
			netData.Client.Player = nil
		}

		room.sendResponseToAll(netData, nil)

		room.removeEliminatedPlayers()

		if room.table.State == poker.TableStateGameOver {
			room.gameOver()

			return
		}

		room.newRound()
	} else {
		room.sendPlayerActionToAll(player, client)
		time.Sleep(2 * time.Second)
		room.sendPlayerTurnToAll()
	}
}

func (room *Room) publicClientInfo(client *Client) *Client {
	pubClient := *client
	pubClient.Player = room.table.PublicPlayerInfo(*client.Player)

	return &pubClient
}

type RoomSettings struct {
	RoomName string
	NumSeats uint8
	Lock     poker.TableLock
	Password string
}

type ClientSettings struct {
	IsSpectator bool

	Name     string
	Password string

	SeatPos uint8
}

func NewClientSettings() *ClientSettings {
	return &ClientSettings{
		Name: "noname",
	}
}

func (room *Room) handleClientSettings(client *Client, settings *ClientSettings) (m string, err error) {
	defer func() {
		if err != nil {
			return // log err in caller to keep this defer small and clean
		}

		log.Debug().Str("client", client.FullName(false)).Msg(m)
	}()

	msg := ""

	const (
		MaxNameLen uint8 = 15
	)

	if client == nil { // NOTE: this is currently an impossible condition because the callers access client.ID beforehand
		return "", errors.New("room.handleClientSettings(): BUG: client == nil")
	} else if settings == nil {
		return "", errors.New("Room.handleClientSettings(): BUG: settings == nil")
	}

	settings.Name = strings.TrimSpace(settings.Name)
	if settings.Name != "" {
		if len(settings.Name) > int(MaxNameLen) {
			msg += fmt.Sprintf("You've requested a name that was longer than %v characters. "+
				"Using a default name.\n\n", MaxNameLen)
			settings.Name = ""
		} else {
			if player := client.Player; player != nil {
				if player.Name == settings.Name {
					msg += "name: unchanged\n\n"
				} else {
					_, found := room.clients.ByName(settings.Name)
					if !found {
						for _, defaultName := range room.table.DefaultPlayerNames() {
							if settings.Name == defaultName {
								found = true
								break
							}
						}
					} else {
						msg += fmt.Sprintf("Name '%s' already in use. Current name unchanged.\n\n",
							settings.Name)
					}
				}
			} else {
				for _, player := range *room.table.Players() {
					if settings.Name == player.Name {
						msg += fmt.Sprintf("Name '%s' already in use. Using a default name.\n\n",
							settings.Name)
						settings.Name = ""
						break
					}
				}
			}
		}
	}
	msg = "server response: settings changes:\n\n" + msg

	return msg, nil
}

func (room *Room) handleRoomSettings(client *Client, settings *RoomSettings) (m string, err error) {
	defer func() {
		if err != nil {
			return
		}

		log.Debug().Str("client", client.FullName(false)).Msg(m)
	}()

	const MaxPassLen uint8 = 50

	if client == nil {
		return "", errors.New("room.handleRoomSettings(): BUG: client == nil")
	} else if settings == nil {
		return "", errors.New("Room.handleRoomSettings(): BUG: settings == nil")
	} else if client.ID != room.tableAdminID {
		return "", errors.New("only the table admin can do that")
	}

	msg := ""
	errs := ""

	lock := poker.TableLockToString(settings.Lock)
	if lock == "" {
		errs += fmt.Sprintf("invalid table lock: '%v'\n", settings.Lock)
	} else if settings.Lock == room.table.Lock {
		msg += "table lock: unchanged\n"
	} else {
		msg += "table lock: changed\n"
	}

	if settings.Password != room.table.Password {
		msg += "table password: "
		if settings.Password == "" {
			msg += "removed\n"
		} else if len(settings.Password) > int(MaxPassLen) {
			return "", errors.New(fmt.Sprintf("Your password is too long. Please choose a "+
				"password that is less than %v characters.", MaxPassLen))
		} else {
			msg += "changed\n"
		}
	} else {
		msg += "table password: unchanged\n"
	}

	if errs != "" {
		return "", errors.New(errs)
	}

	return msg, nil
}

func commState2NetDataResponse(room *Room) NetAction {
	commStateNetDataMap := map[poker.TableState]NetAction{
		poker.TableStateFlop:  NetDataFlop,
		poker.TableStateTurn:  NetDataTurn,
		poker.TableStateRiver: NetDataRiver,
	}

	if netDataResponse, ok := commStateNetDataMap[room.table.CommState]; ok {
		return netDataResponse
	}

	log.Error().Msgf("bad commState `%v`", room.table.CommState)
	return NetDataBadRequest
}

func (room *Room) applyClientSettings(client *Client, settings *ClientSettings) {
	if settings == nil {
		log.Warn().Str("client", client.Name).Msg("nil ClientSettings, using defaults")
		settings = NewClientSettings()
	}
	client.Settings = settings

	if player := client.Player; player != nil {
		player.SetName(settings.Name)
		client.SetName(player.Name)
	} else {
		client.SetName(settings.Name)
	}
}

func (room *Room) applyRoomSettings(settings *RoomSettings) {
	if settings == nil {
		return
	}

	room.table.Mtx().Lock()
	room.table.Lock = settings.Lock
	room.table.Password = settings.Password
	room.table.Mtx().Unlock()
}

func (room *Room) newClient(conn *websocket.Conn, connType string, clientSettings *ClientSettings) *Client {
	room.Lock()
	defer room.Unlock()

	client, ID, privID := &Client{conn: conn, connType: connType, mtx: &sync.Mutex{}}, "", ""
	for {
		// 62^10 is plenty ;)
		ID = poker.RandString(10)
		privID = poker.RandString(10)

		_, foundID := room.clients.ByID(ID)
		_, foundPrivID := room.clients.ByPrivID(privID)
		if !foundID && !foundPrivID {
			client.ID = ID
			client.privID = privID

			room.clients.Register(client, conn)

			break
		} else {
			if foundID {
				log.Warn().Str("ID", ID).Msg("possible bug: ID already found in IDClientMap")
			}
			if foundPrivID {
				log.Warn().Str("privID", privID).Msg("possible bug: privID already found in privIDClientMap")
			}
		}
	}

	client.SetName(clientSettings.Name)
	if client.Name != "" {
		room.clients.SetName(client, client.Name)
	}

	return client
}

func (room *Room) isTableLocked() bool {
	room.table.Mtx().Lock()
	defer room.table.Mtx().Unlock()

	if room.table.Lock == poker.TableLockAll ||
		(room.table.Lock == poker.TableLockSpectators &&
			room.table.GetNumOpenSeats() == 0) {
		return true
	}

	return false
}
