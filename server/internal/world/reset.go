package world

import (
	"fmt"
	"slices"
	"strings"

	"kdoiro/server/internal/game"
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

// resetChoices are the Reset NPC's first menu entries, in order; 1st jobs
// also get "Change job" after them.
var resetChoices = []string{ResetStats, ResetSkills, ResetAll}

// Reset NPC dialog menus (Player.dialogMenu).
const (
	resetMain = iota
	resetJobMenu
)

func (w *World) resetTalk(p *Player, n *NPC) {
	p.dialog, p.dialogMenu = n.ID, resetMain
	options := []string{"Reset stats", "Reset skills", "Reset both"}
	text := "Want to start over? I can give you back every status point or skill point you've spent, free of charge.\nWhat would you like to reset?"
	if slices.Contains(game.FirstJobs, p.data.Job) {
		options = append(options, "Change job")
		text += "\nI can also change you to another 1st job: your stats and skills start over, your job level stays."
	}
	p.out.Send(proto.NpcDialog(n.ID, n.Name, text, append(options, "Cancel")))
}

// otherJobs are the 1st jobs a player could switch to.
func otherJobs(p *Player) []uint16 {
	var out []uint16
	for _, job := range game.FirstJobs {
		if job != p.data.Job {
			out = append(out, job)
		}
	}
	return out
}

func (w *World) resetReply(p *Player, n *NPC, choice int) {
	if p.dialogMenu == resetJobMenu {
		if jobs := otherJobs(p); choice < len(jobs) {
			w.switchJob(p, n, jobs[choice])
		}
		return
	}
	switch {
	case choice < len(resetChoices):
		p.out.Send(proto.NpcDialog(n.ID, n.Name, w.reset(p, resetChoices[choice])+"\nSpend your points again however you like.", nil))
	case choice == len(resetChoices) && slices.Contains(game.FirstJobs, p.data.Job):
		jobs := otherJobs(p)
		options := make([]string, 0, len(jobs)+1)
		for _, job := range jobs {
			options = append(options, game.Job(job).Name)
		}
		p.dialog, p.dialogMenu = n.ID, resetJobMenu
		p.out.Send(proto.NpcDialog(n.ID, n.Name,
			fmt.Sprintf("You're a %s now. Which job would you like instead?\n(Your stats and skills start over; gear the new job can't use comes off.)", game.Job(p.data.Job).Name),
			append(options, "Cancel")))
	}
}

// switchJob turns a 1st job into another, with the stat and skill reset that
// comes with it, and shows everyone the job change.
func (w *World) switchJob(p *Player, n *NPC, job uint16) {
	before := p.look()
	if err := p.data.SwitchJob(job); err != nil {
		p.out.Send(proto.NpcDialog(n.ID, n.Name, "I can't do that for you, sorry.", nil))
		return
	}
	w.cancelCast(p)
	name := game.Job(job).Name
	p.out.Send(proto.NpcDialog(n.ID, n.Name, fmt.Sprintf(
		"There you go: you're a %s now!\nYou have %d status points and %d skill points to spend.", name, p.data.StatusPoints, p.data.SkillPoints), nil))
	w.broadcast(p.m, proto.PlayerJob(p.id, job))
	w.broadcast(p.m, proto.LevelUp(p.id, proto.JobChange))
	if after := p.look(); after != before { // gear the new job can't use came off
		w.broadcast(p.m, proto.PlayerLook(p.id, after))
	}
	w.sendInventory(p)
	w.sendStatus(p)
	w.sendSkills(p)
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
