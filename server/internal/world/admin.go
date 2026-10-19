package world

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// Admin actions for the server console: they act on online characters live.

// ErrNotOnline is returned for console commands about a character not logged in.
var ErrNotOnline = errors.New("not online")

// maxGrant caps one EXP grant (the EXP tables top out well below this).
const maxGrant = 1_000_000_000

// OnlinePlayer is one line of the console's "who".
type OnlinePlayer struct {
	Name                string
	Job                 string
	BaseLevel, JobLevel int
	Map                 string
	X, Y                int
}

// Online lists the characters currently logged in, by name.
func (w *World) Online() []OnlinePlayer {
	var out []OnlinePlayer
	w.do(func() {
		for _, p := range w.players {
			out = append(out, OnlinePlayer{p.Name, game.Job(p.data.Job).Name, p.data.BaseLevel, p.data.JobLevel, p.m.name, p.x, p.y})
		}
	})
	slices.SortFunc(out, func(a, b OnlinePlayer) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out
}

// MaxExpRate caps the server EXP rate (x100).
const MaxExpRate = 100

// expRatePercent turns a multiplier (1.5 = x1.5) into whole percent.
func expRatePercent(rate float64) (int, error) {
	if !(rate > 0 && rate <= MaxExpRate) {
		return 0, fmt.Errorf("EXP rate must be above 0 and at most x%d", MaxExpRate)
	}
	return max(int(math.Round(rate*100)), 1), nil
}

// ExpRate is the current monster EXP multiplier (1 = x1).
func (w *World) ExpRate() float64 {
	var pct int
	w.do(func() { pct = w.expRate })
	return float64(pct) / 100
}

// SetExpRate changes the monster EXP multiplier for every kill from now on
// (2 = "x2") and tells everyone online.
func (w *World) SetExpRate(rate float64) error {
	pct, err := expRatePercent(rate)
	if err != nil {
		return err
	}
	return w.do(func() {
		w.expRate = pct
		msg := proto.Message("Server EXP rate is now " + FormatExpRate(float64(pct)/100) + ".")
		for _, p := range w.players {
			p.out.Send(msg)
		}
	})
}

// FormatExpRate writes a multiplier the way players say it: "x2", "x1.5".
func FormatExpRate(rate float64) string {
	return "x" + strconv.FormatFloat(rate, 'f', -1, 64)
}

// GrantExp gives an online character base and job EXP, levelling them up
// (with the effects and points) exactly as kills do. It returns a summary.
func (w *World) GrantExp(name string, base, job int) (string, error) {
	if base < 0 || job < 0 || base > maxGrant || job > maxGrant {
		return "", fmt.Errorf("EXP must be between 0 and %d", maxGrant)
	}
	var summary string
	var err error
	doErr := w.do(func() {
		p := w.onlinePlayer(strings.TrimSpace(name))
		if p == nil {
			err = fmt.Errorf("%s is %w", name, ErrNotOnline)
			return
		}
		b0, j0 := p.data.BaseLevel, p.data.JobLevel
		w.giveExp(p, base, job)
		p.out.Send(proto.Message(fmt.Sprintf("You received %d base and %d job EXP.", base, job)))
		summary = fmt.Sprintf("%s: base level %d -> %d, job level %d -> %d", p.Name, b0, p.data.BaseLevel, j0, p.data.JobLevel)
	})
	if doErr != nil {
		return "", doErr
	}
	return summary, err
}

// GrantZeny adds zeny to an online character (a negative amount takes it
// away), keeping the total between 0 and game.MaxZeny. It returns a summary.
func (w *World) GrantZeny(name string, amount int) (string, error) {
	if amount < -game.MaxZeny || amount > game.MaxZeny {
		return "", fmt.Errorf("zeny must be between %d and %d", -game.MaxZeny, game.MaxZeny)
	}
	var summary string
	var err error
	doErr := w.do(func() {
		p := w.onlinePlayer(strings.TrimSpace(name))
		if p == nil {
			err = fmt.Errorf("%s is %w", name, ErrNotOnline)
			return
		}
		before := p.data.Zeny
		p.data.Zeny = min(max(before+amount, 0), game.MaxZeny)
		change := p.data.Zeny - before
		switch {
		case change > 0:
			p.out.Send(proto.Message(fmt.Sprintf("You received %d zeny.", change)))
		case change < 0:
			p.out.Send(proto.Message(fmt.Sprintf("%d zeny was taken from you.", -change)))
		}
		w.sendStatus(p)
		summary = fmt.Sprintf("%s: zeny %d -> %d", p.Name, before, p.data.Zeny)
	})
	if doErr != nil {
		return "", doErr
	}
	return summary, err
}

// WarpAll moves every online character to a map cell (the nearest walkable
// one), as a GM recall would. It returns how many were moved.
func (w *World) WarpAll(mapName string, x, y int) (int, error) {
	m := w.maps[mapName]
	if m == nil {
		return 0, fmt.Errorf("map %q isn't loaded", mapName)
	}
	if x < 0 || y < 0 || x >= m.grid.Width || y >= m.grid.Height {
		return 0, fmt.Errorf("(%d, %d) is outside %s (%dx%d)", x, y, mapName, m.grid.Width, m.grid.Height)
	}
	if _, _, ok := m.grid.NearestWalkable(x, y); !ok {
		return 0, fmt.Errorf("%s has no walkable cell near (%d, %d)", mapName, x, y)
	}
	moved := 0
	err := w.do(func() {
		for _, p := range w.players {
			w.leaveRoom(p, proto.LeftByChoice) // chat room members can't otherwise move
			w.cancelCast(p)
			p.target, p.pending, p.pickup = 0, nil, 0
			w.removeFromMap(p)
			w.place(p, m, x, y)
			p.out.Send(proto.Message("You have been summoned."))
			moved++
		}
	})
	return moved, err
}

// SaveAll sets every online character's respawn point (as a Kafra's Save
// would), snapped to the nearest walkable cell. It returns how many.
func (w *World) SaveAll(mapName string, x, y int) (int, error) {
	m := w.maps[mapName]
	if m == nil {
		return 0, fmt.Errorf("map %q isn't loaded", mapName)
	}
	if x < 0 || y < 0 || x >= m.grid.Width || y >= m.grid.Height {
		return 0, fmt.Errorf("(%d, %d) is outside %s (%dx%d)", x, y, mapName, m.grid.Width, m.grid.Height)
	}
	sx, sy, ok := m.grid.NearestWalkable(x, y)
	if !ok {
		return 0, fmt.Errorf("%s has no walkable cell near (%d, %d)", mapName, x, y)
	}
	n := 0
	err := w.do(func() {
		for _, p := range w.players {
			p.data.Save = game.SavePoint{Map: mapName, X: sx, Y: sy}
			p.out.Send(proto.Message("Your respawn point has been changed."))
			n++
		}
	})
	return n, err
}
