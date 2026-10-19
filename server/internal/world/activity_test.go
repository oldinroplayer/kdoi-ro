package world

import (
	"encoding/binary"
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/path"
)

// discard is an Outbox for players in tests that don't read their packets.
type discard struct{}

func (discard) Send([]byte) {}

const wandererMob = 9100

func init() {
	game.Mobs[wandererMob] = &game.MobDef{
		ID: wandererMob, Name: "Wanderer", Sprite: "poring", Level: 1, HP: 50,
		Range: 1, ChaseRange: 12, WalkSpeed: 100, AttackDelay: 1000, AttackMotion: 500, DamageMotion: 300,
		Mobile: true,
	}
}

func openMap(t *testing.T, w, h int) *gat.Map {
	t.Helper()
	data := make([]byte, 14+w*h*20)
	copy(data, "GRAT")
	binary.LittleEndian.PutUint32(data[6:], uint32(w))
	binary.LittleEndian.PutUint32(data[10:], uint32(h))
	m, err := gat.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func positions(m *mapInstance) map[uint32]path.Point {
	out := map[uint32]path.Point{}
	for id, mob := range m.monsters {
		out[id] = mob.pos()
	}
	return out
}

// run ticks the world every 50 ms of simulated time from *now for d.
func run(w *World, now *time.Time, d time.Duration) {
	for end := now.Add(d); now.Before(end); *now = now.Add(TickInterval) {
		w.tick(*now)
	}
}

func moved(before, after map[uint32]path.Point) bool {
	for id, p := range before {
		if after[id] != p {
			return true
		}
	}
	return false
}

func TestMapsSleepWithoutPlayers(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"field": openMap(t, 40, 40)},
		Spawns:    []Spawn{{Map: "field", MobID: wandererMob, Count: 5}},
		SavePoint: Location{Map: "field", X: 20, Y: 20},
		Seed:      7,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := w.maps["field"]
	now := time.Now()

	// Nobody here: the monsters stay put.
	before := positions(m)
	run(w, &now, 30*time.Second)
	if !m.asleep || moved(before, positions(m)) {
		t.Fatalf("an empty map should sleep (asleep=%v)", m.asleep)
	}

	// A player arrives: they wander again.
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Visitor", data: game.NewChar("field", 20, 20), out: discard{}}
	w.players[p.id] = p
	w.place(p, m, 20, 20)
	before = positions(m)
	run(w, &now, 30*time.Second)
	if m.asleep || !moved(before, positions(m)) {
		t.Fatalf("a visited map should wake (asleep=%v)", m.asleep)
	}

	// The player leaves: the map stays awake through the grace period...
	w.removeFromMap(p)
	delete(w.players, p.id)
	now = time.Now()
	run(w, &now, mapGrace-5*time.Second)
	if m.asleep {
		t.Fatal("the map fell asleep during its grace period")
	}
	// ...then sleeps, with nobody left mid-walk.
	run(w, &now, 10*time.Second)
	if !m.asleep {
		t.Fatal("the map should sleep once the grace period is over")
	}
	for _, mob := range m.monsters {
		if mob.walking() || mob.target != 0 {
			t.Fatalf("monster %d still busy while asleep", mob.id)
		}
	}
	before = positions(m)
	run(w, &now, 30*time.Second)
	if moved(before, positions(m)) {
		t.Fatal("monsters moved on a sleeping map")
	}
}
