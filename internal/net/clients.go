package net

import (
	"sync"

	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/gorilla/websocket"
)

type Clients struct {
  mtx      sync.RWMutex
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
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  client, ok := c.byConn[conn]
  return client, ok
}

func (c *Clients) ByPlayer(player *poker.Player) (*Client, bool) {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  client, ok := c.byPlayer[player]
  return client, ok
}

func (c *Clients) ByID(id string) (*Client, bool) {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  client, ok := c.byID[id]
  return client, ok
}

func (c *Clients) ByPrivID(privID string) (*Client, bool) {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  client, ok := c.byPrivID[privID]
  return client, ok
}

func (c *Clients) ByName(name string) (*Client, bool) {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  client, ok := c.byName[name]
  return client, ok
}

func (c *Clients) All() []*Client {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  clients := make([]*Client, 0, len(c.byConn))
  for _, client := range c.byConn {
    clients = append(clients, client)
  }
  return clients
}

func (c *Clients) Players() []*Client {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  clients := make([]*Client, 0, len(c.byPlayer))
  for _, client := range c.byPlayer {
    clients = append(clients, client)
  }
  return clients
}

func (c *Clients) Conns() []*websocket.Conn {
  c.mtx.RLock()
  defer c.mtx.RUnlock()
  conns := make([]*websocket.Conn, 0, len(c.byConn))
  for conn := range c.byConn {
    conns = append(conns, conn)
  }
  return conns
}

// Register adds client to the conn, ID, and privID indexes.
// Called during newClient after ID generation.
func (c *Clients) Register(client *Client, conn *websocket.Conn) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  c.byConn[conn] = client
  c.byID[client.ID] = client
  c.byPrivID[client.privID] = client
}

func (c *Clients) SetName(client *Client, name string) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  c.byName[name] = client
}

func (c *Clients) SetPlayer(client *Client, player *poker.Player) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  c.byPlayer[player] = client
}

func (c *Clients) ClearPlayer(client *Client) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  delete(c.byPlayer, client.Player)
}

// ReserveConn sets a placeholder entry so duplicate connection
// guards see this conn as occupied while newClient runs.
func (c *Clients) ReserveConn(conn *websocket.Conn) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  c.byConn[conn] = &Client{}
}

func (c *Clients) SetConn(conn *websocket.Conn, client *Client) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  c.byConn[conn] = client
}

func (c *Clients) RemoveConn(conn *websocket.Conn) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  delete(c.byConn, conn)
}

func (c *Clients) Remove(client *Client) {
  c.mtx.Lock()
  defer c.mtx.Unlock()
  delete(c.byName, client.Name)
  delete(c.byID, client.ID)
  delete(c.byPlayer, client.Player)
  delete(c.byPrivID, client.privID)
}
