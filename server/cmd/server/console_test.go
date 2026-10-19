package main

import (
	"context"
	"strings"
	"testing"

	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/world"
)

func TestConsoleCommands(t *testing.T) {
	grid, err := gat.Parse(append([]byte("GRAT\x01\x02\x0a\x00\x00\x00\x0a\x00\x00\x00"), make([]byte, 10*10*20)...))
	if err != nil {
		t.Fatal(err)
	}
	w, err := world.New(world.Config{Maps: map[string]*gat.Map{"town": grid}, SavePoint: world.Location{Map: "town", X: 1, Y: 1}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	for line, want := range map[string]string{
		"help":                "Commands:",
		"who":                 "Nobody is online.",
		"exp Nobody 100":      "error: Nobody is not online",
		"exp Someone":         "usage:",
		"exp Someone many":    "usage:",
		"dance":               `unknown command "dance"`,
		"exp Someone -5":      "error: EXP must be between",
		"zeny Nobody 100":     "error: Nobody is not online",
		"zeny Someone":        "usage: zeny",
		"zeny Someone lots":   "usage: zeny",
		"reset Nobody":        "error: Nobody is not online",
		"reset Nobody luck":   "error: reset what?",
		"reset":               "usage: reset",
		"warpall nowhere 1 1": "error: map \"nowhere\" isn't loaded",
		"warpall town 50 50":  "error: (50, 50) is outside town",
		"warpall town 1":      "usage: warpall",
		"warpall town 3 4":    "moved 0 online players to town (3, 4); all 7 saved characters will start there",
		"saveall nowhere 1 1": "error: map \"nowhere\" isn't loaded",
		"saveall town 2":      "usage: saveall",
		"saveall town 3 4":    "respawn point set to town (3, 4) for 0 online players and all 7 saved characters",
	} {
		if got := consoleCommand(line, w, &fakeSaved{}); !strings.HasPrefix(got, want) {
			t.Errorf("%q -> %q, want prefix %q", line, got, want)
		}
	}

	// The EXP rate, in order since each line changes it.
	for _, c := range []struct{ line, want string }{
		{"exprate", "EXP rate is x1"},
		{"exprate 2", "EXP rate is now x2 "},
		{"exprate x1.5", "EXP rate is now x1.5 "},
		{"exprate", "EXP rate is x1.5"},
		{"exprate 0", "error: EXP rate must be above 0"},
		{"exprate lots", "usage: exprate"},
		{"rate 5", "EXP rate is now x5 "},
		{"droprate", "Drop rate is x1"},
		{"droprate x3", "Drop rate is now x3 "},
		{"droprate 0", "error: drop rate must be above 0"},
		{"chatmode", "Chat mode is local"},
		{"chatmode server", "Chat mode is now server"},
		{"chatmode", "Chat mode is server"},
		{"chatmode local", "Chat mode is now local"},
		{"chatmode loud", "usage: chatmode"},
		{"update", "Restarting for an update"},
		{"update", "An update restart is already under way."},
	} {
		if got := consoleCommand(c.line, w, &fakeSaved{}); !strings.HasPrefix(got, c.want) {
			t.Errorf("%q -> %q, want prefix %q", c.line, got, c.want)
		}
	}
	select {
	case <-updateRequests: // the update was asked for; take it back
	default:
		t.Error("update didn't request a restart")
	}
}

// fakeSaved stands in for the character database.
type fakeSaved struct{ mapName string }

func (f *fakeSaved) RelocateAll(mapName string, x, y int) (int64, error) {
	f.mapName = mapName
	return 7, nil
}

func (f *fakeSaved) SaveAll(mapName string, x, y int) (int64, error) {
	f.mapName = mapName
	return 7, nil
}
