package world

import (
	"context"
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestTrainingGuide(t *testing.T) {
	guide := NPC{ID: 901, Name: "Training Guide", Sprite: "4_f_job_hunter", Map: "town", X: 5, Y: 6, Kind: KindTraining,
		Training: []TrainingRange{
			{Name: "Lv 1-10", Spots: []Teleport{{Name: "Nowhere", Map: "unloaded", X: 1, Y: 1}}}, // dropped: map not loaded
			{Name: "Lv 10-20", Spots: []Teleport{
				{Name: "Field", Map: "field", X: 7, Y: 8, Price: 999}, // free anyway
				{Name: "Gone", Map: "unloaded", X: 1, Y: 1},
			}},
		}}
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"town": openMap(t, 10, 10), "field": openMap(t, 20, 20)},
		SavePoint: Location{Map: "town", X: 1, Y: 1},
		NPCs:      []NPC{guide},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := w.trainingRanges(&guide); len(got) != 1 || got[0].Name != "Lv 10-20" || len(got[0].Spots) != 1 || got[0].Spots[0].Price != 0 {
		t.Fatalf("offered ranges: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	data := game.NewChar("town", 5, 5)
	data.Zeny = 0
	id, err := w.Join("Hero", data, discard{})
	if err != nil {
		t.Fatal(err)
	}
	// Talk, pick the only range offered, then its first map.
	if err := w.ClickNpc(id, guide.ID); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []uint8{0, 0} {
		if err := w.NpcReply(id, guide.ID, choice); err != nil {
			t.Fatal(err)
		}
	}
	var at Location
	w.do(func() {
		p := w.players[id]
		at = Location{Map: p.m.name, X: p.x, Y: p.y}
	})
	if at != (Location{Map: "field", X: 7, Y: 8}) {
		t.Fatalf("after picking Field: at %+v", at)
	}
}
