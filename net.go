package main

import (
	"bytes"
	"encoding/gob"

	"github.com/gorilla/websocket"
)

// requests/responses sent between client and server
const (
  NetDataClose = iota
  NetDataNewConn

  NetDataYourPlayer
  NetDataNewPlayer
  NetDataCurPlayers
  NetDataUpdatePlayer
  NetDataUpdateTable
  NetDataPlayerLeft
  NetDataClientExited
  NetDataClientSettings
  NetDataReset

  NetDataServerClosed

  NetDataTableLocked
  NetDataBadAuth
  NetDataMakeAdmin
  NetDataStartGame

  NetDataChatMsg

  NetDataPlayerAction
  NetDataPlayerTurn
  NetDataPlayerHead
  NetDataAllIn
  NetDataBet
  NetDataCall
  NetDataCheck
  NetDataRaise
  NetDataFold

  NetDataCurHand
  NetDataShowHand

  NetDataFirstAction
  NetDataMidroundAddition
  NetDataEliminated
  NetDataVacantSeat

  NetDataDeal
  NetDataFlop
  NetDataTurn
  NetDataRiver
  NetDataBestHand
  NetDataRoundOver

  NetDataServerMsg
  NetDataBadRequest
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
    NetDataClose:          "NetDataClose",
    NetDataNewConn:        "NetDataNewConn",

    NetDataYourPlayer:     "NetDataYourPlayer",
    NetDataNewPlayer:      "NetDataNewPlayer",
    NetDataCurPlayers:     "NetDataCurPlayers",
    NetDataUpdatePlayer:   "NetDataUpdatePlayer",
    NetDataUpdateTable:    "NetDataUpdateTable",
    NetDataPlayerLeft:     "NetDataPlayerLeft",
    NetDataClientExited:   "NetDataClientExited",
    NetDataClientSettings: "NetDataClientSettings",
    NetDataReset:          "NetDataReset",

    NetDataTableLocked: "NetDataTableLocked",
    NetDataBadAuth:     "NetDataBadAuth",
    NetDataMakeAdmin:   "NetDataMakeAdmin",
    NetDataStartGame:   "NetDataStartGame",

    NetDataChatMsg: "NetDataChatMsg",

    NetDataPlayerAction: "NetDataPlayerAction",
    NetDataPlayerTurn:   "NetDataPlayerTurn",
    NetDataPlayerHead:   "NetDataPlayerHead",
    NetDataAllIn:        "NetDataAllIn",
    NetDataBet:          "NetDataBet",
    NetDataCall:         "NetDataCall",
    NetDataCheck:        "NetDataCheck",
    NetDataRaise:        "NetDataRaise",
    NetDataFold:         "NetDataFold",

    NetDataCurHand:      "NetDataCurHand",
    NetDataShowHand:     "NetDataShowHand",

    NetDataFirstAction:      "NetDataFirstAction",
    NetDataMidroundAddition: "NetDataMidroundAddition",
    NetDataEliminated:       "NetDataEliminated",
    NetDataVacantSeat:       "NetDataVacantSeat",

    NetDataDeal:      "NetDataDeal",
    NetDataFlop:      "NetDataFlop",
    NetDataTurn:      "NetDataTurn",
    NetDataRiver:     "NetDataRiver",
    NetDataBestHand:  "NetDataBestHand",
    NetDataRoundOver: "NetDataRoundOver",

    NetDataServerMsg:  "NetDataServerMsg",
    NetDataBadRequest: "NetDataBadRequest",
  }

  // XXX remove me
  reqOrRes := NetDataClose
  if netData.Request == NetDataClose {
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

  var gobBuf bytes.Buffer
  enc := gob.NewEncoder(&gobBuf)

  enc.Encode(data)

  //fmt.Fprintf(os.Stderr, "sendData(): send %s (%v bytes) to %p\n", netDataReqToString(data), len(gobBuf.Bytes()), conn)

  conn.WriteMessage(websocket.BinaryMessage, gobBuf.Bytes())
}
