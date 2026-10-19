package world

import (
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

const (
	aggressiveMob = 9101
	sensorMob     = 9102
	passiveMob    = 9103
)

func init() {
	base := game.MobDef{
		Sprite: "zombie", Level: 1, HP: 1000, Atk1: 1, Atk2: 1, Range: 1, ChaseRange: 20,
		WalkSpeed: 100, AttackDelay: 500, AttackMotion: 100, DamageMotion: 100, Mobile: true, Fights: true,
	}
	add := func(id int, name string, t game.MobTraits) {
		d := base
		d.ID, d.Name, d.MobTraits = id, name, t
		game.Mobs[id] = &d
	}
	add(aggressiveMob, "Biter", game.MobTraits{ElementLevel: 1, ViewRange: 6, Aggressive: true})
	add(sensorMob, "Listener", game.MobTraits{ElementLevel: 1, ViewRange: 6, CastSensor: true})
	add(passiveMob, "Grazer", game.MobTraits{ElementLevel: 1, ViewRange: 6})
}

func TestAggressiveMonsters(t *testing.T) {
	w, err := New(Config{
		Maps: map[string]*gat.Map{"field": openMap(t, 40, 40)},
		Spawns: []Spawn{
			{Map: "field", X: 10, Y: 10, MobID: aggressiveMob, Count: 1},
			{Map: "field", X: 10, Y: 30, MobID: sensorMob, Count: 1},
			{Map: "field", X: 30, Y: 30, MobID: passiveMob, Count: 1},
		},
		SavePoint: Location{Map: "field", X: 1, Y: 1},
		Seed:      3,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := w.maps["field"]
	mobs := map[int]*Monster{}
	for _, mob := range m.monsters {
		mobs[mob.def.ID] = mob
	}
	// Keep them where they spawned so distances are predictable.
	for _, mob := range mobs {
		mob.x, mob.y, mob.path = mob.spawn.X, mob.spawn.Y, nil
	}
	now := time.Now()

	// A player well out of every monster's view: nobody reacts.
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Walker", data: game.NewChar("field", 20, 20), out: discard{}}
	w.players[p.id] = p
	w.place(p, m, 20, 20)
	for _, mob := range mobs {
		mob.x, mob.y, mob.path = mob.spawn.X, mob.spawn.Y, nil
	}
	run(w, &now, 2*time.Second)
	for id, mob := range mobs {
		if mob.target != 0 {
			t.Fatalf("monster %d went for a player out of view", id)
		}
	}

	// Next to the aggressive and the passive monster: only the aggressive one attacks.
	p.x, p.y = 12, 11
	run(w, &now, 3*time.Second)
	if mobs[aggressiveMob].target != p.id {
		t.Fatal("the aggressive monster should attack a player in view")
	}
	if p.data.HP == p.data.Derive().MaxHP {
		t.Fatal("the aggressive monster should have hit by now")
	}
	p.x, p.y = 30, 28
	run(w, &now, time.Second)
	if mobs[passiveMob].target != 0 {
		t.Fatal("a passive monster waits to be attacked")
	}

	// Casting near the cast sensor wakes it.
	p.x, p.y = 12, 30
	w.castSensed(p, now)
	if mobs[sensorMob].target != p.id {
		t.Fatal("the cast sensor should go after a caster in view")
	}
}
