package server

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kdoiro/server/internal/proto"
)

func TestLoginReplacesOnlineSession(t *testing.T) {
	url, _ := startServer(t)
	first := dial(t, url)
	first.login("Alice", "pass1234")
	first.expectLogin(proto.LoginCreated)
	first.expectEnter()
	// Keep reading, as a browser does, so the server's close handshake completes.
	closed := make(chan websocket.StatusCode, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			if _, _, err := first.conn.Read(ctx); err != nil {
				closed <- websocket.CloseStatus(err)
				return
			}
		}
	}()

	// A wrong password doesn't kick anyone.
	stranger := dial(t, url)
	stranger.login("Alice", "wrongpass")
	stranger.expectLogin(proto.LoginWrongPassword)

	// The right one takes over: the new login gets in, the old one is closed.
	second := dial(t, url)
	second.login("alice", "pass1234")
	second.expectLogin(proto.LoginOK)
	second.expectEnter()

	if code := <-closed; code != proto.CloseReplaced {
		t.Fatalf("old session closed with %d, want CloseReplaced", code)
	}
}
