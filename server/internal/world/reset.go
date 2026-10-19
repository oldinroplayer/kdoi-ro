package world

import (
	"fmt"
	"strings"

	"kdoiro/server/internal/proto"
)

// KindReset is a free stat and skill reset NPC (not in the 2004 data; RO
// servers often add one, like rAthena's Reset Girl).
const KindReset = "reset"

// What a reset returns.
const (
	ResetStats  = "stats"
	ResetSkills = "skills"
	ResetAll    = "all"
)

// resetChoices are the Reset NPC's menu entries, in order.
var resetChoices = []string{ResetStats, ResetSkills, ResetAll}

func (w *World) resetTalk(p *Player, n *NPC) {
	p.dialog = n.ID
	p.out.Send(proto.NpcDialog(n.ID, n.Name,
		"Want to start over? I can give you back every status point or skill point you've spent, free of charge.\nWhat would you like to reset?",
		[]string{"Reset stats", "Reset skills", "Reset both", "Cancel"}))
}

func (w *World) resetReply(p *Player, n *NPC, choice int) {
	if choice >= len(resetChoices) {
		return // Cancel
	}
	p.out.Send(proto.NpcDialog(n.ID, n.Name, w.reset(p, resetChoices[choice])+"\nSpend your points again however you like.", nil))
}

// reset gives back a player's stat and/or skill points (what: ResetStats,
// ResetSkills or ResetAll) and returns a line saying what happened.
func (w *World) reset(p *Player, what string) string {
	var done []string
	if what == ResetStats || what == ResetAll {
		p.data.ResetStats()
		done = append(done, fmt.Sprintf("%d status points", p.data.StatusPoints))
	}
	if what == ResetSkills || what == ResetAll {
		p.data.ResetSkills()
		done = append(done, fmt.Sprintf("%d skill points", p.data.SkillPoints))
	}
	w.sendStatus(p)
	w.sendSkills(p)
	return "Done! You now have " + strings.Join(done, " and ") + " to spend."
}

// ResetPlayer resets an online character's stats, skills or both for the
// server console. It returns a summary.
func (w *World) ResetPlayer(name, what string) (string, error) {
	if what != ResetStats && what != ResetSkills && what != ResetAll {
		return "", fmt.Errorf("reset what? %s, %s or %s", ResetStats, ResetSkills, ResetAll)
	}
	var summary string
	var err error
	doErr := w.do(func() {
		p := w.onlinePlayer(strings.TrimSpace(name))
		if p == nil {
			err = fmt.Errorf("%s is %w", name, ErrNotOnline)
			return
		}
		line := w.reset(p, what)
		p.out.Send(proto.Message("Your points were reset. " + strings.TrimPrefix(line, "Done! ")))
		summary = p.Name + ": " + strings.TrimPrefix(line, "Done! You now have ")
	})
	if doErr != nil {
		return "", doErr
	}
	return summary, err
}
