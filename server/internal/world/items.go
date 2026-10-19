package world

import (
	"errors"
	"fmt"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// ClickNpc opens an NPC's shop if the player is close enough.
func (w *World) ClickNpc(id, npcID uint32) error {
	return w.do(func() {
		p, n := w.players[id], w.npcs[npcID]
		if p == nil || p.dead || p.room != nil {
			return
		}
		if n == nil || n.Map != p.m.name || chebyshev(p.x, p.y, n.X, n.Y) > NpcRange {
			p.out.Send(proto.Message("You are too far away."))
			return
		}
		if n.Vendor != nil {
			w.openVendor(p, n)
			return
		}
		switch n.Kind {
		case KindJobChange:
			w.jobGuideTalk(p, n)
			return
		case KindKafra:
			w.kafraTalk(p, n)
			return
		case KindWarper:
			w.warperTalk(p, n)
			return
		case KindReset:
			w.resetTalk(p, n)
			return
		case KindTraining:
			w.trainingTalk(p, n)
			return
		}
		if len(n.Shop) == 0 {
			return
		}
		items := make([]proto.ShopItem, len(n.Shop))
		for i, itemID := range n.Shop {
			items[i] = proto.ShopItem{ItemID: itemID, Price: n.priceFor(p, itemID)}
		}
		p.out.Send(proto.ShopOpen(n.ID, n.Name, items, p.data.SellRate(), false))
	})
}

// priceFor is what a player pays for one of a shop's items: the NPC's fixed
// price if it has one, else the database price (less the player's Discount).
func (n *NPC) priceFor(p *Player, itemID int) int {
	if n.Price > 0 {
		return n.Price
	}
	return p.data.BuyPrice(game.Items[itemID].Price)
}

// shopPlayer returns the player if it's alive and near a shop NPC.
func (w *World) shopPlayer(id, npcID uint32) (*Player, *NPC) {
	p, n := w.players[id], w.npcs[npcID]
	if p == nil {
		return nil, nil
	}
	if p.dead || n == nil || (len(n.Shop) == 0 && n.Vendor == nil) || n.Map != p.m.name || chebyshev(p.x, p.y, n.X, n.Y) > shopRange {
		p.out.Send(proto.ShopResult(proto.ShopInvalid))
		return nil, nil
	}
	return p, n
}

// Buy purchases an order from an NPC shop; it either fully succeeds or does nothing.
func (w *World) Buy(id, npcID uint32, lines []proto.ShopLine) error {
	return w.do(func() {
		p, n := w.shopPlayer(id, npcID)
		if p == nil {
			return
		}
		if len(lines) == 0 {
			p.out.Send(proto.ShopResult(proto.ShopInvalid))
			return
		}
		order := make([]game.Purchase, len(lines))
		for i, l := range lines {
			order[i] = game.Purchase{ItemID: int(l.ItemID), Amount: int(l.Amount)}
		}
		before := len(p.data.Inventory)
		err := p.data.Buy(order, func(itemID int) (int, bool) {
			if n.Vendor != nil {
				it, ok := n.Vendor.ware(itemID)
				return it.Price, ok
			}
			for _, sold := range n.Shop {
				if sold == itemID {
					return n.priceFor(p, itemID), true
				}
			}
			return 0, false
		})
		if err == nil && n.Vendor != nil {
			stampElements(&p.data, n.Vendor, before)
		}
		switch {
		case err == nil:
			p.out.Send(proto.ShopResult(proto.ShopOK))
			w.sendInventory(p)
			w.sendStatus(p)
		case errors.Is(err, game.ErrNotEnoughZeny):
			p.out.Send(proto.ShopResult(proto.ShopNoZeny))
		case errors.Is(err, game.ErrOverweight):
			p.out.Send(proto.ShopResult(proto.ShopOverweight))
		case errors.Is(err, game.ErrInventoryFull):
			p.out.Send(proto.ShopResult(proto.ShopFull))
		default:
			p.out.Send(proto.ShopResult(proto.ShopInvalid))
		}
	})
}

// Sell sells inventory items to any shop NPC at half their price (more with Overcharge).
func (w *World) Sell(id, npcID uint32, lines []proto.SellLine) error {
	return w.do(func() {
		p, n := w.shopPlayer(id, npcID)
		if p == nil {
			return
		}
		if n.Vendor != nil { // stalls only sell
			p.out.Send(proto.ShopResult(proto.ShopInvalid))
			return
		}
		order := make([]game.Sale, len(lines))
		for i, l := range lines {
			order[i] = game.Sale{Index: int(l.Index), Amount: int(l.Amount)}
		}
		earned, err := p.data.Sell(order)
		if err != nil || len(lines) == 0 {
			p.out.Send(proto.ShopResult(proto.ShopInvalid))
			return
		}
		p.out.Send(proto.ShopResult(proto.ShopSold))
		p.out.Send(proto.Message(fmt.Sprintf("Sold for %d zeny.", earned)))
		w.sendInventory(p)
		w.sendStatus(p)
	})
}

// Equip wears an inventory item; Unequip takes it off.
func (w *World) Equip(id uint32, index int) error {
	return w.update(id, func(p *Player) error { return w.changeLook(p, func() error { return p.data.Equip(index) }) })
}

func (w *World) Unequip(id uint32, index int) error {
	return w.update(id, func(p *Player) error { return w.changeLook(p, func() error { return p.data.Unequip(index) }) })
}

// changeLook applies an equipment change and shows the map if the visible
// equipment (weapon, shield, headgear) changed.
func (w *World) changeLook(p *Player, change func() error) error {
	before := p.look()
	if err := change(); err != nil {
		return err
	}
	if after := p.look(); after != before {
		w.broadcast(p.m, proto.PlayerLook(p.id, after))
	}
	return nil
}

// SetAppearance changes a player's hairstyle, colour and gender and shows everyone.
func (w *World) SetAppearance(id uint32, style, color int, female bool) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if err := w.changeLook(p, func() error { return p.data.SetAppearance(style, color, female) }); err != nil {
			p.out.Send(proto.Message("That hairstyle isn't available."))
		}
	})
}

// RaiseStat spends status points on one of STR..LUK (0-5).
func (w *World) RaiseStat(id uint32, stat int) error {
	return w.update(id, func(p *Player) error { return p.data.RaiseStat(stat) })
}

// update applies a change to a living player's character data, then
// refreshes the client's inventory and status, or tells it why the change
// was refused.
func (w *World) update(id uint32, change func(*Player) error) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if p.dead {
			p.out.Send(proto.Message("You can't do that while knocked out."))
			return
		}
		if err := change(p); err != nil {
			p.out.Send(proto.Message(messageFor(err)))
			return
		}
		d := p.data.Derive()
		p.data.HP, p.data.SP = min(p.data.HP, d.MaxHP), min(p.data.SP, d.MaxSP)
		w.sendInventory(p)
		w.sendStatus(p)
	})
}

func messageFor(err error) string {
	switch {
	case errors.Is(err, game.ErrNotACard):
		return "That's not a card."
	case errors.Is(err, game.ErrCardDoesntFit):
		return "That card doesn't go in that kind of equipment."
	case errors.Is(err, game.ErrNoFreeSlot):
		return "That item has no free card slot."
	case errors.Is(err, game.ErrLevelTooLow):
		return "Your level is too low to equip that."
	case errors.Is(err, game.ErrNotEquippable):
		return "That item can't be equipped."
	case errors.Is(err, game.ErrEquipped):
		return "Take it off first."
	case errors.Is(err, game.ErrOverweight):
		return "you're carrying too much"
	case errors.Is(err, game.ErrInventoryFull):
		return "your inventory is full"
	case errors.Is(err, game.ErrWrongJob):
		return "Your job can't use that."
	case errors.Is(err, game.ErrNotUsable):
		return "That item can't be used."
	case errors.Is(err, game.ErrNotEnoughPoint):
		return "Not enough status points."
	case errors.Is(err, game.ErrStatMaxed):
		return "That stat is already at its maximum."
	default:
		return "You can't do that."
	}
}

func (w *World) sendInventory(p *Player) {
	entries := make([]proto.InvEntry, len(p.data.Inventory))
	for i, it := range p.data.Inventory {
		entries[i] = proto.InvEntry{Index: it.Index, ItemID: it.ItemID, Amount: it.Amount, Equip: it.Equip, Cards: it.Cards, Element: it.Element}
	}
	p.out.Send(proto.Inventory(entries))
}

func (w *World) sendStatus(p *Player) {
	c := &p.data
	d := c.Derive()
	info := proto.StatusInfo{
		BaseLevel: c.BaseLevel, JobLevel: c.JobLevel,
		Stats:        c.Stats,
		StatusPoints: c.StatusPoints, Zeny: c.Zeny,
		HP: c.HP, MaxHP: d.MaxHP, SP: c.SP, MaxSP: d.MaxSP,
		Atk: d.Atk, Atk2: d.Atk2, MatkMin: d.MatkMin, MatkMax: d.MatkMax,
		Def: d.Def, Def2: d.Def2, Mdef: d.Mdef, Mdef2: d.Mdef2,
		Hit: d.Hit, Flee: d.Flee, Flee2: d.Flee2, Crit: d.Crit, Aspd: d.Aspd,
		Weight: d.Weight, MaxWeight: d.MaxWeight,
		BaseExp: c.BaseExp, BaseNext: c.BaseNext(),
		JobExp: c.JobExp, JobNext: c.JobNext(),
		Job: c.Job, Bonus: d.Bonus,
	}
	for i, v := range c.Stats {
		info.RaiseCosts[i] = game.RaiseCost(v)
	}
	p.out.Send(proto.Status(info))
}

// Compound puts a card into a slot of unequipped equipment, for good.
func (w *World) Compound(id uint32, cardIndex, equipIndex int) error {
	return w.update(id, func(p *Player) error {
		if err := p.data.Compound(cardIndex, equipIndex); err != nil {
			return err
		}
		p.out.Send(proto.Message("The card was compounded."))
		return nil
	})
}
