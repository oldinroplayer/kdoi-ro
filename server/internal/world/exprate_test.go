package world

import (
	"testing"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestExpRate(t *testing.T) {
	if _, err := New(Config{Maps: map[string]*gat.Map{"field": openMap(t, 20, 20)}, SavePoint: Location{Map: "field", X: 1, Y: 1}, ExpRate: -1}); err == nil {
		t.Fatal("a negative EXP rate should be refused")
	}
	w, err := New(Config{Maps: map[string]*gat.Map{"field": openMap(t, 20, 20)}, SavePoint: Location{Map: "field", X: 1, Y: 1}})
	if err != nil {
		t.Fatal(err)
	}
	data := game.NewChar("field", 5, 5)
	data.BaseLevel = 50 // so a few hundred EXP doesn't level up and reset the counter
	data.JobLevel = 8   // below the Novice cap of 10
	p := &Player{unit: unit{id: 1, speed: WalkSpeed}, Name: "Ann", data: data, out: discard{}}
	w.players[1] = p
	w.online["ann"] = 1
	w.place(p, w.maps["field"], 5, 5)

	def := &game.MobDef{ID: 9999, Name: "Dummy", BaseExp: 10, JobExp: 7}
	kill := func() (int, int) {
		b0, j0 := p.data.BaseExp, p.data.JobExp
		mob := &Monster{unit: unit{id: 500, m: w.maps["field"], x: 6, y: 6}, def: def, spawn: &Spawn{}, damage: map[uint32]int{1: 30}}
		w.killMonster(mob, time.Now())
		return p.data.BaseExp - b0, p.data.JobExp - j0
	}

	if b, j := kill(); b != 10 || j != 7 {
		t.Fatalf("x1 kill: %d/%d EXP, want 10/7", b, j)
	}
	w.expRate = 200
	if b, j := kill(); b != 20 || j != 14 {
		t.Fatalf("x2 kill: %d/%d EXP, want 20/14", b, j)
	}
	if pct, err := expRatePercent(1.5); err != nil || pct != 150 {
		t.Fatalf("x1.5 = %d%% %v", pct, err)
	}
	w.expRate = 150
	if b, j := kill(); b != 15 || j != 10 {
		t.Fatalf("x1.5 kill: %d/%d EXP, want 15/10", b, j)
	}
	if FormatExpRate(1.5) != "x1.5" || FormatExpRate(3) != "x3" {
		t.Fatalf("format: %s %s", FormatExpRate(1.5), FormatExpRate(3))
	}
}
