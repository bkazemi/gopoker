package main

import (
	"bytes"
	"encoding/gob"

	"github.com/gorilla/websocket"
)

// requests/responses sent between client and server
type NetAction uint64
const (
  NetDataClose NetAction = 1 << iota
  NetDataNewConn

  //NetDataNeedsTable
  //NetDataNeedsPlayer
  //NetDataNeedsID

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
) // 41 flags, 23 left

const NetActionNeedsTableBitMask = (NetDataNewConn | NetDataClientExited | NetDataUpdateTable)

const NetActionNeedsPlayerBitMask = (NetDataYourPlayer | NetDataNewPlayer | NetDataCurPlayers |
         NetDataPlayerLeft | NetDataPlayerAction | NetDataPlayerTurn |
         NetDataUpdatePlayer | NetDataCurHand | NetDataShowHand |
         NetDataEliminated)

const NetActionNeedsBitMask = (NetActionNeedsTableBitMask | NetActionNeedsPlayerBitMask)

// data that gets sent between client and server
type NetData struct {
  ID       string
  Request  NetAction
  Response NetAction
  Msg      string // server msg or client chat msg

  ClientSettings *ClientSettings // client requested settings
  Table          *Table
  PlayerData     *Player
}

/*func (netData *NetData) Init() {
  return
}*/

// check which NetActions must have a Table struct included
func (netData *NetData) NeedsTable() bool {
  if netData.Request != 0 { // its a request
    return netData.Request & NetActionNeedsTableBitMask != 0
  }

  // it's a response
  return netData.Response & NetActionNeedsTableBitMask != 0
}

// check which NetActions must have a Player struct included
func (netData *NetData) NeedsPlayer() bool {
  if netData.Request != 0 { // it's a request
    return netData.Request & NetActionNeedsPlayerBitMask != 0
  }

  // it's a response
  return netData.Response & NetActionNeedsPlayerBitMask != 0
}

// return the string representation of a NetAction
// NOTE: tmp for debugging
func (netData *NetData) NetActionToString() string {
  if netData == nil {
    return "netData == nil"
  }

  netDataReqStringMap := map[NetAction]string{
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
  var reqOrRes NetAction
  if netData.Request != 0 {
    reqOrRes = netData.Request
  } else {
    reqOrRes = netData.Response
  }

  if netDataStr, ok := netDataReqStringMap[reqOrRes]; ok {
    return netDataStr
  }

  return "invalid NetData request"
}

func (netData *NetData) Send(conn *websocket.Conn) {
  if conn == nil {
    panic("NetData.Send(): websocket == nil")
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

  enc.Encode(netData)

  //fmt.Fprintf(os.Stderr, "NetData.Send(): send %s (%v bytes) to %p\n", netData.NetActionToString(), len(gobBuf.Bytes()), conn)

  conn.WriteMessage(websocket.BinaryMessage, gobBuf.Bytes())
}
