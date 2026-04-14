package net

import (
	"fmt"
	"sync/atomic"

	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

// wsSession carries the per-connection state that the WSClient read loop and
// its async request dispatchers need to share. cleanExit is atomic so the
// top-level defer (handleDisconnect) observes writes from both the dispatch
// goroutines and the read loop.
type wsSession struct {
	server    *Server
	room      *Room
	conn      *websocket.Conn
	connType  string
	cleanExit atomic.Bool
}

// dispatch routes a decoded NetData to the correct handler. Designed to be
// called in a goroutine from the read loop; exiting the loop is signaled via
// s.requestInputLoopExit.
func (s *wsSession) dispatch(client *Client, netData NetData) {
	switch netData.Request {
	case NetDataClientExited:
		s.requestInputLoopExit()
	case NetDataPlayerLeft: // NOTE: used when a player moves to spectator
		s.room.cleanupPlayerOnExit(client, playerExitToSpectator)
	case NetDataNewPlayer:
		s.handleNewPlayer(client, netData)
	case NetDataClientSettings:
		s.handleClientSettings(client, netData)
	case NetDataAdminSettings:
		s.handleAdminSettings(client, netData)
	case NetDataStartGame:
		s.handleStartGame(client, netData)
	case NetDataChatMsg:
		s.handleChatMsg(client, netData)
	case NetDataAllIn, NetDataBet, NetDataCall, NetDataCheck, NetDataFold:
		s.handlePlayerAction(client, netData)
	default:
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = fmt.Sprintf("bad request %v", netData.Request)
		netData.Send()
	}
}

func (s *wsSession) handleNewPlayer(client *Client, netData NetData) {
	room := s.room
	if !room.TryLock() {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "can't join as player right now. try again later"
		netData.Send()
		return
	}
	defer room.Unlock()

	if room.table.Lock == poker.TableLockAll ||
		room.table.Lock == poker.TableLockPlayers {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "this table is currently not accepting new players"
		netData.Send()
		return
	}

	seatPos := uint8(0)
	if netData.Client.Settings != nil {
		log.Debug().Uint8("seatPos", netData.Client.Settings.SeatPos).Msg("NewPlayer request")
		seatPos = netData.Client.Settings.SeatPos
	} else {
		log.Warn().Msg("NewPlayer request with nil Settings")
	}

	netData.ClearData(client)

	player := room.table.GetSeat(seatPos)
	if player == nil {
		netData.Response = NetDataServerMsg
		netData.Msg = "failed to join at this seat"
		netData.Send()
		return
	}

	// Bind the seat to the client immediately. GetSeat already marked
	// the seat occupied and bumped NumPlayers; if a concurrent
	// disconnect cleanup ran before addPlayer, a nil client.Player
	// would cause cleanupPlayerOnExit to skip removePlayer and orphan
	// the seat.
	client.Player = player
	room.clients.SetPlayer(client, player)

	client.Settings.IsSpectator = false
	player.SetName(client.Name)
	client.SetName(player.Name)

	log.Info().
		Str("room", room.name).
		Str("client", client.FullName(true)).
		Str("player", player.Name).
		Uint("tPos", player.TablePos).
		Msg("adding new player")

	room.addPlayer(client, player, &netData, false)

	if room.tableAdminID == "" {
		room.makeAdmin(client)
	}
}

func (s *wsSession) handleClientSettings(client *Client, netData NetData) {
	room := s.room
	if !room.TryLock() {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "cannot change your settings right now. please try again later"
		netData.Send()
		return
	}
	defer room.Unlock()

	settings := netData.Client.Settings

	msg, err := room.handleClientSettings(client, settings)
	if err != nil {
		log.Error().Err(err).Str("client", client.FullName(false)).Msg("handleClientSettings failed")
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = err.Error()
		netData.Send()
		return
	}
	room.applyClientSettings(client, settings)

	netData.ClearData(client)
	if client.Player != nil { // send updated player info to other clients
		netData.Response = NetDataUpdatePlayer
		netData.Client = room.publicClientInfo(client)
		room.sendResponseToAll(&netData, client)
	}

	netData.Client = client
	netData.Response = NetDataClientSettings
	netData.Send()

	room.sendTable(nil)

	// TODO: combine server msg with prev response
	netData.Response = NetDataServerMsg
	netData.Msg = msg
	netData.Send()
}

func (s *wsSession) handleAdminSettings(client *Client, netData NetData) {
	room := s.room
	server := s.server
	if !room.TryLock() {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "cannot change your settings right now. please try again later"
		netData.Send()
		return
	}
	defer room.Unlock()

	prevRoomSettings := room.getRoomSettings()

	var clientSettings *ClientSettings
	if netData.Client != nil {
		clientSettings = netData.Client.Settings
	}

	if client == nil || client.ID != room.tableAdminID {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "only the table admin can change room settings"
		netData.Send()
	} else if roomSettings, msg, err := server.handleRoomSettings(room, client, netData.RoomSettings); roomSettings != nil {
		roomSettingsChanged := prevRoomSettings.RoomName != roomSettings.RoomName ||
			prevRoomSettings.NumSeats != roomSettings.NumSeats ||
			prevRoomSettings.Lock != roomSettings.Lock ||
			prevRoomSettings.Password != roomSettings.Password

		if roomSettingsChanged {
			netData.ClearData(nil)
			netData.Client = nil
			netData.Response = NetDataRoomSettings
			netData.RoomSettings = roomSettings
			room.sendResponseToAll(&netData, nil)
			room.sendTable(nil)
		}

		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		if err != nil {
			log.Error().
				Err(err).
				Str("client", client.FullName(false)).
				Msg("handleRoomSettings partial error")
			netData.Msg = msg + err.Error()
		} else {
			netData.Msg = msg
		}
		netData.Send()
	} else if err != nil {
		log.Error().Err(err).Str("client", client.FullName(false)).Msg("handleRoomSettings failed")
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = msg + err.Error()
		netData.Send()
	}

	settings := clientSettings
	if client != nil && settings != nil {
		msg, err := room.handleClientSettings(client, settings)
		if err != nil {
			log.Error().
				Err(err).
				Str("client", client.FullName(false)).
				Msg("handleClientSettings failed (admin)")
			netData.ClearData(client)
			netData.Response = NetDataServerMsg
			netData.Msg = err.Error()
			netData.Send()
			return
		}
		room.applyClientSettings(client, settings)

		netData.ClearData(client)
		if client.Player != nil {
			netData.Response = NetDataUpdatePlayer
			netData.Client = room.publicClientInfo(client)
			room.sendResponseToAll(&netData, client)
		}

		netData.Client = client
		netData.Response = NetDataClientSettings
		netData.Send()

		room.sendTable(nil)

		netData.Response = NetDataServerMsg
		netData.Msg = msg
		netData.Send()
	}
}

func (s *wsSession) handleStartGame(client *Client, netData NetData) {
	room := s.room
	netData.ClearData(client)
	if client.ID != room.tableAdminID {
		netData.Response = NetDataBadRequest
		netData.Msg = "only the table admin can do that"
		netData.Send()
		return
	}
	if room.table.NumPlayers < 2 {
		netData.Response = NetDataBadRequest
		netData.Msg = "not enough players to start"
		netData.Send()
		return
	}
	if room.table.State != poker.TableStateNotStarted {
		netData.Response = NetDataBadRequest
		netData.Msg = "this game has already started"
		netData.Send()
		return
	}

	room.table.NextTableAction()

	room.sendDeals()
	room.sendCurHands()
	room.sendAllPlayerInfo(nil, false, true)
	room.sendPlayerTurnToAll()
	room.sendTable(nil)
}

func (s *wsSession) handleChatMsg(client *Client, netData NetData) {
	room := s.room
	msg := netData.Msg

	netData.ClearData(client)
	netData.Response = NetDataChatMsg
	netData.Msg = msg

	if len(netData.Msg) > int(s.server.MaxChatMsgLen) {
		netData.Msg = netData.Msg[:s.server.MaxChatMsgLen] + "(snipped)"
	}

	if client.Player != nil { // only chooses bracket style, never dereferenced
		netData.Msg = fmt.Sprintf("[%s id: %s]: %s", client.Name,
			client.ID[:7], netData.Msg)
	} else {
		netData.Msg = fmt.Sprintf("{%s id: %s}: %s", client.Name,
			client.ID[:7], netData.Msg)
	}

	room.sendResponseToAll(&netData, nil)
}

func (s *wsSession) handlePlayerAction(client *Client, netData NetData) {
	room := s.room
	if room.IsLocked() {
		log.Warn().
			Str("client", client.FullName(true)).
			Msg("tried to send action while room was locked")
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = "that action is not valid at this time"
		netData.Send()
		return
	}

	// Snapshot player pointer: removePlayer may concurrently nil
	// client.Player. Capturing it once prevents a TOCTOU race where
	// the nil-check passes but a later dereference crashes.
	player := client.Player
	if player == nil {
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = "you are not a player"
		netData.Send()
		return
	}

	if room.table.State == poker.TableStateNotStarted {
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = "a game has not been started yet"
		netData.Send()
		return
	}

	if !room.table.IsCurPlayer(player) {
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = "it's not your turn"
		netData.Send()
		return
	}

	room.Lock()

	// Revalidate under the lock: the snapshot may be stale if
	// removePlayer ran (clearing the seat) or the client left and
	// rejoined on a different seat. Pointer identity confirms this
	// is still the same player-seat binding.
	if client.Player != player || player.IsVacant {
		room.Unlock()
		return
	}
	defer room.Unlock()

	if err := room.table.PlayerAction(player, netData.Client.Player.Action); err != nil {
		netData.ClearData(client)
		netData.Response = NetDataBadRequest
		netData.Msg = err.Error()
		netData.Send()
	} else {
		room.postPlayerAction(client, &netData)
	}
}
