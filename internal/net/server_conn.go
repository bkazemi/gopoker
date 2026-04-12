package net

import (
	"fmt"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

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

			if player := room.table.GetSeat(seatPos); player != nil {
				client.Player = player
				room.clients.SetPlayer(client, player)

				processClient()

				log.Info().
					Str("room", room.name).
					Str("client", client.ID).
					Str("name", client.Name).
					Str("player", player.Name).
					Uint("tPos", player.TablePos).
					Msg("adding player (creator)")

				player.Action.Action = playerState.FirstAction
				room.table.CurPlayers().AddPlayer(player)
				room.table.ActivePlayers().AddPlayer(player)

				if room.table.CurPlayer() == nil {
					room.table.SetCurPlayer(room.table.CurPlayers().Head)
				}

				if room.table.Dealer == nil {
					room.table.Dealer = room.table.ActivePlayers().Head
				} else if room.table.SmallBlind == nil {
					room.table.SmallBlind = room.table.Dealer.Next()
				} else if room.table.BigBlind == nil {
					room.table.BigBlind = room.table.SmallBlind.Next()
				}

				// while unlikely, it is still possible that non-room creators could
				// join while we are handling the room creator
				netData.Client = room.publicClientInfo(client)
				netData.Response = NetDataNewPlayer
				netData.Table = room.table

				room.sendResponseToAll(&netData, client)

				netData.Client = client
				netData.Response = NetDataYourPlayer
				netData.Send()
			} else { // sanity check
				panic(fmt.Sprintf("Server.handleNewConn(): {%s}: GetSeat(%v) failed for a room creator", room.name, seatPos))
			}

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
			client.Player = player
			room.clients.SetPlayer(client, player)

			room.applyClientSettings(client, netData.Client.Settings)
			log.Info().
				Str("room", room.name).
				Str("client", client.ID).
				Str("name", client.Name).
				Str("player", player.Name).
				Uint("tPos", player.TablePos).
				Msg("adding player")

			if room.table.State == poker.TableStateNotStarted {
				player.Action.Action = playerState.FirstAction
				room.table.CurPlayers().AddPlayer(player)
			} else {
				player.Action.Action = playerState.MidroundAddition
			}
			room.table.ActivePlayers().AddPlayer(player)

			if room.table.CurPlayer() == nil {
				room.table.SetCurPlayer(room.table.CurPlayers().Head)
			}

			if room.table.Dealer == nil {
				room.table.Dealer = room.table.ActivePlayers().Head
			} else if room.table.SmallBlind == nil {
				room.table.SmallBlind = room.table.Dealer.Next()
			} else if room.table.BigBlind == nil {
				room.table.BigBlind = room.table.SmallBlind.Next()
			}

			netData.Client = room.publicClientInfo(client)
			netData.Response = NetDataNewPlayer
			netData.Table = room.table

			room.sendResponseToAll(&netData, client)

			netData.Client = client
			netData.Response = NetDataYourPlayer
			netData.Send()

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
