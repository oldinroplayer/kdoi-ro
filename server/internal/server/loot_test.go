package server

import (
	"testing"

	"kdoiro/server/internal/proto"
)

func TestGroundLoot(t *testing.T) {
	url, _ := startServer(t)
	winner := dial(t, url)
	dummy := winner.loginAndFindMonsters("Winner")[dummyMob][0] // dies to one hit, always drops a Red Potion
	snatcher := dial(t, url)
	snatcher.login("Snatcher", "pass1234")
	snatcher.expectLogin(proto.LoginCreated)
	snatcherID, _, _ := snatcher.expectEnter()
	snatcher.expect(proto.OpPlayerAppear)
	winner.expect(proto.OpPlayerAppear)

	winner.send(proto.NewPacket(proto.OpAttack).U32(dummy).Bytes())

	// Both see the potion land; only the winner may take it at first.
	var potion uint32
	snatcher.readUntil("the drop", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpFloorItem {
			return false
		}
		potion = r.U32()
		r.U16()
		r.U16()
		r.U16()
		r.U16()
		r.U8()
		r.U8()
		if r.U8() != 1 {
			t.Fatal("a fresh drop should be marked as just dropped")
		}
		return true
	})
	snatcher.send(proto.NewPacket(proto.OpPickUp).U32(potion).Bytes())
	snatcher.readUntil("the refusal", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpMessage {
			return false
		}
		if msg := r.String(); msg != "You can't pick that up yet: it belongs to whoever won the fight." {
			t.Fatalf("snatcher was told %q", msg)
		}
		return true
	})

	winner.send(proto.NewPacket(proto.OpPickUp).U32(potion).Bytes())
	winner.readUntil("the pick-up", func(op uint16, r *proto.Reader) bool {
		return op == proto.OpFloorItemGone && r.U32() == potion
	})
	inv := winner.expectInventoryAfter()
	potionEntry, ok := inv[501]
	if !ok || potionEntry.amount != 1 {
		t.Fatalf("winner's inventory: %+v", inv)
	}

	// Dropped items belong to nobody: the snatcher can have this one.
	winner.send(proto.NewPacket(proto.OpDropItem).U16(uint16(potionEntry.index)).U16(1).Bytes())
	var dropped uint32
	snatcher.readUntil("the dropped potion", func(op uint16, r *proto.Reader) bool {
		if op == proto.OpFloorItem {
			dropped = r.U32()
			return true
		}
		return false
	})
	snatcher.send(proto.NewPacket(proto.OpPickUp).U32(dropped).Bytes())
	snatcher.readUntil("the snatcher's pick-up", func(op uint16, r *proto.Reader) bool {
		return op == proto.OpPlayerPickUp && r.U32() == snatcherID
	})
	if inv := snatcher.expectInventoryAfter(); inv[501].amount != 1 {
		t.Fatalf("snatcher's inventory: %+v", inv)
	}
}

// expectInventoryAfter reads until the next Inventory packet.
func (c *client) expectInventoryAfter() map[int]invEntry {
	c.t.Helper()
	byItem := map[int]invEntry{}
	c.readUntil("inventory", func(op uint16, r *proto.Reader) bool {
		if op != proto.OpInventory {
			return false
		}
		for range int(r.U16()) {
			e := readInvEntry(r)
			byItem[e.itemID] = e
		}
		return true
	})
	return byItem
}
