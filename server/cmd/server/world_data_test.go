package main

import (
	"os"
	"path/filepath"
	"testing"

	"kdoiro/server/internal/gat"
)

// TestCityContentOnWalkableCells checks the generated warps, NPCs and Kafra
// destinations against the client's maps (skipped when they aren't extracted):
// eAthena's November 2004 data sometimes describes later versions of a map.
func TestCityContentOnWalkableCells(t *testing.T) {
	dir := "../../../ro-client/data"
	maps := map[string]*gat.Map{}
	for _, name := range append([]string{"morocc", "moc_fild07", "moc_fild06"}, cityMaps...) {
		m, err := gat.Load(filepath.Join(dir, name+".gat"))
		if os.IsNotExist(err) {
			t.Skip("maps not extracted")
		}
		if err != nil {
			t.Fatal(err)
		}
		maps[name] = m
	}
	// Within 2 cells of a walkable cell counts (warps and NPCs sit on edges).
	near := func(mapName string, x, y int) bool {
		m := maps[mapName]
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if m.Walkable(x+dx, y+dy) {
					return true
				}
			}
		}
		return false
	}
	for _, w := range cityWarps {
		if !near(w.Map, w.X, w.Y) {
			t.Errorf("warp at %s (%d,%d) isn't near a walkable cell", w.Map, w.X, w.Y)
		}
		if !near(w.DestMap, w.DestX, w.DestY) {
			t.Errorf("warp %s (%d,%d) lands on %s (%d,%d), not walkable", w.Map, w.X, w.Y, w.DestMap, w.DestX, w.DestY)
		}
	}
	for _, n := range cityNPCs {
		if !near(n.Map, n.X, n.Y) {
			t.Errorf("NPC %d %s at %s (%d,%d) isn't near a walkable cell", n.ID, n.Name, n.Map, n.X, n.Y)
		}
		if n.Kafra == nil {
			continue
		}
		if s := n.Kafra.Save; !near(s.Map, s.X, s.Y) {
			t.Errorf("Kafra %d saves at %s (%d,%d), not walkable", n.ID, s.Map, s.X, s.Y)
		}
		for _, tp := range n.Kafra.Teleports {
			if maps[tp.Map] != nil && !near(tp.Map, tp.X, tp.Y) {
				t.Errorf("Kafra teleport to %s lands at (%d,%d), not walkable", tp.Map, tp.X, tp.Y)
			}
		}
	}
}
