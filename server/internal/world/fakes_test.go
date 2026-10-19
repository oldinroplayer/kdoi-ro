package world

import (
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestFakePlayers(t *testing.T) {
	w, err := New(Config{
		Maps:        map[string]*gat.Map{"field": openMap(t, 40, 40), "town": openMap(t, 10, 10)},
		SavePoint:   Location{Map: "town", X: 1, Y: 1},
		Spawns:      []Spawn{{Map: "field", MobID: wandererMob, Count: 30}},
		FakePlayers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	field := w.maps["field"]
	now := time.Now()
	w.tick(now)
	if len(field.fakes) != 0 {
		t.Fatal("no fakes on a map without real players")
	}

	// A real player arrives: 15-20 fakes join the map.
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Ann", data: game.NewChar("field", 5, 5), out: discard{}}
	w.players[1] = p
	w.online["ann"] = 1
	w.place(p, field, 5, 5)
	w.tick(now)
	n := len(field.fakes)
	if n < fakeMin || n > fakeMax || len(field.players) != n+1 {
		t.Fatalf("%d fakes (%d players on the map)", n, len(field.players))
	}
	for _, id := range field.fakes {
		f := w.players[id]
		if !f.fake || len(f.Name) < 4 || f.data.HP == 0 || w.online[lower(f.Name)] != 0 {
			t.Fatalf("fake %+v", f.Name)
		}
	}
	if s := len(w.online); s != 1 {
		t.Fatalf("online count %d: fakes aren't online players", s)
	}

	// They think and act for a while without trouble (and never leave the map).
	fought, walked := false, false
	for i := range 200 {
		w.tick(now.Add(time.Duration(i) * 100 * time.Millisecond))
		for _, id := range field.fakes {
			f := w.players[id]
			fought = fought || f.target != 0
			walked = walked || f.walking()
		}
	}
	hurt := false
	for _, mob := range field.monsters {
		hurt = hurt || len(mob.damage) > 0
	}
	if !fought || !walked || !(hurt || len(w.respawns) > 0) {
		t.Fatalf("fakes fought %v, walked %v, hurt a monster %v", fought, walked, hurt || len(w.respawns) > 0)
	}
	for _, id := range field.fakes {
		if f := w.players[id]; f == nil || f.m != field {
			t.Fatal("a fake left its map")
		}
	}

	// The real player leaves: once the map sleeps, the fakes go too.
	w.removeFromMap(p)
	delete(w.players, 1)
	delete(w.online, "ann")
	later := now.Add(20*time.Second + mapGrace + time.Second)
	w.tick(later)
	if len(field.fakes) != 0 || len(field.players) != 0 {
		t.Fatalf("after the map slept: %d fakes, %d players", len(field.fakes), len(field.players))
	}

	// Turning fakes off clears a busy map too.
	w.place(p, field, 5, 5)
	w.players[1] = p
	w.tick(later)
	if len(field.fakes) == 0 {
		t.Fatal("fakes should return with a real player")
	}
	w.fakesOn = false
	w.tick(later)
	if len(field.fakes) != 0 || len(field.players) != 1 {
		t.Fatalf("fakes off: %d fakes, %d players", len(field.fakes), len(field.players))
	}
}

func TestFakeLevelsAndSkills(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"field": openMap(t, 30, 30)},
		SavePoint: Location{Map: "field", X: 1, Y: 1},
		Spawns:    []Spawn{{Map: "field", X: 15, Y: 15, SpanX: 1, SpanY: 1, MobID: wandererMob, Count: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	field := w.maps["field"]
	now := time.Now()
	// Monsters here are level 1: fakes are 5-8 levels above, so 6 to 9.
	for range 20 {
		f := w.newFake(field, mobLevel(field), 14, 14, now)
		if lv := f.data.BaseLevel; lv < 1+fakeLevelAbove || lv > 1+fakeLevelAbove+fakeLevelSpread {
			t.Fatalf("fake level %d for level 1 monsters", lv)
		}
	}
	// Every 1st job uses its skills in a fight (made at a level where 1st
	// jobs appear): Mages bolt, Acolytes heal and buff themselves, the rest
	// use their attack skill now and then.
	for _, job := range game.FirstJobs {
		// A fresh world each, so one job's kill doesn't leave the next without a target.
		w, err := New(Config{
			Maps:      map[string]*gat.Map{"field": openMap(t, 30, 30)},
			SavePoint: Location{Map: "field", X: 1, Y: 1},
			Spawns:    []Spawn{{Map: "field", X: 15, Y: 15, SpanX: 1, SpanY: 1, MobID: wandererMob, Count: 1}},
		})
		if err != nil {
			t.Fatal(err)
		}
		field := w.maps["field"]
		var mob *Monster
		for _, m := range field.monsters {
			mob = m
		}
		var f *Player
		for range 500 {
			if c := w.newFake(field, 20, 14, 14, now); c.data.Job == job {
				f = c
				break
			}
		}
		if f == nil {
			t.Fatalf("no fake %s", game.Job(job).Name)
		}
		w.players[f.id] = f
		field.fakes = append(field.fakes, f.id)
		w.place(f, field, 14, 14)
		if job == game.JobAcolyte {
			f.data.HP = f.data.Derive().MaxHP / 2 // hurt: should heal itself
		}
		cast := false
		for i := range 60 {
			if mob.hp <= 0 || w.monsters[mob.id] == nil {
				break
			}
			f.target = mob.id
			sp := f.data.SP
			w.tickFake(f, now.Add(time.Duration(i)*2*time.Second))
			if f.casting != nil || f.pending != nil || f.data.SP < sp {
				cast = true
				break
			}
		}
		if !cast {
			t.Errorf("fake %s never used a skill (SP %d)", game.Job(job).Name, f.data.SP)
		}
	}
}
