package net

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRunWSInputLoopReturnsOnClientExitedSignal(t *testing.T) {
	t.Parallel()

	server := &Server{}
	cleanExit := true
	sess := &wsSession{
		cleanExit:           &cleanExit,
		returnFromInputLoop: make(chan bool, 1),
	}

	readCalled := make(chan struct{}, 1)
	sess.returnFromInputLoop <- true

	server.runWSInputLoop(sess, func(_ *websocket.Conn, _ string, _ *Room, _ int64) (NetData, bool, error) {
		readCalled <- struct{}{}
		return NetData{}, false, errors.New("read should not be called after ClientExited")
	})

	if !cleanExit {
		t.Fatal("expected cleanExit to be true after ClientExited")
	}

	select {
	case <-readCalled:
		t.Fatal("readFn was called after ClientExited signaled an immediate return")
	default:
	}
}

func TestRunWSInputLoopContinuesAfterNonExitSignal(t *testing.T) {
	t.Parallel()

	server := &Server{}
	cleanExit := false
	sess := &wsSession{
		cleanExit:           &cleanExit,
		returnFromInputLoop: make(chan bool, 1),
	}

	readCalled := make(chan struct{}, 1)
	sess.returnFromInputLoop <- false

	server.runWSInputLoop(sess, func(_ *websocket.Conn, _ string, _ *Room, _ int64) (NetData, bool, error) {
		readCalled <- struct{}{}
		return NetData{}, false, errors.New("stop loop after verifying non-exit signal")
	})

	select {
	case <-readCalled:
	default:
		t.Fatal("expected readFn to be called after a non-exit signal")
	}

	if cleanExit {
		t.Fatal("expected cleanExit to remain false after non-exit signal")
	}
}
