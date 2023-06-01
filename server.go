package main

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
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

const MaxClientNameLen = 20
type Client struct {
  ID       string
  Name     string
  Player   *Player
  Settings *ClientSettings // XXX: Settings.Name is redundant now
  conn     *websocket.Conn
  connType string
}

func NewClient(settings *ClientSettings) *Client {
  client := &Client{
    Settings: settings,
  }

  return client
}

func (client *Client) SetName(name string) {
  if len(name) > MaxClientNameLen {
    fmt.Printf("Client.SetName(): requested name too long. rejecting\n")
    return
  }

  fmt.Printf("Client.SetName(): <%s> (%p) '%s' => '%s'\n", client.ID, client.conn, client.Name, name)
  client.Name = name
}

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

    if _, found := server.rooms[roomName]; found {
      w.WriteHeader(http.StatusOK)
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

// TODO: important: need to ensure these pointers
// will be consistent throughout the program (mainly Player pointers)
type Room struct {
  name string

  connClientMap map[*websocket.Conn]*Client
  playerClientMap map[*Player]*Client
  IDClientMap map[string]*Client
  nameClientMap map[string]*Client

  table *Table
  tableAdminID string

  creatorToken string

  IsLocked bool
  mtx sync.Mutex
}

func NewRoom(name string, table *Table, creatorToken string) *Room {
  return &Room{
    name: name,

    connClientMap: make(map[*websocket.Conn]*Client),
    playerClientMap: make(map[*Player]*Client),
    IDClientMap: make(map[string]*Client),
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
  for _, client := range room.connClientMap {
    if except == nil || client.ID != except.ID {
      netData.SendTo(client)
    }
  }
}

func (room *Room) getPlayerClient(player *Player) *Client {
  if (player == nil ) {
    fmt.Println("Room.getPlayerClient(): player == nil")
    return nil
  }

  if client, ok := room.playerClientMap[player]; ok {
    return client
  }
  fmt.Printf("Room.getPlayerClient(): WARNING: player (%s) not found in playerClientMap\n", player.Name)

  for _, client := range room.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return client
    }
  }
  fmt.Printf("Room.getPlayerClient(): WARNING: player %s not found in connClientMap\n", player.Name)

  return nil
}

func (room *Room) getPlayerConn(player *Player) *websocket.Conn {
  if (player == nil) {
    fmt.Println("Room.getPlayerConn(): player == nil")
    return nil
  }

  if client, ok := room.playerClientMap[player]; ok {
    return client.conn
  }
  fmt.Printf("Room.getPlayerConn(): WARNING: player (%s) not found in playerClientMap\n", player.Name)

  for conn, client := range room.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return conn
    }
  }
  fmt.Printf("Room.getPlayerConn(): WARNING: player %s not found in connClientMap\n", player.Name)

  return nil
}

func (room *Room) removeClient(conn *websocket.Conn) {
  room.table.mtx.Lock()
  defer room.table.mtx.Unlock()

  if (conn == nil) {
    fmt.Println("Room.removeClient(): conn == nil")
    return
  }

  client := room.connClientMap[conn]
  if client == nil {
    fmt.Printf("Room.removeClient(): couldn't find conn %p in connClientMap\n", conn)
  } else {
    delete(room.nameClientMap, client.Name)
    delete(room.IDClientMap, client.ID)
    delete(room.connClientMap, conn)
    delete(room.playerClientMap, client.Player)

    // NOTE: connections that don't become clients (e.g. in the case of a lock)
    //       never increment NumConnected
    room.table.NumConnected--
  }

  // TODO: send client info
  netData := &NetData{
    Response: NetDataClientExited,
    Table:    room.table,
  }

  room.sendResponseToAll(netData, nil)
}

func (room *Room) removePlayer(client *Client, calledFromClientExit bool) {
  reset := false // XXX race condition guard
  noPlayersLeft := false // XXX race condition guard

  room.table.mtx.Lock()
  defer func() {
    fmt.Println("Room.removePlayer(): cleanup defer CALLED");
    if reset {
      if calledFromClientExit {
        room.Lock()
      }

      if noPlayersLeft {
        fmt.Println("Room.removePlayers(): no players left, resetting")
        room.table.reset(nil)
        room.sendReset(nil)
      } else if !calledFromClientExit {
        fmt.Println("Room.removePlayer(): !calledFromClientExit, returning")
        return
      } else if room.table.State == TableStateNotStarted {
        fmt.Printf("Room.removePlayer(): State == TableStateNotStarted\n")
      } else {
        // XXX: if a player who hasn't bet preflop is
        //      the last player left he receives the mainpot chips.
        //      if he's a blind he should also get (only) his blind chips back.
        fmt.Println("Room.removePlayer(): state != (rndovr || gameovr)")

        room.table.finishRound()
        room.table.State = TableStateGameOver
        room.gameOver()
      }

      room.Unlock()
    } else if room.table.State == TableStateDoneBetting ||
              room.table.State == TableStateRoundOver {
      if calledFromClientExit {
        room.Lock()
      }

      fmt.Println("Room.removePlayer(): defer postPlayerAction")
      room.postPlayerAction(nil, &NetData{})

      if calledFromClientExit {
        room.Unlock()
      }
    }
  }()
  defer room.table.mtx.Unlock()

  table := room.table

  if player := client.Player; player != nil { // else client was a spectator
    fmt.Printf("Room.removePlayer(): removing %s\n", player.Name)

    table.activePlayers.RemovePlayer(player)
    table.curPlayers.RemovePlayer(player)

    player.Clear()

    table.NumPlayers--

    netData := &NetData{
      Client:     client,
      Response:   NetDataPlayerLeft,
      Table:      table,
    }
    room.sendResponseToAll(netData, client)

    client.Player = nil
    delete(room.playerClientMap, player)

    if client.ID == room.tableAdminID {
      if table.activePlayers.len == 0 {
        room.makeAdmin(nil)
      } else {
        activePlayerHeadClient := room.getPlayerClient(table.activePlayers.head.Player)
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
  if room.table.curPlayer == nil {
    fmt.Println("Room.sendPlayerTurn(): curPlayer == nil")
    return
  }

  curPlayer := room.table.curPlayer.Player
  curPlayerClient := room.getPlayerClient(curPlayer)
  if curPlayerClient == nil {
    panic(fmt.Sprintf("Room.sendPlayerTurn(): BUG: %s not found in connClientMap\n",
                     curPlayer.Name))
  }

  netData := &NetData{
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
  if room.table.curPlayer == nil {
    fmt.Println("Room.sendPlayerTurnToAll(): curPlayer == nil")
    return
  }

  curPlayer := room.table.curPlayer.Player
  curPlayerClient := room.getPlayerClient(curPlayer)
  if curPlayerClient == nil {
    panic(fmt.Sprintf("Room.sendPlayerTurnToAll(): BUG: curPlayer <%s> not found in any maps\n",
                      curPlayer.Name))
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
    fmt.Println("Room.sendPlayerHead(): sending clear player head")
    room.sendResponseToAll(
      &NetData{
        Response: NetDataPlayerHead,
      }, nil)

    return
  }

  playerHeadNode := room.table.curPlayers.head
  curPlayerNode := room.table.curPlayer
  if playerHeadNode != nil && curPlayerNode != nil &&
     playerHeadNode.Player.Name != curPlayerNode.Player.Name {
    playerHead := playerHeadNode.Player
    playerHeadClient := room.getPlayerClient(playerHead)
    if playerHead == nil {
      panic(fmt.Sprintf("Room.sendPlayerTurnToAll(): BUG: playerHead <%s> " +
                        "not found in any maps\n", playerHead.Name))
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

func (room *Room) sendPlayerActionToAll(player *Player, client *Client) {
  fmt.Printf("Room.sendPlayerActionToAll(): <%s> sending %s\n",
             player.Name, player.ActionToString())

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
  netData := &NetData{Response: NetDataDeal, Table: room.table}

  for _, player := range room.table.curPlayers.ToPlayerArray() {
    client := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = client

    netData.Send()
  }
}

func (room *Room) sendHands() {
  netData := &NetData{Response: NetDataShowHand, Table: room.table}

  for _, player := range room.table.curPlayers.ToPlayerArray() {
    client := room.playerClientMap[player]
    //assert(client != nil, "Room.sendHands(): player not in playerMap")
    netData.Client = room.publicClientInfo(client)

    room.sendResponseToAll(netData, client)
  }
}

// NOTE: hand is currently computed on client side
func (room *Room) sendCurHands() {
  netData := &NetData{Response: NetDataCurHand, Table: room.table}

  for _, client := range room.playerClientMap {
    netData.Client = client
    netData.Send()
  }
}

func (room *Room) sendActivePlayers(client *Client) {
  if client == nil {
    fmt.Println("Room.sendActivePlayers(): conn is nil")
    return
  }

  netData := &NetData{
    Response: NetDataCurPlayers,
    Table:    room.table,
  }

  for _, player := range room.table.activePlayers.ToPlayerArray() {
    playerClient := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = room.publicClientInfo(playerClient)
    netData.SendTo(client)
  }
}

func (room *Room) sendAllPlayerInfo(client *Client, curPlayers bool, sendToSelf bool) {
  netData := &NetData{Response: NetDataUpdatePlayer}

  var players playerList
  if curPlayers {
    players = room.table.curPlayers
  } else {
    players = room.table.activePlayers
  }

  for _, player := range players.ToPlayerArray() {
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

  for _, player := range room.table.getEliminatedPlayers() {
    client := room.connClientMap[room.getPlayerConn(player)]
    netData.Client = client
    netData.Response = NetDataEliminated
    netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", client.Player.Name,
                              netData.Client.ID[:7])

    room.removePlayer(client, false)
    room.sendResponseToAll(netData, nil)
  }
}

func (room *Room) sendLock(conn *websocket.Conn, connType string) {
  fmt.Printf("Room.sendLock(): locked out %p with %s\n", conn,
             room.table.TableLockToString())

  netData := &NetData{
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataTableLocked,
    Msg: fmt.Sprintf("table lock: %s", room.table.TableLockToString()),
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (room *Room) sendBadAuth(conn *websocket.Conn, connType string) {
  fmt.Printf("Room.sendBadAuth(): %p had bad authentication\n", conn)

  netData := &NetData{
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataBadAuth,
    Msg: "your password was incorrect",
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (room *Room) makeAdmin(client *Client) {
  if client == nil {
    fmt.Printf("Room.makeAdmin(): client is nil, unsetting tableAdmin\n")
    room.tableAdminID = ""
    return
  } else {
    fmt.Printf("Room.makeAdmin(): making <%s> (%s) table admin\n", client.ID, client.Name)
    room.tableAdminID = client.ID
  }

  netData := &NetData{
    Client: client,
    Response: NetDataMakeAdmin,
    Table: room.table,
  }

  netData.Send()
}

func (room *Room) newRound() {
  room.table.newRound()
  room.table.nextTableAction()
  room.checkBlindsAutoAllIn()
  room.sendDeals()

  // XXX
  room.table.mtx.Lock()
  realRoundState := room.table.State
  room.table.State = TableStateNewRound
  room.sendAllPlayerInfo(nil, true, false)
  room.sendPlayerTurnToAll()
  room.table.State = realRoundState
  room.table.mtx.Unlock()

  room.sendTable()
}

// NOTE: called w/ room lock acquired in handleAsyncRequest()
func (room *Room) roundOver() {
  if room.table.State == TableStateReset ||
     room.table.State == TableStateNewRound {
    //room.mtx.Unlock()
    return
  }

  room.table.finishRound()
  room.sendHands()

  netData := &NetData{
    Response: NetDataRoundOver,
    Table:    room.table,
    Msg:      room.table.WinInfo,
  }

  for i, sidePot := range room.table.sidePots.GetAllPots() {
    netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
  }

  room.sendResponseToAll(netData, nil)
  room.sendAllPlayerInfo(nil, false, true)

  room.removeEliminatedPlayers()

  if room.table.State == TableStateGameOver {
    time.Sleep(5 * time.Second)
    room.gameOver()

    return
  }

  time.Sleep(5 * time.Second)
  room.newRound()
}

func (room *Room) gameOver() {
  fmt.Printf("Room.gameOver(): ** game over %s wins **\n", room.table.Winners[0].Name)
  winner := room.table.Winners[0]

  netData := &NetData{
    Response: NetDataServerMsg,
    Msg:      "game over, " + winner.Name + " wins",
  }

  room.sendResponseToAll(netData, nil)

  room.table.reset(winner) // make a new game while keeping winner connected

  winnerClient := room.getPlayerClient(winner)
  if winnerClient == nil {
    fmt.Printf("Room.getPlayerClient(): winner (%s) not found in any maps\n", winner.Name)
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
  if room.table.SmallBlind.Player.Action.Action == NetDataAllIn {
    fmt.Printf("Room.checkBlindsAutoAllIn(): smallblind (%s) forced to go all in\n",
               room.table.SmallBlind.Player.Name)

    if room.table.curPlayer.Player.Name == room.table.SmallBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      room.table.setNextPlayerTurn()
    } else {
      room.table.curPlayers.RemovePlayer(room.table.SmallBlind.Player)
    }

    room.sendPlayerActionToAll(room.table.SmallBlind.Player, nil)
  }
  if room.table.BigBlind.Player.Action.Action == NetDataAllIn {
    fmt.Printf("Room.checkBlindsAutoAllIn(): bigblind (%s) forced to go all in\n",
               room.table.BigBlind.Player.Name)

    if room.table.curPlayer.Player.Name == room.table.BigBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      room.table.setNextPlayerTurn()
    } else {
      room.table.curPlayers.RemovePlayer(room.table.BigBlind.Player)
    }

    room.sendPlayerActionToAll(room.table.BigBlind.Player, nil)
  }
}

func (room *Room) postBetting(player *Player, netData *NetData, client *Client) {
  if client != nil {
    player := client.Player
    defer func() {
      client.Player = player
    }()
  }

  if player != nil {
    room.sendPlayerActionToAll(player, client)
    time.Sleep(2 * time.Second)
    room.sendPlayerTurnToAll()
  }

  fmt.Println("Server.postBetting(): done betting...")

  if room.table.bettingIsImpossible() {
    fmt.Println("Server.postBetting(): no more betting possible this round")

    tmpReq := netData.Request
    tmpClient := netData.Client

    netData.Request = 0
    netData.Table = room.table
    netData.Client = nil
    for room.table.State != TableStateRoundOver {
      room.table.nextCommunityAction()
      netData.Response = room.table.commState2NetDataResponse()

      room.sendResponseToAll(netData, nil)

      time.Sleep(2500 * time.Millisecond)
    }

    netData.Request = tmpReq
    netData.Client = tmpClient
  } else {
    room.table.nextCommunityAction()
  }

  if room.table.State == TableStateRoundOver {
    room.roundOver()

    //if room.table.State == TableStateGameOver {
    //  ;
    //}
  } else { // new community card(s)
    netData.Response = room.table.commState2NetDataResponse()
    netData.Table = room.table
    if client != nil {
      netData.Client.Player = nil
    }

    room.sendResponseToAll(netData, nil)

    room.table.Bet, room.table.better = 0, nil
    for _, player := range room.table.curPlayers.ToPlayerArray() {
      fmt.Printf("Server.postBetting(): clearing %v's action\n", player.Name)
      player.Action.Clear()
    }

    room.sendAllPlayerInfo(nil, true, true)
    room.table.reorderPlayers()
    room.sendPlayerTurnToAll()
    room.sendPlayerHead(nil, true)
    // let players know they should update their current hand after
    // the community action
    room.sendCurHands()
  }
}

func (room *Room) postPlayerAction(client *Client, netData *NetData) {
  var player *Player = nil

  if client != nil {
    player = client.Player
    defer func() {
      client.Player = player
    }()
  }

  if room.table.State == TableStateDoneBetting {
    room.postBetting(player, netData, client)
  } else if room.table.State == TableStateRoundOver {
    // all other players folded before all comm cards were dealt
    // TODO: check for this state in a better fashion
    room.table.finishRound()
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

    if room.table.State == TableStateGameOver {
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

func (room *Room) publicClientInfo(client *Client) *Client {
  pubClient := *client
  pubClient.Player = room.table.PublicPlayerInfo(*client.Player)

  return &pubClient
}

type ClientSettings struct {
  Name     string
  Password string

  Admin struct {
    RoomName string
    Lock     TableLock
    Password string
  }
}

func (server *Server) handleRoomSettings(room *Room, client *Client, settings *ClientSettings) (string, error) {
  if client == nil {
    fmt.Println("Server.handleRoomSettings(): called with a nil parameter")

    return "", errors.New("room.handleRoomSettings(): BUG: client == nil")
  } else if settings == nil {
    fmt.Println("Server.handleRoomSettings(): called with a nil parameter")

    return "", errors.New("Server.handleClientSettings(): BUG: settings == nil")
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

  if errs != "" {
    return "", errors.New(errs)
  }

  return msg, nil
}

func (room *Room) handleClientSettings(client *Client, settings *ClientSettings) (string, error) {
  msg := ""
  errs := ""

  const (
    MaxNameLen uint8 = 15
    MaxPassLen uint8 = 50
  )

  if client == nil {
    fmt.Println("Server.handleClientSettings(): called with a nil parameter")

    return "", errors.New("room.handleClientSettings(): BUG: client == nil")
  } else if settings == nil {
    fmt.Println("Server.handleClientSettings(): called with a nil parameter")

    return "", errors.New("Server.handleClientSettings(): BUG: settings == nil")
  }

  fmt.Printf("Server.handleClientSettings(): <%s> settings: %v\n", client.Name, settings)

  settings.Name = strings.TrimSpace(settings.Name)
  if settings.Name != "" {
    if len(settings.Name) > int(MaxNameLen) {
      fmt.Printf("Server.handleClientSettings(): %p requested a name that was longer " +
                 "than %v characters. using a default name\n", client.conn, MaxNameLen)
      msg += fmt.Sprintf("You've requested a name that was longer than %v characters. " +
              "Using a default name.\n\n", MaxNameLen)
      settings.Name = ""
    } else {
      if player := client.Player; player != nil {
        if player.Name == settings.Name {
          fmt.Println("Server.handleClientSettings(): name unchanged")
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
        for _, player := range room.table.players {
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

    lock := TableLockToString(settings.Admin.Lock)
    if lock == "" {
      fmt.Printf("Server.handleClientSettings(): %p requested invalid table lock '%v'\n",
                 client.conn, settings.Admin.Lock)
      errs += fmt.Sprintf("invalid table lock: '%v'\n", settings.Admin.Lock)
    } else if settings.Admin.Lock == room.table.Lock {
      fmt.Println("Server.handleClientSettings(): table lock unchanged")
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
      fmt.Println("Server.handleClientSettings(): table password unchanged")
      msg += "table password: unchanged\n"
    }
  }

  if errs != "" {
    errs = "server response: unable to complete request due to following errors:\n\n" + errs
    return "", errors.New(errs)
  }

  msg = "server response: settings changes:\n\n" + msg

  fmt.Println(msg)

  return msg, nil
}

func (room *Room) applyClientSettings(client *Client, settings *ClientSettings) {
  client.Settings = settings
  if player := client.Player; player != nil {
    player.setName(settings.Name)
    client.SetName(player.Name)

    if client.ID == room.tableAdminID {
      room.table.mtx.Lock()
      room.table.Lock = settings.Admin.Lock
      room.table.Password = settings.Admin.Password
      room.table.mtx.Unlock()
    }
  } else {
    client.SetName(settings.Name)
  }
}

func (room *Room) newClient(conn *websocket.Conn, connType string, clientSettings *ClientSettings) *Client {
  room.Lock()
  defer room.Unlock()

  client, ID := &Client{conn: conn, connType: connType}, ""
  for {
    ID = randString(10) // 62^10 is plenty ;)
    if _, found := room.IDClientMap[ID]; !found {
      client.ID = ID
      room.IDClientMap[ID] = client
      room.connClientMap[conn] = client

      break
    } else {
      fmt.Printf("room.newClient(): WARNING: possible bug: ID '%s' already found in IDClientMap\n", ID)
    }
  }

  client.SetName(clientSettings.Name)
  if client.Name != "" {
    room.nameClientMap[client.Name] = client
  }

  return client
}

type RoomOpts struct {
  RoomName string    `json:"roomName"`
  NumSeats uint8     `json:"numSeats"`
  Lock     TableLock `json:"lock"`
  Password string    `json:"password"`
}

type RoomList struct {
  RoomName     string    `json:"roomName"`
  TableLock    TableLock `json:"tableLock"`
  NeedPassword bool      `json:"needPassword"`
  NumSeats     uint8     `json:"numSeats"`
  NumPlayers   uint8     `json:"numPlayers"`
  NumOpenSeats uint8     `json:"numOpenSeats"`
  NumConnected uint64    `json:"numConnected"`
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
      roomOpts.RoomName = randString(10) // 62^10 is plenty ;)
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

  deck := NewDeck()

  randSeed()
  deck.Shuffle()

  table, tableErr := NewTable(deck, roomOpts.NumSeats, roomOpts.Lock, roomOpts.Password,
                              make([]bool, roomOpts.NumSeats))
  if tableErr != nil {
    fmt.Printf("Server.createNewRoom(): problem creating new table: %v\n", tableErr)
    http.Error(w, fmt.Sprintf("couldn't create a new table: %v", tableErr), http.StatusBadRequest)

    return
  }

  fmt.Printf("table.Lock: %v table.Password: %v table.NumSeats: %v\n", table.Lock, table.Password, table.NumSeats)

  fmt.Printf("Server.createNewRoom(): creating new room with roomName `%s`\n", roomOpts.RoomName)

  room := NewRoom(roomOpts.RoomName, table, randString(17))
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

  if client := room.connClientMap[conn]; client != nil {
    netData.Client = client
    netData.Response = NetDataServerMsg
    netData.Msg = "you are already connected to the room."

    netData.Send()

    return
  }

  netData.Response = NetDataNewConn

  // check if this connection was the room creator
  if room.creatorToken != "" &&
     netData.Client.Settings.Password == room.creatorToken {
    room.connClientMap[conn] = &Client{}

    client := room.newClient(conn, connType, netData.Client.Settings)

    room.table.mtx.Lock()
    room.table.NumConnected++
    room.table.mtx.Unlock()

    room.sendResponseToAll(&netData, nil)
    if player := room.table.getOpenSeat(); player != nil {
      client.Player = player
      room.playerClientMap[player] = client

      room.applyClientSettings(client, netData.Client.Settings)
      fmt.Printf("Server.handleNewConn(): adding <%s> (%p) (%s) as player '%s'\n",
                 client.ID, &conn, client.Name, player.Name)

      player.Action.Action = NetDataFirstAction
      room.table.curPlayers.AddPlayer(player)
      room.table.activePlayers.AddPlayer(player)

      if room.table.curPlayer == nil {
        room.table.curPlayer = room.table.curPlayers.head
      }

      if room.table.Dealer == nil {
        room.table.Dealer = room.table.activePlayers.head
      } else if room.table.SmallBlind == nil {
        room.table.SmallBlind = room.table.Dealer.next
      } else if room.table.BigBlind == nil {
        room.table.BigBlind = room.table.SmallBlind.next
      }

      /*netData.Client = room.publicClientInfo(client)
      netData.Response = NetDataNewPlayer
      netData.Table = room.table

      room.sendResponseToAll(&netData, client)*/

      netData.Client = client
      netData.Response = NetDataYourPlayer
      netData.Send()
    } else { // sanity check
      panic("Server.handleNewConn(): getOpenSeats() failed for a room creator")
    }

    room.makeAdmin(client)

    fmt.Printf("Server.handleNewConn(): %v (%v) used creatorToken (%v), removing token\n",
               client.Name, client.ID, room.creatorToken)

    room.creatorToken = "" // token gets invalidated after first use

    return
  }

  if room.table.Lock == TableLockAll {
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
      Response: NetDataBadRequest,
      Client: client,
      Msg: err.Error(),
    }).Send()
  }

  room.table.mtx.Lock()
  room.table.NumConnected++
  room.table.mtx.Unlock()

  room.sendResponseToAll(&netData, nil)

  // send current player info to this client
  if room.table.NumConnected > 1 {
    room.sendActivePlayers(client)
  }

  /*if client.ID == room.tableAdminID {
    msg, err := server.handleRoomSettings(room, client, netData.Client.Settings)
    if err != nil {
      (&NetData{
        Response: NetDataBadRequest,
        Client: client,
        Msg: err.Error(),
      }).Send()
    } else {
      netData.Client = client
      netData.Response = NetDataRoomSettings
      netData.Msg = msg
      netData.Send()
    }
  }

  room.applyClientSettings(client, netData.Client.Settings)
  netData.Client = client
  netData.Response = NetDataClientSettings
  netData.Send()*/

  if room.table.Lock == TableLockPlayers {
    netData.Response = NetDataServerMsg
    //netData.Client = &Client{conn: conn}
    netData.Msg = "This table is not allowing new players. " +
                  "You have been added as a spectator."
    netData.Send()
  } else if player := room.table.getOpenSeat(); player != nil {
    client.Player = player
    room.playerClientMap[player] = client

    room.applyClientSettings(client, netData.Client.Settings)
    fmt.Printf("Server.handleNewConn(): adding <%s> (%p) (%s) as player '%s'\n",
               client.ID, &conn, client.Name, player.Name)

    if room.table.State == TableStateNotStarted {
      player.Action.Action = NetDataFirstAction
      room.table.curPlayers.AddPlayer(player)
    } else {
      player.Action.Action = NetDataMidroundAddition
    }
    room.table.activePlayers.AddPlayer(player)

    if room.table.curPlayer == nil {
      room.table.curPlayer = room.table.curPlayers.head
    }

    if room.table.Dealer == nil {
      room.table.Dealer = room.table.activePlayers.head
    } else if room.table.SmallBlind == nil {
      room.table.SmallBlind = room.table.Dealer.next
    } else if room.table.BigBlind == nil {
      room.table.BigBlind = room.table.SmallBlind.next
    }

    netData.Client = room.publicClientInfo(client)
    netData.Response = NetDataNewPlayer
    netData.Table = room.table

    room.sendResponseToAll(&netData, client)

    netData.Client = client
    netData.Response = NetDataYourPlayer
    netData.Send()
  } else if room.table.Lock == TableLockSpectators {
      room.sendLock(conn, connType)

      return
  } else {
    netData.Response = NetDataServerMsg
    netData.Msg = "No open seats available. You have been added as a spectator"

    netData.Send()
  }

  room.sendAllPlayerInfo(client, false, false)

  if room.table.State != TableStateNotStarted {
    room.sendPlayerTurn(client)
  }

  //if room.tableAdminID == "" {
  //  room.makeAdmin(client)
  //}
}

func (server *Server) WSClient(w http.ResponseWriter, req *http.Request, room *Room, connType string) {
  if req.Header.Get("keepalive") != "" {
    return // NOTE: for heroku
  }

  if connType != "cli" && connType != "web" {
    fmt.Printf("Server.WSClient(): connType '%s' is invalid.\n", connType)

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

  cleanExit := false
  defer func() {
    if server.panicked { // room panic was already recovered in previous client handler
      return
    }

    if err := recover(); err != nil {
      server.serverError(panicRetToError(err), room)
    } else { // not a room panic()
      client := room.connClientMap[conn]
      if client != nil && client.Player != nil {
        player := client.Player
        if !cleanExit {
          fmt.Printf("Server.WSClient(): %s had an unclean exit\n", player.Name)
        }
        if room.table.activePlayers.len > 1 &&
           room.table.curPlayer != nil &&
           room.table.curPlayer.Player.Name == player.Name {
          room.table.curPlayer.Player.Action.Action = NetDataFold
          room.table.setNextPlayerTurn()
          room.sendPlayerTurnToAll()
        }

        room.removePlayer(client, true)
      }

      room.removeClient(conn)
      server.closeConn(conn)

      if room.table.NumConnected == 0 {
        server.removeRoom(room)
      }
    }
  }()

  fmt.Printf("Server.WSClient(): => new conn from %s\n", req.Host)

  stopPing := make(chan bool)
  go func() {
    ticker := time.NewTicker(10 * time.Second)

    for {
      select {
      case <-stopPing:
        return
      case <-ticker.C:
        if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
          fmt.Printf("Server.WSClient(): ping err: %s\n", err.Error())
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
    case NetDataClientSettings: // TODO: check pointers
      if !room.TryLock() {
        netData.ClearData(client)
        netData.Response = NetDataServerMsg
        netData.Msg = "cannot change your settings right now. please try again later"
        netData.Send()

        returnFromInputLoop <- false
        return
      }

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

      msg, err := room.handleClientSettings(client, netData.Client.Settings)
      if err == nil {
        room.applyClientSettings(client, netData.Client.Settings)

        netData.ClearData(client)
        if client.Player != nil { // send updated player info to other clients
          netData.Response = NetDataUpdatePlayer
          netData.Client = room.publicClientInfo(client)

          room.sendResponseToAll(&netData, client)
        }

        //netData.Client = client
        netData.Response = NetDataClientSettings
        netData.Send()

        room.sendTable()

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
      } else if room.table.State != TableStateNotStarted {
        netData.Response = NetDataBadRequest
        netData.Msg = "this game has already started"

        netData.Send()
      } else { // start game
        room.table.nextTableAction()

        room.sendDeals()
        room.sendAllPlayerInfo(nil, false, true)
        room.sendPlayerTurnToAll()
        room.sendTable()
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

      if room.table.State == TableStateNotStarted {
        netData.ClearData(client)
        netData.Response = NetDataBadRequest
        netData.Msg = "a game has not been started yet"

        netData.Send()

        returnFromInputLoop <- false
        return
      }

      if client.Player.Name != room.table.curPlayer.Player.Name {
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
          if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
            fmt.Printf("Server.WSClient(): cli: readConn() conn: %p err: %v\n", conn, err)
          }

          return
        }

        // we need to set Table member to nil otherwise gob will
        // modify our room.table structure if a user sends that member
        nd := NetData{Response: NetDataNewConn, Table: nil}

        if err := gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&nd);
          err != nil {
          fmt.Printf("Server.WSClient(): cli: %p had a problem decoding gob stream: %s\n", conn, err.Error())

          return
        }

        nd.Table = room.table

        fmt.Printf("Server.WSClient(): cli: recv %s (%d bytes) from %p\n",
                   nd.NetActionToString(), len(rawData), conn)

        if int64(len(rawData)) > server.MaxConnBytes {
          fmt.Printf("Server.WSClient(): cli: conn: %p sent too many bytes (> %v)\n",
                     conn, server.MaxConnBytes)
          return
        }

        netData = nd
      } else { // webclient
        _, rawData, err := conn.ReadMessage()
        if err != nil {
          if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
            fmt.Printf("Server.WSClient(): web: readConn() conn: %p err: %v\n", conn, err)
          }

          return
        }

        err = msgpack.Unmarshal(rawData, &netData)
        if err != nil {
          fmt.Printf("Server.WSClient(): web: %p had a problem decoding msgpack steam: %s\n", conn, err.Error())

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
          fmt.Printf("Server.WSClient(): web: WARNING: (%p) netData.Client == nil\n", conn)
        }

        fmt.Printf("Server.WSClient(): web: recv msgpack: %v nd.Request == %v\n", netData, netData.Request)
        fmt.Printf("Server.WSClient(): web: nd %s\n", netData.NetActionToString())
        netData.Table = room.table
      }

      if netData.Request == NetDataNewConn {
        server.handleNewConn(room, netData, conn, connType)
        /*
        netData.Request = 0

        if client := room.connClientMap[conn]; client != nil {
          netData.Client = client
          netData.Response = NetDataServerMsg
          netData.Msg = "you are already connected to the room."

          netData.Send()

          return
        }

        netData.Response = NetDataNewConn
        if room.table.Lock == TableLockAll {
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
            Response: NetDataBadRequest,
            Client: client,
            Msg: err.Error(),
          }).Send()
        }

        room.table.mtx.Lock()
        room.table.NumConnected++
        room.table.mtx.Unlock()

        room.sendResponseToAll(&netData, nil)

        // send current player info to this client
        if room.table.NumConnected > 1 {
          room.sendActivePlayers(client)
        }

        if client.ID == room.tableAdminID {
          msg, err := server.handleRoomSettings(room, client, netData.Client.Settings)
          if err != nil {
            (&NetData{
              Response: NetDataBadRequest,
              Client: client,
              Msg: err.Error(),
            }).Send()
          } else {
            netData.Client = client
            netData.Response = NetDataRoomSettings
            netData.Msg = msg
            netData.Send()
          }
        }

        room.applyClientSettings(client, netData.Client.Settings)
        netData.Client = client
        netData.Response = NetDataClientSettings
        netData.Send()

        if room.table.Lock == TableLockPlayers {
          netData.Response = NetDataServerMsg
          //netData.Client = &Client{conn: conn}
          netData.Msg = "This table is not allowing new players. " +
                        "You have been added as a spectator."
          netData.Send()
        } else if player := room.table.getOpenSeat(); player != nil {
          client.Player = player
          room.playerClientMap[player] = client

          room.applyClientSettings(client, netData.Client.Settings)
          fmt.Printf("Server.WSClient(): adding <%s> (%p) (%s) as player '%s'\n",
                     client.ID, &conn, client.Name, player.Name)

          if room.table.State == TableStateNotStarted {
            player.Action.Action = NetDataFirstAction
            room.table.curPlayers.AddPlayer(player)
          } else {
            player.Action.Action = NetDataMidroundAddition
          }
          room.table.activePlayers.AddPlayer(player)

          if room.table.curPlayer == nil {
            room.table.curPlayer = room.table.curPlayers.head
          }

          if room.table.Dealer == nil {
            room.table.Dealer = room.table.activePlayers.head
          } else if room.table.SmallBlind == nil {
            room.table.SmallBlind = room.table.Dealer.next
          } else if room.table.BigBlind == nil {
            room.table.BigBlind = room.table.SmallBlind.next
          }

          netData.Client = room.publicClientInfo(client)
          netData.Response = NetDataNewPlayer
          netData.Table = room.table

          room.sendResponseToAll(&netData, client)

          netData.Client = client
          netData.Response = NetDataYourPlayer
          netData.Send()
        } else if room.table.Lock == TableLockSpectators {
            room.sendLock(conn, connType)

            return
        } else {
          netData.Response = NetDataServerMsg
          netData.Msg = "No open seats available. You have been added as a spectator"

          netData.Send()
        }

        room.sendAllPlayerInfo(client, false, false)

        if room.table.State != TableStateNotStarted {
          room.sendPlayerTurn(client)
        }

        if room.tableAdminID == "" {
          room.makeAdmin(client)
        }*/
      } else {
        client := room.connClientMap[conn]
        go handleAsyncRequest(client, netData)
      } // else{} end
    } // returnFromInputLoop select end
  } //for loop end
} // func end

func (server *Server) run() error {
  fmt.Printf("Server.run(): starting server on %v\n", server.http.Addr)

  go func() {
    if err := server.http.ListenAndServe(); err != nil {
      fmt.Printf("Server.run(): http.ListenAndServe(): %s\n", err.Error())
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
