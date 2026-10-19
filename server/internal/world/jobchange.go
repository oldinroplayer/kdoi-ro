package world

import (
	"errors"
	"fmt"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// NPC kinds.
const (
	KindShop      = ""          // sells its Shop items (and buys anything)
	KindJobChange = "jobchange" // turns Novices into 1st jobs
)

// jobGuideTalk opens the Job Guide's conversation. The requirements follow the
// 2004 eAthena job quests (npc/jobs/1-1/): Job Level 10, all skill points spent.
func (w *World) jobGuideTalk(p *Player, n *NPC) {
	c := &p.data
	var text string
	switch err := c.CanChangeJob(); {
	case errors.Is(err, game.ErrNotNovice):
		text = fmt.Sprintf("You're already a %s. Keep training, and come back when there's more to learn!", game.Job(c.Job).Name)
	case errors.Is(err, game.ErrJobLevelTooLow):
		text = "So you want a job? Train a little more first.\nCome back when you reach Job Level 10."
	case errors.Is(err, game.ErrUnspentPoints):
		text = "You need to use up all of your skill points before I can give you a job.\n(Open your skills with Alt+S.)"
	default:
		options := make([]string, 0, len(game.FirstJobs)+1)
		for _, job := range game.FirstJobs {
			options = append(options, game.Job(job).Name)
		}
		options = append(options, "Not yet")
		p.dialog = n.ID
		p.out.Send(proto.NpcDialog(n.ID, n.Name, "Well done, Novice! You've learned all the basics.\nWhich path will you walk?", options))
		return
	}
	p.dialog = 0
	p.out.Send(proto.NpcDialog(n.ID, n.Name, text, nil))
}

// NpcReply handles the player's answer to an NPC dialog.
func (w *World) NpcReply(id, npcID uint32, choice uint8) error {
	return w.do(func() {
		p, n := w.players[id], w.npcs[npcID]
		if p == nil || n == nil || p.dialog != npcID {
			return
		}
		p.dialog = 0
		if choice == proto.NpcReplyClose || p.dead || n.Map != p.m.name || chebyshev(p.x, p.y, n.X, n.Y) > shopRange {
			return
		}
		switch {
		case n.Kind == KindKafra:
			w.kafraReply(p, n, int(choice))
		case n.Kind == KindWarper:
			w.warperReply(p, n, int(choice))
		case n.Kind == KindReset:
			w.resetReply(p, n, int(choice))
		case n.Kind == KindTraining:
			w.trainingReply(p, n, int(choice))
		case n.Kind == KindJobChange && int(choice) < len(game.FirstJobs):
			w.changeJob(p, n, game.FirstJobs[choice])
		}
	})
}

func (w *World) changeJob(p *Player, n *NPC, job uint16) {
	before := p.look()
	if err := p.data.ChangeJob(job); err != nil {
		w.jobGuideTalk(p, n) // re-explain what's missing
		return
	}
	name := game.Job(job).Name
	p.out.Send(proto.NpcDialog(n.ID, n.Name, fmt.Sprintf("Congratulations! You are now a %s.\nMay your journey be a great one.", name), nil))
	w.broadcast(p.m, proto.PlayerJob(p.id, job))
	w.broadcast(p.m, proto.LevelUp(p.id, proto.JobChange))
	if after := p.look(); after != before { // gear the new job can't use came off
		w.broadcast(p.m, proto.PlayerLook(p.id, after))
	}
	w.sendInventory(p)
	w.sendStatus(p)
	w.sendSkills(p)
}
