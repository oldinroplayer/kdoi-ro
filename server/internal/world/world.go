// Package world runs the game state: maps, players on them, and walking.
//
// All state is owned by one goroutine (Run). Other goroutines talk to it
// through Join/Move/Leave, which queue a command and wait for it to run.
package world

import (
	"context"
	"errors"
	"strings"
	"time"

	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

const (
	TickInterval = 50 * time.Millisecond
	// WalkSpeed is ms per straight cell (RO default for a Novice).
	WalkSpeed = 150
	// MaxWalkSteps caps a single walk request, like rAthena's MAX_WALKPATH.
	MaxWalkSteps = 32
)

var (
	ErrAlreadyOnline = errors.New("character already online")
	ErrUnknownMap    = errors.New("unknown map")
	ErrNoWalkable    = errors.New("map has no walkable cell")
	ErrStopped       = errors.New("world stopped")
)

// Outbox receives packets for one player. Send must not block.
type Outbox interface {
	Send(packet []byte)
}

type Player struct {
	ID   uint32
	Name string
	Job  uint16
	X, Y int
	Dir  uint8

	m        *mapInstance
	path     []path.Point // remaining cells; path[0] is the step in progress
	nextStep time.Time    // when path[0] is reached
	out      Outbox
}

type mapInstance struct {
	name    string
	grid    *gat.Map
	players map[uint32]*Player
}

type World struct {
	maps    map[string]*mapInstance // read-only after New
	players map[uint32]*Player
	online  map[string]uint32 // lowercase name -> id
	nextID  uint32

	cmds    chan func()
	stopped chan struct{}
}

func New(maps map[string]*gat.Map) *World {
	w := &World{
		maps:    map[string]*mapInstance{},
		players: map[uint32]*Player{},
		online:  map[string]uint32{},
		nextID:  100000, // keep player IDs clear of small reserved values
		cmds:    make(chan func(), 256),
		stopped: make(chan struct{}),
	}
	for name, grid := range maps {
		w.maps[name] = &mapInstance{name: name, grid: grid, players: map[uint32]*Player{}}
	}
	return w
}

// HasMap is safe to call from any goroutine.
func (w *World) HasMap(name string) bool {
	_, ok := w.maps[name]
	return ok
}

// Run processes commands and walking until ctx is cancelled.
func (w *World) Run(ctx context.Context) {
	defer close(w.stopped)
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-w.cmds:
			f()
		case now := <-ticker.C:
			w.tick(now)
		}
	}
}

// do runs f on the world goroutine and waits for it.
func (w *World) do(f func()) error {
	done := make(chan struct{})
	select {
	case w.cmds <- func() { f(); close(done) }:
	case <-w.stopped:
		return ErrStopped
	}
	select {
	case <-done:
		return nil
	case <-w.stopped:
		return ErrStopped
	}
}

// Join places a character on a map (snapping to the nearest walkable cell)
// and sends it EnterMap plus everyone already there.
func (w *World) Join(name string, job uint16, mapName string, x, y int, out Outbox) (uint32, error) {
	var id uint32
	var joinErr error
	err := w.do(func() {
		key := strings.ToLower(name)
		if _, ok := w.online[key]; ok {
			joinErr = ErrAlreadyOnline
			return
		}
		m, ok := w.maps[mapName]
		if !ok {
			joinErr = ErrUnknownMap
			return
		}
		x, y, ok = m.grid.NearestWalkable(x, y)
		if !ok {
			joinErr = ErrNoWalkable
			return
		}
		w.nextID++
		p := &Player{ID: w.nextID, Name: name, Job: job, X: x, Y: y, Dir: 4, m: m, out: out}
		id = p.ID

		out.Send(proto.EnterMap(p.ID, m.name, p.X, p.Y, p.Dir, p.Job, WalkSpeed))
		appear := proto.PlayerAppear(p.ID, p.Name, p.Job, p.X, p.Y, p.Dir, WalkSpeed)
		for _, other := range m.players {
			out.Send(proto.PlayerAppear(other.ID, other.Name, other.Job, other.X, other.Y, other.Dir, WalkSpeed))
			if len(other.path) > 0 {
				out.Send(proto.PlayerMove(other.ID, other.X, other.Y, cells(other.path)))
			}
			other.out.Send(appear)
		}

		w.players[p.ID] = p
		w.online[key] = p.ID
		m.players[p.ID] = p
	})
	if err != nil {
		return 0, err
	}
	return id, joinErr
}

// Move starts walking a player toward (x, y). Unreachable targets are ignored.
func (w *World) Move(id uint32, x, y int) error {
	return w.do(func() {
		p, ok := w.players[id]
		if !ok {
			return
		}
		target := path.Point{X: x, Y: y}
		walking := len(p.path) > 0

		// Let the step in progress finish, then route from that cell.
		start := path.Point{X: p.X, Y: p.Y}
		if walking {
			start = p.path[0]
		}
		var route []path.Point
		if target != start {
			route = path.Find(p.m.grid, start, target, MaxWalkSteps)
			if route == nil {
				return
			}
		}
		if walking {
			route = append([]path.Point{start}, route...)
		} else if len(route) == 0 {
			return
		} else {
			p.nextStep = time.Now().Add(stepDuration(p.X, p.Y, route[0]))
		}
		p.path = route
		p.Dir = dirFor(route[0].X-p.X, route[0].Y-p.Y)
		w.broadcast(p.m, proto.PlayerMove(p.ID, p.X, p.Y, cells(route)))
	})
}

// Leave removes a player and reports the cell it was last standing on.
func (w *World) Leave(id uint32) (mapName string, x, y int, ok bool) {
	err := w.do(func() {
		p, found := w.players[id]
		if !found {
			return
		}
		delete(w.players, id)
		delete(w.online, strings.ToLower(p.Name))
		delete(p.m.players, id)
		w.broadcast(p.m, proto.PlayerDisappear(id))
		mapName, x, y, ok = p.m.name, p.X, p.Y, true
	})
	return mapName, x, y, ok && err == nil
}

func (w *World) tick(now time.Time) {
	for _, p := range w.players {
		for len(p.path) > 0 && !now.Before(p.nextStep) {
			next := p.path[0]
			p.Dir = dirFor(next.X-p.X, next.Y-p.Y)
			p.X, p.Y = next.X, next.Y
			p.path = p.path[1:]
			if len(p.path) > 0 {
				p.nextStep = p.nextStep.Add(stepDuration(p.X, p.Y, p.path[0]))
			}
		}
	}
}

func (w *World) broadcast(m *mapInstance, packet []byte) {
	for _, p := range m.players {
		p.out.Send(packet)
	}
}

func stepDuration(x, y int, to path.Point) time.Duration {
	if x != to.X && y != to.Y {
		return WalkSpeed * 14 / 10 * time.Millisecond
	}
	return WalkSpeed * time.Millisecond
}

// dirFor maps a step to RO's facing: 0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE
// (north is +y).
func dirFor(dx, dy int) uint8 {
	switch {
	case dx == 0 && dy > 0:
		return 0
	case dx < 0 && dy > 0:
		return 1
	case dx < 0 && dy == 0:
		return 2
	case dx < 0 && dy < 0:
		return 3
	case dx == 0 && dy < 0:
		return 4
	case dx > 0 && dy < 0:
		return 5
	case dx > 0 && dy == 0:
		return 6
	default:
		return 7
	}
}

func cells(ps []path.Point) [][2]int {
	out := make([][2]int, len(ps))
	for i, p := range ps {
		out[i] = [2]int{p.X, p.Y}
	}
	return out
}
