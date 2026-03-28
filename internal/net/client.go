package net

import (
	"fmt"
	"sync"
	"time"

	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"

	"github.com/gorilla/websocket"
)

const MaxClientNameLen = 20

type Client struct {
	ID       string
	Name     string
	Player   *poker.Player
	Settings *ClientSettings // XXX: Settings.Name is redundant now

	privID         string // used for reconnecting
	conn           *websocket.Conn
	connType       string
	mtx            *sync.Mutex
	isDisconnected bool
	reconnectTimer *time.Timer
}

func NewClient(settings *ClientSettings) *Client {
	if settings == nil {
		log.Warn().Msg("nil ClientSettings, using defaults")
		settings = NewClientSettings()
	}

	client := &Client{
		Settings: settings,
		mtx:      &sync.Mutex{},
	}

	return client
}

func (client *Client) FullName(includeConn bool) string {
	name := client.Name
	if name == "" {
		name = "noname"
	}

	conn := ""
	if includeConn {
		conn = fmt.Sprintf(" (%p)", &client.conn)
	}

	return fmt.Sprintf("%s (%s)%s", name, client.ID, conn)
}

func (client *Client) SetName(name string) *Client {
	if len(name) > MaxClientNameLen {
		log.Warn().Msg("requested name too long, rejecting")

		return client
	}

	log.Debug().Str("client", client.ID).Str("from", client.Name).Str("to", name).Msg("name changed")
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

func (client *Client) IsPlayer() bool {
	return client.Player != nil
}
