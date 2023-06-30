package net

import (
	"fmt"

	"github.com/bkazemi/gopoker/internal/poker"

	"github.com/gorilla/websocket"
)

const MaxClientNameLen = 20
type Client struct {
  ID       string
  Name     string
  Player   *poker.Player
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

func (client *Client) SetName(name string) *Client {
  if len(name) > MaxClientNameLen {
    fmt.Printf("Client.SetName(): requested name too long. rejecting\n")

    return client
  }

  fmt.Printf("Client.SetName(): <%s> (%p) '%s' => '%s'\n", client.ID, client.conn, client.Name, name)
  client.Name = name

  return client
}

func (client *Client) SetConn(conn *websocket.Conn) *Client {
  client.conn = conn

  return client
}

func (client *Client) SetConnType(connType string) *Client {
  client.connType = connType

  return client
}
