package world

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestGrantExp(t *testing.T) {
	w, err := New(Config{Maps: map[string]*gat.Map{"town": openMap(t, 10, 10)}, SavePoint: Location{Map: "town", X: 1, Y: 1}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	if _, err := w.Join("Hero", game.NewChar("town", 5, 5), discard{}); err != nil {
		t.Fatal(err)
	}

	// A level 1 Novice needs 9 base and 10 job EXP for level 2 (as the dummy kill test).
	got, err := w.GrantExp("hero", 9, 10)
	if err != nil || got != "Hero: base level 1 -> 2, job level 1 -> 2" {
		t.Fatalf("grant: %q %v", got, err)
	}
	if online := w.Online(); len(online) != 1 || online[0].BaseLevel != 2 || online[0].Map != "town" {
		t.Fatalf("who after grant: %+v", online)
	}
	if _, err := w.GrantExp("Ghost", 1, 1); !errors.Is(err, ErrNotOnline) {
		t.Fatalf("offline character: %v", err)
	}

	// Zeny: added, taken away (never below 0), capped at the maximum.
	start := game.StartZeny
	for _, c := range []struct {
		amount int
		want   string
	}{
		{500, fmt.Sprintf("Hero: zeny %d -> %d", start, start+500)},
		{-1_000_000, fmt.Sprintf("Hero: zeny %d -> 0", start+500)},
		{game.MaxZeny, fmt.Sprintf("Hero: zeny 0 -> %d", game.MaxZeny)},
		{1, fmt.Sprintf("Hero: zeny %d -> %d", game.MaxZeny, game.MaxZeny)},
	} {
		if got, err := w.GrantZeny("Hero", c.amount); err != nil || got != c.want {
			t.Fatalf("zeny %d: %q %v, want %q", c.amount, got, err, c.want)
		}
	}
	if _, err := w.GrantZeny("Ghost", 1); !errors.Is(err, ErrNotOnline) {
		t.Fatalf("offline character: %v", err)
	}
}

func TestWarpAll(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"town": openMap(t, 10, 10), "field": openMap(t, 20, 20)},
		SavePoint: Location{Map: "town", X: 1, Y: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	for _, name := range []string{"One", "Two"} {
		if _, err := w.Join(name, game.NewChar("town", 2, 2), discard{}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := w.WarpAll("field", 15, 16); err != nil || n != 2 {
		t.Fatalf("warpall: %d %v", n, err)
	}
	for _, p := range w.Online() {
		if p.Map != "field" || p.X != 15 || p.Y != 16 {
			t.Fatalf("%s is at %s (%d, %d)", p.Name, p.Map, p.X, p.Y)
		}
	}
	if _, err := w.WarpAll("nowhere", 1, 1); err == nil {
		t.Fatal("an unloaded map should be refused")
	}
}

func TestSaveAll(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"town": openMap(t, 10, 10), "camp": openMap(t, 20, 20)},
		SavePoint: Location{Map: "town", X: 1, Y: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	if _, err := w.Join("Saver", game.NewChar("town", 2, 2), discard{}); err != nil {
		t.Fatal(err)
	}
	if n, err := w.SaveAll("camp", 12, 13); err != nil || n != 1 {
		t.Fatalf("saveall: %d %v", n, err)
	}
	data, ok := w.Leave(w.online["saver"])
	if !ok || data.Save != (game.SavePoint{Map: "camp", X: 12, Y: 13}) {
		t.Fatalf("save point after saveall: %+v", data.Save)
	}
}

func TestResetPlayer(t *testing.T) {
	w, err := New(Config{Maps: map[string]*gat.Map{"town": openMap(t, 10, 10)}, SavePoint: Location{Map: "town", X: 1, Y: 1}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	data := game.NewChar("town", 5, 5)
	data.Stats[game.STR], data.StatusPoints = 9, 0
	data.Skills[game.SkillBasic], data.SkillPoints = 5, 0
	if _, err := w.Join("Hero", data, discard{}); err != nil {
		t.Fatal(err)
	}
	got, err := w.ResetPlayer("hero", "all")
	if want := fmt.Sprintf("Hero: %d status points and 5 skill points to spend.", game.TotalStatusPoints(1)); err != nil || got != want {
		t.Fatalf("reset: %q %v, want %q", got, err, want)
	}
	if _, err := w.ResetPlayer("Ghost", "stats"); !errors.Is(err, ErrNotOnline) {
		t.Fatalf("offline character: %v", err)
	}
}

func TestResetMaidChangesJob(t *testing.T) {
	maid := NPC{ID: 900, Name: "Reset Maid", Sprite: "4_f_maid", Map: "town", X: 5, Y: 6, Kind: KindReset}
	w, err := New(Config{Maps: map[string]*gat.Map{"town": openMap(t, 10, 10)}, SavePoint: Location{Map: "town", X: 1, Y: 1}, NPCs: []NPC{maid}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	data := game.NewChar("town", 5, 5)
	data.Job, data.JobLevel = game.JobSwordman, 15
	data.Skills[game.SkillBash], data.SkillPoints = 5, 0
	id, err := w.Join("Hero", data, discard{})
	if err != nil {
		t.Fatal(err)
	}
	// Talk, pick "Change job" (after the three resets), then the first job
	// offered: Mage (Swordman, the current one, is left out of the list).
	if err := w.ClickNpc(id, maid.ID); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []uint8{3, 0} {
		if err := w.NpcReply(id, maid.ID, choice); err != nil {
			t.Fatal(err)
		}
	}
	var job uint16
	var points, level int
	w.do(func() {
		p := w.players[id]
		job, points, level = p.data.Job, p.data.SkillPoints, p.data.JobLevel
	})
	if job != game.JobMage || points != 5 || level != 15 {
		t.Fatalf("after the switch: job %d, %d skill points, job level %d; want Mage, 5, 15", job, points, level)
	}
}
