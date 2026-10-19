package server

import (
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

func readLook(r *proto.Reader) proto.Look {
	var l proto.Look
	for i := range l {
		l[i] = int(r.U16())
	}
	return l
}

func TestHairstyle(t *testing.T) {
	url, store := startServer(t)
	cs, ids := joinAll(t, url, "Stylist", "Onlooker")
	stylist, onlooker := cs[0], cs[1]

	// A new character hasn't chosen yet (the client asks on first login).
	ch, _, err := store.Authenticate("Stylist", "pass1234", game.CharData{})
	if err != nil || ch.Hair != 0 {
		t.Fatalf("new character hair = %d, err %v", ch.Hair, err)
	}

	stylist.send(proto.NewPacket(proto.OpSetAppearance).U8(7).U8(3).U8(1).Bytes())
	for _, c := range cs {
		r := c.expect(proto.OpPlayerLook)
		if id, l := r.U32(), readLook(r); id != ids[0] || l[5] != 7 || l[6] != 3 || l[7] != 1 {
			t.Fatalf("look after SetHair: %d %v", id, l)
		}
	}

	// Out of range: refused, nothing broadcast (the onlooker's next packet is
	// the valid change below).
	stylist.send(proto.NewPacket(proto.OpSetAppearance).U8(game.HairStyles + 1).U8(0).U8(1).Bytes())
	if msg := stylist.expect(proto.OpMessage).String(); msg != "That hairstyle isn't available." {
		t.Fatalf("bad hair: %q", msg)
	}
	stylist.send(proto.NewPacket(proto.OpSetAppearance).U8(1).U8(game.HairColors - 1).U8(0).Bytes())
	if r := onlooker.expect(proto.OpPlayerLook); r.U32() != ids[0] {
		t.Fatal("expected the second change")
	} else if l := readLook(r); l[6] != game.HairColors-1 || l[7] != 0 {
		t.Fatalf("second change: %v (back to male)", l)
	}
}
