package world

import (
	"testing"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
)

func TestEvenEXPShare(t *testing.T) {
	w, err := New(Config{
		Maps:      map[string]*gat.Map{"field": openMap(t, 20, 20), "town": openMap(t, 20, 20)},
		SavePoint: Location{Map: "field", X: 1, Y: 1},
		Parties:   []PartyRecord{{ID: 1, Name: "Pals", Leader: "Ann", Members: []string{"Ann", "Ben", "Cid"}, EvenEXP: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	join := func(id uint32, name, mapName string, level int) *Player {
		data := game.NewChar(mapName, 5, 5)
		data.BaseLevel = level
		p := &Player{unit: unit{id: id, speed: WalkSpeed}, Name: name, data: data, out: discard{}}
		w.players[id] = p
		w.online[lower(name)] = id
		w.place(p, w.maps[mapName], 5, 5)
		return p
	}
	ann := join(1, "Ann", "field", 20)
	ben := join(2, "Ben", "field", 25)
	cid := join(3, "Cid", "town", 20) // elsewhere: gets nothing
	loner := join(4, "Loner", "field", 20)

	// Ann did all the damage; Ben shares it, Cid is on another map.
	w.shareEXP(w.maps["field"], map[*Player][2]int{ann: {100, 60}, loner: {10, 6}})
	// (30 job EXP levels up a Novice, so compare job levels, not the counter.)
	if ann.data.BaseExp != 50 || ben.data.BaseExp != 50 || ann.data.JobLevel != ben.data.JobLevel || ben.data.JobLevel == 1 ||
		cid.data.BaseExp != 0 {
		t.Fatalf("even share: ann %d ben %d (job lv %d/%d) cid %d",
			ann.data.BaseExp, ben.data.BaseExp, ann.data.JobLevel, ben.data.JobLevel, cid.data.BaseExp)
	}
	if loner.data.BaseExp != 10 {
		t.Fatalf("someone outside the party keeps their own share: %d", loner.data.BaseExp)
	}

	// Levels too far apart now: each keeps their own.
	ben.data.BaseLevel = 40
	ann.data.BaseExp, ben.data.BaseExp = 0, 0
	w.shareEXP(w.maps["field"], map[*Player][2]int{ann: {100, 60}})
	if ann.data.BaseExp != 100 || ben.data.BaseExp != 0 {
		t.Fatalf("level gap > 10 should fall back to each take: ann %d ben %d", ann.data.BaseExp, ben.data.BaseExp)
	}
}
