package net

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

type Server struct {
	rooms map[string]*Room

	MaxConnBytes   int64
	MaxChatMsgLen  int32
	MaxRoomNameLen int32

	router *mux.Router

	http     *http.Server
	upgrader websocket.Upgrader

	sigChan  chan os.Signal
	errChan  chan error
	panicked bool

	mtx sync.Mutex
}

func NewServer(addr string) *Server {
	const (
		MaxConnBytes   = 10e3
		MaxChatMsgLen  = 256
		MaxRoomNameLen = 50
		IdleTimeout    = 0
		ReadTimeout    = 0
	)

	router := mux.NewRouter()

	server := &Server{
		rooms: make(map[string]*Room),

		MaxConnBytes:   MaxConnBytes,
		MaxChatMsgLen:  MaxChatMsgLen,
		MaxRoomNameLen: MaxRoomNameLen,

		errChan:  make(chan error),
		panicked: false,

		upgrader: websocket.Upgrader{
			EnableCompression: true,
			Subprotocols:      []string{"permessage-deflate"},
			ReadBufferSize:    4096,
			WriteBufferSize:   4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // XXX TMP REMOVE ME
			},
		},

		router: router,

		http: &http.Server{
			Addr:        addr,
			IdleTimeout: IdleTimeout,
			ReadTimeout: ReadTimeout,
			Handler:     router,
		},

		sigChan: make(chan os.Signal, 1),
	}

	handleRoom := func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		roomName := vars["roomName"]

		if room, found := server.rooms[roomName]; found {
			if room.isTableLocked() {
				w.WriteHeader(http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		} else {
			http.NotFound(w, req)
		}
	}

	handleClient := func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		roomName := vars["roomName"]
		connType := vars["connType"]

		if (connType != "cli" && connType != "web") ||
			server.rooms[roomName] == nil {
			http.NotFound(w, req)

			return
		}

		server.WSClient(w, req, server.rooms[roomName], connType)
	}

	server.http.SetKeepAlivesEnabled(true)
	router.HandleFunc("/health", healthCheck).Methods("GET")
	router.HandleFunc("/status", status).Methods("GET")
	router.HandleFunc("/new", server.createNewRoom).Methods("POST")
	router.HandleFunc("/roomCount", server.roomCount).Methods("GET")
	router.HandleFunc("/rooms", server.listRooms).Methods("GET")
	router.HandleFunc("/room/{roomName}", handleRoom)
	router.HandleFunc("/room/{roomName}/{connType}", handleClient).Methods("GET")

	signal.Notify(server.sigChan, os.Interrupt)

	return server
}

func healthCheck(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func status(w http.ResponseWriter, req *http.Request) {
	res := struct {
		Status string `json:"status"`
	}{
		Status: "running",
	}

	jsonBody, err := json.Marshal(res)
	if err != nil {
		http.Error(w, "failed to encode JSON", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody)
}

func closeConn(conn *websocket.Conn) {
	log.Debug().Str("remote", conn.RemoteAddr().String()).Msg("closing connection")
	conn.Close()
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error, room *Room) {
	log.Error().Msg("server panicked")

	for _, conn := range room.clients.Conns() {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
				err.Error()))
	}

	server.errChan <- err
	server.panicked = true
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

func (server *Server) WSClient(w http.ResponseWriter, req *http.Request, room *Room, connType string) {
	if req.Header.Get("keepalive") != "" {
		return // NOTE: for heroku
	}

	if connType != "cli" && connType != "web" {
		log.Warn().Str("room", room.name).Str("connType", connType).Msg("invalid connType")

		return
	}

	conn, err := server.upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Error().Err(err).Msg("WS upgrade error")

		return
	}

	conn.SetReadLimit(server.MaxConnBytes)
	conn.EnableWriteCompression(true)
	conn.SetCompressionLevel(flate.BestCompression)

	// TODO: move me
	playerCleanup := func(client *Client, isClientExit bool) {
		if client == nil {
			return
		}
		// Snapshot: client.Player may be nilled concurrently by another
		// removePlayer call (e.g. from a different goroutine's cleanup path).
		player := client.Player
		if player == nil {
			return
		}

		if room.table.ActivePlayers().Len > 1 &&
			room.table.CurPlayer() != nil &&
			room.table.CurPlayer().Player.Name == player.Name {
			player.Action.Action = playerState.Fold
			room.table.SetNextPlayerTurn()
			room.sendPlayerTurnToAll()
		}

		room.removePlayer(client, isClientExit, !isClientExit)
	}

	cleanExit := false
	defer func() {
		if server.panicked { // room panic was already recovered in previous client handler
			return
		}

		if err := recover(); err != nil {
			server.serverError(poker.PanicRetToError(err), room)
		} else { // not a room panic()
			if client, ok := room.clients.ByConn(conn); ok {
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

					playerCleanup(client, true)
					room.removeClient(client)
				})

				client.mtx.Unlock()
			} else {
				log.Warn().Str("room", room.name).Msgf("defer: couldn't find conn %p in connClientMap", conn)
			}
		}
	}()

	log.Info().Str("room", room.name).Str("host", req.Host).Msg("new websocket connection")

	stopPing := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)

		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					log.Error().Err(err).Str("room", room.name).Msg("ping error")
					return
				}
			}
		}
	}()
	defer func() {
		stopPing <- true
	}()

	//netData := NetData{}

	returnFromInputLoop := make(chan bool)

	handleAsyncRequest := func(client *Client, netData NetData) {
		switch netData.Request {
		case NetDataClientExited:
			cleanExit = true

			returnFromInputLoop <- true
			return
		case NetDataPlayerLeft: // NOTE: used when a player moves to spectator
			playerCleanup(client, false)
		case NetDataNewPlayer:
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

			if player := room.table.GetSeat(seatPos); player != nil {
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

				if room.tableAdminID == "" {
					room.makeAdmin(client)
				}
			} else {
				netData.Response = NetDataServerMsg
				netData.Msg = "failed to join at this seat"
				netData.Send()
			}
		case NetDataClientSettings: // TODO: check pointers
			if !room.TryLock() {
				netData.ClearData(client)
				netData.Response = NetDataServerMsg
				netData.Msg = "cannot change your settings right now. please try again later"
				netData.Send()

				returnFromInputLoop <- false
				return
			}

			settings := netData.Client.Settings

			msg, err := room.handleClientSettings(client, settings)
			if err == nil {
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
			} else {
				log.Error().Err(err).Str("client", client.FullName(false)).Msg("handleClientSettings failed")

				netData.ClearData(client)
				netData.Response = NetDataServerMsg
				netData.Msg = err.Error()
				netData.Send()
			}

			room.Unlock()
		case NetDataAdminSettings:
			if !room.TryLock() {
				netData.ClearData(client)
				netData.Response = NetDataServerMsg
				netData.Msg = "cannot change your settings right now. please try again later"
				netData.Send()

				returnFromInputLoop <- false
				return
			}

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
				if err == nil {
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
				} else {
					log.Error().
						Err(err).
						Str("client", client.FullName(false)).
						Msg("handleClientSettings failed (admin)")

					netData.ClearData(client)
					netData.Response = NetDataServerMsg
					netData.Msg = err.Error()
					netData.Send()
				}
			}

			room.Unlock()
		case NetDataStartGame:
			netData.ClearData(client)
			if client.ID != room.tableAdminID {
				netData.Response = NetDataBadRequest
				netData.Msg = "only the table admin can do that"

				netData.Send()
			} else if room.table.NumPlayers < 2 {
				netData.Response = NetDataBadRequest
				netData.Msg = "not enough players to start"

				netData.Send()
			} else if room.table.State != poker.TableStateNotStarted {
				netData.Response = NetDataBadRequest
				netData.Msg = "this game has already started"

				netData.Send()
			} else { // start game
				room.table.NextTableAction()

				room.sendDeals()
				room.sendCurHands()
				room.sendAllPlayerInfo(nil, false, true)
				room.sendPlayerTurnToAll()
				room.sendTable(nil)
			}
		case NetDataChatMsg:
			msg := netData.Msg

			netData.ClearData(client)
			netData.Response = NetDataChatMsg
			netData.Msg = msg

			if len(netData.Msg) > int(server.MaxChatMsgLen) {
				netData.Msg = netData.Msg[:server.MaxChatMsgLen] + "(snipped)"
			}

			if client.Player != nil { // only chooses bracket style, never dereferenced
				netData.Msg = fmt.Sprintf("[%s id: %s]: %s", client.Name,
					client.ID[:7], netData.Msg)
			} else {
				netData.Msg = fmt.Sprintf("{%s id: %s}: %s", client.Name,
					client.ID[:7], netData.Msg)
			}

			room.sendResponseToAll(&netData, nil)
		case NetDataAllIn, NetDataBet, NetDataCall, NetDataCheck, NetDataFold:
			if room.IsLocked() {
				log.Warn().
					Str("client", client.FullName(true)).
					Msg("tried to send action while room was locked")
				netData.ClearData(client)
				netData.Response = NetDataBadRequest
				netData.Msg = "that action is not valid at this time"

				netData.Send()

				returnFromInputLoop <- false
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

				returnFromInputLoop <- false
				return
			}

			if room.table.State == poker.TableStateNotStarted {
				netData.ClearData(client)
				netData.Response = NetDataBadRequest
				netData.Msg = "a game has not been started yet"

				netData.Send()

				returnFromInputLoop <- false
				return
			}

			if player.Name != room.table.CurPlayer().Player.Name {
				netData.ClearData(client)
				netData.Response = NetDataBadRequest
				netData.Msg = "it's not your turn"

				netData.Send()

				returnFromInputLoop <- false
				return
			}

			room.Lock()

			// Revalidate under the lock: the snapshot may be stale if
			// removePlayer ran (clearing the seat) or the client left and
			// rejoined on a different seat. Pointer identity confirms this
			// is still the same player-seat binding.
			if client.Player != player || player.IsVacant {
				room.Unlock()

				returnFromInputLoop <- false
				return
			}

			if err := room.table.PlayerAction(player, netData.Client.Player.Action); err != nil {
				netData.ClearData(client)
				netData.Response = NetDataBadRequest
				netData.Msg = err.Error()

				netData.Send()
			} else {
				room.postPlayerAction(client, &netData)
			}

			room.Unlock()
		default:
			netData.ClearData(client)
			netData.Response = NetDataBadRequest
			netData.Msg = fmt.Sprintf("bad request %v", netData.Request)

			netData.Send()
		}
	}

	for {
		var netData NetData

		select {
		case isReturn := <-returnFromInputLoop:
			if isReturn {
				break
			} // else, implicit continue

		default:
			if connType == "cli" {
				_, rawData, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.Error().
							Str("room", room.name).
							Err(err).
							Msgf("cli: readConn error on conn %p", conn)
					} else {
						log.Debug().
							Str("room", room.name).
							Err(err).
							Msgf("cli: readConn conn %p ws closed cleanly", conn)
						cleanExit = true
					}

					return
				}

				// we need to set Table member to nil otherwise gob will
				// modify our room.table structure if a user sends that member
				nd := NetData{Response: NetDataNewConn, Table: nil}

				if err := gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&nd); err != nil {
					log.Error().
						Str("room", room.name).
						Err(err).
						Msgf("cli: problem decoding gob stream from %p", conn)

					return
				}

				nd.Table = room.table

				log.Debug().
					Str("room", room.name).
					Str("action", nd.NetActionToString()).
					Int("bytes", len(rawData)).
					Msgf("cli: recv from %p", conn)

				if int64(len(rawData)) > server.MaxConnBytes {
					log.Warn().
						Str("room", room.name).
						Int64("max", server.MaxConnBytes).
						Msgf("cli: conn %p sent too many bytes", conn)
					return
				}

				netData = nd
			} else { // webclient
				_, rawData, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						log.Error().
							Str("room", room.name).
							Err(err).
							Msgf("web: readConn error on conn %p", conn)
					} else {
						log.Debug().
							Str("room", room.name).
							Err(err).
							Msgf("web: readConn conn %p ws closed cleanly", conn)
						cleanExit = true
					}

					return
				}

				err = msgpack.Unmarshal(rawData, &netData)
				if err != nil {
					log.Error().
						Str("room", room.name).
						Err(err).
						Msgf("web: problem decoding msgpack stream from %p", conn)

					return
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
			}

			if netData.Request == NetDataNewConn {
				server.handleNewConn(room, netData, conn, connType)
			} else if netData.Request == NetDataPlayerReconnecting {
				server.handleReconnect(room, netData, conn, connType)
			} else {
				client, _ := room.clients.ByConn(conn)
				go handleAsyncRequest(client, netData)
			} // else{} end
		} // returnFromInputLoop select end
	} //for loop end
} // func end

func (server *Server) Run() error {
	log.Info().Str("addr", server.http.Addr).Msg("starting server")

	go func() {
		if err := server.http.ListenAndServe(); err != nil {
			log.Error().Err(err).Msg("http.ListenAndServe failed")
		}
	}()

	select {
	case sig := <-server.sigChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Info().Str("signal", sig.String()).Msg("received signal")

		// TODO: ignore irrelevant signals
		for _, room := range server.rooms {
			room.sendResponseToAll(&NetData{Response: NetDataServerClosed}, nil)
		}

		if err := server.http.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server.http.Shutdown failed")
			return err
		}

		return nil
	case err := <-server.errChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Error().Err(err).Msg("irrecoverable server error")

		if err := server.http.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server.http.Shutdown failed")
			return err
		}

		return err
	}
}
