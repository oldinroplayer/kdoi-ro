package server

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"kdoiro/server/internal/proto"
)

func TestUpdateHandsOutResumeToken(t *testing.T) {
	url, _, srv := startTestServer(t)
	c := dial(t, url)
	c.login("Alice", "pass1234")
	c.expectLogin(proto.LoginCreated)
	c.expectEnter()

	go srv.BeginUpdate(30)
	r := c.expect(proto.OpServerUpdating)
	wait, token := r.U16(), r.String()
	if wait != 30 || len(token) != 64 {
		t.Fatalf("update notice: wait %d token %q", wait, token)
	}
	// Then the connection closes with the "updating" code.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			if code := websocket.CloseStatus(err); code != proto.CloseUpdating {
				t.Fatalf("closed with %v (%d), want CloseUpdating", err, code)
			}
			break
		}
	}

	// Back with the token: no password needed, and only once.
	again := dial(t, url)
	again.send(proto.NewPacket(proto.OpResume).String(token).Bytes())
	again.expectLogin(proto.LoginOK)
	again.expectEnter()

	thief := dial(t, url)
	thief.send(proto.NewPacket(proto.OpResume).String(token).Bytes())
	thief.expectLogin(proto.LoginResumeFailed)
}
