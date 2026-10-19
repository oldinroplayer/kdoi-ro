package world

import (
	"context"
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/proto"
)

func TestVendorStall(t *testing.T) {
	stall := NPC{ID: 950, Name: "Blacksmith", Map: "town", X: 5, Y: 6, Vendor: &Vendor{
		Title: "Fire weapons", Job: 10,
		Stock: []VendItem{{ItemID: 1207, Price: 5000, Element: game.ElemFire}, {ItemID: 501, Price: 40}},
	}}
	w, err := New(Config{Maps: map[string]*gat.Map{"town": openMap(t, 10, 10)}, SavePoint: Location{Map: "town", X: 1, Y: 1}, NPCs: []NPC{stall}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	data := game.NewChar("town", 5, 5)
	data.Zeny, data.Inventory = 6000, nil
	id, err := w.Join("Hero", data, discard{})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Buy(id, stall.ID, []proto.ShopLine{{ItemID: 1207, Amount: 1}, {ItemID: 501, Amount: 2}}); err != nil {
		t.Fatal(err)
	}
	var inv []game.InvItem
	var zeny int
	w.do(func() { inv, zeny = w.players[id].data.Inventory, w.players[id].data.Zeny })
	if zeny != 6000-5000-80 {
		t.Fatalf("zeny %d after buying: the stall's prices", zeny)
	}
	var gauche, potions game.InvItem
	for _, it := range inv {
		switch it.ItemID {
		case 1207:
			gauche = it
		case 501:
			potions = it
		}
	}
	if gauche.Element != game.ElemFire || potions.Amount != 2 || potions.Element != 0 {
		t.Fatalf("bought: %+v and %+v", gauche, potions)
	}
	// Stalls don't buy.
	if err := w.Sell(id, stall.ID, []proto.SellLine{{Index: uint16(potions.Index), Amount: 1}}); err != nil {
		t.Fatal(err)
	}
	w.do(func() { zeny = w.players[id].data.Zeny })
	if zeny != 6000-5000-80 {
		t.Fatalf("a stall bought something: zeny %d", zeny)
	}
}
