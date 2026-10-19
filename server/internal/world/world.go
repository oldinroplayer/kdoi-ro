// Package world runs the game state: maps, players, NPCs and monsters on them,
// walking, combat, shops, inventories and stats.
//
// All state is owned by one goroutine (Run). Other goroutines talk to it
// through Join/Move/Attack/Leave..., which queue a command and wait for it to run.
package world

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"kdoiro/server/internal/game"
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
	// maxChaseSteps lets a player reach any monster they can click on screen.
	maxChaseSteps = 64
	// NpcRange is how close (in cells) a player must be to talk to an NPC.
	NpcRange = 4
	// shopRange is a little looser, so stepping away mid-order still works.
	shopRange = NpcRange + 3
	// playerHurtMotion is how long a player flinches when hit (ms).
	playerHurtMotion = 230
	hpRegenInterval  = 6 * time.Second
	spRegenInterval  = 8 * time.Second
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

// Location is a map and cell.
type Location struct {
	Map  string
	X, Y int
}

// Config describes the world's content.
type Config struct {
	Maps   map[string]*gat.Map
	Warps  []Warp
	NPCs   []NPC
	Spawns []Spawn
	// SavePoint is where dead players return to.
	SavePoint Location
	// Seed makes combat rolls reproducible; 0 picks a random seed.
	Seed uint64
	// ExpRate multiplies the EXP monsters give (2 = "x2"); 0 means x1.
	ExpRate float64
	// DropRate multiplies monsters' drop chances (2 = "x2"); 0 means x1.
	DropRate float64
	// Parties saved from earlier runs, and where to save changes (nil: don't).
	Parties    []PartyRecord
	PartyStore PartyStore
}

type Player struct {
	unit
	Name string
	// data's Map/X/Y are only updated when the player leaves; unit.x/y are live.
	data game.CharData
	out  Outbox

	dead        bool
	sitting     bool      // can't walk or attack; regenerates twice as fast
	casting     *cast     // skill being cast, if any
	pending     *cast     // skill waiting for the player to walk into range
	room        *chatRoom // chat room the player is in (can't walk or fight)
	pickup      uint32    // floor item the player is walking over to pick up
	dialogMenu  int       // which menu of the NPC's dialog the player is in
	nextEmote   time.Time // no emoticons before this (cooldown)
	partyInvite uint32    // party that invited the player, awaiting an answer
	buffEnds    map[int]time.Time
	dialog      uint32    // NPC whose question the player is answering
	nextSkill   time.Time // after-cast delay: no skills before this
	target      uint32    // monster being attacked
	chaseTarget path.Point
	nextAttack  time.Time
	nextRepath  time.Time
	nextHPRegen time.Time
	nextSPRegen time.Time
}

type mapInstance struct {
	name     string
	grid     *gat.Map
	players  map[uint32]*Player
	monsters map[uint32]*Monster
	items    map[uint32]*floorItem
	warps    []Warp
	npcs     []*NPC

	// Sleeping (see activity.go): asleep is set once the map has gone quiet;
	// sleepAt ends the grace period after its last player left.
	asleep  bool
	sleepAt time.Time
}

// Warp moves a player who steps within SpanX/SpanY cells of (X, Y) on Map
// to (DestX, DestY) on DestMap, like an rAthena "warp" NPC.
type Warp struct {
	Map          string
	X, Y         int
	SpanX, SpanY int
	DestMap      string
	DestX, DestY int
}

func (wp Warp) contains(x, y int) bool {
	return abs(x-wp.X) <= wp.SpanX && abs(y-wp.Y) <= wp.SpanY
}

// NPC is a non-player character; with a Shop it sells those items (and buys anything).
type NPC struct {
	ID     uint32
	Name   string
	Sprite string // file under data/sprite/npc/, without extension
	Map    string
	X, Y   int
	Dir    uint8
	Shop   []int      // item IDs, sold at their database price
	Price  int        // when set, every item in Shop costs this many zeny instead
	Kind   string     // KindShop (default), KindJobChange, KindKafra, KindWarper or KindReset
	Kafra  *Kafra     // for KindKafra
	Warps  []Teleport // for KindWarper
}

type World struct {
	maps     map[string]*mapInstance // read-only after New
	npcs     map[uint32]*NPC         // read-only after New
	players  map[uint32]*Player
	monsters map[uint32]*Monster
	online   map[string]uint32 // lowercase name -> id
	rooms    map[uint32]*chatRoom
	// Parties by ID, and each member's party by lowercase character name.
	parties         map[uint32]*party
	partyByMember   map[string]uint32
	nextPartyID     uint32
	partyStore      PartyStore
	nextPartyStatus time.Time
	nextID          uint32
	savePoint       Location
	rng             *rand.Rand
	respawns        []respawn
	// expRate and dropRate are the monster EXP and drop chance multipliers
	// in percent (100 = x1).
	expRate, dropRate int
	// serverChat sends public chat to every player online instead of only
	// those nearby (the console's chatmode).
	serverChat bool
	// nextFloorSweep is when expired floor items are next cleared.
	nextFloorSweep time.Time

	cmds    chan func()
	stopped chan struct{}
}

// New creates a world from its content description.
func New(cfg Config) (*World, error) {
	seed := cfg.Seed
	if seed == 0 {
		seed = rand.Uint64()
	}
	w := &World{
		maps:          map[string]*mapInstance{},
		npcs:          map[uint32]*NPC{},
		players:       map[uint32]*Player{},
		monsters:      map[uint32]*Monster{},
		online:        map[string]uint32{},
		rooms:         map[uint32]*chatRoom{},
		parties:       map[uint32]*party{},
		partyByMember: map[string]uint32{},
		partyStore:    cfg.PartyStore,
		nextID:        100000, // keep unit IDs clear of NPC IDs and small reserved values
		savePoint:     cfg.SavePoint,
		rng:           rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		cmds:          make(chan func(), 256),
		stopped:       make(chan struct{}),
	}
	for name, grid := range cfg.Maps {
		w.maps[name] = &mapInstance{
			name: name, grid: grid,
			players: map[uint32]*Player{}, monsters: map[uint32]*Monster{},
			items: map[uint32]*floorItem{},
		}
	}
	if _, ok := w.maps[cfg.SavePoint.Map]; !ok {
		return nil, fmt.Errorf("save point map %q not loaded", cfg.SavePoint.Map)
	}
	for _, r := range []struct {
		what string
		rate float64
		into *int
	}{{"EXP", cfg.ExpRate, &w.expRate}, {"drop", cfg.DropRate, &w.dropRate}} {
		if r.rate == 0 {
			r.rate = 1
		}
		pct, err := ratePercent(r.what, r.rate)
		if err != nil {
			return nil, err
		}
		*r.into = pct
	}
	for _, rec := range cfg.Parties {
		pt := &party{PartyRecord: rec}
		w.parties[rec.ID] = pt
		for _, m := range rec.Members {
			w.partyByMember[lower(m)] = rec.ID
		}
		w.nextPartyID = max(w.nextPartyID, rec.ID)
	}
	for _, wp := range cfg.Warps {
		src, ok := w.maps[wp.Map]
		if !ok {
			return nil, fmt.Errorf("warp source map %q not loaded", wp.Map)
		}
		if _, ok := w.maps[wp.DestMap]; !ok {
			return nil, fmt.Errorf("warp destination map %q not loaded", wp.DestMap)
		}
		src.warps = append(src.warps, wp)
	}
	for i := range cfg.NPCs {
		n := cfg.NPCs[i]
		m, ok := w.maps[n.Map]
		if !ok {
			return nil, fmt.Errorf("NPC %q: map %q not loaded", n.Name, n.Map)
		}
		if _, dup := w.npcs[n.ID]; dup || n.ID == 0 || n.ID >= w.nextID {
			return nil, fmt.Errorf("NPC %q: ID %d is duplicate or out of range", n.Name, n.ID)
		}
		for _, id := range n.Shop {
			if _, ok := game.Items[id]; !ok {
				return nil, fmt.Errorf("NPC %q sells unknown item %d", n.Name, id)
			}
		}
		w.npcs[n.ID] = &n
		m.npcs = append(m.npcs, &n)
	}
	now := time.Now()
	for i := range cfg.Spawns {
		sp := cfg.Spawns[i]
		if _, ok := w.maps[sp.Map]; !ok {
			return nil, fmt.Errorf("spawn: map %q not loaded", sp.Map)
		}
		if _, ok := game.Mobs[sp.MobID]; !ok {
			return nil, fmt.Errorf("spawn: unknown monster %d", sp.MobID)
		}
		for range sp.Count {
			w.spawnMonster(&sp, now)
		}
	}
	return w, nil
}

// HasMap is safe to call from any goroutine.
func (w *World) HasMap(name string) bool {
	_, ok := w.maps[name]
	return ok
}

// Run processes commands, walking and combat until ctx is cancelled.
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

func (w *World) tick(now time.Time) {
	for _, p := range w.players {
		w.tickPlayer(p, now)
	}
	w.tickMaps(now)
	w.tickRespawns(now)
	w.tickFloorItems(now)
	w.tickParties(now)
}

// Join places a character on its saved map (snapping to the nearest walkable
// cell), sends it EnterMap plus everything already there, then its inventory
// and status.
func (w *World) Join(name string, data game.CharData, out Outbox) (uint32, error) {
	var id uint32
	var joinErr error
	err := w.do(func() {
		key := strings.ToLower(name)
		if _, ok := w.online[key]; ok {
			joinErr = ErrAlreadyOnline
			return
		}
		m, ok := w.maps[data.Map]
		if !ok {
			joinErr = ErrUnknownMap
			return
		}
		if _, _, ok := m.grid.NearestWalkable(data.X, data.Y); !ok {
			joinErr = ErrNoWalkable
			return
		}
		data.ClampVitals()
		w.nextID++
		now := time.Now()
		p := &Player{
			unit: unit{id: w.nextID, dir: 4, speed: WalkSpeed},
			Name: name, data: data, out: out,
			nextHPRegen: now.Add(hpRegenInterval), nextSPRegen: now.Add(spRegenInterval),
		}
		id = p.id
		w.players[p.id] = p
		w.online[key] = p.id
		w.place(p, m, data.X, data.Y)
		w.partyLogin(p)
		w.sendInventory(p)
		w.sendStatus(p)
		w.sendSkills(p)
	})
	if err != nil {
		return 0, err
	}
	return id, joinErr
}

// place puts p on map m at the nearest walkable cell to (x, y) and tells it
// and everyone already there. The caller has removed p from any previous map.
func (w *World) place(p *Player, m *mapInstance, x, y int) {
	if wx, wy, ok := m.grid.NearestWalkable(x, y); ok {
		x, y = wx, wy
	}
	p.m, p.x, p.y, p.path = m, x, y, nil
	p.pending, p.pickup = nil, 0

	p.out.Send(proto.EnterMap(p.id, m.name, p.x, p.y, p.dir, p.data.Job, uint16(p.speed), p.look()))
	w.sendBuffs(p, time.Now())
	for _, wp := range m.warps {
		p.out.Send(proto.WarpPortal(wp.X, wp.Y))
	}
	for _, n := range m.npcs {
		p.out.Send(proto.NpcAppear(n.ID, n.Name, n.Sprite, n.X, n.Y, n.Dir))
	}
	for _, mob := range m.monsters {
		p.out.Send(mob.appearPacket())
		if mob.walking() {
			p.out.Send(proto.PlayerMove(mob.id, mob.x, mob.y, cells(mob.path)))
		}
	}
	appear := p.appearPacket()
	for _, other := range m.players {
		p.out.Send(other.appearPacket())
		if other.walking() {
			p.out.Send(proto.PlayerMove(other.id, other.x, other.y, cells(other.path)))
		}
		other.out.Send(appear)
	}
	m.players[p.id] = p
	w.sendRoomSigns(p)
	w.sendFloorItems(p)
}

func (p *Player) appearPacket() []byte {
	return proto.PlayerAppear(p.id, p.Name, p.data.Job, p.x, p.y, p.dir, uint16(p.speed), p.dead, p.look(), p.sitting)
}

func (p *Player) look() proto.Look {
	l := p.data.Look()
	return proto.Look{l.Weapon, l.Shield, l.HeadTop, l.HeadMid, l.HeadLow, l.Hair, l.HairColor, boolInt(l.Female)}
}

// removeFromMap takes p off its map and stops anything fighting it.
func (w *World) removeFromMap(p *Player) {
	delete(p.m.players, p.id)
	if len(p.m.players) == 0 {
		p.m.emptied(time.Now())
	}
	w.broadcast(p.m, proto.PlayerDisappear(p.id, proto.GoneOut))
	p.target = 0
	p.path = nil
	for _, mob := range p.m.monsters {
		if mob.target == p.id {
			mob.target = 0
		}
	}
}

// warpAt returns the warp covering (x, y) on m, if any.
func (m *mapInstance) warpAt(x, y int) (Warp, bool) {
	for _, wp := range m.warps {
		if wp.contains(x, y) {
			return wp, true
		}
	}
	return Warp{}, false
}

func (w *World) warp(p *Player, wp Warp) {
	w.removeFromMap(p)
	w.place(p, w.maps[wp.DestMap], wp.DestX, wp.DestY)
}

// Move starts walking a player toward (x, y), cancelling any attack.
// Unreachable targets are ignored.
func (w *World) Move(id uint32, x, y int) error {
	return w.do(func() {
		p, ok := w.players[id]
		if !ok || p.dead || p.sitting || p.room != nil {
			return // sitting players stand up first and chatters leave their room, as in RO
		}
		p.target = 0
		p.pending, p.pickup = nil, 0
		w.cancelCast(p) // walking away interrupts casting, as in RO
		if route := p.routeTo(path.Point{X: x, Y: y}, 0, MaxWalkSteps, time.Now()); route != nil {
			w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
		}
	})
}

// Leave removes a player and returns its character data, positioned on the
// cell it was last standing on (or the save point, if it was dead).
func (w *World) Leave(id uint32) (data game.CharData, ok bool) {
	err := w.do(func() {
		p, found := w.players[id]
		if !found {
			return
		}
		w.leaveRoom(p, proto.LeftByChoice)
		w.removeFromMap(p)
		delete(w.players, id)
		delete(w.online, strings.ToLower(p.Name))
		w.partyLogout(p)
		data = p.data.Clone()
		data.Map, data.X, data.Y = p.m.name, p.x, p.y
		if p.dead {
			save := w.savePointOf(&data)
			data.Map, data.X, data.Y = save.Map, save.X, save.Y
			reviveVitals(&data)
		}
		ok = true
	})
	return data, ok && err == nil
}

// reviveVitals sets HP/SP for a character returning from death (half of max).
func reviveVitals(c *game.CharData) {
	d := c.Derive()
	c.HP, c.SP = max(1, d.MaxHP/2), d.MaxSP/2
}

// broadcast sends a packet to every player on the map.
func (w *World) broadcast(m *mapInstance, packet []byte) {
	for _, p := range m.players {
		p.out.Send(packet)
	}
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

func chebyshev(ax, ay, bx, by int) int {
	return max(abs(ax-bx), abs(ay-by))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
