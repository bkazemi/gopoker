package net

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRunWSInputLoopMarksCleanExitOnCleanClose(t *testing.T) {
	t.Parallel()

	server := &Server{}
	sess := &wsSession{}

	server.runWSInputLoop(sess, func(_ *websocket.Conn, _ string, _ *Room, _ int64) (NetData, bool, error) {
		return NetData{}, true, errors.New("peer close")
	})

	if !sess.cleanExit.Load() {
		t.Fatal("expected cleanExit to be true after a clean close")
	}
}

func TestRunWSInputLoopReturnsOnReadError(t *testing.T) {
	t.Parallel()

	server := &Server{}
	sess := &wsSession{}

	readCalled := 0
	server.runWSInputLoop(sess, func(_ *websocket.Conn, _ string, _ *Room, _ int64) (NetData, bool, error) {
		readCalled++
		return NetData{}, false, errors.New("read error")
	})

	if readCalled != 1 {
		t.Fatalf("expected readFn called exactly once, got %d", readCalled)
	}
	if sess.cleanExit.Load() {
		t.Fatal("expected cleanExit to remain false on non-clean error")
	}
}
