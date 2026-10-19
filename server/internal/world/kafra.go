package world

import (
	"fmt"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// KindKafra NPCs offer RO's Kafra services: saving your respawn point and
// teleporting to other cities for zeny (2004 npc/kafras/).
const KindKafra = "kafra"

// KindWarper NPCs stand at each town's centre and send players to any other
// town's Warper (NPC.Warps; free unless a destination has a price).
const KindWarper = "warper"

// Kafra is what one Kafra Employee offers.
type Kafra struct {
	Save      Location // respawn point she saves
	Teleports []Teleport
}

// Teleport is a Kafra destination and its price.
type Teleport struct {
	Name  string
	Map   string
	X, Y  int
	Price int
}

// Kafra dialog menus (Player.dialogMenu).
const (
	kafraMain = iota
	kafraTeleportMenu
)

// teleportsFrom lists a Kafra's destinations on loaded maps. The teleport
// service is free in KDoiRo: the 2004 prices stay in the data but are waived.
func (w *World) teleportsFrom(n *NPC) []Teleport {
	var out []Teleport
	for _, t := range n.Kafra.Teleports {
		if _, ok := w.maps[t.Map]; ok {
			t.Price = 0
			out = append(out, t)
		}
	}
	return out
}

// kafraTalk opens the Kafra's main menu.
func (w *World) kafraTalk(p *Player, n *NPC) {
	p.dialog, p.dialogMenu = n.ID, kafraMain
	options := []string{"Save"}
	if len(w.teleportsFrom(n)) > 0 {
		options = append(options, "Use Teleport Service")
	}
	options = append(options, "Cancel")
	p.out.Send(proto.NpcDialog(n.ID, n.Name, "Welcome to the Kafra Corporation.\nThe Kafra services are always on your side.\nHow can I help you today?", options))
}

func (w *World) kafraReply(p *Player, n *NPC, choice int) {
	teleports := w.teleportsFrom(n)
	if p.dialogMenu == kafraTeleportMenu {
		if choice >= len(teleports) {
			return // Cancel
		}
		w.teleport(p, n, teleports[choice])
		return
	}
	switch {
	case choice == 0: // Save
		p.data.Save = game.SavePoint{Map: n.Kafra.Save.Map, X: n.Kafra.Save.X, Y: n.Kafra.Save.Y}
		p.out.Send(proto.NpcDialog(n.ID, n.Name, "Your respawn point has been saved here.\nThank you for using the Kafra Service.", nil))
	case choice == 1 && len(teleports) > 0:
		options := make([]string, 0, len(teleports)+1)
		for _, t := range teleports {
			if t.Price > 0 {
				options = append(options, fmt.Sprintf("%s -> %d z", t.Name, t.Price))
			} else {
				options = append(options, t.Name)
			}
		}
		options = append(options, "Cancel")
		p.dialog, p.dialogMenu = n.ID, kafraTeleportMenu
		p.out.Send(proto.NpcDialog(n.ID, n.Name, "Please choose your destination.", options))
	}
}

// teleport sends a player to a destination, charging its price.
func (w *World) teleport(p *Player, n *NPC, t Teleport) {
	if p.data.Zeny < t.Price {
		p.out.Send(proto.NpcDialog(n.ID, n.Name, "I'm sorry, you don't have enough zeny for that teleport.", nil))
		return
	}
	p.data.Zeny -= t.Price
	w.cancelCast(p)
	w.removeFromMap(p)
	w.place(p, w.maps[t.Map], t.X, t.Y)
	w.sendStatus(p)
}

// loadedWarps are a Warper's destinations on loaded maps.
func (w *World) loadedWarps(n *NPC) []Teleport {
	var out []Teleport
	for _, t := range n.Warps {
		if _, ok := w.maps[t.Map]; ok {
			out = append(out, t)
		}
	}
	return out
}

// warperTalk lists the towns a Warper can send you to.
func (w *World) warperTalk(p *Player, n *NPC) {
	dests := w.loadedWarps(n)
	options := make([]string, 0, len(dests)+1)
	for _, t := range dests {
		if t.Price > 0 {
			options = append(options, fmt.Sprintf("%s -> %d z", t.Name, t.Price))
		} else {
			options = append(options, t.Name)
		}
	}
	options = append(options, "Cancel")
	p.dialog = n.ID
	p.out.Send(proto.NpcDialog(n.ID, n.Name, "Hello, traveller!\nI can send you to the centre of any town.\nWhere would you like to go?", options))
}

func (w *World) warperReply(p *Player, n *NPC, choice int) {
	if dests := w.loadedWarps(n); choice < len(dests) {
		w.teleport(p, n, dests[choice])
	}
}

// savePointOf is where a player respawns: the Kafra save point if it has one
// on a loaded map, otherwise the world's default.
func (w *World) savePointOf(c *game.CharData) Location {
	if _, ok := w.maps[c.Save.Map]; ok && c.Save.Map != "" {
		return Location{Map: c.Save.Map, X: c.Save.X, Y: c.Save.Y}
	}
	return w.savePoint
}

// itemAt is the definition of the inventory item at index.
func itemAt(c *game.CharData, index int) (game.ItemDef, bool) {
	for _, it := range c.Inventory {
		if it.Index == index {
			return game.Items[it.ItemID], true
		}
	}
	return game.ItemDef{}, false
}

// useWing spends a Fly Wing (a random spot on this map) or Butterfly Wing
// (the save point) and teleports the player there.
func (w *World) useWing(p *Player, index int) {
	if p.room != nil {
		p.out.Send(proto.Message("Leave the chat room first."))
		return
	}
	// Where to first, so a wing that can't go anywhere isn't spent.
	def, _ := itemAt(&p.data, index)
	to := w.savePointOf(&p.data)
	if def.Warp == game.WarpRandom {
		x, y, ok := w.randomCell(p.m)
		if !ok {
			p.out.Send(proto.Message("Nowhere to fly to."))
			return
		}
		to = Location{Map: p.m.name, X: x, Y: y}
	}
	if _, err := p.data.UseWarpItem(index); err != nil {
		p.out.Send(proto.Message(messageFor(err)))
		return
	}
	w.broadcast(p.m, proto.Effect(p.id, proto.EffectTeleport))
	p.target, p.pending, p.pickup = 0, nil, 0
	w.cancelCast(p)
	w.removeFromMap(p)
	w.place(p, w.maps[to.Map], to.X, to.Y)
	w.sendInventory(p)
	w.sendStatus(p)
}

// randomCell picks a random walkable cell on a map that isn't on a warp
// portal, like RO's random teleport.
func (w *World) randomCell(m *mapInstance) (x, y int, ok bool) {
	for range 1000 {
		x, y = w.rng.IntN(m.grid.Width), w.rng.IntN(m.grid.Height)
		if !m.grid.Walkable(x, y) {
			continue
		}
		if _, onWarp := m.warpAt(x, y); onWarp {
			continue
		}
		return x, y, true
	}
	return 0, 0, false
}
