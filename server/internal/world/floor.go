package world

import (
	"fmt"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

const (
	// floorItemLifetime is how long items stay on the ground (rAthena's
	// flooritem_lifetime).
	floorItemLifetime = 60 * time.Second
	// lootPriority is how long only the top damage dealer may pick up a
	// monster's drops (rAthena's item_first_get_time).
	lootPriority = 3 * time.Second
	// pickUpRange is how close (in cells) a player must stand to pick up.
	pickUpRange = 1
)

// floorItem is an item lying on the ground.
type floorItem struct {
	id         uint32
	m          *mapInstance
	x, y       int
	subX, subY uint8 // position within the cell, 0-15
	itemID     int
	amount     int
	owner      uint32 // player with first pick, until ownerUntil
	ownerUntil time.Time
	expires    time.Time
}

func (it *floorItem) packet(dropped bool) []byte {
	return proto.FloorItem(it.id, it.itemID, it.amount, it.x, it.y, it.subX, it.subY, dropped)
}

// dropItem puts items on the ground on a random walkable cell next to (x, y),
// like eAthena's map_searchrandfreecell around the corpse.
func (w *World) dropItem(m *mapInstance, x, y, itemID, amount int, owner uint32, now time.Time) {
	cx, cy := x, y
	for range 8 {
		tx, ty := x-1+w.rng.IntN(3), y-1+w.rng.IntN(3)
		if m.grid.Walkable(tx, ty) {
			cx, cy = tx, ty
			break
		}
	}
	w.nextID++
	it := &floorItem{
		id: w.nextID, m: m, x: cx, y: cy,
		subX: uint8(3 + w.rng.IntN(10)), subY: uint8(3 + w.rng.IntN(10)),
		itemID: itemID, amount: amount, expires: now.Add(floorItemLifetime),
	}
	if owner != 0 {
		it.owner, it.ownerUntil = owner, now.Add(lootPriority)
	}
	m.items[it.id] = it
	w.broadcast(m, it.packet(true))
}

func (w *World) removeFloorItem(it *floorItem) {
	delete(it.m.items, it.id)
	w.broadcast(it.m, proto.FloorItemGone(it.id))
}

// tickFloorItems clears items that have lain on the ground too long.
func (w *World) tickFloorItems(now time.Time) {
	if now.Before(w.nextFloorSweep) {
		return
	}
	w.nextFloorSweep = now.Add(time.Second)
	for _, m := range w.maps {
		for _, it := range m.items {
			if !now.Before(it.expires) {
				w.removeFloorItem(it)
			}
		}
	}
}

// PickUp walks the player to a floor item and picks it up.
func (w *World) PickUp(id, itemID uint32) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || p.dead || p.sitting || p.room != nil {
			return
		}
		it := p.m.items[itemID]
		if it == nil {
			return
		}
		p.target, p.pending = 0, nil
		w.cancelCast(p)
		p.pickup = it.id
		p.nextRepath = time.Time{}
		w.tickPickUp(p, time.Now())
	})
}

// tickPickUp moves a player toward the item it wants and picks it up in reach.
func (w *World) tickPickUp(p *Player, now time.Time) {
	it := p.m.items[p.pickup]
	if it == nil {
		p.pickup = 0
		return
	}
	if chebyshev(p.x, p.y, it.x, it.y) <= pickUpRange {
		p.pickup = 0
		w.halt(&p.unit)
		w.pickUp(p, it, now)
		return
	}
	if now.Before(p.nextRepath) {
		return
	}
	p.nextRepath = now.Add(400 * time.Millisecond)
	if p.walking() {
		return // already on the way
	}
	if route := p.routeTo(path.Point{X: it.x, Y: it.y}, pickUpRange, maxChaseSteps, now); route != nil {
		w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
	} else {
		p.pickup = 0
		p.out.Send(proto.Message("You can't reach that item."))
	}
}

// pickUp moves a floor item into the player's inventory if it may and can.
func (w *World) pickUp(p *Player, it *floorItem, now time.Time) {
	name := game.Items[it.itemID].Name
	if it.owner != 0 && it.owner != p.id && now.Before(it.ownerUntil) {
		p.out.Send(proto.Message("You can't pick that up yet: it belongs to whoever won the fight."))
		return
	}
	if err := p.data.PickUp(it.itemID, it.amount); err != nil {
		p.out.Send(proto.Message(fmt.Sprintf("You can't carry %s: %s.", name, messageFor(err))))
		return
	}
	w.broadcast(p.m, proto.PlayerPickUp(p.id))
	w.removeFloorItem(it)
	if it.amount > 1 {
		p.out.Send(proto.Message(fmt.Sprintf("You got %d %s.", it.amount, name)))
	} else {
		p.out.Send(proto.Message("You got " + name + "."))
	}
	w.sendInventory(p)
	w.sendStatus(p)
}

// DropItem puts inventory items on the ground at the player's feet; anyone
// may pick them up.
func (w *World) DropItem(id uint32, index, amount int) error {
	return w.update(id, func(p *Player) error {
		if p.room != nil {
			return errCantUseNow
		}
		itemID, err := p.data.Drop(index, amount)
		if err != nil {
			return err
		}
		w.dropItem(p.m, p.x, p.y, itemID, amount, 0, time.Now())
		return nil
	})
}

// sendFloorItems shows a player arriving on a map what lies on the ground.
func (w *World) sendFloorItems(p *Player) {
	for _, it := range p.m.items {
		p.out.Send(it.packet(false))
	}
}
