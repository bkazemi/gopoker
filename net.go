package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
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

const NetActionNeedsTableBitMask = (NetDataNewConn | NetDataClientExited | NetDataUpdateTable | NetDataDeal)

const NetActionNeedsPlayerBitMask = (NetDataYourPlayer | NetDataNewPlayer | NetDataCurPlayers |
         NetDataPlayerLeft | NetDataPlayerAction | NetDataPlayerTurn |
         NetDataUpdatePlayer | NetDataCurHand | NetDataShowHand | NetDataDeal)

const NetActionNeedsActionBitMask = (NetDataAllIn | NetDataBet | NetDataCall | NetDataCheck | NetDataFold | NetDataRaise)

const NetActionNeedsBitMask = (NetActionNeedsTableBitMask | NetActionNeedsPlayerBitMask)

// data that gets sent between client and server
type NetData struct {
  Client   *Client
  Request  NetAction
  Response NetAction
  Msg      string // server msg or client chat msg

  Table    *Table
}

/*func NewNewData() *NetData {
  netData := &NetData{}

  return netData
}

func (netData *NetData) WithClient(client *Client) *NetData {
  netData.Client = client

  return netData
}*/

// clear all fields besides the client.
// used when recycling a netData instance
func (netData *NetData) ClearData(client *Client) {
  netData.Request = 0
  netData.Response = 0
  netData.Msg = ""
  netData.Table = nil

  // we pass a client with NetData structs that clients send
  // to ensure that the client member is valid (not modified by the client)
  if client != nil {
    netData.Client = client
  }
}

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

  return fmt.Sprintf("invalid NetData request: %v", reqOrRes)
}

// send a NetData structure to the client assigned to it's Client member.
func (netData *NetData) Send() {
  if netData.Client == nil {
    panic("NetData.Send(): .Client == nil")
  } else if netData.Client.conn == nil {
    panic("NetData.Send(): .Client.conn == nil")
  }

  //fmt.Printf("NetData.Send(): send %s to `%s` (%s)\n", netData.NetActionToString(),
  //           netData.Client.Name, netData.Client.ID)

  netData.unwrappedSender(netData.Client.conn, netData.Client.connType)
}

// send a NetData struct to a different client than the one assigned to it's
// Client member.
func (netData *NetData) SendTo(client *Client) {
  if client == nil {
    panic("NetData.SendTo(): client == nil")
  } else if client.conn == nil {
    panic("NetData.SendTo(): client.conn == nil")
  }

  //fmt.Printf("NetData.SendTo(): send %s to `%s` (%s)\n", netData.NetActionToString(),
  //           client.Name, client.ID)

  netData.unwrappedSender(client.conn, client.connType)
}

// send a NetData struct to a websocket.Conn. only used when a client is
// initiating their connection to the server.
func (netData *NetData) SendToConn(conn *websocket.Conn, connType string) {
  if conn == nil {
    panic("NetData.SendToConn(): conn == nil")
  }

  //fmt.Printf("NetData.SendToConn(): send %s to %p\n", netData.NetActionToString(), conn)

  netData.unwrappedSender(conn, connType)
}


// internal function that actually send the message to the websocket. do not call directly!
func (netData *NetData) unwrappedSender(conn *websocket.Conn, connType string) {
  // TODO: move this
  // XXX modifies global table
  /*if (data.Table != nil) {
    data.Table.Dealer     = data.Table.PublicPlayerInfo(*data.Table.Dealer)
    data.Table.SmallBlind = data.Table.PublicPlayerInfo(*data.Table.SmallBlind)
    data.Table.BigBlind   = data.Table.PublicPlayerInfo(*data.Table.BigBlind)
  }*/
  if connType == "cli" {
    var gobBuf bytes.Buffer
    enc := gob.NewEncoder(&gobBuf)

    enc.Encode(netData)

    //fmt.Fprintf(os.Stderr, "NETDATA: cli: sending %v to %p\n", netData.NetActionToString(), conn)

    conn.WriteMessage(websocket.BinaryMessage, gobBuf.Bytes())
  } else if connType == "web" {
    //conn.WriteJSON(netData)
    b, err := msgpack.Marshal(netData)
    if err != nil {
      panic(err)
    }

    fmt.Fprintf(os.Stderr, "NETDATA: web: sending: %v to %p\n", netData.NetActionToString(), conn)

    conn.WriteMessage(websocket.BinaryMessage, b)
  } else {
    panic(fmt.Sprintf("netData.unwrappedSender(): bad connType '%s'", connType))
  }
}
