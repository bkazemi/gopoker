package net

import (
	"compress/flate"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Server struct {
	rooms map[string]*Room

	MaxConnBytes   int64
	MaxChatMsgLen  int32
	MaxRoomNameLen int32

	router *mux.Router

	http     *http.Server
	upgrader websocket.Upgrader

	sigChan  chan os.Signal
	errChan  chan error
	panicked bool

	mtx sync.Mutex
}

func NewServer(addr string) *Server {
	const (
		MaxConnBytes   = 10e3
		MaxChatMsgLen  = 256
		MaxRoomNameLen = 50
		IdleTimeout    = 0
		ReadTimeout    = 0
	)

	router := mux.NewRouter()

	server := &Server{
		rooms: make(map[string]*Room),

		MaxConnBytes:   MaxConnBytes,
		MaxChatMsgLen:  MaxChatMsgLen,
		MaxRoomNameLen: MaxRoomNameLen,

		errChan:  make(chan error),
		panicked: false,

		upgrader: websocket.Upgrader{
			EnableCompression: true,
			Subprotocols:      []string{"permessage-deflate"},
			ReadBufferSize:    4096,
			WriteBufferSize:   4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // XXX TMP REMOVE ME
			},
		},

		router: router,

		http: &http.Server{
			Addr:        addr,
			IdleTimeout: IdleTimeout,
			ReadTimeout: ReadTimeout,
			Handler:     router,
		},

		sigChan: make(chan os.Signal, 1),
	}

	handleRoom := func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		roomName := vars["roomName"]

		if room, found := server.rooms[roomName]; found {
			if room.isTableLocked() {
				w.WriteHeader(http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		} else {
			http.NotFound(w, req)
		}
	}

	handleClient := func(w http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		roomName := vars["roomName"]
		connType := vars["connType"]

		if (connType != "cli" && connType != "web") ||
			server.rooms[roomName] == nil {
			http.NotFound(w, req)

			return
		}

		server.WSClient(w, req, server.rooms[roomName], connType)
	}

	server.http.SetKeepAlivesEnabled(true)
	router.HandleFunc("/health", healthCheck).Methods("GET")
	router.HandleFunc("/status", status).Methods("GET")
	router.HandleFunc("/new", server.createNewRoom).Methods("POST")
	router.HandleFunc("/roomCount", server.roomCount).Methods("GET")
	router.HandleFunc("/rooms", server.listRooms).Methods("GET")
	router.HandleFunc("/room/{roomName}", handleRoom)
	router.HandleFunc("/room/{roomName}/{connType}", handleClient).Methods("GET")

	signal.Notify(server.sigChan, os.Interrupt)

	return server
}

func healthCheck(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func status(w http.ResponseWriter, req *http.Request) {
	res := struct {
		Status string `json:"status"`
	}{
		Status: "running",
	}

	jsonBody, err := json.Marshal(res)
	if err != nil {
		http.Error(w, "failed to encode JSON", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody)
}

func closeConn(conn *websocket.Conn) {
	log.Debug().Str("remote", conn.RemoteAddr().String()).Msg("closing connection")
	conn.Close()
}

// cleanly close connections after a server panic()
func (server *Server) serverError(err error, room *Room) {
	log.Error().Msg("server panicked")

	for _, conn := range room.clients.Conns() {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr,
				err.Error()))
	}

	server.errChan <- err
	server.panicked = true
}

func (server *Server) WSClient(w http.ResponseWriter, req *http.Request, room *Room, connType string) {
	if req.Header.Get("keepalive") != "" {
		return // NOTE: for heroku
	}

	if connType != "cli" && connType != "web" {
		log.Warn().Str("room", room.name).Str("connType", connType).Msg("invalid connType")
		return
	}

	conn, err := server.upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Error().Err(err).Msg("WS upgrade error")
		return
	}

	conn.SetReadLimit(server.MaxConnBytes)
	conn.EnableWriteCompression(true)
	conn.SetCompressionLevel(flate.BestCompression)

	sess := &wsSession{
		server:   server,
		room:     room,
		conn:     conn,
		connType: connType,
	}
	defer func() { server.handleDisconnect(room, conn, sess.cleanExit.Load()) }()

	log.Info().Str("room", room.name).Str("host", req.Host).Msg("new websocket connection")

	stopPing := startPingLoop(conn, room.name)
	defer stopPing()

	server.runWSInputLoop(sess, readNetData)
}

// requestInputLoopExit marks the session as a clean exit. The read loop
// isn't actively interrupted; it unblocks when the peer's close frame
// arrives, or when the ping loop's pong timeout tears down the connection.
func (s *wsSession) requestInputLoopExit() {
	s.cleanExit.Store(true)
}

func (server *Server) runWSInputLoop(
	sess *wsSession,
	readFn func(*websocket.Conn, string, *Room, int64) (NetData, bool, error),
) {
	for {
		netData, cleanClose, err := readFn(sess.conn, sess.connType, sess.room, server.MaxConnBytes)
		if err != nil {
			if cleanClose {
				sess.cleanExit.Store(true)
			}
			return
		}

		switch netData.Request {
		case NetDataNewConn:
			server.handleNewConn(sess.room, netData, sess.conn, sess.connType)
		case NetDataPlayerReconnecting:
			server.handleReconnect(sess.room, netData, sess.conn, sess.connType)
		default:
			client, _ := sess.room.clients.ByConn(sess.conn)
			go sess.dispatch(client, netData)
		}
	}
}

func (server *Server) Run() error {
	log.Info().Str("addr", server.http.Addr).Msg("starting server")

	go func() {
		if err := server.http.ListenAndServe(); err != nil {
			log.Error().Err(err).Msg("http.ListenAndServe failed")
		}
	}()

	select {
	case sig := <-server.sigChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Info().Str("signal", sig.String()).Msg("received signal")

		// TODO: ignore irrelevant signals
		for _, room := range server.rooms {
			room.sendResponseToAll(&NetData{Response: NetDataServerClosed}, nil)
		}

		if err := server.http.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server.http.Shutdown failed")
			return err
		}

		return nil
	case err := <-server.errChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Error().Err(err).Msg("irrecoverable server error")

		if err := server.http.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server.http.Shutdown failed")
			return err
		}

		return err
	}
}
