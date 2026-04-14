package net

import (
	"io"
	"sync"
	"testing"

	"github.com/bkazemi/gopoker/internal/playerState"
	"github.com/bkazemi/gopoker/internal/poker"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Reproduces the panic observed when two players disconnect near-simultaneously:
//
//	panic: Table.getNonFoldedPlayers(): BUG: len(players) == 0
//
// The race: removePlayer's cleanup defer runs AFTER table.Mtx is released
// (defers are LIFO), so a second concurrent cleanupPlayerOnExit can mutate
// table state while the first defer is still deciding whether to call
// FinishRound / Reset. Result: FinishRound is invoked with an empty
// activePlayers list and GetNonFoldedPlayers asserts.
func TestConcurrentCleanupPlayerOnExitDoesNotPanic(t *testing.T) {
	t.Parallel()

	// Silence zerolog so test output stays clean.
	prev := log.Logger
	log.Logger = zerolog.New(io.Discard)
	t.Cleanup(func() { log.Logger = prev })

	// Loop to widen odds of hitting the race: the bug window is the short
	// gap between removePlayer releasing table.Mtx and its cleanup defer
	// running. 500 iterations is reliable on the reference machine; a few
	// thousand is still well under a second.
	const iterations = 2000

	for i := 0; i < iterations; i++ {
		deck := poker.NewDeck()
		table, err := poker.NewTable(deck, 2, poker.TableLockNone, "", []bool{false, false})
		if err != nil {
			t.Fatalf("NewTable: %v", err)
		}

		room := NewRoom("test", table, "")

		p0 := table.GetOpenSeat()
		p1 := table.GetOpenSeat()
		if p0 == nil || p1 == nil {
			t.Fatalf("failed to seat players")
		}

		mkClient := func(id string, conn *websocket.Conn) *Client {
			c := NewClient(nil)
			c.ID = id
			c.privID = id + "-priv"
			c.conn = conn
			c.Name = id
			room.clients.Register(c, conn)
			// Nil the conn after registration so SendTo/Send see conn==nil
			// and skip the wire write (isDisconnected short-circuits both).
			c.conn = nil
			c.isDisconnected = true
			return c
		}
		// Distinct non-nil pointers so the byConn map keeps both entries.
		client0 := mkClient("c0", new(websocket.Conn))
		client1 := mkClient("c1", new(websocket.Conn))

		room.addPlayer(client0, p0, &NetData{}, true)
		room.addPlayer(client1, p1, &NetData{}, true)

		// Mirror the bug scenario: a round is in progress, p0 is folded,
		// p1 is the current player with a live action.
		table.State = poker.TableStatePreFlop
		p0.Action.Action = playerState.Fold
		p1.Action.Action = playerState.FirstAction

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			room.cleanupPlayerOnExit(client0, playerExitDisconnect)
		}()
		go func() {
			defer wg.Done()
			room.cleanupPlayerOnExit(client1, playerExitDisconnect)
		}()
		wg.Wait()
	}
}
