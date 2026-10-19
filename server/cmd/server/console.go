package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"kdoiro/server/internal/world"
)

// consoleHelp lists the server console's commands.
const consoleHelp = `Commands:
  who                        list who is online
  exp <name> <base> [job]    give an online character EXP (levels up live)
  zeny <name> <amount>       give an online character zeny (negative takes it away)
  reset <name> [stats|skills|all]   give an online character back their stat and/or skill points
  warpall <map> <x> <y>      move everyone there: online players now, saved ones at next login
  saveall <map> <x> <y>      set everyone's respawn point there (online and saved)
  exprate [rate]             show or set the monster EXP rate: exprate 2 (or x2, 1.5...)
  droprate [rate]            show or set the monster drop rate the same way
  chatmode [server|local]    show or set who hears public chat: everyone online, or nearby
  fakeplayers [on|off]       show or set fake players (15-20 per map with real players on it)
  update                     restart into a new build: players wait and come back by themselves
  help                       show this list`

// runConsole reads admin commands from the server's terminal (stdin) until it
// closes. Running in the background (no terminal) just ends it quietly.
func runConsole(in io.Reader, out io.Writer, w *world.World, saved relocator) {
	fmt.Fprintln(out, "Server console ready: type help for commands.")
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		if reply := consoleCommand(scanner.Text(), w, saved); reply != "" {
			fmt.Fprintln(out, reply)
		}
	}
}

// relocator changes saved (offline) characters: the character database.
type relocator interface {
	RelocateAll(mapName string, x, y int) (int64, error)
	SaveAll(mapName string, x, y int) (int64, error)
}

// consoleCommand runs one console line and returns what to print.
func consoleCommand(line string, w *world.World, saved relocator) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	switch strings.ToLower(f[0]) {
	case "help", "?":
		return consoleHelp
	case "who":
		online := w.Online()
		if len(online) == 0 {
			return "Nobody is online."
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d online:", len(online))
		for _, p := range online {
			fmt.Fprintf(&b, "\n  %-23s %-9s Lv %d/%d  %s (%d, %d)", p.Name, p.Job, p.BaseLevel, p.JobLevel, p.Map, p.X, p.Y)
		}
		return b.String()
	case "exp":
		if len(f) < 3 || len(f) > 4 {
			return "usage: exp <name> <base> [job]"
		}
		base, err1 := strconv.Atoi(f[2])
		job := 0
		var err2 error
		if len(f) == 4 {
			job, err2 = strconv.Atoi(f[3])
		}
		if err1 != nil || err2 != nil {
			return "usage: exp <name> <base> [job] (numbers)"
		}
		summary, err := w.GrantExp(f[1], base, job)
		if err != nil {
			return "error: " + err.Error()
		}
		return summary
	case "zeny":
		if len(f) != 3 {
			return "usage: zeny <name> <amount>"
		}
		amount, err := strconv.Atoi(f[2])
		if err != nil {
			return "usage: zeny <name> <amount> (a number; negative takes zeny away)"
		}
		summary, err := w.GrantZeny(f[1], amount)
		if err != nil {
			return "error: " + err.Error()
		}
		return summary
	case "warpall":
		if len(f) != 4 {
			return "usage: warpall <map> <x> <y>"
		}
		x, err1 := strconv.Atoi(f[2])
		y, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil {
			return "usage: warpall <map> <x> <y> (numbers)"
		}
		mapName := strings.ToLower(f[1])
		online, err := w.WarpAll(mapName, x, y)
		if err != nil {
			return "error: " + err.Error()
		}
		// Saved characters too, so everyone starts there next login (the online
		// ones save their new spot when they log out anyway).
		stored, err := saved.RelocateAll(mapName, x, y)
		if err != nil {
			return fmt.Sprintf("moved %d online players, but saving the spot for the others failed: %v", online, err)
		}
		return fmt.Sprintf("moved %d online players to %s (%d, %d); all %d saved characters will start there", online, mapName, x, y, stored)
	case "saveall":
		if len(f) != 4 {
			return "usage: saveall <map> <x> <y>"
		}
		x, err1 := strconv.Atoi(f[2])
		y, err2 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil {
			return "usage: saveall <map> <x> <y> (numbers)"
		}
		mapName := strings.ToLower(f[1])
		online, err := w.SaveAll(mapName, x, y)
		if err != nil {
			return "error: " + err.Error()
		}
		stored, err := saved.SaveAll(mapName, x, y)
		if err != nil {
			return fmt.Sprintf("set %d online players' respawn point, but saving it for the others failed: %v", online, err)
		}
		return fmt.Sprintf("respawn point set to %s (%d, %d) for %d online players and all %d saved characters", mapName, x, y, online, stored)
	case "exprate", "rate":
		if len(f) == 1 {
			return "EXP rate is " + world.FormatRate(w.ExpRate())
		}
		if len(f) != 2 {
			return "usage: exprate [rate]"
		}
		rate, err := strconv.ParseFloat(strings.TrimPrefix(strings.ToLower(f[1]), "x"), 64)
		if err != nil {
			return "usage: exprate <rate> (a number like 2, x3 or 1.5)"
		}
		if err := w.SetExpRate(rate); err != nil {
			return "error: " + err.Error()
		}
		return "EXP rate is now " + world.FormatRate(w.ExpRate()) + " (until the server restarts; use -exp-rate to keep it)"
	case "reset":
		if len(f) < 2 || len(f) > 3 {
			return "usage: reset <name> [stats|skills|all]"
		}
		what := world.ResetAll
		if len(f) == 3 {
			what = strings.ToLower(f[2])
		}
		summary, err := w.ResetPlayer(f[1], what)
		if err != nil {
			return "error: " + err.Error()
		}
		return summary
	case "update":
		if !requestUpdate() {
			return "An update restart is already under way."
		}
		return fmt.Sprintf("Restarting for an update: players wait up to %ds and rejoin by themselves (needs run.sh to start the new build).", UpdateWaitSeconds)
	case "droprate":
		if len(f) == 1 {
			return "Drop rate is " + world.FormatRate(w.DropRate())
		}
		if len(f) != 2 {
			return "usage: droprate [rate]"
		}
		rate, err := strconv.ParseFloat(strings.TrimPrefix(strings.ToLower(f[1]), "x"), 64)
		if err != nil {
			return "usage: droprate <rate> (a number like 2, x3 or 1.5)"
		}
		if err := w.SetDropRate(rate); err != nil {
			return "error: " + err.Error()
		}
		return "Drop rate is now " + world.FormatRate(w.DropRate()) + " (until the server restarts; use -drop-rate to keep it)"
	case "fakeplayers", "fakes":
		if len(f) == 1 {
			return "Fake players are " + onOff(w.FakePlayers())
		}
		var on bool
		switch strings.ToLower(f[1]) {
		case "on", "yes", "1":
			on = true
		case "off", "no", "0":
		default:
			return "usage: fakeplayers [on|off]"
		}
		if err := w.SetFakePlayers(on); err != nil {
			return "error: " + err.Error()
		}
		if on {
			return "Fake players are now on: 15-20 join each map that has real players on it."
		}
		return "Fake players are now off: they've all left."
	case "chatmode":
		if len(f) == 1 {
			return "Chat mode is " + chatModeName(w.ServerChat())
		}
		var on bool
		switch strings.ToLower(f[1]) {
		case "server", "global", "all", "on":
			on = true
		case "local", "near", "nearby", "off":
		default:
			return "usage: chatmode [server|local]"
		}
		if len(f) != 2 {
			return "usage: chatmode [server|local]"
		}
		if err := w.SetServerChat(on); err != nil {
			return "error: " + err.Error()
		}
		return "Chat mode is now " + chatModeName(on)
	default:
		return fmt.Sprintf("unknown command %q (type help)", f[0])
	}
}

func chatModeName(server bool) string {
	if server {
		return "server: public chat reaches everyone online"
	}
	return "local: public chat reaches players within 14 cells"
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
