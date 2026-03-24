package net

import (
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/gorilla/websocket"
)

type Clients struct {
  byConn   map[*websocket.Conn]*Client
  byPlayer map[*poker.Player]*Client
  byID     map[string]*Client
  byPrivID map[string]*Client
  byName   map[string]*Client
}

func NewClients() *Clients {
  return &Clients{
    byConn:   make(map[*websocket.Conn]*Client),
    byPlayer: make(map[*poker.Player]*Client),
    byID:     make(map[string]*Client),
    byPrivID: make(map[string]*Client),
    byName:   make(map[string]*Client),
  }
}

func (c *Clients) ByConn(conn *websocket.Conn) (*Client, bool) {
  client, ok := c.byConn[conn]
  return client, ok
}

func (c *Clients) ByPlayer(player *poker.Player) (*Client, bool) {
  client, ok := c.byPlayer[player]
  return client, ok
}

func (c *Clients) ByID(id string) (*Client, bool) {
  client, ok := c.byID[id]
  return client, ok
}

func (c *Clients) ByPrivID(privID string) (*Client, bool) {
  client, ok := c.byPrivID[privID]
  return client, ok
}

func (c *Clients) ByName(name string) (*Client, bool) {
  client, ok := c.byName[name]
  return client, ok
}

func (c *Clients) All() []*Client {
  clients := make([]*Client, 0, len(c.byConn))
  for _, client := range c.byConn {
    clients = append(clients, client)
  }
  return clients
}

func (c *Clients) Players() []*Client {
  clients := make([]*Client, 0, len(c.byPlayer))
  for _, client := range c.byPlayer {
    clients = append(clients, client)
  }
  return clients
}

func (c *Clients) Conns() []*websocket.Conn {
  conns := make([]*websocket.Conn, 0, len(c.byConn))
  for conn := range c.byConn {
    conns = append(conns, conn)
  }
  return conns
}

// Register adds client to the conn, ID, and privID indexes.
// Called during newClient after ID generation.
func (c *Clients) Register(client *Client, conn *websocket.Conn) {
  c.byConn[conn] = client
  c.byID[client.ID] = client
  c.byPrivID[client.privID] = client
}

func (c *Clients) SetName(client *Client, name string) {
  c.byName[name] = client
}

func (c *Clients) SetPlayer(client *Client, player *poker.Player) {
  c.byPlayer[player] = client
}

func (c *Clients) ClearPlayer(client *Client) {
  delete(c.byPlayer, client.Player)
}

// ReserveConn sets a placeholder entry so duplicate connection
// guards see this conn as occupied while newClient runs.
func (c *Clients) ReserveConn(conn *websocket.Conn) {
  c.byConn[conn] = &Client{}
}

func (c *Clients) SetConn(conn *websocket.Conn, client *Client) {
  c.byConn[conn] = client
}

func (c *Clients) RemoveConn(conn *websocket.Conn) {
  delete(c.byConn, conn)
}

func (c *Clients) Remove(client *Client) {
  delete(c.byName, client.Name)
  delete(c.byID, client.ID)
  delete(c.byPlayer, client.Player)
  delete(c.byPrivID, client.privID)
}
