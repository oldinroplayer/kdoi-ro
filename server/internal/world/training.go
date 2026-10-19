package world

import (
	"fmt"

	"kdoiro/server/internal/proto"
)

// KindTraining is a Training Guide: she sends players, for free, to a map
// suited to their level, chosen from level ranges (not in the 2004 data).
const KindTraining = "training"

// TrainingRange is one entry of a Training Guide's menu: a level range and
// the maps (or dungeons) she suggests for it.
type TrainingRange struct {
	Name  string     // e.g. "Lv 20-30"
	Spots []Teleport // where to; Price is ignored (free)
}

// Training Guide dialog menus (Player.dialogMenu): 0 is the range list,
// trainingSpots+i the spots of range i.
const trainingSpots = 1

// trainingRanges are a guide's ranges with at least one spot on a loaded map.
func (w *World) trainingRanges(n *NPC) []TrainingRange {
	var out []TrainingRange
	for _, r := range n.Training {
		var spots []Teleport
		for _, t := range r.Spots {
			if _, ok := w.maps[t.Map]; ok {
				t.Price = 0
				spots = append(spots, t)
			}
		}
		if len(spots) > 0 {
			out = append(out, TrainingRange{Name: r.Name, Spots: spots})
		}
	}
	return out
}

func (w *World) trainingTalk(p *Player, n *NPC) {
	ranges := w.trainingRanges(n)
	options := make([]string, 0, len(ranges)+1)
	for _, r := range ranges {
		options = append(options, r.Name)
	}
	p.dialog, p.dialogMenu = n.ID, 0
	p.out.Send(proto.NpcDialog(n.ID, n.Name, fmt.Sprintf(
		"Looking for a good place to train? You're base level %d.\nPick a level range and I'll send you somewhere that suits it, free of charge.", p.data.BaseLevel),
		append(options, "Cancel")))
}

func (w *World) trainingReply(p *Player, n *NPC, choice int) {
	ranges := w.trainingRanges(n)
	if p.dialogMenu >= trainingSpots {
		i := p.dialogMenu - trainingSpots
		if i >= len(ranges) {
			return
		}
		spots := ranges[i].Spots
		switch {
		case choice < len(spots):
			w.teleport(p, n, spots[choice])
		case choice == len(spots): // Back
			w.trainingTalk(p, n)
		}
		return
	}
	if choice >= len(ranges) {
		return // Cancel
	}
	r := ranges[choice]
	options := make([]string, 0, len(r.Spots)+2)
	for _, t := range r.Spots {
		options = append(options, t.Name)
	}
	p.dialog, p.dialogMenu = n.ID, trainingSpots+choice
	p.out.Send(proto.NpcDialog(n.ID, n.Name, fmt.Sprintf("%s: where would you like to go?", r.Name), append(options, "Back", "Cancel")))
}
