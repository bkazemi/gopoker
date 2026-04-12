package net

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/rs/zerolog/log"
)

var invalidRoomNames map[string]bool

func init() {
	invalidRoomNames = map[string]bool{
		".":  true,
		"..": true,
	}
}

type RoomOpts struct {
	RoomName string          `json:"roomName"`
	NumSeats uint8           `json:"numSeats"`
	Lock     poker.TableLock `json:"lock"`
	Password string          `json:"password"`
}

type RoomList struct {
	RoomName     string          `json:"roomName"`
	TableLock    poker.TableLock `json:"tableLock"`
	NeedPassword bool            `json:"needPassword"`
	NumSeats     uint8           `json:"numSeats"`
	NumPlayers   uint8           `json:"numPlayers"`
	NumOpenSeats uint8           `json:"numOpenSeats"`
	NumConnected uint64          `json:"numConnected"`
}

// NOTE: caller must hold room.Lock() while invoking this helper.
// Room-scoped validation and apply steps rely on that lock for serialization;
// server.mtx is only used here for the global room-name registry.
func (server *Server) handleRoomSettings(room *Room, client *Client, settings *RoomSettings) (roomSettings *RoomSettings, m string, err error) {
	defer func() {
		if err != nil {
			return // log err in caller to keep this defer small and clean
		}

		log.Debug().Str("client", client.FullName(false)).Msg(m)
	}()

	if client == nil { // NOTE: this is currently an impossible condition because the callers access client.ID beforehand
		return nil, "", errors.New("Server.handleRoomSettings(): BUG: client == nil")
	} else if settings == nil {
		return nil, "", errors.New("Server.handleRoomSettings(): BUG: settings == nil")
	}

	msg := "server response: room settings changes:\n\n"
	errs := ""

	roomMsg, roomErr := room.handleRoomSettings(client, settings)
	msg += roomMsg

	server.mtx.Lock()
	defer server.mtx.Unlock()

	validatedRoomName, renameRoomOk, renameRoomErr := server.validateRoomRename(room, settings.RoomName)
	if roomErr != nil {
		errs += roomErr.Error()
		if !strings.HasSuffix(errs, "\n") {
			errs += "\n"
		}
	}
	if roomErr == nil {
		room.applyRoomSettings(settings)
	}

	if renameRoomOk {
		server.applyRoomRename(room, validatedRoomName)
		msg += "room name: changed\n"
	} else if renameRoomErr != nil {
		if errs != "" {
			errs += "\n"
		}
		errs += "room name: " + renameRoomErr.Error()
	} else {
		msg += "room name: unchanged\n"
	}

	if settings.NumSeats != room.table.NumSeats {
		if err := room.table.SetNumSeats(settings.NumSeats); err != nil {
			if errs != "" {
				errs += "\n"
			}
			errs += "num seats: " + err.Error()
		} else {
			msg += "num seats: changed\n"
		}
	} else {
		msg += "num seats: unchanged\n"
	}

	roomSettings = room.getRoomSettings()

	if errs != "" {
		return roomSettings, msg, errors.New("server response: unable to complete request due to following errors:\n\n" + errs)
	}

	return roomSettings, msg, nil
}

func (server *Server) validateRoomRename(room *Room, newName string) (validatedName string, changed bool, err error) {
	if newName == "" || room.name == newName {
		return room.name, false, nil
	}

	if false {
		return room.name, false, errors.New("invalid name requested")
	}

	if int32(len(newName)) > server.MaxRoomNameLen {
		log.Warn().
			Str("roomName", newName[:server.MaxRoomNameLen+1]+"...").
			Int("len", len(newName)).
			Int32("max", server.MaxRoomNameLen).
			Msg("room name too long, using random name")

		newName = server.randRoomName()
	}

	if server.hasRoom(newName) {
		return room.name, false, errors.New(fmt.Sprintf("requested name '%v' already taken",
			newName))
	}

	return newName, true, nil
}

func (server *Server) applyRoomRename(room *Room, newName string) {
	delete(server.rooms, room.name)
	room.name = newName
	server.rooms[newName] = room
}

func (server *Server) renameRoom(room *Room, newName string) (bool, error) {
	validatedName, changed, err := server.validateRoomRename(room, newName)
	if err != nil || !changed {
		return changed, err
	}

	server.applyRoomRename(room, validatedName)

	return true, nil
}

func (server *Server) roomCount(w http.ResponseWriter, req *http.Request) {
	roomCnt := len(server.rooms)

	res := struct {
		RoomCount int `json:"roomCount"`
	}{
		RoomCount: roomCnt,
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

// NOTE: caller needs to handle server locking
func (server *Server) hasRoom(name string) bool {
	_, found := server.rooms[name]

	return found
}

func (server *Server) listRooms(w http.ResponseWriter, req *http.Request) {
	roomListArr := make([]RoomList, 0)

	for name, room := range server.rooms {
		table := room.table

		roomListArr = append(
			roomListArr,
			RoomList{
				RoomName:     name,
				TableLock:    table.Lock,
				NeedPassword: table.Password != "",
				NumSeats:     table.NumSeats,
				NumPlayers:   table.NumPlayers,
				NumOpenSeats: table.NumSeats - table.NumPlayers,
				NumConnected: table.NumConnected,
			},
		)
	}

	jsonBody, err := json.Marshal(roomListArr)
	if err != nil {
		http.Error(w, "failed to encode JSON", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonBody)
}

func (server *Server) randRoomName() string {
	name := ""

	for {
		name = poker.RandString(10) // 62^10 is plenty ;)
		if _, found := server.rooms[name]; found {
			log.Warn().Str("roomName", name).Msg("possible bug: roomName already found in rooms")
		} else {
			break
		}
	}

	return name
}

func (server *Server) createNewRoom(w http.ResponseWriter, req *http.Request) {
	server.mtx.Lock()
	defer server.mtx.Unlock()

	var roomOpts RoomOpts
	if err := json.NewDecoder(req.Body).Decode(&roomOpts); err != nil {
		log.Error().Err(err).Msg("problem decoding POST request")
		http.Error(w, "failed to parse JSON body", http.StatusBadRequest)

		return
	}

	log.Debug().Msgf("roomOpts: %v", roomOpts)

	if roomOpts.RoomName == "" {
		log.Debug().Msg("empty roomName given")
		roomOpts.RoomName = server.randRoomName()
	} else if invalidRoomNames[roomOpts.RoomName] {
		log.Warn().Str("roomName", roomOpts.RoomName).Msg("roomName is invalid")
		roomOpts.RoomName = server.randRoomName()
	} else if server.rooms[roomOpts.RoomName] != nil {
		log.Warn().Str("roomName", roomOpts.RoomName).Msg("roomName already taken")
		roomOpts.RoomName = server.randRoomName()
	} else if int32(len(roomOpts.RoomName)) > server.MaxRoomNameLen {
		roomOpts.RoomName = roomOpts.RoomName[:server.MaxRoomNameLen+1] + "..."
		log.Warn().
			Str("roomName", roomOpts.RoomName).
			Int("len", len(roomOpts.RoomName)).
			Int32("max", server.MaxRoomNameLen).
			Msg("roomName too long, clamping")
		roomOpts.RoomName = server.randRoomName()
	}

	if roomOpts.NumSeats < 2 || roomOpts.NumSeats > 7 {
		log.Warn().
			Uint8("numSeats", roomOpts.NumSeats).
			Msg("requested NumSeats out of range, using default (7)")
		roomOpts.NumSeats = 7
	}

	deck := poker.NewDeck()

	deck.Shuffle()

	table, tableErr := poker.NewTable(deck, roomOpts.NumSeats, roomOpts.Lock, roomOpts.Password,
		make([]bool, roomOpts.NumSeats))
	if tableErr != nil {
		log.Error().Err(tableErr).Msg("problem creating new table")
		http.Error(w, fmt.Sprintf("couldn't create a new table: %v", tableErr), http.StatusBadRequest)

		return
	}

	log.Debug().
		Msgf("table.Lock: %v table.Password: %v table.NumSeats: %v", table.Lock, table.Password, table.NumSeats)

	log.Info().Str("roomName", roomOpts.RoomName).Msg("creating new room")

	room := NewRoom(roomOpts.RoomName, table, poker.RandString(17))
	server.rooms[roomOpts.RoomName] = room

	res := struct {
		URL          string `json:"URL"`
		RoomName     string `json:"roomName"`
		CreatorToken string `json:"creatorToken"`
	}{
		URL:          fmt.Sprintf("/room/%s", url.QueryEscape(roomOpts.RoomName)),
		RoomName:     roomOpts.RoomName,
		CreatorToken: room.creatorToken,
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

func (server *Server) removeRoom(room *Room) {
	server.mtx.Lock()
	defer server.mtx.Unlock()

	if _, found := server.rooms[room.name]; found {
		log.Info().Str("room", room.name).Msg("removing room")

		delete(server.rooms, room.name)
	} else {
		log.Warn().Str("room", room.name).Msg("room not found")
	}
}
