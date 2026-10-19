package world

import (
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestWings(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"town": openMap(t, 20, 20), "field": openMap(t, 30, 30)},
		SavePoint: Location{Map: "town", X: 3, Y: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	data := game.NewChar("field", 5, 5)
	if err := data.PickUp(601, 2); err != nil { // Fly Wings
		t.Fatal(err)
	}
	if err := data.PickUp(602, 1); err != nil { // Butterfly Wing
		t.Fatal(err)
	}
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Ann", data: data, out: discard{}}
	w.players[1] = p
	w.online["ann"] = 1
	w.place(p, w.maps["field"], 5, 5)
	fly, butterfly := p.data.Inventory[0].Index, p.data.Inventory[1].Index

	moved := false
	for range 2 {
		w.useWing(p, fly)
		if p.m.name != "field" {
			t.Fatalf("Fly Wing left the map: %s", p.m.name)
		}
		moved = moved || p.x != 5 || p.y != 5
	}
	if !moved {
		t.Fatal("Fly Wing never moved the player")
	}
	if _, ok := itemAt(&p.data, fly); ok {
		t.Fatal("both Fly Wings should be used up")
	}
	w.useWing(p, butterfly)
	if p.m.name != "town" || p.x != 3 || p.y != 4 {
		t.Fatalf("Butterfly Wing: at %s (%d, %d), want the save point town (3, 4)", p.m.name, p.x, p.y)
	}
	if len(p.data.Inventory) != 0 {
		t.Fatalf("all wings spent: %+v", p.data.Inventory)
	}
}
