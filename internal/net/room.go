package net

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/gorilla/websocket"
)

// TODO: important: need to ensure these pointers
// will be consistent throughout the program (mainly Player pointers)
type Room struct {
  name string

  connClientMap map[*websocket.Conn]*Client
  playerClientMap map[*poker.Player]*Client
  IDClientMap map[string]*Client
  privIDClientMap map [string]*Client
  nameClientMap map[string]*Client

  table *poker.Table
  tableAdminID string

  creatorToken string

  IsLocked bool
  mtx sync.Mutex
}

func NewRoom(name string, table *poker.Table, creatorToken string) *Room {
  return &Room{
    name: name,

    connClientMap: make(map[*websocket.Conn]*Client),
    playerClientMap: make(map[*poker.Player]*Client),
    IDClientMap: make(map[string]*Client),
    privIDClientMap: make(map[string]*Client),
    nameClientMap: make(map[string]*Client),

    creatorToken: creatorToken,

    table: table,
  }
}

// wrap the mutex Lock() so that we can check if the lock is active
// without acquiring it like TryLock() does.
func (room *Room) Lock() {
  room.mtx.Lock()
  room.IsLocked = true
}

func (room *Room) TryLock() bool {
  if room.mtx.TryLock() {
    room.IsLocked = true

    return true
  }

  return false
}

func (room *Room) Unlock() {
  room.IsLocked = false
  room.mtx.Unlock()
}

func (room *Room) sendResponseToAll(netData *NetData, except *Client) {
  if netData != nil && netData.room == nil {
    netData.room = room
  }

  for _, client := range room.connClientMap {
    if except == nil || client.ID != except.ID {
      netData.SendTo(client)
    }
  }
}

func (room *Room) getPlayerClient(player *poker.Player) *Client {
  if (player == nil ) {
    fmt.Printf("Room.getPlayerClient(): {%s}: player == nil\n", room.name)
    return nil
  }

  if client, ok := room.playerClientMap[player]; ok {
    return client
  }
  fmt.Printf("Room.getPlayerClient(): {%s}: WARNING: player (%s) not found in playerClientMap\n", room.name, player.Name)

  for _, client := range room.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return client
    }
  }
  fmt.Printf("Room.getPlayerClient(): {%s}: WARNING: player %s not found in connClientMap\n", room.name, player.Name)

  return nil
}

func (room *Room) getPlayerConn(player *poker.Player) *websocket.Conn {
  if (player == nil) {
    fmt.Printf("Room.getPlayerConn(): {%s}: player == nil\n", room.name)
    return nil
  }

  if client, ok := room.playerClientMap[player]; ok {
    return client.conn
  }
  fmt.Printf("Room.getPlayerConn(): {%s}: WARNING: player (%s) not found in playerClientMap\n", room.name, player.Name)

  for conn, client := range room.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return conn
    }
  }
  fmt.Printf("Room.getPlayerConn(): {%s}: WARNING: player %s not found in connClientMap\n", room.name, player.Name)

  return nil
}

func (room *Room) removeClient(client *Client) {
  room.table.Mtx().Lock()
  defer room.table.Mtx().Unlock()

  if (client == nil) {
    fmt.Printf("Room.removeClient(): {%s}: client == nil\n", room.name)
    return
  }

  delete(room.nameClientMap, client.Name)
  delete(room.IDClientMap, client.ID)
  delete(room.playerClientMap, client.Player)
  delete(room.privIDClientMap, client.privID)

  // NOTE: connections that don't become clients (e.g. in the case of a lock)
  //       never increment NumConnected
  room.table.NumConnected--

  netData := &NetData{
    Client:   client,
    Response: NetDataClientExited,
    Table:    room.table,
  }

  room.sendResponseToAll(netData, nil)
}

func (room *Room) removePlayer(client *Client, calledFromClientExit bool, movedToSpectator bool) {
  reset := false // XXX race condition guard
  noPlayersLeft := false // XXX race condition guard

  room.table.Mtx().Lock()
  defer func() {
    fmt.Printf("Room.removePlayer(): {%s}: cleanup defer CALLED\n", room.name);
    if reset {
      if calledFromClientExit || movedToSpectator {
        room.Lock()
      }

      if noPlayersLeft {
        fmt.Printf("Room.removePlayers(): {%s}: no players left, resetting\n", room.name)
        room.table.Reset(nil)
        room.sendReset(nil)
      } else if !calledFromClientExit && !movedToSpectator {
        fmt.Printf("Room.removePlayer(): {%s}: !calledFromClientExit, returning\n", room.name)
        return
      } else if room.table.State == poker.TableStateNotStarted {
        fmt.Printf("Room.removePlayer(): {%s}: State == poker.TableStateNotStarted\n", room.name)
      } else {
        // XXX: if a player who hasn't bet preflop is
        //      the last player left he receives the mainpot chips.
        //      if he's a blind he should also get (only) his blind chips back.
        fmt.Printf("Room.removePlayer(): {%s}: state != (rndovr || gameovr)\n", room.name)

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

      fmt.Printf("Room.removePlayer(): {%s}: defer postPlayerAction\n", room.name)
      room.postPlayerAction(nil, &NetData{})

      if calledFromClientExit || movedToSpectator {
        room.Unlock()
      }
    }
  }()
  defer room.table.Mtx().Unlock()

  table := room.table

  if player := client.Player; player != nil { // else client was a spectator
    fmt.Printf("Room.removePlayer(): {%s}: removing %s\n", room.name, player.Name)

    table.ActivePlayers().RemovePlayer(player)
    table.CurPlayers().RemovePlayer(player)

    player.Clear()

    table.NumPlayers--

    netData := &NetData{
      Client:     client,
      Response:   NetDataPlayerLeft,
      Table:      table,
    }
    exceptClient := client
    if movedToSpectator {
      exceptClient = nil
    }
    room.sendResponseToAll(netData, exceptClient)

    client.Player = nil
    delete(room.playerClientMap, player)

    if client.ID == room.tableAdminID {
      if table.ActivePlayers().Len == 0 {
        room.makeAdmin(nil)
      } else {
        activePlayerHeadClient := room.getPlayerClient(table.ActivePlayers().Head.Player)
        room.makeAdmin(activePlayerHeadClient)
      }
    }

    if table.NumPlayers < 2 {
      reset = true
      if table.NumPlayers == 0 {
        noPlayersLeft = true
        room.tableAdminID = ""
      }
      return
    }

    if table.Dealer != nil &&
       player.Name == table.Dealer.Player.Name {
      table.Dealer = nil
    }
    if table.SmallBlind != nil &&
       player.Name == table.SmallBlind.Player.Name {
      table.SmallBlind = nil
    }
    if table.BigBlind != nil &&
       player.Name == table.BigBlind.Player.Name {
      table.BigBlind = nil
    }
  }
}

func (room *Room) sendPlayerTurn(client *Client) {
  if room.table.CurPlayer() == nil {
    fmt.Printf("Room.sendPlayerTurn(): {%s}: curPlayer == nil\n", room.name)
    return
  }

  curPlayer := room.table.CurPlayer().Player
  curPlayerClient := room.getPlayerClient(curPlayer)
  if curPlayerClient == nil {
    panic(fmt.Sprintf("Room.sendPlayerTurn(): {%s}: BUG: %s not found in connClientMap\n",
                     room.name, curPlayer.Name))
  }

  netData := &NetData{
    room:       room,
    Client:     curPlayerClient,
    Response:   NetDataPlayerTurn,
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
    fmt.Printf("Room.sendPlayerTurnToAll(): {%s}: curPlayer == nil\n", room.name)
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
    fmt.Printf("Room.sendPlayerHead(): {%s}: sending clear player head\n", room.name)
    room.sendResponseToAll(
      &NetData{
        Response: NetDataPlayerHead,
      }, nil)

    return
  }

  playerHeadNode := room.table.CurPlayers().Head
  curPlayerNode := room.table.CurPlayer()
  if playerHeadNode != nil && curPlayerNode != nil &&
     playerHeadNode.Player.Name != curPlayerNode.Player.Name {
    playerHead := playerHeadNode.Player
    playerHeadClient := room.getPlayerClient(playerHead)
    if playerHead == nil {
      panic(fmt.Sprintf("Room.sendPlayerTurnToAll(): {%s}: BUG: playerHead <%s> " +
                        "not found in any maps\n", room.name, playerHead.Name))
    }

    netData := &NetData {
      Client: room.publicClientInfo(playerHeadClient),
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
  fmt.Printf("Room.sendPlayerActionToAll(): {%s}: <%s> sending %s\n",
             room.name, player.Name, player.ActionToString())

  var c *Client
  if client == nil {
    c = room.connClientMap[room.getPlayerConn(player)]
  } else {
    c = client
  }

  netData := &NetData{
    Client:     room.publicClientInfo(c),
    Response:   NetDataPlayerAction,
    Table:      room.table,
  }

  room.sendResponseToAll(netData, c)

  if client != nil { // client is nil for blind auto allin corner case
    netData.Client.Player = player
    netData.SendTo(c)
  }
}

func (room *Room) sendDeals() {
  netData := &NetData{
    room: room,
    Response: NetDataDeal,
    Table: room.table,
  }

  for _, player := range room.table.CurPlayers().ToPlayerArray() {
    client := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = client

    netData.Send()
  }
}

func (room *Room) sendHands() {
  netData := &NetData{
    room: room,
    Response: NetDataShowHand,
    Table: room.table,
  }

  for _, player := range room.table.GetNonFoldedPlayers() {
    client := room.playerClientMap[player]
    //assert(client != nil, "Room.sendHands(): player not in playerMap")
    netData.Client = room.publicClientInfo(client)

    room.sendResponseToAll(netData, client)
  }
}

// NOTE: hand is currently computed on client side
func (room *Room) sendCurHands() {
  netData := &NetData{
    room: room,
    Response: NetDataCurHand,
    Table: room.table,
  }

  for _, client := range room.playerClientMap {
    netData.Client = client
    netData.Send()
  }
}

func (room *Room) sendActivePlayers(client *Client) {
  if client == nil {
    fmt.Printf("Room.sendActivePlayers(): {%s}: conn is nil\n", room.name)
    return
  }

  netData := &NetData{
    room: room,
    Response: NetDataCurPlayers,
    Table:    room.table,
  }

  for _, player := range room.table.ActivePlayers().ToPlayerArray() {
    playerClient := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = room.publicClientInfo(playerClient)
    netData.SendTo(client)
  }
}

func (room *Room) sendAllPlayerInfo(client *Client, isCurPlayers bool, sendToSelf bool) {
  netData := &NetData{
    room: room,
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
    playerClient := room.connClientMap[room.getPlayerConn(player)]

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

func (room *Room) sendTable() {
  room.sendResponseToAll(&NetData{
    Response: NetDataUpdateTable,
    Table:    room.table,
  }, nil)
}

func (room *Room) sendReset(winner *Client) {
  room.sendResponseToAll(&NetData{
    Client:   winner,
    Response: NetDataReset,
    Table:    room.table,
  }, nil)
}

func (room *Room) removeEliminatedPlayers() {
  netData := &NetData{Response: NetDataEliminated}

  for _, player := range room.table.GetEliminatedPlayers() {
    client := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = client
    netData.Response = NetDataEliminated
    netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", client.Player.Name,
                              netData.Client.ID[:7])

    room.removePlayer(client, false, false)
    room.sendResponseToAll(netData, nil)
  }
}

func (room *Room) sendLock(conn *websocket.Conn, connType string) {
  fmt.Printf("Room.sendLock(): {%s}: locked out %p with %s\n", room.name, conn,
             room.table.TableLockToString())

  netData := &NetData{
    room: room,
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataTableLocked,
    Msg: fmt.Sprintf("table lock: %s", room.table.TableLockToString()),
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (room *Room) sendBadAuth(conn *websocket.Conn, connType string) {
  fmt.Printf("Room.sendBadAuth(): {%s}: %p had bad authentication\n", room.name, conn)

  netData := &NetData{
    room: room,
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataBadAuth,
    Msg: "your password was incorrect",
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (room *Room) makeAdmin(client *Client) {
  if client == nil {
    fmt.Printf("Room.makeAdmin(): {%s}: client is nil, unsetting tableAdmin\n", room.name)
    room.tableAdminID = ""
    return
  } else {
    fmt.Printf("Room.makeAdmin(): {%s}: making <%s> (%s) table admin\n", room.name, client.ID, client.Name)
    room.tableAdminID = client.ID
  }

  netData := &NetData{
    room: room,
    Client: client,
    Response: NetDataMakeAdmin,
    Table: room.table,
  }

  netData.Send()
}

func (room *Room) newRound() {
  room.table.NewRound()
  room.table.NextTableAction()
  room.checkBlindsAutoAllIn()
  room.sendDeals()

  // XXX
  room.table.Mtx().Lock()
  realRoundState := room.table.State
  room.table.State = poker.TableStateNewRound
  room.sendAllPlayerInfo(nil, true, false)
  room.sendPlayerTurnToAll()
  room.table.State = realRoundState
  room.table.Mtx().Unlock()

  room.sendTable()
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
    Table:    room.table,
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
  fmt.Printf("Room.gameOver(): {%s}: ** game over %s wins **\n", room.name, room.table.Winners[0].Name)
  winner := room.table.Winners[0]

  netData := &NetData{
    Response: NetDataServerMsg,
    Msg:      "game over, " + winner.Name + " wins",
  }

  room.sendResponseToAll(netData, nil)

  room.table.Reset(winner) // make a new game while keeping winner connected

  winnerClient := room.getPlayerClient(winner)
  if winnerClient == nil {
    fmt.Printf("Room.getPlayerClient(): {%s}: winner (%s) not found in any maps\n", room.name, winner.Name)
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
    fmt.Printf("Room.checkBlindsAutoAllIn(): {%s}: smallblind (%s) forced to go all in\n",
               room.name, room.table.SmallBlind.Player.Name)

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
    fmt.Printf("Room.checkBlindsAutoAllIn(): {%s}: bigblind (%s) forced to go all in\n",
               room.name, room.table.BigBlind.Player.Name)

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

  fmt.Println("Room.postBetting(): done betting...")

  if room.table.BettingIsImpossible() {
    fmt.Println("Room.postBetting(): no more betting possible this round")

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
      fmt.Printf("Room.postBetting(): clearing %v's action\n", player.Name)
      player.Action.Clear()
    }

    room.sendAllPlayerInfo(nil, true, true)
    room.table.ReorderPlayers()
    room.sendPlayerTurnToAll()
    room.sendPlayerHead(nil, true)
    // let players know they should update their current hand after
    // the community action
    room.sendCurHands()
  }
}

func (room *Room) postPlayerAction(client *Client, netData *NetData) {
  player := client.Player

  if room.table.State == poker.TableStateDoneBetting {
    room.postBetting(player, netData, client)
  } else if room.table.State == poker.TableStateRoundOver {
    // all other players folded before all comm cards were dealt
    // TODO: check for this state in a better fashion
    room.table.FinishRound()
    fmt.Printf("winner # %d\n", len(room.table.Winners))
    fmt.Println(room.table.Winners[0].Name + " wins by folds")

    netData.Response = NetDataRoundOver
    netData.Table = room.table
    netData.Msg = room.table.Winners[0].Name + " wins by folds"
    if netData.Client != nil { // XXX ?
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

type ClientSettings struct {
  IsSpectator bool

  Name     string
  Password string

  Admin struct {
    RoomName string
    NumSeats uint8
    Lock     poker.TableLock
    Password string
  }
}

func NewClientSettings() *ClientSettings {
  return &ClientSettings{
    Name: "noname",
  }
}

func (room *Room) handleClientSettings(client *Client, settings *ClientSettings) (string, error) {
  msg := ""
  errs := ""

  const (
    MaxNameLen uint8 = 15
    MaxPassLen uint8 = 50
  )

  if client == nil {
    fmt.Println("Room.handleClientSettings(): called with a nil parameter")

    return "", errors.New("room.handleClientSettings(): BUG: client == nil")
  } else if settings == nil {
    fmt.Println("Room.handleClientSettings(): called with a nil parameter")

    return "", errors.New("Room.handleClientSettings(): BUG: settings == nil")
  }

  //fmt.Printf("Room.handleClientSettings(): <%s> settings: %v\n", client.Name, settings)

  settings.Name = strings.TrimSpace(settings.Name)
  if settings.Name != "" {
    if len(settings.Name) > int(MaxNameLen) {
      fmt.Printf("Room.handleClientSettings(): %p requested a name that was longer " +
                 "than %v characters. using a default name\n", client.conn, MaxNameLen)
      msg += fmt.Sprintf("You've requested a name that was longer than %v characters. " +
              "Using a default name.\n\n", MaxNameLen)
      settings.Name = ""
    } else {
      if player := client.Player; player != nil {
        if player.Name == settings.Name {
          fmt.Println("Room.handleClientSettings(): name unchanged")
          msg += "name: unchanged\n\n"
        } else {
          _, found := room.nameClientMap[settings.Name]
          if !found {
            for _, defaultName := range room.table.DefaultPlayerNames() {
              if settings.Name == defaultName {
                found = true
                break
              }
            }
          } else {
            fmt.Printf("%p requested the name `%s` which is reserved or already taken\n",
                       client.conn, settings.Name)
            msg += fmt.Sprintf("Name '%s' already in use. Current name unchanged.\n\n",
                                settings.Name)
          }
        }
      } else {
        for _, player := range *room.table.Players() {
          if settings.Name == player.Name {
            fmt.Printf("%p requested the name `%s` which is reserved or already taken. " +
                       "using a default name\n", client.conn, settings.Name)
            msg += fmt.Sprintf("Name '%s' already in use. Using a default name.\n\n",
                                settings.Name)
            settings.Name = ""
            break
          }
        }
      }
    }
  }
  if client.ID == room.tableAdminID {
    msg += "admin settings:\n\n"

    lock := poker.TableLockToString(settings.Admin.Lock)
    if lock == "" {
      fmt.Printf("Room.handleClientSettings(): %p requested invalid table lock '%v'\n",
                 client.conn, settings.Admin.Lock)
      errs += fmt.Sprintf("invalid table lock: '%v'\n", settings.Admin.Lock)
    } else if settings.Admin.Lock == room.table.Lock {
      fmt.Println("Room.handleClientSettings(): table lock unchanged")
      msg += "table lock: unchanged\n"
    } else {
      msg += "table lock: changed\n"
    }

    if settings.Admin.Password != room.table.Password {
      msg += "table password: "
      if settings.Admin.Password == "" {
        msg += "removed\n"
      } else if len(settings.Admin.Password) > int(MaxPassLen) {
        return "", errors.New(fmt.Sprintf("Your password is too long. Please choose a " +
                                          "password that is less than %v characters.", MaxPassLen))
      } else {
        msg += "changed\n"
      }
    } else {
      fmt.Println("Room.handleClientSettings(): table password unchanged")
      msg += "table password: unchanged\n"
    }
  }

  if errs != "" {
    errs = "server response: unable to complete request due to following errors:\n\n" + errs
    return "", errors.New(errs)
  }

  msg = "server response: settings changes:\n\n" + msg

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

  fmt.Printf("Room.commState2NetDataResponse(): bad state `%v`\n", room.table.CommState)
  return NetDataBadRequest
}

func (room *Room) applyClientSettings(client *Client, settings *ClientSettings) {
  if settings == nil {
    fmt.Printf("Room.applyClientSettings(): %s had nil ClientSettings, using defaults\n", client.Name)
    client.Settings = NewClientSettings()
  } else {
    client.Settings = settings
  }

  if player := client.Player; player != nil {
    player.SetName(settings.Name)
    client.SetName(player.Name)

    if client.ID == room.tableAdminID {
      room.table.Mtx().Lock()
      room.table.Lock = settings.Admin.Lock
      room.table.Password = settings.Admin.Password
      room.table.Mtx().Unlock()
    }
  } else {
    client.SetName(settings.Name)
  }
}

func (room *Room) newClient(conn *websocket.Conn, connType string, clientSettings *ClientSettings) *Client {
  room.Lock()
  defer room.Unlock()

  client, ID, privID := &Client{conn: conn, connType: connType}, "", ""
  for {
    // 62^10 is plenty ;)
    ID = poker.RandString(10)
    privID = poker.RandString(10)

    _, foundID := room.IDClientMap[ID]
    _, foundPrivID := room.privIDClientMap[privID]
    if !foundID && !foundPrivID {
      client.ID = ID
      client.privID = privID

      room.IDClientMap[ID] = client
      room.privIDClientMap[privID] = client
      room.connClientMap[conn] = client

      break
    } else {
      if foundID {
        fmt.Printf("room.newClient(): WARNING: possible bug: ID '%s' already found in IDClientMap\n", ID)
      }
      if foundPrivID {
        fmt.Printf("room.newClient(): WARNING: possible bug: privID '%s' already found in privIDClientMap\n", privID)
      }
    }
  }

  client.SetName(clientSettings.Name)
  if client.Name != "" {
    room.nameClientMap[client.Name] = client
  }

  return client
}

func (room *Room) isLocked() bool {
  room.table.Mtx().Lock()
  defer room.table.Mtx().Unlock()

  if room.table.Lock == poker.TableLockAll ||
     (room.table.Lock == poker.TableLockSpectators &&
      room.table.GetNumOpenSeats() == 0) {
    return true
  }

  return false
}
