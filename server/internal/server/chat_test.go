package server

import (
	"testing"
	"time"

	"kdoiro/server/internal/proto"
)

// joinAll logs characters in one after another, reading the PlayerAppear each
// earlier one sees for every later arrival. It returns their unit IDs.
func joinAll(t *testing.T, url string, names ...string) ([]*client, []uint32) {
	t.Helper()
	var clients []*client
	var ids []uint32
	for _, name := range names {
		c := dial(t, url)
		c.login(name, "pass1234")
		c.expectLogin(proto.LoginCreated)
		id, _, _ := c.expectEnter()
		for range clients {
			c.expect(proto.OpPlayerAppear) // everyone already here
		}
		for _, earlier := range clients {
			earlier.expect(proto.OpPlayerAppear) // the newcomer
		}
		clients = append(clients, c)
		ids = append(ids, id)
	}
	return clients, ids
}

func chatLine(text string) []byte { return proto.NewPacket(proto.OpChat).String(text).Bytes() }

func (c *client) expectChat(wantID uint32, wantKind uint8, wantName, wantText string) {
	c.t.Helper()
	r := c.expect(proto.OpChatMessage)
	id, kind, name, text := r.U32(), r.U8(), r.String(), r.String()
	if id != wantID || kind != wantKind || name != wantName || text != wantText {
		c.t.Fatalf("chat = %d %d %q %q, want %d %d %q %q", id, kind, name, text, wantID, wantKind, wantName, wantText)
	}
}

func TestPublicChatAndWhispers(t *testing.T) {
	url, _ := startServer(t)
	cs, ids := joinAll(t, url, "Alice", "Bobby")
	alice, bob := cs[0], cs[1]

	alice.send(chatLine("  hello\tthere  "))
	for _, c := range cs {
		c.expectChat(ids[0], proto.ChatPublic, "Alice", "hello there")
	}

	alice.send(proto.NewPacket(proto.OpWhisper).String("bobby").String("psst").Bytes())
	if r := bob.expect(proto.OpWhisperFrom); r.String() != "Alice" || r.String() != "psst" {
		t.Fatal("bob should get alice's whisper")
	}
	if r := alice.expect(proto.OpWhisperResult); r.U8() != proto.WhisperSent || r.String() != "Bobby" {
		t.Fatal("alice should hear the whisper was delivered")
	}

	alice.send(proto.NewPacket(proto.OpWhisper).String("Nobody").String("hi?").Bytes())
	if r := alice.expect(proto.OpWhisperResult); r.U8() != proto.WhisperOffline {
		t.Fatal("whispering someone offline should fail")
	}
}

func TestChatFloodGuard(t *testing.T) {
	url, _ := startServer(t)
	cs, ids := joinAll(t, url, "Chatty")
	c := cs[0]
	for range chatBurst {
		c.send(chatLine("spam"))
		c.expectChat(ids[0], proto.ChatPublic, "Chatty", "spam")
	}
	c.send(chatLine("one too many"))
	if msg := c.expect(proto.OpMessage).String(); msg != "You're talking too fast. Wait a moment." {
		t.Fatalf("flood notice: %q", msg)
	}
}

type roomSign struct {
	room, owner    uint32
	title          string
	members, limit int
	public         bool
}

func (c *client) expectSign() roomSign {
	c.t.Helper()
	r := c.expect(proto.OpRoomSign)
	return roomSign{r.U32(), r.U32(), r.String(), int(r.U8()), int(r.U8()), r.U8() == 1}
}

func (c *client) expectRoomResult(want uint8) {
	c.t.Helper()
	if got := c.expect(proto.OpRoomResult).U8(); got != want {
		c.t.Fatalf("room result = %d, want %d", got, want)
	}
}

func TestChatRooms(t *testing.T) {
	url, _ := startServer(t)
	cs, ids := joinAll(t, url, "Alice", "Bobby", "Carol")
	alice, bob, carol := cs[0], cs[1], cs[2]

	// Alice opens a private room for two.
	alice.send(proto.NewPacket(proto.OpRoomCreate).String("Selling eggs").U8(2).U8(0).String("1234").Bytes())
	r := alice.expect(proto.OpRoomEnter)
	roomID := r.U32()
	if title := r.String(); title != "Selling eggs" {
		t.Fatalf("room title %q", title)
	}
	for _, c := range cs {
		if s := c.expectSign(); s.room != roomID || s.owner != ids[0] || s.members != 1 || s.limit != 2 || s.public {
			t.Fatalf("sign: %+v", s)
		}
	}

	// Wrong password, then the right one.
	bob.send(proto.NewPacket(proto.OpRoomJoin).U32(roomID).String("0000").Bytes())
	bob.expectRoomResult(proto.RoomWrongPassword)
	bob.send(proto.NewPacket(proto.OpRoomJoin).U32(roomID).String("1234").Bytes())
	if r := alice.expect(proto.OpRoomMember); r.U8() != proto.MemberJoined || r.String() != "Bobby" {
		t.Fatal("alice should see bob join")
	}
	r = bob.expect(proto.OpRoomEnter)
	r.U32()
	_ = r.String()
	r.U8()
	r.U8()
	if owner := r.String(); owner != "Alice" || r.U8() != 2 {
		t.Fatal("bob's room view should list alice as owner and two members")
	}
	for _, c := range cs {
		if s := c.expectSign(); s.members != 2 {
			t.Fatalf("sign after join: %+v", s)
		}
	}

	// The room is full now.
	carol.send(proto.NewPacket(proto.OpRoomJoin).U32(roomID).String("1234").Bytes())
	carol.expectRoomResult(proto.RoomFull)

	// Room chat reaches members only (carol's next packet is the sign update below).
	bob.send(chatLine("hi alice"))
	alice.expectChat(ids[1], proto.ChatRoom, "Bobby", "hi alice")
	bob.expectChat(ids[1], proto.ChatRoom, "Bobby", "hi alice")

	// Members can't walk off: alice's move is ignored (the next thing bob
	// sees is the kick, not a PlayerMove).
	alice.send(proto.NewPacket(proto.OpMove).U16(12).U16(10).Bytes())
	time.Sleep(100 * time.Millisecond)

	// Kicked members can't come back.
	alice.send(proto.NewPacket(proto.OpRoomKick).String("bobby").Bytes())
	if bob.expect(proto.OpRoomLeft).U8() != proto.LeftKicked {
		t.Fatal("bob should be told he was kicked")
	}
	if r := alice.expect(proto.OpRoomMember); r.U8() != proto.MemberKicked || r.String() != "Bobby" {
		t.Fatal("alice should see bob kicked")
	}
	for _, c := range cs {
		if s := c.expectSign(); s.members != 1 {
			t.Fatalf("sign after kick: %+v", s)
		}
	}
	bob.send(proto.NewPacket(proto.OpRoomJoin).U32(roomID).String("1234").Bytes())
	bob.expectRoomResult(proto.RoomBanned)

	// Carol joins; when alice leaves, carol owns the room.
	carol.send(proto.NewPacket(proto.OpRoomJoin).U32(roomID).String("1234").Bytes())
	alice.expect(proto.OpRoomMember)
	carol.expect(proto.OpRoomEnter)
	for _, c := range cs {
		c.expectSign()
	}
	alice.send(proto.NewPacket(proto.OpRoomLeave).Bytes())
	alice.expect(proto.OpRoomLeft)
	if r := carol.expect(proto.OpRoomMember); r.U8() != proto.MemberLeft || r.String() != "Alice" {
		t.Fatal("carol should see alice leave")
	}
	r = carol.expect(proto.OpRoomSettings)
	_ = r.String()
	r.U8()
	r.U8()
	if owner := r.String(); owner != "Carol" {
		t.Fatalf("new owner %q", owner)
	}
	for _, c := range cs {
		if s := c.expectSign(); s.owner != ids[2] || s.members != 1 {
			t.Fatalf("sign after the owner left: %+v", s)
		}
	}

	// The last one out closes it.
	carol.send(proto.NewPacket(proto.OpRoomLeave).Bytes())
	carol.expect(proto.OpRoomLeft)
	for _, c := range cs {
		if c.expect(proto.OpRoomGone).U32() != roomID {
			t.Fatal("expected RoomGone")
		}
	}
}
