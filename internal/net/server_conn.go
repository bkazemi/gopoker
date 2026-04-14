package net

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

// startPingLoop runs a 10s websocket ping keep-alive. Returns a stop function
// the caller should defer; calling it terminates the goroutine.
func startPingLoop(conn *websocket.Conn, roomName string) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					log.Error().Err(err).Str("room", roomName).Msg("ping error")
					return
				}
			}
		}
	}()
	return func() { close(stop) }
}

// readNetData blocks on conn.ReadMessage and decodes it per connType.
// cleanClose is true when the peer closed the socket normally (no error
// surfaced to the caller). On any decode/read failure it returns err != nil.
func readNetData(conn *websocket.Conn, connType string, room *Room, maxConnBytes int64) (netData NetData, cleanClose bool, err error) {
	_, rawData, readErr := conn.ReadMessage()
	if readErr != nil {
		tag := connType
		if websocket.IsUnexpectedCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			log.Error().
				Str("room", room.name).
				Err(readErr).
				Msgf("%s: readConn error on conn %p", tag, conn)
		} else {
			log.Debug().
				Str("room", room.name).
				Err(readErr).
				Msgf("%s: readConn conn %p ws closed cleanly", tag, conn)
			cleanClose = true
		}
		return NetData{}, cleanClose, readErr
	}

	if connType == "cli" {
		// set Table member to nil otherwise gob will modify our room.table
		// structure if a user sends that member
		nd := NetData{Response: NetDataNewConn, Table: nil}
		if decErr := gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&nd); decErr != nil {
			log.Error().
				Str("room", room.name).
				Err(decErr).
				Msgf("cli: problem decoding gob stream from %p", conn)
			return NetData{}, false, decErr
		}
		nd.Table = room.table

		log.Debug().
			Str("room", room.name).
			Str("action", nd.NetActionToString()).
			Int("bytes", len(rawData)).
			Msgf("cli: recv from %p", conn)

		if int64(len(rawData)) > maxConnBytes {
			log.Warn().
				Str("room", room.name).
				Int64("max", maxConnBytes).
				Msgf("cli: conn %p sent too many bytes", conn)
			return NetData{}, false, fmt.Errorf("cli: too many bytes")
		}
		return nd, false, nil
	}

	// webclient
	if decErr := msgpack.Unmarshal(rawData, &netData); decErr != nil {
		log.Error().
			Str("room", room.name).
			Err(decErr).
			Msgf("web: problem decoding msgpack stream from %p", conn)
		return NetData{}, false, decErr
	}

	if netData.HasClient() {
		if netData.Client.conn == nil {
			netData.Client.conn = conn
		}
		if netData.Client.Settings == nil {
			netData.Client.Settings = &ClientSettings{}
		}
	} else {
		log.Warn().Str("room", room.name).Msgf("web: (%p) netData.HasClient() == false", conn)
	}

	log.Debug().
		Str("room", room.name).
		Str("action", netData.NetActionToString()).
		Msgf("web: recv msgpack, request=%v", netData.Request)
	if netData.room == nil {
		netData.room = room
	}
	netData.Table = room.table
	return netData, false, nil
}

// handleDisconnect is the deferred cleanup path for a terminated WS client:
// recover from room panics, schedule reconnect-window cleanup for unclean
// exits, or remove the last-client room on clean exit.
func (server *Server) handleDisconnect(room *Room, conn *websocket.Conn, cleanExit bool) {
	if server.panicked { // room panic was already recovered in previous client handler
		return
	}

	if err := recover(); err != nil {
		server.serverError(poker.PanicRetToError(err), room)
		return
	}

	client, ok := room.clients.ByConn(conn)
	if !ok {
		log.Warn().Str("room", room.name).Msgf("defer: couldn't find conn %p in connClientMap", conn)
		return
	}

	minsToWait := 0 * time.Minute

	client.mtx.Lock()

	// If the client already reconnected on a new socket,
	// this is a stale cleanup for the old connection — skip it.
	if client.conn != conn {
		client.mtx.Unlock()
		room.clients.RemoveConn(conn)
		closeConn(conn)
		return
	}

	client.isDisconnected = true

	if !cleanExit {
		log.Debug().
			Str("client", client.FullName(true)).
			Msg("unclean exit, waiting 1 min for reconnect until cleanup")

		if client.Player != nil {
			room.sendResponseToAll(&NetData{
				Response: NetDataPlayerReconnecting,
				Client:   room.publicClientInfo(client),
			}, client)
		}
		minsToWait = 1 * time.Minute
	}

	room.clients.RemoveConn(conn)
	closeConn(conn)

	if client.reconnectTimer != nil {
		client.reconnectTimer.Stop()
	}
	// the 0 min gofunc is kinda dumb, but they're cheap and it eliminates
	// some redundancy
	client.reconnectTimer = time.AfterFunc(minsToWait, func() {
		client.mtx.Lock()
		defer client.mtx.Unlock()

		if !client.isDisconnected {
			return
		}

		// if IsLocked is true then there must be at least one other client
		if !room.IsLocked() && room.table.NumConnected == 1 {
			log.Info().
				Str("client", client.FullName(true)).
				Str("room", room.name).
				Msg("last client left, removing room")
			server.removeRoom(room)
			return
		}

		room.cleanupPlayerOnExit(client, playerExitDisconnect)
		room.removeClient(client)
	})

	client.mtx.Unlock()
}

func (server *Server) handleNewConn(
	room *Room, netData NetData, conn *websocket.Conn, connType string,
) {
	netData.Request = 0

	if netData.Client == nil { // XXX
		netData.Client = NewClient(nil).SetConn(conn).SetConnType(connType)
		netData.Response = NetDataBadRequest
		netData.Msg = "netData.Client was not created by the client"

		netData.Send()

		return
	}

	if client, ok := room.clients.ByConn(conn); ok {
		netData.Client = client
		netData.Response = NetDataServerMsg
		netData.Msg = "you are already connected to the room."

		netData.Send()

		return
	}

	netData.Response = NetDataNewConn

	// we add this here so we don't accidently deference any nil pointers
	// need it here for the IsSpectator test below
	if netData.Client.Settings == nil {
		log.Warn().Msgf("%p had nil ClientSettings, using defaults", conn)
		netData.Client.Settings = NewClientSettings()
	}

	// check if this connection was the room creator
	if room.creatorToken != "" &&
		netData.Client.Settings.Password == room.creatorToken {
		room.clients.ReserveConn(conn)

		client := room.newClient(conn, connType, netData.Client.Settings)

		room.table.Mtx().Lock()
		room.table.NumConnected++
		room.table.Mtx().Unlock()

		processClient := func() {
			room.applyClientSettings(client, netData.Client.Settings)

			// while unlikely, it is still possible that non-room creators could
			// join while we are handling the room creator
			netData.Client = nil
			room.sendResponseToAll(&netData, client)

			netData.Client = client
			netData.Msg = client.privID
			netData.Send() // send NewConn after we've processed their settings
		}

		if !netData.Client.Settings.IsSpectator {
			seatPos := netData.Client.Settings.SeatPos

			// reserve the seat before any broadcasts so a racing non-creator
			// join can't claim it out from under us.
			player := room.table.GetSeat(seatPos)
			if player == nil { // sanity check
				panic(fmt.Sprintf("Server.handleNewConn(): {%s}: GetSeat(%v) failed for a room creator", room.name, seatPos))
			}
			client.Player = player
			room.clients.SetPlayer(client, player)

			// processClient must run before addPlayer so that the client
			// receives its NewConn and settings before the NewPlayer broadcast.
			// client.Player is now bound, so applyClientSettings will sync
			// the requested name onto player.Name.
			processClient()

			room.addPlayer(client, player, &netData, true)
			log.Info().
				Str("room", room.name).
				Str("client", client.ID).
				Str("name", client.Name).
				Str("player", player.Name).
				Uint("tPos", player.TablePos).
				Msg("adding player (creator)")

			room.makeAdmin(client)
		} else {
			processClient()
		}

		log.Debug().
			Str("room", room.name).
			Str("client", client.FullName(false)).
			Msg("used creatorToken, removing token")

		room.creatorToken = ""

		return
	}

	if room.table.Lock == poker.TableLockAll {
		room.sendLock(conn, connType)

		return
	}

	if room.table.Password != "" &&
		netData.Client.Settings.Password != room.table.Password {
		room.sendBadAuth(conn, connType)

		return
	}

	// set this to a nonnil value so that the guard at the top of this block
	// works if newClient is waiting on the room lock
	// XXX: I have to check if is actually necessary. probably not
	room.clients.ReserveConn(conn)

	client := room.newClient(conn, connType, netData.Client.Settings)

	if _, err := room.handleClientSettings(client, netData.Client.Settings); err != nil {
		log.Error().Err(err).Str("client", client.FullName(false)).Msg("handleClientSettings failed")

		(&NetData{
			room:     room,
			Response: NetDataBadRequest,
			Client:   client,
			Msg:      err.Error(),
		}).Send()
	}

	room.table.Mtx().Lock()
	room.table.NumConnected++
	room.table.Mtx().Unlock()

	room.applyClientSettings(client, netData.Client.Settings)

	netData.Client = nil
	room.sendResponseToAll(&netData, client) // send NewConn to other connected clients

	netData.Client = client
	netData.Msg = client.privID
	netData.Send() // send NewConn with Client info to this client

	// send current player info to this client
	if room.table.NumConnected > 1 {
		room.sendActivePlayers(client)
	}

	if !client.Settings.IsSpectator {
		if room.table.Lock == poker.TableLockPlayers {
			netData.Response = NetDataServerMsg
			netData.Msg = "This table is not allowing new players. " +
				"You have been added as a spectator."
			netData.Send()

			netData.ClearData(nil)
			// send NetDataClientSettings so frontend can update isSpectator
			client.Settings.IsSpectator = true
			netData.Response = NetDataClientSettings
			netData.Send()
		} else if player := room.table.GetSeat(client.Settings.SeatPos); player != nil {
			// bind player and sync the requested name onto player.Name
			// before addPlayer broadcasts NetDataNewPlayer — otherwise
			// other clients see the default seat name
			client.Player = player
			player.SetName(client.Settings.Name)
			client.SetName(player.Name)

			room.addPlayer(client, player, &netData, false)
			log.Info().
				Str("room", room.name).
				Str("client", client.ID).
				Str("name", client.Name).
				Str("player", player.Name).
				Uint("tPos", player.TablePos).
				Msg("adding player")

			if room.creatorToken == "" && room.tableAdminID == "" {
				room.makeAdmin(client)
			}
		} else if room.table.Lock == poker.TableLockSpectators {
			room.sendLock(conn, connType)

			return
		} else {
			netData.Response = NetDataServerMsg
			netData.Msg = "No open seats available. You have been added as a spectator"
			netData.Send()

			netData.ClearData(nil)
			// send NetDataClientSettings so frontend can update isSpectator
			client.Settings.IsSpectator = true
			netData.Response = NetDataClientSettings
			netData.Send()
		}
	}

	room.sendAllPlayerInfo(client, false, false)

	if room.table.State != poker.TableStateNotStarted {
		room.sendPlayerTurn(client)
	}
}

func (server *Server) handleReconnect(
	room *Room, netData NetData, conn *websocket.Conn, connType string,
) {
	if netData.Client == nil { // XXX
		netData.ClearData(NewClient(nil).SetConn(conn).SetConnType(connType))
		netData.Response = NetDataBadRequest
		netData.Msg = "netData.Client was not created by the client"

		netData.Send()

		return
	}

	if client, ok := room.clients.ByConn(conn); ok {
		netData.ClearData(client)
		netData.Response = NetDataServerMsg
		netData.Msg = "reconnect attempted while connected to server"

		netData.Send()

		return
	}

	// We put the private ID in the Msg member so we don't need to add an extra
	// member to the struct. An extra member would almost never be used and is
	// more likely be leaked to others via programmer error.
	if client, ok := room.clients.ByPrivID(netData.Msg); ok {
		client.mtx.Lock()

		// timer callback already ran cleanup — client is gone
		if _, stillValid := room.clients.ByPrivID(client.privID); !stillValid {
			client.mtx.Unlock()

			(&NetData{
				Client:   &Client{conn: conn, connType: connType},
				Response: NetDataBadRequest,
				Msg:      "failed to reconnect: session expired during reconnect",
			}).Send()

			return
		}

		if client.reconnectTimer != nil {
			client.reconnectTimer.Stop()
		}

		client.conn = conn
		room.clients.SetConn(conn, client)
		client.isDisconnected = false
		client.mtx.Unlock()

		netData.ClearData(room.publicClientInfo(client))
		netData.Response = NetDataPlayerReconnected
		room.sendResponseToAll(&netData, client)

		netData.Client = client
		netData.Send()

		// make sure the client gets current game state
		room.sendAllPlayerInfo(client, false, false)
		room.sendTable(client)
		if room.table.State != poker.TableStateNotStarted {
			room.sendPlayerTurn(client)
		}
	} else {
		netData.ClearData(NewClient(nil).SetConn(conn).SetConnType(connType))
		netData.Response = NetDataBadRequest
		netData.Msg = "failed to reconnect: invalid or expired private ID"
		netData.Send()
	}
}
