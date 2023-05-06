package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

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
  connClientMap map[*websocket.Conn]*Client
  playerClientMap map[*Player]*Client
  IDClientMap map[string]*Client
  nameClientMap map[string]*Client

  table *Table
  tableAdminID string

  MaxConnBytes  int64
  MaxChatMsgLen int32

  http *http.Server
  upgrader websocket.Upgrader

  sigChan chan os.Signal
  errChan chan error
  panicked bool

  mtx sync.Mutex
}

func NewServer(table *Table, addr string) *Server {
  const (
    MaxConnBytes = 10e3
    MaxChatMsgLen = 256
    IdleTimeout = 0
    ReadTimeout = 0
  )

  server := &Server{
    connClientMap: make(map[*websocket.Conn]*Client),
    playerClientMap: make(map[*Player]*Client),
    nameClientMap: make(map[string]*Client),
    IDClientMap: make(map[string]*Client),

    table: table,

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

    http: &http.Server{
      Addr:        addr,
      IdleTimeout: IdleTimeout,
      ReadTimeout: ReadTimeout,
    },

    sigChan: make(chan os.Signal, 1),
  }

  server.http.SetKeepAlivesEnabled(true)
  http.HandleFunc("/cli", server.WSCLIClient)
  http.HandleFunc("/web", server.WSWebClient)

  signal.Notify(server.sigChan, os.Interrupt)

  return server
}

func (server *Server) closeConn(conn *websocket.Conn) {
  fmt.Printf("Server.closeConn(): <= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

func (server *Server) sendResponseToAll(netData *NetData, except *Client) {
  for _, client := range server.connClientMap {
    if except == nil || client.ID != except.ID {
      netData.SendTo(client)
    }
  }
}

func (server *Server) getPlayerClient(player *Player) *Client {
  if (player == nil ) {
    fmt.Println("Server.getPlayerClient(): player == nil")
    return nil
  }

  if client, ok := server.playerClientMap[player]; ok {
    return client
  }
  fmt.Printf("Server.getPlayerClient(): WARNING: player (%s) not found in playerClientMap\n", player.Name)

  for _, client := range server.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return client
    }
  }
  fmt.Printf("Server.getPlayerClient(): WARNING: player %s not found in connClientMap\n", player.Name)

  return nil
}

func (server *Server) getPlayerConn(player *Player) *websocket.Conn {
  if (player == nil) {
    fmt.Println("Server.getPlayerConn(): player == nil")
    return nil
  }

  if client, ok := server.playerClientMap[player]; ok {
    return client.conn
  }
  fmt.Printf("Server.getPlayerConn(): WARNING: player (%s) not found in playerClientMap\n", player.Name)

  for conn, client := range server.connClientMap {
    if client.Player != nil && client.Player.Name == player.Name {
      return conn
    }
  }
  fmt.Printf("Server.getPlayerConn(): WARNING: player %s not found in connClientMap\n", player.Name)

  return nil
}

func (server *Server) removeClient(conn *websocket.Conn) {
  server.table.mtx.Lock()
  defer server.table.mtx.Unlock()

  if (conn == nil) {
    fmt.Println("Server.removeClient(): conn == nil")
    return
  }

  client := server.connClientMap[conn]
  if client == nil {
    fmt.Printf("Server.removeClient(): couldn't find conn %p in connClientMap\n", conn)
  } else {
    delete(server.nameClientMap, client.Name)
    delete(server.IDClientMap, client.ID)
    delete(server.connClientMap, conn)
    delete(server.playerClientMap, client.Player)

    // NOTE: connections that don't become clients (e.g. in the case of a lock)
    //       never increment NumConnected
    server.table.NumConnected--
  }

  // TODO: send client info
  netData := &NetData{
    Response: NetDataClientExited,
    Table:    server.table,
  }

  server.sendResponseToAll(netData, nil)
}

func (server *Server) removePlayer(client *Client) {
  reset := false // XXX race condition guard
  noPlayersLeft := false // XXX race condition guard

  server.table.mtx.Lock()
  defer func() {
    if reset {
      if noPlayersLeft {
        server.table.reset(nil)
        server.sendReset(nil)
      } else if server.table.State == TableStateNotStarted {
        return
      } else {
        // XXX: if a player who hasn't bet preflop is
        //      the last player left he receives the mainpot chips.
        //      if he's a blind he should also get (only) his blind chips back.
        if server.table.State != TableStateRoundOver &&
           server.table.State != TableStateGameOver {
          fmt.Println("Server.removePlayer(): state != (rndovr || gameovr)")
          server.table.finishRound()
          server.table.State = TableStateGameOver
          server.gameOver()
        } else {
          fmt.Println("Server.removePlayer(): state == rndovr || gameovr")
          /*server.table.finishRound()
          server.table.State = TableStateGameOver
          server.gameOver()*/
          return
        }
      }
    } else if server.table.State == TableStateDoneBetting ||
              server.table.State == TableStateRoundOver {
      fmt.Println("Server.removePlayer(): defer postPlayerAction")
      server.postPlayerAction(nil, &NetData{})
    }
  }()
  defer server.table.mtx.Unlock()

  table := server.table

  if player := client.Player; player != nil { // else client was a spectator
    fmt.Printf("Server.removePlayer(): removing %s\n", player.Name)

    table.activePlayers.RemovePlayer(player)
    table.curPlayers.RemovePlayer(player)

    player.Clear()

    table.NumPlayers--

    netData := &NetData{
      Client:     client,
      Response:   NetDataPlayerLeft,
      Table:      table,
    }
    server.sendResponseToAll(netData, client)

    client.Player = nil
    delete(server.playerClientMap, player)

    if client.ID == server.tableAdminID {
      if table.activePlayers.len == 0 {
        server.makeAdmin(nil)
      } else {
        activePlayerHeadClient := server.getPlayerClient(table.activePlayers.head.Player)
        server.makeAdmin(activePlayerHeadClient)
      }
    }

    if table.NumPlayers < 2 {
      reset = true
      if table.NumPlayers == 0 {
        noPlayersLeft = true
        server.tableAdminID = ""
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

func (server *Server) sendPlayerTurn(client *Client) {
  if server.table.curPlayer == nil {
    fmt.Println("Server.sendPlayerTurn(): curPlayer == nil")
    return
  }

  curPlayer := server.table.curPlayer.Player
  curPlayerClient := server.getPlayerClient(curPlayer)
  if curPlayerClient == nil {
    panic(fmt.Sprintf("Server.sendPlayerTurn(): BUG: %s not found in connClientMap\n",
                     curPlayer.Name))
  }

  netData := &NetData{
    Client:     curPlayerClient,
    Response:   NetDataPlayerTurn,
  }

  //netData.Client.Player.Action.Action = NetDataPlayerTurn

  netData.SendTo(client)

  if server.table.InBettingState() {
    server.sendPlayerHead(client, false)
  } else {
    server.sendPlayerHead(nil, true)
  }
}

func (server *Server) sendPlayerTurnToAll() {
  if server.table.curPlayer == nil {
    fmt.Println("Server.sendPlayerTurnToAll(): curPlayer == nil")
    return
  }

  curPlayer := server.table.curPlayer.Player
  curPlayerClient := server.getPlayerClient(curPlayer)
  if curPlayerClient == nil {
    panic(fmt.Sprintf("Server.sendPlayerTurnToAll(): BUG: curPlayer <%s> not found in any maps\n",
                      curPlayer.Name))
  }

  netData := &NetData{
    Client:   server.publicClientInfo(curPlayerClient),
    Response: NetDataPlayerTurn,
  }

  //netData.Client.Player.Action.Action = NetDataPlayerTurn

  server.sendResponseToAll(netData, nil)

  if server.table.InBettingState() {
    server.sendPlayerHead(nil, false)
  } else {
    server.sendPlayerHead(nil, true)
  }
}

// XXX: this response gets sent too often
func (server *Server) sendPlayerHead(client *Client, clear bool) {
  if clear {
    fmt.Println("Server.sendPlayerHead(): sending clear player head")
    server.sendResponseToAll(
      &NetData{
        Response: NetDataPlayerHead,
      }, nil)

    return
  }

  playerHeadNode := server.table.curPlayers.head
  curPlayerNode := server.table.curPlayer
  if playerHeadNode != nil && curPlayerNode != nil &&
     playerHeadNode.Player.Name != curPlayerNode.Player.Name {
    playerHead := playerHeadNode.Player
    playerHeadClient := server.getPlayerClient(playerHead)
    if playerHead == nil {
      panic(fmt.Sprintf("Server.sendPlayerTurnToAll(): BUG: playerHead <%s> " +
                        "not found in any maps\n", playerHead.Name))
    }

    netData := &NetData {
      Client: server.publicClientInfo(playerHeadClient),
      Response: NetDataPlayerHead,
    }
    if client == nil {
      server.sendResponseToAll(netData, nil)
    } else {
      netData.SendTo(client)
    }
  }
}

func (server *Server) sendPlayerActionToAll(player *Player, client *Client) {
  fmt.Printf("Server.sendPlayerActionToAll(): <%s> sending %s\n",
             player.Name, player.ActionToString())

  var c *Client
  if client == nil {
    c = server.connClientMap[server.getPlayerConn(player)]
  } else {
    c = client
  }

  netData := &NetData{
    Client:     server.publicClientInfo(c),
    Response:   NetDataPlayerAction,
    Table:      server.table,
  }

  server.sendResponseToAll(netData, c)

  if client != nil { // client is nil for blind auto allin corner case
    netData.Client.Player = player
    netData.SendTo(c)
  }
}

func (server *Server) sendDeals() {
  netData := &NetData{Response: NetDataDeal, Table: server.table}

  for _, player := range server.table.curPlayers.ToPlayerArray() {
    client := server.connClientMap[server.getPlayerConn(player)]
    netData.Client = client

    netData.Send()
  }
}

func (server *Server) sendHands() {
  netData := &NetData{Response: NetDataShowHand, Table: server.table}

  for _, player := range server.table.curPlayers.ToPlayerArray() {
    client := server.playerClientMap[player]
    //assert(client != nil, "Server.sendHands(): player not in playerMap")
    netData.Client = server.publicClientInfo(client)

    server.sendResponseToAll(netData, client)
  }
}

// NOTE: hand is currently computed on client side
func (server *Server) sendCurHands() {
  netData := &NetData{Response: NetDataCurHand, Table: server.table}

  for _, client := range server.playerClientMap {
    netData.Client = client
    netData.Send()
  }
}

func (server *Server) sendActivePlayers(client *Client) {
  if client == nil {
    fmt.Println("Server.sendActivePlayers(): conn is nil")
    return
  }

  netData := &NetData{
    Response: NetDataCurPlayers,
    Table:    server.table,
  }

  for _, player := range server.table.activePlayers.ToPlayerArray() {
    playerClient := server.connClientMap[server.getPlayerConn(player)]
    netData.Client = server.publicClientInfo(playerClient)
    netData.SendTo(client)
  }
}

func (server *Server) sendAllPlayerInfo(client *Client, curPlayers bool, sendToSelf bool) {
  netData := &NetData{Response: NetDataUpdatePlayer}

  var players playerList
  if curPlayers {
    players = server.table.curPlayers
  } else {
    players = server.table.activePlayers
  }

  for _, player := range players.ToPlayerArray() {
    playerClient := server.connClientMap[server.getPlayerConn(player)]

    netData.Client = playerClient

    if client != nil {
      if !sendToSelf && playerClient.ID == client.ID {
        continue
      } else if playerClient.ID != client.ID {
        netData.Client = server.publicClientInfo(playerClient)
      }
      netData.SendTo(client)
    } else {
      netData.Send()
      netData.Client = server.publicClientInfo(playerClient)
      server.sendResponseToAll(netData, playerClient)
    }
  }
}

func (server *Server) sendTable() {
  server.sendResponseToAll(&NetData{
    Response: NetDataUpdateTable,
    Table:    server.table,
  }, nil)
}

func (server *Server) sendReset(winner *Client) {
  server.sendResponseToAll(&NetData{
    Client:   winner,
    Response: NetDataReset,
    Table:    server.table,
  }, nil)
}

func (server *Server) removeEliminatedPlayers() {
  netData := &NetData{Response: NetDataEliminated}

  for _, player := range server.table.getEliminatedPlayers() {
    client := server.connClientMap[server.getPlayerConn(player)]
    netData.Client = client
    netData.Response = NetDataEliminated
    netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", client.Player.Name,
                              netData.Client.ID[:7])

    server.removePlayer(client)
    server.sendResponseToAll(netData, nil)
  }
}

func (server *Server) sendLock(conn *websocket.Conn, connType string) {
  fmt.Printf("Server.sendLock(): locked out %p with %s\n", conn,
             server.table.TableLockToString())

  netData := &NetData{
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataTableLocked,
    Msg: fmt.Sprintf("table lock: %s", server.table.TableLockToString()),
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (server *Server) sendBadAuth(conn *websocket.Conn, connType string) {
  fmt.Printf("Server.sendBadAuth(): %p had bad authentication\n", conn)

  netData := &NetData{
    Client: &Client{conn: conn, connType: connType},
    Response: NetDataBadAuth,
    Msg: "your password was incorrect",
  }

  netData.Send()

  time.Sleep(1 * time.Second)
}

func (server *Server) makeAdmin(client *Client) {
  server.mtx.Lock() // XXX lock probably unnecessary
  defer server.mtx.Unlock()

  if client == nil {
    fmt.Printf("Server.makeAdmin(): client is nil, unsetting tableAdmin\n")
    server.tableAdminID = ""
    return
  } else {
    fmt.Printf("Server.makeAdmin(): making <%s> (%s) table admin\n", client.ID, client.Name)
    server.tableAdminID = client.ID
  }

  netData := &NetData{
    Client: client,
    Response: NetDataMakeAdmin,
    Table: server.table,
  }

  netData.Send()
}

func (server *Server) newRound() {
  server.table.newRound()
  server.table.nextTableAction()
  server.checkBlindsAutoAllIn()
  server.sendDeals()

  // XXX
  server.table.mtx.Lock()
  realRoundState := server.table.State
  server.table.State = TableStateNewRound
  server.sendPlayerTurnToAll()
  server.table.State = realRoundState
  server.table.mtx.Unlock()

  server.sendTable()
}

func (server *Server) roundOver() {
  server.mtx.Lock()
  if server.table.State == TableStateReset ||
     server.table.State == TableStateNewRound {
    server.mtx.Unlock()
    return
  }
  server.mtx.Unlock()

  server.table.finishRound()
  server.sendHands()

  netData := &NetData{
    Response: NetDataRoundOver,
    Table:    server.table,
    Msg:      server.table.WinInfo,
  }

  for i, sidePot := range server.table.sidePots.GetAllPots() {
    netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
  }

  server.sendResponseToAll(netData, nil)
  server.sendAllPlayerInfo(nil, false, true)

  server.removeEliminatedPlayers()

  if server.table.State == TableStateGameOver {
    server.gameOver()

    return
  }

  server.newRound()
}

func (server *Server) gameOver() {
  fmt.Printf("Server.gameOver(): ** game over %s wins **\n", server.table.Winners[0].Name)
  winner := server.table.Winners[0]

  netData := &NetData{
    Response: NetDataServerMsg,
    Msg:      "game over, " + winner.Name + " wins",
  }

  server.sendResponseToAll(netData, nil)

  server.table.reset(winner) // make a new game while keeping winner connected

  winnerClient := server.getPlayerClient(winner)
  if winnerClient == nil {
    fmt.Printf("Server.getPlayerClient(): winner (%s) not found in any maps\n", winner.Name)
    server.makeAdmin(nil)
    server.sendReset(nil)
    return
  }

  if winnerClient.ID != server.tableAdminID {
    server.makeAdmin(winnerClient)
    server.sendPlayerTurnToAll()
  }
  server.sendReset(winnerClient)
}

// XXX: need to add to sidepots
func (server *Server) checkBlindsAutoAllIn() {
  if server.table.SmallBlind.Player.Action.Action == NetDataAllIn {
    fmt.Printf("Server.checkBlindsAutoAllIn(): smallblind (%s) forced to go all in\n",
               server.table.SmallBlind.Player.Name)

    if server.table.curPlayer.Player.Name == server.table.SmallBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      server.table.setNextPlayerTurn()
    } else {
      server.table.curPlayers.RemovePlayer(server.table.SmallBlind.Player)
    }

    server.sendPlayerActionToAll(server.table.SmallBlind.Player, nil)
  }
  if server.table.BigBlind.Player.Action.Action == NetDataAllIn {
    fmt.Printf("Server.checkBlindsAutoAllIn(): bigblind (%s) forced to go all in\n",
               server.table.BigBlind.Player.Name)

    if server.table.curPlayer.Player.Name == server.table.BigBlind.Player.Name {
      // because blind is curPlayer setNextPlayerTurn() will remove the blind
      // from the list for us
      server.table.setNextPlayerTurn()
    } else {
      server.table.curPlayers.RemovePlayer(server.table.BigBlind.Player)
    }

    server.sendPlayerActionToAll(server.table.BigBlind.Player, nil)
  }
}

func (server *Server) postBetting(player *Player, netData *NetData, client *Client) {
  if client != nil {
    player := client.Player
    defer func() {
      client.Player = player
    }()
  }

  if player != nil {
    server.sendPlayerActionToAll(player, client)
    server.sendPlayerTurnToAll()
  }

  fmt.Println("Server.postBetting(): done betting...")

  if server.table.bettingIsImpossible() {
    fmt.Println("Server.postBetting(): no more betting possible this round")

    for server.table.State != TableStateRoundOver {
      server.table.nextCommunityAction()
    }
  } else {
    server.table.nextCommunityAction()
  }

  if server.table.State == TableStateRoundOver {
    server.roundOver()

    if server.table.State == TableStateGameOver {
      return // XXX
    }
  } else { // new community card(s)
    netData.Response = server.table.commState2NetDataResponse()
    netData.Table = server.table
    if client != nil {
      netData.Client.Player = nil
    }

    server.sendResponseToAll(netData, nil)

    server.table.Bet, server.table.better = 0, nil
    for _, player := range server.table.curPlayers.ToPlayerArray() {
      fmt.Printf("Server.postBetting(): clearing %v's action\n", player.Name)
      player.Action.Clear()
    }

    server.sendAllPlayerInfo(nil, true, true)
    server.table.reorderPlayers()
    server.sendPlayerTurnToAll()
    server.sendPlayerHead(nil, true)
    // let players know they should update their current hand after
    // the community action
    server.sendCurHands()
  }
}

func (server *Server) postPlayerAction(client *Client, netData *NetData) {
  var player *Player = nil

  if client != nil {
    player = client.Player
    defer func() {
      client.Player = player
    }()
  }

  if server.table.State == TableStateDoneBetting {
    server.postBetting(player, netData, client)
  } else if server.table.State == TableStateRoundOver {
    // all other players folded before all comm cards were dealt
    // TODO: check for this state in a better fashion
    server.table.finishRound()
    fmt.Printf("winner # %d\n", len(server.table.Winners))
    fmt.Println(server.table.Winners[0].Name + " wins by folds")

    netData.Response = NetDataRoundOver
    netData.Table = server.table
    netData.Msg = server.table.Winners[0].Name + " wins by folds"
    if netData.Client != nil { // XXX ?
      netData.Client.Player = nil
    }

    server.sendResponseToAll(netData, nil)

    server.removeEliminatedPlayers()

    if server.table.State == TableStateGameOver {
      server.gameOver()

      return
    }

    server.newRound()
  } else {
    server.sendPlayerActionToAll(player, client)
    server.sendPlayerTurnToAll()
  }
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error) {
  fmt.Println("server panicked")

  for conn := range server.connClientMap {
    conn.WriteMessage(websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
        err.Error()))
  }

  server.errChan <- err
  server.panicked = true
}

func (server *Server) publicClientInfo(client *Client) *Client {
  pubClient := *client
  pubClient.Player = server.table.PublicPlayerInfo(*client.Player)

  return &pubClient
}

type ClientSettings struct {
  Name     string
  Password string

  Admin struct {
    Lock     TableLock
    Password string
  }
}

func (server *Server) handleClientSettings(client *Client, settings *ClientSettings) (string, error) {
  msg := ""
  errs := ""

  const (
    MaxNameLen uint8 = 15
    MaxPassLen uint8 = 50
  )

  if client == nil {
    fmt.Println("Server.handleClientSettings(): called with a nil parameter")

    return "", errors.New("server.handleClientSettings(): BUG: client == nil")
  } else if settings == nil {
    fmt.Println("Server.handleClientSettings(): called with a nil parameter")

    return "", errors.New("Server.handleClientSettings(): BUG: settings == nil")
  }

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
          _, found := server.nameClientMap[settings.Name]
          if !found {
            for _, defaultName := range server.table.DefaultPlayerNames() {
              if settings.Name == defaultName {
                found = true
                break
              }
            }
          }
          if found {
            fmt.Printf("%p requested the name `%s` which is reserved or already taken\n",
                       client.conn, settings.Name)
            msg += fmt.Sprintf("Name '%s' already in use. Current name unchanged.\n\n",
                                settings.Name)
          }
        }
      } else {
        for _, player := range server.table.players {
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
  if client.ID == server.tableAdminID {
    msg += "admin settings:\n\n"

    lock := TableLockToString(settings.Admin.Lock)
    if lock == "" {
      fmt.Printf("Server.handleClientSettings(): %p requested invalid table lock '%v'\n",
                 client.conn, settings.Admin.Lock)
      errs += fmt.Sprintf("invalid table lock: '%v'\n", settings.Admin.Lock)
    } else if settings.Admin.Lock == server.table.Lock {
      fmt.Println("Server.handleClientSettings(): table lock unchanged")
      msg += "table lock: unchanged\n"
    } else {
      msg += "table lock: changed\n"
    }

    if settings.Admin.Password != server.table.Password {
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

  return msg, nil
}

func (server *Server) applyClientSettings(client *Client, settings *ClientSettings) {
  client.Settings = settings
  if player := client.Player; player != nil {
    player.setName(settings.Name)
    client.SetName(player.Name)

    if client.ID == server.tableAdminID {
      server.table.mtx.Lock()
      server.table.Lock = settings.Admin.Lock
      server.table.Password = settings.Admin.Password
      server.table.mtx.Unlock()
    }
  } else {
    client.SetName(settings.Name)
  }
}

func (server *Server) newClient(conn *websocket.Conn, connType string, clientSettings *ClientSettings) *Client {
  server.mtx.Lock()
  defer server.mtx.Unlock()

  client, ID := &Client{conn: conn, connType: connType}, ""
  for {
    ID = randString(10) // 62^10 is plenty ;)
    if _, found := server.IDClientMap[ID]; !found {
      client.ID = ID
      server.IDClientMap[ID] = client
      server.connClientMap[conn] = client

      break
    } else {
      fmt.Printf("server.newClient(): WARNING: possible bug: ID '%s' already found in IDClientMap\n", ID)
    }
  }

  client.SetName(clientSettings.Name)
  if client.Name != "" {
    server.nameClientMap[client.Name] = client
  }

  return client
}

func (server *Server) WSCLIClient(w http.ResponseWriter, req *http.Request) {
  server.WSClient(w, req, "cli")
}

func (server *Server) WSWebClient(w http.ResponseWriter, req *http.Request) {
  server.WSClient(w, req, "web")
}

func (server *Server) WSClient(w http.ResponseWriter, req *http.Request, connType string) {
  if req.Header.Get("keepalive") != "" {
    return // NOTE: for heroku
  }

  if connType != "cli" && connType != "web" {
    fmt.Printf("Server.WSClient: connType '%s' is invalid.\n", connType)

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
    if server.panicked { // server panic was already recovered in previous client handler
      return
    }

    if err := recover(); err != nil {
      server.serverError(panicRetToError(err))
    } else { // not a server panic()
      client := server.connClientMap[conn]
      if client != nil && client.Player != nil {
        player := client.Player
        if !cleanExit {
          fmt.Printf("Server.WSClient(): %s had an unclean exit\n", player.Name)
        }
        if server.table.activePlayers.len > 1 &&
           server.table.curPlayer != nil &&
           server.table.curPlayer.Player.Name == player.Name {
          server.table.curPlayer.Player.Action.Action = NetDataFold
          server.table.setNextPlayerTurn()
          server.sendPlayerTurnToAll()
        }

        server.removePlayer(client)
      }

      server.removeClient(conn)
      server.closeConn(conn)
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

  for {
    var netData NetData

    if connType == "cli" {
      _, rawData, err := conn.ReadMessage()
      if err != nil {
        if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
          fmt.Printf("Server.WSClient(): cli: readConn() conn: %p err: %v\n", conn, err)
        }

        return
      }

      // we need to set Table member to nil otherwise gob will
      // modify our server.table structure if a user sends that member
      nd := NetData{Response: NetDataNewConn, Table: nil}

      if err := gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&nd);
        err != nil {
        fmt.Printf("Server.WSClient(): cli: %p had a problem decoding gob stream: %s\n", conn, err.Error())

        return
      }

      nd.Table = server.table

      fmt.Printf("Server.WSClient(): cli: recv %s (%d bytes) from %p\n",
                 nd.NetActionToString(), len(rawData), conn)

      if int64(len(rawData)) > server.MaxConnBytes {
        fmt.Printf("Server.WSClient(): cli: conn: %p sent too many bytes (> %v)\n",
                   conn, server.MaxConnBytes)
        return
      }

      netData = nd
    } else { // webclient
      /*if err := conn.ReadJSON(&netData); err != nil {
        if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
          fmt.Printf("Server.WSClient(): ReadJSON(): conn: %p err %v\n", conn, err)
        }

        //fmt.Printf("Server.WSClient(): ReadJSON() err: %s\n", err.Error())

        return
      }*/
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

      if netData.Client.conn == nil {
        netData.Client.conn = conn
      }
      if netData.Client.Settings == nil {
        netData.Client.Settings = &ClientSettings{}
      }
      fmt.Printf("Server.WSClient(): web: recv msgpack: %v nd.Request == %v\n", netData, netData.Request)
      fmt.Printf("Server.WSClient(): web: nd %s\n", netData.NetActionToString())
      netData.Table = server.table
    }

    if netData.Request == NetDataNewConn {
      netData.Response = NetDataNewConn
      netData.Request = 0
      if server.table.Lock == TableLockAll {
        server.sendLock(conn, connType)

        return
      }

      if server.table.Password != "" &&
         netData.Client.Settings.Password != server.table.Password {
        server.sendBadAuth(conn, connType)

        return
      }

      client := server.newClient(conn, connType, netData.Client.Settings)

      if _, err := server.handleClientSettings(client, netData.Client.Settings); err != nil {
        (&NetData{
          Response: NetDataBadRequest,
          Client: client,
          Msg: err.Error(),
        }).Send()
      }

      server.table.mtx.Lock()
      server.table.NumConnected++
      server.table.mtx.Unlock()

      server.sendResponseToAll(&netData, nil)

      // send current player info to this client
      if server.table.NumConnected > 1 {
        server.sendActivePlayers(client)
      }

      server.applyClientSettings(client, netData.Client.Settings)
      netData.Client = client
      netData.Response = NetDataClientSettings
      netData.Send()

      if server.table.Lock == TableLockPlayers {
        netData.Response = NetDataServerMsg
        //netData.Client = &Client{conn: conn}
        netData.Msg = "This table is not allowing new players. " +
                      "You have been added as a spectator."
        netData.Send()
      } else if player := server.table.getOpenSeat(); player != nil {
        client.Player = player
        server.playerClientMap[player] = client

        server.applyClientSettings(client, netData.Client.Settings)
        fmt.Printf("Server.WSClient(): adding <%s> (%p) (%s) as player '%s'\n",
                   client.ID, &conn, client.Name, player.Name)

        if server.table.State == TableStateNotStarted {
          player.Action.Action = NetDataFirstAction
          server.table.curPlayers.AddPlayer(player)
        } else {
          player.Action.Action = NetDataMidroundAddition
        }
        server.table.activePlayers.AddPlayer(player)

        if server.table.curPlayer == nil {
          server.table.curPlayer = server.table.curPlayers.head
        }

        if server.table.Dealer == nil {
          server.table.Dealer = server.table.activePlayers.head
        } else if server.table.SmallBlind == nil {
          server.table.SmallBlind = server.table.Dealer.next
        } else if server.table.BigBlind == nil {
          server.table.BigBlind = server.table.SmallBlind.next
        }

        netData.Client = server.publicClientInfo(client)
        netData.Response = NetDataNewPlayer
        netData.Table = server.table

        server.sendResponseToAll(&netData, client)

        netData.Client = client
        netData.Response = NetDataYourPlayer
        netData.Send()
      } else if server.table.Lock == TableLockSpectators {
          server.sendLock(conn, connType)

          return
      } else {
        netData.Response = NetDataServerMsg
        netData.Msg = "No open seats available. You have been added as a spectator"

        netData.Send()
      }

      server.sendAllPlayerInfo(client, false, false)

      if server.table.State != TableStateNotStarted {
        server.sendPlayerTurn(client)
      }

      if server.tableAdminID == "" {
        server.makeAdmin(client)
      }
    } else {
      client := server.connClientMap[conn]

      switch netData.Request {
      case NetDataClientExited:
        cleanExit = true

        return
      case NetDataClientSettings: // TODO: check pointers
        if msg, err := server.handleClientSettings(client, netData.Client.Settings); err == nil {
          server.applyClientSettings(client, netData.Client.Settings)

          netData.ClearData(client)
          if client.Player != nil { // send updated player info to other clients
            netData.Response = NetDataUpdatePlayer
            netData.Client = server.publicClientInfo(client)

            server.sendResponseToAll(&netData, client)
          }

          netData.Client = client
          netData.Response = NetDataClientSettings
          netData.Send()

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
      case NetDataStartGame:
        netData.ClearData(client)
        if client.ID != server.tableAdminID {
          netData.Response = NetDataBadRequest
          netData.Msg = "only the table admin can do that"

          netData.Send()
        } else if server.table.NumPlayers < 2 {
          netData.Response = NetDataBadRequest
          netData.Msg = "not enough players to start"

          netData.Send()
        } else if server.table.State != TableStateNotStarted {
          netData.Response = NetDataBadRequest
          netData.Msg = "this game has already started"

          netData.Send()
        } else { // start game
          server.table.nextTableAction()

          server.sendDeals()
          server.sendAllPlayerInfo(nil, false, true)
          server.sendPlayerTurnToAll()
          server.sendTable()
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

        server.sendResponseToAll(&netData, nil)
      case NetDataAllIn, NetDataBet, NetDataCall, NetDataCheck, NetDataFold:
        if client.Player == nil {
          netData.ClearData(client)
          netData.Response = NetDataBadRequest
          netData.Msg = "you are not a player"

          netData.Send()
          continue
        }

        if server.table.State == TableStateNotStarted {
          netData.ClearData(client)
          netData.Response = NetDataBadRequest
          netData.Msg = "a game has not been started yet"

          netData.Send()
          continue
        }

        if client.Player.Name != server.table.curPlayer.Player.Name {
          netData.ClearData(client)
          netData.Response = NetDataBadRequest
          netData.Msg = "it's not your turn"

          netData.Send()
          continue
        }

        if err := server.table.PlayerAction(client.Player, netData.Client.Player.Action);
           err != nil {
          netData.ClearData(client)
          netData.Response = NetDataBadRequest
          netData.Msg = err.Error()

          netData.Send()
        } else {
          server.postPlayerAction(client, &netData)
        }
      default:
        netData.ClearData(client)
        netData.Response = NetDataBadRequest
        netData.Msg = fmt.Sprintf("bad request %v", netData.Request)

        netData.Send()
      }
    } // else{} end
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
    server.sendResponseToAll(&NetData{Response: NetDataServerClosed}, nil)

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
