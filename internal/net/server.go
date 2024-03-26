package net

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

// TODO: important: need to ensure these pointers
// will be consistent throughout the program (mainly Player pointers)
type Server struct {
  rooms map[string]*Room

  MaxConnBytes  int64
  MaxChatMsgLen int32

  router *mux.Router

  http *http.Server
  upgrader websocket.Upgrader

  sigChan chan os.Signal
  errChan chan error
  panicked bool

  mtx sync.Mutex
}

func NewServer(addr string) *Server {
  const (
    MaxConnBytes = 10e3
    MaxChatMsgLen = 256
    IdleTimeout = 0
    ReadTimeout = 0
  )

  router := mux.NewRouter()

  server := &Server{
    rooms: make(map[string]*Room),

    MaxConnBytes: MaxConnBytes,
    MaxChatMsgLen: MaxChatMsgLen,

    errChan: make(chan error),
    panicked: false,

    upgrader: websocket.Upgrader{
      EnableCompression: true,
      Subprotocols: []string{"permessage-deflate"},
      ReadBufferSize: 4096,
      WriteBufferSize: 4096,
      CheckOrigin: func(r *http.Request) bool {
        return true; // XXX TMP REMOVE ME
      },
    },

    router: router,

    http: &http.Server{
      Addr:        addr,
      IdleTimeout: IdleTimeout,
      ReadTimeout: ReadTimeout,
      Handler: router,
    },

    sigChan: make(chan os.Signal, 1),
  }

  handleRoom := func(w http.ResponseWriter, req *http.Request) {
    vars := mux.Vars(req)

    roomName := vars["roomName"]

    if room, found := server.rooms[roomName]; found {
      if room.isLocked() {
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
  router.HandleFunc("/status", server.status).Methods("GET")
  router.HandleFunc("/new", server.createNewRoom).Methods("POST")
  router.HandleFunc("/roomCount", server.roomCount).Methods("GET")
  router.HandleFunc("/rooms", server.listRooms).Methods("GET")
  router.HandleFunc("/room/{roomName}", handleRoom)
  router.HandleFunc("/room/{roomName}/{connType}", handleClient).Methods("GET")

  signal.Notify(server.sigChan, os.Interrupt)

  return server
}

func (server *Server) status(w http.ResponseWriter, req *http.Request) {
  res := struct{
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

func (server *Server) closeConn(conn *websocket.Conn) {
  fmt.Printf("Server.closeConn(): <= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error, room *Room) {
  fmt.Println("server panicked")

  for conn := range room.connClientMap {
    conn.WriteMessage(websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
        err.Error()))
  }

  server.errChan <- err
  server.panicked = true
}

func (server *Server) handleRoomSettings(room *Room, client *Client, settings *ClientSettings) (string, error) {
  if client == nil {
    fmt.Println("Server.handleRoomSettings(): called with a nil parameter")

    return "", errors.New("room.handleRoomSettings(): BUG: client == nil")
  } else if settings == nil {
    fmt.Println("Server.handleRoomSettings(): called with a nil parameter")

    return "", errors.New("Server.handleRoomSettings(): BUG: settings == nil")
  }

  server.mtx.Lock()
  defer server.mtx.Unlock()

  msg := "room changes:\n\n"
  errs := ""

  if settings.Admin.RoomName != "" && settings.Admin.RoomName != room.name {
    if false {
      fmt.Printf("Server.handleRoomSettings(): %p requested invalid room name '%v'\n",
                 client.conn, settings.Admin.RoomName)
      errs += "room name: invalid name requested\n"
    } else if server.hasRoom(settings.Admin.RoomName) {
      fmt.Printf("Server.handleRoomSettings(): %p requested unavailable room name '%v'\n",
                 client.conn, settings.Admin.RoomName)

      msg += "room name: requested name already taken\n"
    } else {
      server.renameRoom(room, settings.Admin.RoomName)
      msg += "room name: changed\n"
    }
  } else {
    msg += "room name: unchanged\n"
  }

  if settings.Admin.NumSeats != room.table.NumSeats {
    if err := room.table.SetNumSeats(settings.Admin.NumSeats); err != nil {
      errs += "num seats: " + err.Error() + "\n"
    } else {
      msg += "num seats: changed\n"
    }
  } else {
    msg += "num seats: unchanged"
  }

  if errs != "" {
    return "", errors.New(errs)
  }

  return msg, nil
}

type RoomOpts struct {
  RoomName string          `json:"roomName"`
  NumSeats uint8           `json:"numSeats"`
  Lock     poker.TableLock `json:"lock"`
  Password string          `json:"password"`
}

type RoomList struct {
  RoomName     string          `json:"roomName"`
  TableLock    poker.TableLock `json:"tableLock"`
  NeedPassword bool            `json:"needPassword"`
  NumSeats     uint8           `json:"numSeats"`
  NumPlayers   uint8           `json:"numPlayers"`
  NumOpenSeats uint8           `json:"numOpenSeats"`
  NumConnected uint64          `json:"numConnected"`
}

func (server *Server) roomCount(w http.ResponseWriter, req *http.Request) {
  roomCnt := len(server.rooms)

  res := struct{
    RoomCount int `json:"roomCount"`
  }{
    RoomCount: roomCnt,
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

// NOTE: caller needs to handle server locking
func (server *Server) hasRoom(name string) bool {
  _, found := server.rooms[name]

  return found
}

func (server *Server) listRooms(w http.ResponseWriter, req *http.Request) {
  roomListArr := make([]RoomList, 0)

  for name, room := range server.rooms {
    table := room.table

    roomListArr = append(
      roomListArr,
      RoomList{
        RoomName:     name,
        TableLock:    table.Lock,
        NeedPassword: table.Password != "",
        NumSeats:     table.NumSeats,
        NumPlayers:   table.NumPlayers,
        NumOpenSeats: table.NumSeats - table.NumPlayers,
        NumConnected: table.NumConnected,
      },
    )
  }

  jsonBody, err := json.Marshal(roomListArr)
  if err != nil {
    http.Error(w, "failed to encode JSON", http.StatusInternalServerError)

    return
  }

  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(http.StatusOK)
  w.Write(jsonBody)
}

func (server *Server) createNewRoom(w http.ResponseWriter, req *http.Request) {
  server.mtx.Lock()
  defer server.mtx.Unlock()

  var roomOpts RoomOpts
  if err := json.NewDecoder(req.Body).Decode(&roomOpts); err != nil {
    fmt.Printf("Server.createNewRoom(): problem decoding POST request: %v\n", err)
    http.Error(w, "failed to parse JSON body", http.StatusBadRequest)

    return
  }

  fmt.Printf("Server.createNewRoom(): roomOpts: %v\n", roomOpts)

  if roomOpts.RoomName == "" || server.rooms[roomOpts.RoomName] != nil {
    if server.rooms[roomOpts.RoomName] != nil {
      fmt.Printf("Server.createNewRoom(): roomName %s already taken\n", roomOpts.RoomName)
    }

    for {
      roomOpts.RoomName = poker.RandString(10) // 62^10 is plenty ;)
      if _, found := server.rooms[roomOpts.RoomName]; found {
        fmt.Printf("Server.createNewRoom(): WARNING: possible bug: roomName '%s' already found in rooms\n",
                   roomOpts.RoomName)
      } else {
        break
      }
    }
  }

  if roomOpts.NumSeats < 2 || roomOpts.NumSeats > 7 {
    fmt.Printf("Server.createNewRoom(): requested NumSeats (%v) out of range. setting numSeats to default (7 seats)\n",
               roomOpts.NumSeats)
    roomOpts.NumSeats = 7
  }

  deck := poker.NewDeck()

  poker.RandSeed()
  deck.Shuffle()

  table, tableErr := poker.NewTable(deck, roomOpts.NumSeats, roomOpts.Lock, roomOpts.Password,
                                    make([]bool, roomOpts.NumSeats))
  if tableErr != nil {
    fmt.Printf("Server.createNewRoom(): problem creating new table: %v\n", tableErr)
    http.Error(w, fmt.Sprintf("couldn't create a new table: %v", tableErr), http.StatusBadRequest)

    return
  }

  fmt.Printf("table.Lock: %v table.Password: %v table.NumSeats: %v\n", table.Lock, table.Password, table.NumSeats)

  fmt.Printf("Server.createNewRoom(): creating new room with roomName `%s`\n", roomOpts.RoomName)

  room := NewRoom(roomOpts.RoomName, table, poker.RandString(17))
  server.rooms[roomOpts.RoomName] = room

  res := struct{
    URL string          `json:"URL"`
    CreatorToken string `json:"creatorToken"`
  }{
    URL: fmt.Sprintf("/room/%s", roomOpts.RoomName),
    CreatorToken: room.creatorToken,
  }

  jsonBody, err := json.Marshal(res);
  if err != nil {
    http.Error(w, "failed to encode JSON", http.StatusInternalServerError)

    return
  }

  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(http.StatusOK)
  w.Write(jsonBody)
}

func (server *Server) removeRoom(room *Room) {
  server.mtx.Lock()
  defer server.mtx.Unlock()

  if _, found := server.rooms[room.name]; found {
    fmt.Printf("Server.roomRoom: removing room '%s'\n", room.name)

    delete(server.rooms, room.name)
  } else {
    fmt.Printf("Server.roomRoom: room '%s' not found\n", room.name)
  }
}

// NOTE: caller needs to handle server locking
func (server *Server) renameRoom(room *Room, newName string) {
  delete(server.rooms, room.name)
  room.name = newName
  server.rooms[newName] = room
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

  if client := room.connClientMap[conn]; client != nil {
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
    fmt.Printf("Server.handleNewConn(): %p had nil ClientSettings, using defaults\n", conn)
    netData.Client.Settings = NewClientSettings()
  }

  // check if this connection was the room creator
  if room.creatorToken != "" &&
     netData.Client.Settings.Password == room.creatorToken {
    room.connClientMap[conn] = &Client{}

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
      if player := room.table.GetOpenSeat(); player != nil {
        client.Player = player
        room.playerClientMap[player] = client

        processClient()

        fmt.Printf("Server.handleNewConn(): {%s}: adding <%s> (%p) (%s) as player '%s'\n",
                   room.name, client.ID, &conn, client.Name, player.Name)

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
        panic(fmt.Sprintf("Server.handleNewConn(): {%s}: GetOpenSeats() failed for a room creator", room.name))
      }

      room.makeAdmin(client)
    } else {
      processClient()
    }

    fmt.Printf("Server.handleNewConn(): {%s}: %v (%v) used creatorToken (%v), removing token\n",
               room.name, client.Name, client.ID, room.creatorToken)

    room.creatorToken = "" // token gets invalidated after first use

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
  room.connClientMap[conn] = &Client{}

  client := room.newClient(conn, connType, netData.Client.Settings)

  if _, err := room.handleClientSettings(client, netData.Client.Settings); err != nil {
    (&NetData{
      room: room,
      Response: NetDataBadRequest,
      Client: client,
      Msg: err.Error(),
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
    } else if player := room.table.GetOpenSeat(); player != nil {
      client.Player = player
      room.playerClientMap[player] = client

      room.applyClientSettings(client, netData.Client.Settings)
      fmt.Printf("Server.handleNewConn(): {%s}: adding <%s> (%p) (%s) as player '%s'\n",
                 room.name, client.ID, &conn, client.Name, player.Name)

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
    } else if room.table.Lock == poker.TableLockSpectators {
        room.sendLock(conn, connType)

        return
    } else {
      netData.Response = NetDataServerMsg
      netData.Msg = "No open seats available. You have been added as a spectator"

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

  if client := room.connClientMap[conn]; client != nil {
    netData.ClearData(client)
    netData.Response = NetDataServerMsg
    netData.Msg = "reconnect attempted while connected to server"

    netData.Send()

    return
  }

  // We put the private ID in the Msg member so we don't need to add an extra
  // member to the struct. An extra member would almost never be used and is
  // more likely be leaked to others via programmer error.
  if client, ok := room.privIDClientMap[netData.Msg]; ok {
    client.conn = conn

    room.connClientMap[conn] = client
    // XXX race w/ WSClient defer, time to consider a mutex on Player
    if client.reconnectTimer != nil {
      client.reconnectTimer.Stop()
    }
    client.isDisconnected = false
    netData.ClearData(client)
    netData.Response = NetDataPlayerReconnected
    room.sendResponseToAll(&netData, nil)

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
    fmt.Printf("Server.WSClient(): {%s}: connType '%s' is invalid.\n", room.name, connType)

    return
  }

  conn, err := server.upgrader.Upgrade(w, req, nil)
  if err != nil {
    fmt.Printf("WS upgrade err %s\n", err.Error())

    return
  }

  conn.SetReadLimit(server.MaxConnBytes)
  conn.EnableWriteCompression(true)
  conn.SetCompressionLevel(flate.BestCompression)

  // TODO: move me
  playerCleanup := func(client *Client, isClientExit bool) {
    if client != nil && client.Player != nil {
      player := client.Player

      if room.table.ActivePlayers().Len > 1 &&
         room.table.CurPlayer() != nil &&
         room.table.CurPlayer().Player.Name == player.Name {
        room.table.CurPlayer().Player.Action.Action = playerState.Fold
        room.table.SetNextPlayerTurn()
        room.sendPlayerTurnToAll()
      }

      room.removePlayer(client, isClientExit, !isClientExit)
    }
  }

  cleanExit := false
  defer func() {
    if server.panicked { // room panic was already recovered in previous client handler
      return
    }

    if err := recover(); err != nil {
      server.serverError(poker.PanicRetToError(err), room)
    } else { // not a room panic()
      if client, ok := room.connClientMap[conn]; ok {
        minsToWait := 0 * time.Minute

        client.isDisconnected = true

        if !cleanExit {
          fmt.Printf("Server.WSClient: <%s> unclean exit, waiting 1 min for reconnect until cleanup\n", client.ID)

          if client.Player != nil {
            room.sendResponseToAll(&NetData{
              Response: NetDataPlayerReconnecting,
              Client: client,
            }, client)
          }
          minsToWait = 1 * time.Minute
        }

        delete(room.connClientMap, conn)
        server.closeConn(conn)

        if client.reconnectTimer != nil {
          client.reconnectTimer.Stop()
        }
        // the 0 min gofunc is kinda dumb, but they're cheap and it eliminates
        // some redundancy
        client.reconnectTimer = time.AfterFunc(minsToWait, func() {
          if !client.isDisconnected {
            return
          }

          // if IsLocked is true then there must be at least one other client
          if !room.IsLocked && room.table.NumConnected == 1 {
            fmt.Printf("Server.WSClient(): <%s> defer(): {%s}: last client left, skipping player & client cleanup\n", client.ID, room.name)
            server.removeRoom(room)
            return
          }

          playerCleanup(client, true)
          room.removeClient(client)
        })
      } else {
        fmt.Printf("Server.WSClient(): defer(): {%s}: couldn't find conn %p in connClientMap\n", room.name, conn)
      }
    }
  }()

  fmt.Printf("Server.WSClient(): {%s}: => new conn from %s\n", room.name, req.Host)

  stopPing := make(chan bool)
  go func() {
    ticker := time.NewTicker(10 * time.Second)

    for {
      select {
      case <-stopPing:
        return
      case <-ticker.C:
        if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
          fmt.Printf("Server.WSClient(): {%s}: ping err: %s\n", room.name, err.Error())
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
    case NetDataClientSettings: // TODO: check pointers
      if !room.TryLock() {
        netData.ClearData(client)
        netData.Response = NetDataServerMsg
        netData.Msg = "cannot change your settings right now. please try again later"
        netData.Send()

        returnFromInputLoop <- false
        return
      }

      settings := *netData.Client.Settings

      if client.ID == room.tableAdminID {
        msg, err := server.handleRoomSettings(room, client, netData.Client.Settings)
        if err == nil {
          netData.ClearData(nil)
          netData.Response = NetDataRoomSettings

          room.sendResponseToAll(&netData, nil)

          netData.ClearData(client)
          netData.Response = NetDataServerMsg
          netData.Msg = msg
          netData.Send()
        } else {
          netData.ClearData(client)
          netData.Response = NetDataServerMsg
          netData.Msg = err.Error()
          netData.Send()
        }
      }

      netData.Client.Settings = &settings

      msg, err := room.handleClientSettings(client, netData.Client.Settings)
      if err == nil {
        room.applyClientSettings(client, netData.Client.Settings)

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
        netData.ClearData(client)
        netData.Response = NetDataServerMsg
        netData.Msg = err.Error()
        netData.Send()
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

      if client.Player != nil {
        netData.Msg = fmt.Sprintf("[%s id: %s]: %s", client.Name,
                                  netData.Client.ID[:7], netData.Msg)
      } else {
        netData.Msg = fmt.Sprintf("{%s id: %s}: %s", client.Name,
                                  netData.Client.ID[:7], netData.Msg)
      }

      room.sendResponseToAll(&netData, nil)
    case NetDataAllIn, NetDataBet, NetDataCall, NetDataCheck, NetDataFold:
      if room.IsLocked {
        fmt.Printf("<%s> (%p) (%s) tried to send action while room mtx was locked.\n",
                   client.ID, &client.conn, client.Name)
        netData.ClearData(client)
        netData.Response = NetDataBadRequest
        netData.Msg = "that action is not valid at this time"

        netData.Send()

        returnFromInputLoop <- false
        return
      }

      if client.Player == nil {
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

      if client.Player.Name != room.table.CurPlayer().Player.Name {
        netData.ClearData(client)
        netData.Response = NetDataBadRequest
        netData.Msg = "it's not your turn"

        netData.Send()

        returnFromInputLoop <- false
        return
      }

      room.Lock()

      if err := room.table.PlayerAction(client.Player, netData.Client.Player.Action);
         err != nil {
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
            fmt.Printf("Server.WSClient(): {%s}: cli: readConn() conn: %p err: %v\n", room.name, conn, err)
          } else {
            fmt.Printf("Server.WSClient(): {%s} cli: readConn() conn %p ws closed cleanly: %v\n", room.name, conn, err)
            cleanExit = true
          }

          return
        }

        // we need to set Table member to nil otherwise gob will
        // modify our room.table structure if a user sends that member
        nd := NetData{Response: NetDataNewConn, Table: nil}

        if err := gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&nd);
          err != nil {
          fmt.Printf("Server.WSClient(): {%s}: cli: %p had a problem decoding gob stream: %s\n",
                     room.name, conn, err.Error())

          return
        }

        nd.Table = room.table

        fmt.Printf("Server.WSClient(): {%s}: cli: recv %s (%d bytes) from %p\n",
                   room.name, nd.NetActionToString(), len(rawData), conn)

        if int64(len(rawData)) > server.MaxConnBytes {
          fmt.Printf("Server.WSClient(): {%s}: cli: conn: %p sent too many bytes (> %v)\n",
                     room.name, conn, server.MaxConnBytes)
          return
        }

        netData = nd
      } else { // webclient
        _, rawData, err := conn.ReadMessage()
        if err != nil {
          if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
            fmt.Printf("Server.WSClient(): {%s}: web: readConn() conn: %p err: %v\n", room.name, conn, err)
          } else {
            fmt.Printf("Server.WSClient(): {%s} web: readConn() conn %p ws closed cleanly: %v\n", room.name, conn, err)
            cleanExit = true
          }

          return
        }

        err = msgpack.Unmarshal(rawData, &netData)
        if err != nil {
          fmt.Printf("Server.WSClient(): {%s}: web: %p had a problem decoding msgpack steam: %s\n",
                     room.name, conn, err.Error())

          return
        }

        if netData.Client != nil {
          if netData.Client.conn == nil {
            netData.Client.conn = conn
          }
          if netData.Client.Settings == nil {
            netData.Client.Settings = &ClientSettings{}
          }
        } else {
          fmt.Printf("Server.WSClient(): {%s}: web: WARNING: (%p) netData.Client == nil\n", room.name, conn)
        }

        fmt.Printf("Server.WSClient(): {%s}: web: recv msgpack: %v nd.Request == %v\n",
                   room.name, netData, netData.Request)
        fmt.Printf("Server.WSClient(): {%s}: web: nd %s\n", room.name, netData.NetActionToString())
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
        client := room.connClientMap[conn]
        go handleAsyncRequest(client, netData)
      } // else{} end
    } // returnFromInputLoop select end
  } //for loop end
} // func end

func (server *Server) Run() error {
  fmt.Printf("Server.Run(): starting server on %v\n", server.http.Addr)

  go func() {
    if err := server.http.ListenAndServe(); err != nil {
      fmt.Printf("Server.Run(): http.ListenAndServe(): %s\n", err.Error())
    }
  }()

  select {
  case sig := <-server.sigChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "received signal: %s\n", sig.String())

    // TODO: ignore irrelevant signals
    for _, room := range server.rooms {
      room.sendResponseToAll(&NetData{Response: NetDataServerClosed}, nil)
    }

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.http.Shutdown(): %s\n", err.Error())
      return err
    }

    return nil
  case err := <-server.errChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "irrecoverable server error: %s\n", err.Error())

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.http.Shutdown(): %s\n", err.Error())
      return err
    }

    return err
  }
}
