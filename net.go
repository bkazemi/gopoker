package main

import (
	"bytes"
	"encoding/gob"

	"github.com/gorilla/websocket"
)

// requests/responses sent between client and server
const (
  NETDATA_CLOSE = iota
  NETDATA_NEWCONN

  NETDATA_YOURPLAYER
  NETDATA_NEWPLAYER
  NETDATA_CURPLAYERS
  NETDATA_UPDATEPLAYER
  NETDATA_UPDATETABLE
  NETDATA_PLAYERLEFT
  NETDATA_CLIENTEXITED
  NETDATA_CLIENTSETTINGS
  NETDATA_RESET

  NETDATA_SERVERCLOSED

  NETDATA_MAKEADMIN
  NETDATA_STARTGAME

  NETDATA_CHATMSG

  NETDATA_PLAYERACTION
  NETDATA_PLAYERTURN
  NETDATA_PLAYERHEAD
  NETDATA_ALLIN
  NETDATA_BET
  NETDATA_CALL
  NETDATA_CHECK
  NETDATA_RAISE
  NETDATA_FOLD

  NETDATA_CURHAND
  NETDATA_SHOWHAND

  NETDATA_FIRSTACTION
  NETDATA_MIDROUNDADDITION
  NETDATA_ELIMINATED
  NETDATA_VACANTSEAT

  NETDATA_DEAL
  NETDATA_FLOP
  NETDATA_TURN
  NETDATA_RIVER
  NETDATA_BESTHAND
  NETDATA_ROUNDOVER

  NETDATA_SERVERMSG
  NETDATA_BADREQUEST
)

type NetData struct {
  ID         string
  Request    int
  Response   int
  Msg        string // server msg or client chat msg

  ClientSettings *ClientSettings // client requested settings
  Table          *Table
  PlayerData     *Player
}

func (netData *NetData) Init() {
  return
}

// NOTE: tmp for debugging
func netDataReqToString(netData *NetData) string {
  if netData == nil {
    return "netData == nil"
  }

  netDataReqStringMap := map[int]string{
    NETDATA_CLOSE:          "NETDATA_CLOSE",
    NETDATA_NEWCONN:        "NETDATA_NEWCONN",
    NETDATA_YOURPLAYER:     "NETDATA_YOURPLAYER",
    NETDATA_NEWPLAYER:      "NETDATA_NEWPLAYER",
    NETDATA_CURPLAYERS:     "NETDATA_CURPLAYERS",
    NETDATA_UPDATEPLAYER:   "NETDATA_UPDATEPLAYER",
    NETDATA_UPDATETABLE:    "NETDATA_UPDATETABLE",
    NETDATA_PLAYERLEFT:     "NETDATA_PLAYERLEFT",
    NETDATA_CLIENTEXITED:   "NETDATA_CLIENTEXITED",
    NETDATA_CLIENTSETTINGS: "NETDATA_CLIENTSETTINGS",
    NETDATA_RESET:          "NETDATA_RESET",

    NETDATA_MAKEADMIN: "NETDATA_MAKEADMIN",
    NETDATA_STARTGAME: "NETDATA_STARTGAME",

    NETDATA_CHATMSG: "NETDATA_CHATMSG",

    NETDATA_PLAYERACTION: "NETDATA_PLAYERACTION",
    NETDATA_PLAYERTURN:   "NETDATA_PLAYERTURN",
    NETDATA_PLAYERHEAD:   "NETDATA_PLAYERHEAD",
    NETDATA_ALLIN:        "NETDATA_ALLIN",
    NETDATA_BET:          "NETDATA_BET",
    NETDATA_CALL:         "NETDATA_CALL",
    NETDATA_CHECK:        "NETDATA_CHECK",
    NETDATA_RAISE:        "NETDATA_RAISE",
    NETDATA_FOLD:         "NETDATA_FOLD",
    NETDATA_CURHAND:      "NETDATA_CURHAND",
    NETDATA_SHOWHAND:     "NETDATA_SHOWHAND",

    NETDATA_FIRSTACTION:      "NETDATA_FIRSTACTION",
    NETDATA_MIDROUNDADDITION: "NETDATA_MIDROUNDADDITION",
    NETDATA_ELIMINATED:       "NETDATA_ELIMINATED",
    NETDATA_VACANTSEAT:       "NETDATA_VACANTSEAT",

    NETDATA_DEAL:      "NETDATA_DEAL",
    NETDATA_FLOP:      "NETDATA_FLOP",
    NETDATA_TURN:      "NETDATA_TURN",
    NETDATA_RIVER:     "NETDATA_RIVER",
    NETDATA_BESTHAND:  "NETDATA_BESTHAND",
    NETDATA_ROUNDOVER: "NETDATA_ROUNDOVER",

    NETDATA_SERVERMSG:  "NETDATA_SERVERMSG",
    NETDATA_BADREQUEST: "NETDATA_BADREQUEST",
  }

  // XXX remove me
  reqOrRes := NETDATA_CLOSE
  if netData.Request == NETDATA_CLOSE {
    reqOrRes = netData.Response
  } else {
    reqOrRes = netData.Request
  }

  if netDataStr, ok := netDataReqStringMap[reqOrRes]; ok {
    return netDataStr
  }

  return "invalid NetData request"
}

func sendData(data *NetData, conn *websocket.Conn) {
  if data == nil {
    panic("sendData(): data == nil")
  }

  if conn == nil {
    panic("sendData(): websocket == nil")
  }

  // TODO: move this
  // XXX modifies global table
  /*if (data.Table != nil) {
    data.Table.Dealer     = data.Table.PublicPlayerInfo(*data.Table.Dealer)
    data.Table.SmallBlind = data.Table.PublicPlayerInfo(*data.Table.SmallBlind)
    data.Table.BigBlind   = data.Table.PublicPlayerInfo(*data.Table.BigBlind)
  }*/

  //fmt.Printf("sending %p to %p...\n", data, conn)

  var gobBuf bytes.Buffer
  enc := gob.NewEncoder(&gobBuf)

  enc.Encode(data)

  conn.WriteMessage(websocket.BinaryMessage, gobBuf.Bytes())
}
