package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Server struct {
  clients []*websocket.Conn
  clientIDMap map[*websocket.Conn]string
  playerMap map[*websocket.Conn]*Player
  table *Table
  tableAdmin *websocket.Conn

  http *http.Server
  upgrader websocket.Upgrader

  sigChan chan os.Signal
  errChan chan error
  panicked bool
}

func (server *Server) Init(table *Table, addr string) error {
  server.clients = make([]*websocket.Conn, 0)
  server.clientIDMap = make(map[*websocket.Conn]string)
  server.playerMap = make(map[*websocket.Conn]*Player)
  server.table = table

  server.errChan = make(chan error)
  server.panicked = false

  server.upgrader = websocket.Upgrader{}

  server.http = &http.Server{
    Addr:        addr,
    IdleTimeout: 0,
    ReadTimeout: 0,
  }
  server.http.SetKeepAlivesEnabled(true)
  http.HandleFunc("/cli", server.WSCLIClient)

  server.sigChan = make(chan os.Signal, 1)
  signal.Notify(server.sigChan, os.Interrupt)

  return nil
}

func (server *Server) closeConn(conn *websocket.Conn) {
  fmt.Printf("<= closing conn to %s\n", conn.RemoteAddr().String())
  conn.Close()
}

func (server *Server) sendResponseToAll(data *NetData, except *websocket.Conn) {
  for _, clientConn := range server.clients {
    if clientConn != except {
      sendData(data, clientConn)
    }
  }
}

func (server *Server) getPlayerConn(player *Player) *websocket.Conn {
  for conn, p := range server.playerMap {
    if p.Name == player.Name {
      return conn
    }
  }

  return nil
}

func (server *Server) removeClient(conn *websocket.Conn) {
  server.table.mtx.Lock()
  defer server.table.mtx.Unlock()

  clientIdx := -1
  for i, clientConn := range server.clients {
    if clientConn == conn {
      clientIdx = i
      break
    }
  }
  if clientIdx == -1 {
    fmt.Println("removeClient(): BUG: couldn't find a conn in clients slice")
    return
  } else {
    server.clients = append(server.clients[:clientIdx], server.clients[clientIdx+1:]...)
  }

  server.table.NumConnected--

  netData := &NetData{
    Response: NETDATA_CLIENTEXITED,
    Table:    server.table,
  }

  server.sendResponseToAll(netData, nil)
}

func (server *Server) removePlayerByConn(conn *websocket.Conn) {
  reset := false // XXX race condition guard
  noPlayersLeft := false // XXX race condition guard

  server.table.mtx.Lock()
  defer func() {
    if server.table.State == TABLESTATE_NOTSTARTED {
      return
    }
    if reset {
      if noPlayersLeft {
        server.table.reset(nil)
        server.sendResponseToAll(&NetData{
          Response: NETDATA_RESET,
          Table: server.table,
        }, nil)
      } else {
        if server.table.State != TABLESTATE_ROUNDOVER &&
           server.table.State != TABLESTATE_GAMEOVER {
          fmt.Println("rPlByConn: state != (rndovr || gameovr)")
          server.table.finishRound()
          server.table.State = TABLESTATE_GAMEOVER
          server.gameOver()
        } else {
          fmt.Println("rPlByConn: state == rndovr || gameovr")
          //server.table.finishRound()
          //server.table.State = TABLESTATE_GAMEOVER
          //server.gameOver()
        }
      }
    } else if server.table.State == TABLESTATE_DONEBETTING ||
              server.table.State == TABLESTATE_ROUNDOVER {
      fmt.Println("removePl defer postPlAct")
      server.postPlayerAction(nil, &NetData{}, nil)
    }
  }()
  defer server.table.mtx.Unlock()

  player := server.playerMap[conn]

  if player != nil { // else client was a spectator
    fmt.Printf("removePlayerByConn(): removing %s\n", player.Name)
    delete(server.playerMap, conn)

    server.table.activePlayers.RemovePlayer(player)
    server.table.curPlayers.RemovePlayer(player)

    player.Clear()

    netData := &NetData{
      ID:         server.clientIDMap[conn],
      Response:   NETDATA_PLAYERLEFT,
      Table:      server.table,
      PlayerData: player,
    }

    server.sendResponseToAll(netData, conn)

    server.table.NumPlayers--

    if server.table.NumPlayers < 2 {
      reset = true
      if server.table.NumPlayers == 0 {
        noPlayersLeft = true
        server.tableAdmin = nil
      }
      return
    }

    if conn == server.tableAdmin {
      server.tableAdmin = server.getPlayerConn(server.table.activePlayers.node.Player)
      assert(server.tableAdmin != nil,
             "getPlayerConn(): couldn't find activePlayers head websocket")
      sendData(&NetData{Response: NETDATA_MAKEADMIN}, server.tableAdmin)
    }

    if server.table.Dealer != nil &&
       player.Name == server.table.Dealer.Player.Name {
      server.table.Dealer = nil
    }
    if server.table.SmallBlind != nil &&
       player.Name == server.table.SmallBlind.Player.Name {
      server.table.SmallBlind = nil
    }
    if server.table.BigBlind != nil &&
       player.Name == server.table.BigBlind.Player.Name {
      server.table.BigBlind = nil
    }
  }
}

func (server *Server) removePlayer (player *Player) {
  for conn, p := range server.playerMap {
    if p == player {
      server.removePlayerByConn(conn)

      return
    }
  }
}

func (server *Server) sendPlayerTurn(conn *websocket.Conn) {
  if server.table.curPlayer == nil {
    return
  }

  curPlayer := server.table.curPlayer.Player
  id := server.clientIDMap[server.getPlayerConn(curPlayer)]

  netData := &NetData{
    ID:         id,
    Response:   NETDATA_PLAYERTURN,
    PlayerData: server.table.PublicPlayerInfo(*curPlayer),
  }

  netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

  sendData(netData, conn)
}

func (server *Server) sendPlayerTurnToAll() {
  if server.table.curPlayer == nil {
    return
  }

  curPlayer := server.table.curPlayer.Player
  id := server.clientIDMap[server.getPlayerConn(curPlayer)]

  netData := &NetData{
    ID:         id,
    Response:   NETDATA_PLAYERTURN,
    PlayerData: server.table.PublicPlayerInfo(*curPlayer),
  }

  netData.PlayerData.Action.Action = NETDATA_PLAYERTURN

  server.sendResponseToAll(netData, nil)
}

func (server *Server) sendPlayerActionToAll(player *Player, conn *websocket.Conn) {
  fmt.Printf("%s action => %s\n", player.Name, player.ActionToString())

  var c *websocket.Conn
  if conn == nil {
    c = server.getPlayerConn(player)
  } else {
    c = conn
  }

  netData := &NetData{
    ID:         server.clientIDMap[c],
    Response:   NETDATA_PLAYERACTION,
    Table:      server.table,
    PlayerData: server.table.PublicPlayerInfo(*player),
  }

  server.sendResponseToAll(netData, conn)

  if conn != nil { // conn is nil for blind auto allin corner case
    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendDeals() {
  netData := &NetData{Response: NETDATA_DEAL}

  for conn, player := range server.playerMap {
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = player
    netData.Table = server.table

    sendData(netData, conn)
  }
}

func (server *Server) sendHands() {
  netData := &NetData{Response: NETDATA_SHOWHAND, Table: server.table}

  for _, player := range server.table.curPlayers.ToPlayerArray() {
    conn := server.getPlayerConn(player)
    assert(conn != nil, "sendHands(): player not in playerMap")
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = server.table.PublicPlayerInfo(*player)

    server.sendResponseToAll(netData, conn)
  }
}

// NOTE: hand is currently computed on client side
func (server *Server) sendCurHands() {
  netData := &NetData{Response: NETDATA_CURHAND, Table: server.table}

  for conn, player := range server.playerMap {
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendAllPlayerInfo(curPlayers bool) {
  netData := &NetData{Response: NETDATA_UPDATEPLAYER}

  var players playerList
  if curPlayers {
    players = server.table.curPlayers
  } else {
    players = server.table.activePlayers
  }

  for _, player := range players.ToPlayerArray() {
    conn := server.getPlayerConn(player)
    netData.ID = server.clientIDMap[conn]
    netData.PlayerData = server.table.PublicPlayerInfo(*player)

    server.sendResponseToAll(netData, conn)

    netData.PlayerData = player
    sendData(netData, conn)
  }
}

func (server *Server) sendTable() {
  netData := &NetData{
    Response: NETDATA_UPDATETABLE,
    Table:    server.table,
  }

  server.sendResponseToAll(netData, nil)
}

func (server *Server) removeEliminatedPlayers() {
  netData := &NetData{Response: NETDATA_ELIMINATED}

  for _, player := range server.table.getEliminatedPlayers() {
    conn := server.getPlayerConn(player)
    netData.ID = server.clientIDMap[conn]
    netData.Response = NETDATA_ELIMINATED
    netData.PlayerData = player
    netData.Msg = fmt.Sprintf("<%s id: %s> was eliminated", player.Name, netData.ID[:7])

    server.removePlayer(player)
    server.sendResponseToAll(netData, nil)
  }
}

func (server *Server) roundOver() {
  server.table.finishRound()
  server.sendHands()

  netData := &NetData{
    Response: NETDATA_ROUNDOVER,
    Table:    server.table,
    Msg:      server.table.WinInfo,
  }

  for i, sidePot := range server.table.sidePots.GetAllPots() {
    netData.Msg += fmt.Sprintf("\nsidePot #%d:\n%s", i+1, sidePot.WinInfo)
  }

  server.sendResponseToAll(netData, nil)
  server.sendAllPlayerInfo(false)

  server.removeEliminatedPlayers()

  if server.table.State == TABLESTATE_GAMEOVER {
    server.gameOver()

    return
  }

  server.table.newRound()
  server.table.nextTableAction()
  server.checkBlindsAutoAllIn()
  server.sendDeals()
  server.sendPlayerTurnToAll()
  server.sendTable()
}

func (server *Server) gameOver() {
  fmt.Printf("** game over %s wins **\n", server.table.Winners[0].Name)
  winner := server.table.Winners[0]

  netData := &NetData{
    Response: NETDATA_SERVERMSG,
    Msg:      "game over, " + winner.Name + " wins",
  }

  server.sendResponseToAll(netData, nil)

  server.table.reset(winner) // make a new game while keeping winner connected

  if winnerConn := server.getPlayerConn(winner); winnerConn != server.tableAdmin {
    if winnerConn == nil {
      fmt.Printf("getPlayerConn(): winner (%s) not found\n", winner.Name)
      return
    }
    server.tableAdmin = winnerConn
    sendData(&NetData{Response: NETDATA_MAKEADMIN}, winnerConn)
    server.sendPlayerTurnToAll()

    server.sendResponseToAll(&NetData{
      Response: NETDATA_RESET,
      PlayerData: winner,
      Table: server.table,
    }, nil)
  }
}

func (server *Server) checkBlindsAutoAllIn() {
  if server.table.SmallBlind.Player.Action.Action == NETDATA_ALLIN {
    fmt.Printf("checkBlindsAutoAllIn(): smallblind (%s) forced to go all in\n",
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
  if server.table.BigBlind.Player.Action.Action == NETDATA_ALLIN {
    fmt.Printf("checkBlindsAutoAllIn(): bigblind (%s) forced to go all in\n",
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

func (server *Server) postBetting(player *Player, netData *NetData, conn *websocket.Conn) {
  if player != nil {
    server.sendPlayerActionToAll(player, conn)
    server.sendPlayerTurnToAll()
  }

  fmt.Println("postBetting(): done betting...")

  if server.table.bettingIsImpossible() {
    fmt.Println("postBetting(): no more betting possible this round")

    for server.table.State != TABLESTATE_ROUNDOVER {
      server.table.nextCommunityAction()
    }
  } else {
    server.table.nextCommunityAction()
  }

  if server.table.State == TABLESTATE_ROUNDOVER {
    server.roundOver()

    if server.table.State == TABLESTATE_GAMEOVER {
      return // XXX
    }
  } else { // new community card(s)
    netData.Response = server.table.commState2NetDataResponse()
    netData.Table = server.table
    netData.PlayerData = nil

    server.sendResponseToAll(netData, nil)

    server.table.Bet, server.table.better = 0, nil
    for _, player := range server.table.curPlayers.ToPlayerArray() {
      fmt.Printf("clearing %v's action\n", player.Name)
      player.Action.Action = NETDATA_FIRSTACTION
      player.Action.Amount = 0
    }

    server.sendAllPlayerInfo(true)
    server.table.reorderPlayers()
    server.sendPlayerTurnToAll()
    // let players know they should update their current hand after
    // the community action
    server.sendCurHands()
  }
}

func (server *Server) postPlayerAction(player *Player, netData *NetData, conn *websocket.Conn) {
  if server.table.State == TABLESTATE_DONEBETTING {
    server.postBetting(player, netData, conn)
  } else if server.table.State == TABLESTATE_ROUNDOVER {
      // all other players folded before all comm cards were dealt
      // TODO: check for this state in a better fashion
      server.table.finishRound()
      fmt.Printf("winner # %d\n", len(server.table.Winners))
      fmt.Println(server.table.Winners[0].Name + " wins by folds")

      netData.Response = NETDATA_ROUNDOVER
      netData.Table = server.table
      netData.Msg = server.table.Winners[0].Name + " wins by folds"
      netData.PlayerData = nil

      server.sendResponseToAll(netData, nil)

      server.removeEliminatedPlayers()

      if server.table.State == TABLESTATE_GAMEOVER {
        server.gameOver()

        return
      }

      server.table.newRound()
      server.table.nextTableAction()
      server.checkBlindsAutoAllIn()
      server.sendDeals()
      server.sendPlayerTurnToAll()
      server.sendTable()
  } else {
    server.sendPlayerActionToAll(player, conn)
    server.sendPlayerTurnToAll()
  }
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error) {
  fmt.Println("server panicked")

  for _, conn := range server.clients {
    conn.WriteMessage(websocket.CloseMessage,
      websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
        err.Error()))
  }

  server.errChan <- err
  server.panicked = true
}

type ClientSettings struct {
  Name string
}

func (server *Server) handleClientSettings(conn *websocket.Conn, settings *ClientSettings) error {
  errs := ""

  if conn == nil || settings == nil {
    fmt.Println("handleClientSettings(): called with a nil parameter")

    return errors.New("handleClientSettings(): BUG: called with a nil parameter")
  }

  settings.Name = strings.TrimSpace(settings.Name)
  if settings.Name != "" {
    if len(settings.Name) > 15 {
      fmt.Printf("handleClientSettings(): %p requested a name that was longer " +
                 "than 15 characters. using a default name\n", conn)
      errs += "You've requested a name that was longer than 15 characters. " +
              "Using a default name.\n\n"
      settings.Name = ""
    } else {
      if player := server.playerMap[conn]; player != nil {
        if player.Name == settings.Name {
          fmt.Println("handleClientSettings(): name unchanged")
          errs += "name: unchanged\n\n"
        } else {
          for _, p := range server.table.players {
            if settings.Name == p.Name {
              fmt.Printf("%p requested the name `%s` which is reserved or already taken",
                         conn, settings.Name)
              errs += fmt.Sprintf("Name '%s' already in use. Current name unchanged.\n\n",
                                  settings.Name)
              break
            }
          }
        }
      } else {
        for _, player := range server.table.players {
          if settings.Name == player.Name {
            fmt.Printf("%p requested the name `%s` which is reserved or already taken. " +
                       "using a default name\n", conn, settings.Name)
            errs += fmt.Sprintf("Name '%s' already in use. Using a default name.\n\n",
                                settings.Name)
            settings.Name = ""
            break
          }
        }
      }
    }
  }

  if errs != "" {
    errs = "server response: settings changes: \n\n" + errs
    return errors.New(errs)
  }

  return nil
}

func (server *Server) applyClientSettings(conn *websocket.Conn, settings *ClientSettings) {
  if player := server.playerMap[conn]; player != nil {
    player.setName(settings.Name)
  }
}

func (server *Server) WSCLIClient(w http.ResponseWriter, req *http.Request) {
  if req.Header.Get("keepalive") != "" {
    return // NOTE: for heroku
  }

  conn, err := server.upgrader.Upgrade(w, req, nil)
  if err != nil {
    fmt.Printf("WS upgrade err %s\n", err.Error())

    return
  }

  cleanExit := false
  defer func() {
    if server.panicked { // server panic was already recovered in previous client handler
      return
    }

    if err := recover(); err != nil {
      server.serverError(panicRetToError(err))
    } else { // not a server panic()
      if player := server.playerMap[conn]; player != nil {
        if !cleanExit {
          fmt.Printf("%s had an unclean exit\n", player.Name)
        }
        if server.table.activePlayers.len > 1 &&
           server.table.curPlayer.Player.Name == player.Name {
          server.table.curPlayer.Player.Action.Action = NETDATA_FOLD
          server.table.setNextPlayerTurn()
          server.sendPlayerTurnToAll()
        }
      }

      server.removePlayerByConn(conn)
      server.removeClient(conn)
      server.closeConn(conn)
    }
  }()

  fmt.Printf("=> new conn from %s\n", req.Host)

  stopPing := make(chan bool)
  go func() {
    ticker := time.NewTicker(10 * time.Second)

    for {
      select {
      case <-stopPing:
        return
      case <-ticker.C:
        if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
          fmt.Printf("ping err: %s\n", err.Error())
          return
        }
      }
    }
  }()
  defer func() {
    stopPing <- true
  }()

  server.clientIDMap[conn] = randString(20)

  netData := NetData{
    ID:       server.clientIDMap[conn],
    Response: NETDATA_NEWCONN,
    Table:    server.table,
  }

  for {
    _, rawData, err := conn.ReadMessage()
    if err != nil {
      if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
        fmt.Printf("runServer(): readConn() conn: %p err: %v\n", conn, err)
      }

      return
    }

    // we need to set Table member to nil otherwise gob will
    // modify our server.table structure if a user server.sends that member
    netData = NetData{Response: NETDATA_NEWCONN, Table: nil}

    gob.NewDecoder(bufio.NewReader(bytes.NewReader(rawData))).Decode(&netData)

    netData.Table = server.table

    fmt.Printf("recv %s (%d bytes) from %p\n", netDataReqToString(&netData),
               len(rawData), conn)

    if netData.Request == NETDATA_NEWCONN {
      server.clients = append(server.clients, conn)

      server.table.mtx.Lock()
      server.table.NumConnected++
      server.table.mtx.Unlock()

      server.sendResponseToAll(&netData, nil)

      // server.send current player info to this client
      if server.table.NumConnected > 1 {
        netData.Response = NETDATA_CURPLAYERS
        netData.Table = server.table

        for _, player := range server.table.activePlayers.ToPlayerArray() {
          netData.ID = server.clientIDMap[server.getPlayerConn(player)]
          netData.PlayerData = server.table.PublicPlayerInfo(*player)
          sendData(&netData, conn)
        }
      }

      if err := server.handleClientSettings(conn, netData.ClientSettings); err != nil {
        sendData(&NetData{Response: NETDATA_BADREQUEST, Msg: err.Error()}, conn)
      }

      if player := server.table.getOpenSeat(); player != nil {
        server.playerMap[conn] = player

        server.applyClientSettings(conn, netData.ClientSettings)
        fmt.Printf("adding %p as player '%s'\n", &conn, player.Name)

        if server.table.State == TABLESTATE_NOTSTARTED {
          player.Action.Action = NETDATA_FIRSTACTION
          server.table.curPlayers.AddPlayer(player)
        } else {
          player.Action.Action = NETDATA_MIDROUNDADDITION
        }
        server.table.activePlayers.AddPlayer(player)

        if server.table.curPlayer == nil {
          server.table.curPlayer = server.table.curPlayers.node
        }

        if server.table.Dealer == nil {
          server.table.Dealer = server.table.activePlayers.node
        } else if server.table.SmallBlind == nil {
          server.table.SmallBlind = server.table.Dealer.next
        } else if server.table.BigBlind == nil {
          server.table.BigBlind = server.table.SmallBlind.next
        }

        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_NEWPLAYER
        netData.Table = server.table
        netData.PlayerData = server.table.PublicPlayerInfo(*player)

        server.sendResponseToAll(&netData, conn)

        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_YOURPLAYER
        netData.PlayerData = player
        sendData(&netData, conn)
      } else {
        netData.Response = NETDATA_SERVERMSG
        netData.Msg = "No open seats available. You have been added as a spectator"

        sendData(&netData, conn)
      }

      server.sendPlayerTurn(conn)

      if server.tableAdmin == nil {
        server.table.mtx.Lock()
        server.tableAdmin = conn
        server.table.mtx.Unlock()

        sendData(&NetData{Response: NETDATA_MAKEADMIN}, conn)
      }
    } else {
      switch netData.Request {
      case NETDATA_CLIENTEXITED:
        cleanExit = true

        return
      case NETDATA_CLIENTSETTINGS:
        if err := server.handleClientSettings(conn, netData.ClientSettings); err == nil {
          server.applyClientSettings(conn, netData.ClientSettings)

          if player := server.playerMap[conn]; player != nil {
            netData.Response = NETDATA_UPDATEPLAYER
            netData.ID = server.clientIDMap[conn]
            netData.PlayerData = server.table.PublicPlayerInfo(*player)
            netData.Table, netData.Msg = nil, ""

            server.sendResponseToAll(&netData, conn)

            netData.Response = NETDATA_YOURPLAYER
            sendData(&netData, conn)

            netData.Response = NETDATA_SERVERMSG
            netData.Msg = "server updated your settings"
            sendData(&netData, conn)
          }
        } else {
          sendData(&NetData{Response: NETDATA_SERVERMSG, Msg: err.Error()}, conn)
        }
      case NETDATA_STARTGAME:
        if conn != server.tableAdmin {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "only the table admin can do that"
          netData.Table = nil

          sendData(&netData, conn)
        } else if server.table.NumPlayers < 2 {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "not enough players to start"
          netData.Table = nil

          sendData(&netData, conn)
        } else if server.table.State != TABLESTATE_NOTSTARTED {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "this game has already started"
          netData.Table = nil

          sendData(&netData, conn)
        } else { // start game
          server.table.nextTableAction()

          server.sendDeals()
          server.sendAllPlayerInfo(false)
          server.sendPlayerTurnToAll()
          server.sendTable()
        }
      case NETDATA_CHATMSG:
        netData.ID = server.clientIDMap[conn]
        netData.Response = NETDATA_CHATMSG

        if len(netData.Msg) > 256 {
          netData.Msg = netData.Msg[:256] + "(snipped)"
        }

        if player := server.playerMap[conn]; player != nil {
          netData.Msg = fmt.Sprintf("[%s id: %s]: %s", player.Name,
                                    netData.ID[:7], netData.Msg)
        } else {
          netData.Msg = fmt.Sprintf("[spectator id: %s]: %s",
                                    netData.ID[:7], netData.Msg)
        }

        server.sendResponseToAll(&netData, nil)
      case NETDATA_ALLIN, NETDATA_BET, NETDATA_CALL, NETDATA_CHECK, NETDATA_FOLD:
        player := server.playerMap[conn]

        if player == nil {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "you are not a player"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if server.table.State == TABLESTATE_NOTSTARTED {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "a game has not been started yet"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if player.Name != server.table.curPlayer.Player.Name {
          netData.Response = NETDATA_BADREQUEST
          netData.Msg = "it's not your turn"
          netData.Table = nil

          sendData(&netData, conn)
          continue
        }

        if err := server.table.PlayerAction(player, netData.PlayerData.Action); err != nil {
          netData.Response = NETDATA_BADREQUEST
          netData.Table = nil
          netData.Msg = err.Error()

          sendData(&netData, conn)
        } else {
          server.postPlayerAction(player, &netData, conn)
        }
      default:
        netData.Response = NETDATA_BADREQUEST
        netData.Msg = "bad request"
        netData.Table, netData.PlayerData = nil, nil

        sendData(&netData, conn)
      }
      //sendData(&netData, writeConn)
    } // else{} end
  } //for loop end
} // func end


func (server *Server) run() error {
  fmt.Printf("starting server on %v\n", server.http.Addr)

  go func() {
    if err := server.http.ListenAndServe(); err != nil {
      fmt.Printf("ListenAndServe(): %s\n", err.Error())
    }
  }()

  select {
  case sig := <-server.sigChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "received signal: %s\n", sig.String())

    // TODO: ignore irrelevant signals
    server.sendResponseToAll(&NetData{Response: NETDATA_SERVERCLOSED}, nil)

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
      return err
    }

    return nil
  case err := <-server.errChan:
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    fmt.Fprintf(os.Stderr, "irrecoverable server error: %s\n", err.Error())

    if err := server.http.Shutdown(ctx); err != nil {
      fmt.Fprintf(os.Stderr, "server.Shutdown(): %s\n", err.Error())
      return err
    }

    return err
  }

  return nil
}
