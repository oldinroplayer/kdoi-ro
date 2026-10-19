package server

import (
	"testing"
	"time"

	"kdoiro/server/internal/proto"
)

// readParty reads a PartyInfo: its name, leader, even-share flag and members.
func readParty(r *proto.Reader) (name, leader string, even bool, members []string) {
	r.U32()
	name, leader, even = r.String(), r.String(), r.U8() == 1
	for range int(r.U8()) {
		members = append(members, r.String())
		r.U8()
		_ = r.String()
		r.U16()
		r.U16()
		r.U32()
		r.U32()
		r.U16()
		r.U16()
	}
	return
}

// expectParty waits for a PartyInfo (skipping the once-a-second refreshes'
// neighbours, i.e. other packets) and returns its member names.
func (c *client) expectParty() (leader string, even bool, members []string) {
	c.t.Helper()
	c.readUntil("party info", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpPartyInfo {
			return false
		}
		_, leader, even, members = readParty(r)
		return true
	})
	return
}

func (c *client) expectMessage(want string) {
	c.t.Helper()
	c.readUntil(want, func(op uint16, r *proto.Reader) bool {
		if op != proto.OpMessage {
			return false
		}
		if got := r.String(); got != want {
			c.t.Fatalf("message %q, want %q", got, want)
		}
		return true
	})
}

func TestPartyLifecycle(t *testing.T) {
	url, _ := startServer(t)
	cs, ids := joinAll(t, url, "Leader", "Member", "Outsider")
	leader, member, outsider := cs[0], cs[1], cs[2]

	leader.send(proto.NewPacket(proto.OpPartyCreate).String("Heroes").Bytes())
	leader.expectMessage("You founded the party Heroes.")
	if l, _, ms := leader.expectParty(); l != "Leader" || len(ms) != 1 {
		t.Fatalf("new party: leader %q members %v", l, ms)
	}

	// Only the leader invites; the invitee accepts.
	leader.send(proto.NewPacket(proto.OpPartyInvite).String("member").Bytes())
	leader.expectMessage("You invited Member to the party.")
	var partyID uint32
	member.readUntil("the invitation", func(op uint16, r *proto.Reader) bool {
		if op == proto.OpPartyInvited {
			partyID = r.U32()
			if name, from := r.String(), r.String(); name != "Heroes" || from != "Leader" {
				t.Fatalf("invite %q from %q", name, from)
			}
			return true
		}
		return false
	})
	member.send(proto.NewPacket(proto.OpPartyReply).U32(partyID).U8(1).Bytes())
	for _, c := range []*client{leader, member} {
		c.expectMessage("Member joined the party.")
		if _, _, ms := c.expectParty(); len(ms) != 2 || ms[1] != "Member" {
			t.Fatalf("after joining: %v", ms)
		}
	}

	// Party chat reaches members only (the outsider would fail on an unexpected packet).
	member.send(proto.NewPacket(proto.OpPartyChat).String("hi team").Bytes())
	for _, c := range []*client{leader, member} {
		c.readUntil("party chat", func(op uint16, r *proto.Reader) bool {
			if op != proto.OpChatMessage {
				return false
			}
			if id, kind, name, text := r.U32(), r.U8(), r.String(), r.String(); id != ids[1] || kind != proto.ChatParty || name != "Member" || text != "hi team" {
				t.Fatalf("party chat: %d %d %q %q", id, kind, name, text)
			}
			return true
		})
	}

	// Members can't invite; even share turns on (both level 1).
	member.send(proto.NewPacket(proto.OpPartyInvite).String("Outsider").Bytes())
	member.expectMessage("Only the party leader can invite.")
	leader.send(proto.NewPacket(proto.OpPartyEXP).U8(1).Bytes())
	leader.expectMessage("EXP is now shared evenly.")
	member.expectMessage("EXP is now shared evenly.")
	if _, even, _ := leader.expectParty(); !even {
		t.Fatal("even share should be on")
	}

	// The leader leaves: the member takes over.
	leader.send(proto.NewPacket(proto.OpPartyLeave).Bytes())
	leader.readUntil("party left", func(op uint16, _ *proto.Reader) bool { return op == proto.OpPartyLeft })
	member.expectMessage("Leader left the party.")
	member.expectMessage("Member now leads the party.")
	if l, _, ms := member.expectParty(); l != "Member" || len(ms) != 1 {
		t.Fatalf("after the leader left: %q %v", l, ms)
	}

	// Emoticons reach players nearby, once a second at most.
	member.send(proto.NewPacket(proto.OpEmote).U8(3).Bytes())
	member.send(proto.NewPacket(proto.OpEmote).U8(4).Bytes()) // too soon: dropped
	outsider.readUntil("the emoticon", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpEmotion {
			return false
		}
		if id, kind := r.U32(), r.U8(); id != ids[1] || kind != 3 {
			t.Fatalf("emotion %d from %d", kind, id)
		}
		return true
	})
	time.Sleep(1100 * time.Millisecond)
	member.send(proto.NewPacket(proto.OpEmote).U8(proto.Emotions).Bytes()) // out of range: ignored
	member.send(proto.NewPacket(proto.OpEmote).U8(5).Bytes())
	outsider.readUntil("the second emoticon", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpEmotion {
			return false
		}
		if r.U32(); r.U8() != 5 {
			t.Fatal("expected emoticon 5 (the dropped one and the bad one shouldn't arrive)")
		}
		return true
	})
}
