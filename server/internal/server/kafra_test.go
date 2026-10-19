package server

import (
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// expectDialog reads an NpcDialog and returns its options.
func (c *client) expectDialog() []string {
	c.t.Helper()
	r := c.expect(proto.OpNpcDialog)
	r.U32()
	_ = r.String()
	n := skipDialogText(r)
	options := make([]string, n)
	for i := range options {
		options[i] = r.String()
	}
	return options
}

func TestKafraSaveAndTeleport(t *testing.T) {
	url, store := startServer(t)
	c := dial(t, url)
	c.login("Traveller", "pass1234")
	c.expectLogin(proto.LoginCreated)
	c.expectEnter()
	click := proto.NewPacket(proto.OpNpcClick).U32(kafraNpc).Bytes()
	reply := func(choice uint8) []byte { return proto.NewPacket(proto.OpNpcReply).U32(kafraNpc).U8(choice).Bytes() }

	c.send(click)
	if opts := c.expectDialog(); len(opts) != 3 || opts[0] != "Save" || opts[1] != "Use Teleport Service" {
		t.Fatalf("kafra menu: %q", opts)
	}
	c.send(reply(0))
	if opts := c.expectDialog(); len(opts) != 0 {
		t.Fatalf("save confirmation had options %q", opts)
	}

	c.send(click)
	c.expectDialog()
	c.send(reply(1))
	// The teleport service is free (the 2004 price of 500 z is waived).
	if opts := c.expectDialog(); len(opts) != 2 || opts[0] != "Field" || opts[1] != "Cancel" {
		t.Fatalf("teleport menu: %q", opts)
	}
	c.send(reply(0))
	c.expectEnterMap("field")
	if s := c.expectStatus(); s.zeny != game.StartZeny {
		t.Fatalf("zeny after teleport = %d", s.zeny)
	}
	c.conn.CloseNow()

	// The save point is kept with the character once the session has saved it.
	deadline := time.Now().Add(2 * time.Second)
	for {
		ch, _, err := store.Authenticate("Traveller", "pass1234", game.CharData{})
		if err != nil {
			t.Fatal(err)
		}
		if ch.Map == "field" {
			if ch.Save != (game.SavePoint{Map: "morocc", X: 9, Y: 9}) {
				t.Fatalf("save point %+v", ch.Save)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("character not saved on the field: %+v", ch.CharData)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
