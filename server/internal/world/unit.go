package world

import (
	"time"

	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

// unit is anything that stands on a map and walks: players and monsters.
type unit struct {
	id       uint32
	m        *mapInstance
	x, y     int
	dir      uint8
	speed    int          // ms per straight cell
	path     []path.Point // remaining cells; path[0] is the step in progress
	nextStep time.Time    // when path[0] is reached
}

func (u *unit) walking() bool { return len(u.path) > 0 }

func (u *unit) pos() path.Point { return path.Point{X: u.x, Y: u.y} }

func (u *unit) stepDuration(to path.Point) time.Duration {
	ms := u.speed
	if u.x != to.X && u.y != to.Y {
		ms = ms * 14 / 10
	}
	return time.Duration(ms) * time.Millisecond
}

// routeTo plans a walk of at most maxSteps toward target, stopping once within
// `within` cells (0 = on the target itself). The step in progress is kept so
// movement stays smooth. It returns the new route to broadcast, or nil if
// there's nothing to do.
func (u *unit) routeTo(target path.Point, within, maxSteps int, now time.Time) []path.Point {
	walking := u.walking()
	start := u.pos()
	if walking {
		start = u.path[0]
	}
	var route []path.Point
	if chebyshev(start.X, start.Y, target.X, target.Y) > within {
		route = path.Find(u.m.grid, start, target, maxSteps)
		if route == nil {
			return nil
		}
		for i, c := range route {
			if chebyshev(c.X, c.Y, target.X, target.Y) <= within {
				route = route[:i+1]
				break
			}
		}
	}
	if walking {
		route = append([]path.Point{start}, route...)
	} else if len(route) == 0 {
		return nil
	} else {
		u.nextStep = now.Add(u.stepDuration(route[0]))
	}
	u.path = route
	u.dir = dirFor(route[0].X-u.x, route[0].Y-u.y)
	return route
}

// advance moves along the path for every step that's due. After each step,
// onStep can return false to stop advancing (e.g. the unit warped or reached
// attack range and was stopped).
func (u *unit) advance(now time.Time, onStep func() bool) {
	for u.walking() && !now.Before(u.nextStep) {
		next := u.path[0]
		u.dir = dirFor(next.X-u.x, next.Y-u.y)
		u.x, u.y = next.X, next.Y
		u.path = u.path[1:]
		if !onStep() {
			return
		}
		if u.walking() {
			u.nextStep = u.nextStep.Add(u.stepDuration(u.path[0]))
		}
	}
}

// halt stops walking where the unit stands and tells the map.
func (w *World) halt(u *unit) {
	if !u.walking() {
		return
	}
	u.path = nil
	w.broadcast(u.m, proto.PlayerStop(u.id, u.x, u.y))
}
