package world

import (
	"slices"
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestDroppedEquipmentKeepsCards(t *testing.T) {
	w, err := New(Config{Maps: map[string]*gat.Map{"town": openMap(t, 10, 10)}, SavePoint: Location{Map: "town", X: 1, Y: 1}})
	if err != nil {
		t.Fatal(err)
	}
	data := game.NewChar("town", 5, 5)
	data.Inventory = []game.InvItem{{Index: 0, ItemID: 1201, Amount: 1, Cards: []int{4002, 4004}}}
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Ann", data: data, out: discard{}}
	w.players[1] = p
	w.online["ann"] = 1
	w.place(p, w.maps["town"], 5, 5)

	dropped, err := p.data.Drop(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	it := w.dropItem(p.m, p.x, p.y, dropped.ItemID, 1, 0, time.Now())
	it.cards = dropped.Cards
	w.pickUp(p, it, time.Now())
	if len(p.data.Inventory) != 1 || !slices.Equal(p.data.Inventory[0].Cards, []int{4002, 4004}) {
		t.Fatalf("after picking it back up: %+v", p.data.Inventory)
	}
}
