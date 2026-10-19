package world

import (
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

// Spawn places Count monsters of MobID on Map, like an rAthena "monster" line:
// within SpanX/SpanY cells of (X, Y), or anywhere on the map when X and Y are 0.
type Spawn struct {
	Map                string
	X, Y, SpanX, SpanY int
	MobID              int
	Count              int
	// Respawn time after death is minRespawn + Delay + rand(Variance).
	Delay, Variance time.Duration
}

// minRespawn is the shortest time before a killed monster comes back.
const minRespawn = 5 * time.Second

// wanderRadius is how far an idle monster strolls from where it stands.
const wanderRadius = 5

type Monster struct {
	unit
	def   *game.MobDef
	spawn *Spawn
	hp    int

	target     uint32 // player it's fighting back against
	nextAttack time.Time
	nextThink  time.Time // next wander or chase re-path
	busyUntil  time.Time // flinching after being hit
	damage     map[uint32]int
}

type respawn struct {
	at    time.Time
	spawn *Spawn
}

func (mob *Monster) appearPacket() []byte {
	return proto.MonsterAppear(mob.id, mob.def.ID, mob.def.Name, mob.def.Sprite, mob.x, mob.y, mob.dir, mob.speed)
}

// spawnMonster creates a monster at a random walkable cell of its spawn area.
func (w *World) spawnMonster(sp *Spawn, now time.Time) {
	m := w.maps[sp.Map]
	x, y, ok := w.spawnCell(m, sp)
	if !ok {
		return
	}
	def := game.Mobs[sp.MobID]
	w.nextID++
	mob := &Monster{
		unit:      unit{id: w.nextID, m: m, x: x, y: y, dir: uint8(w.rng.IntN(8)), speed: def.WalkSpeed},
		def:       def,
		spawn:     sp,
		hp:        def.HP,
		damage:    map[uint32]int{},
		nextThink: now.Add(time.Duration(w.rng.IntN(5000)) * time.Millisecond),
	}
	w.monsters[mob.id] = mob
	m.monsters[mob.id] = mob
	w.broadcast(m, mob.appearPacket())
}

func (w *World) spawnCell(m *mapInstance, sp *Spawn) (int, int, bool) {
	for range 100 {
		var x, y int
		if sp.X == 0 && sp.Y == 0 {
			x, y = w.rng.IntN(m.grid.Width), w.rng.IntN(m.grid.Height)
		} else {
			x = sp.X - sp.SpanX + w.rng.IntN(2*sp.SpanX+1)
			y = sp.Y - sp.SpanY + w.rng.IntN(2*sp.SpanY+1)
		}
		if m.grid.Walkable(x, y) {
			return x, y, true
		}
	}
	return 0, 0, false
}

func (w *World) tickRespawns(now time.Time) {
	kept := w.respawns[:0]
	for _, r := range w.respawns {
		if now.Before(r.at) {
			kept = append(kept, r)
			continue
		}
		w.spawnMonster(r.spawn, now)
	}
	w.respawns = kept
}

func (w *World) tickMonster(mob *Monster, now time.Time) {
	mob.advance(now, func() bool {
		if p := w.players[mob.target]; p != nil && chebyshev(mob.x, mob.y, p.x, p.y) <= mob.def.Range {
			w.halt(&mob.unit)
			return false
		}
		return true
	})
	if now.Before(mob.busyUntil) {
		return
	}

	if mob.target != 0 {
		p := w.players[mob.target]
		if p == nil || p.dead || p.m != mob.m || chebyshev(mob.x, mob.y, p.x, p.y) > mob.def.ChaseRange {
			mob.target = 0
			return
		}
		if chebyshev(mob.x, mob.y, p.x, p.y) <= mob.def.Range {
			w.halt(&mob.unit)
			if !now.Before(mob.nextAttack) {
				w.monsterHits(mob, p, now)
			}
			return
		}
		if mob.def.Mobile && !now.Before(mob.nextThink) {
			mob.nextThink = now.Add(400 * time.Millisecond)
			if route := mob.routeTo(p.pos(), mob.def.Range, MaxWalkSteps, now); route != nil {
				w.broadcast(mob.m, proto.PlayerMove(mob.id, mob.x, mob.y, cells(route)))
			}
		}
		return
	}

	// Idle: stroll somewhere nearby every few seconds.
	if mob.def.Mobile && !mob.walking() && !now.Before(mob.nextThink) {
		mob.nextThink = now.Add(3*time.Second + time.Duration(w.rng.IntN(5000))*time.Millisecond)
		for range 8 {
			dest := path.Point{
				X: mob.x - wanderRadius + w.rng.IntN(2*wanderRadius+1),
				Y: mob.y - wanderRadius + w.rng.IntN(2*wanderRadius+1),
			}
			if !mob.m.grid.Walkable(dest.X, dest.Y) {
				continue
			}
			if route := mob.routeTo(dest, 0, MaxWalkSteps, now); route != nil {
				w.broadcast(mob.m, proto.PlayerMove(mob.id, mob.x, mob.y, cells(route)))
				break
			}
		}
	}
}

// killMonster removes a dead monster, shares out EXP by damage dealt, drops
// its loot on the ground (whoever did the most damage gets first pick), and
// schedules its respawn.
func (w *World) killMonster(mob *Monster, now time.Time) {
	delete(w.monsters, mob.id)
	delete(mob.m.monsters, mob.id)
	w.broadcast(mob.m, proto.PlayerDisappear(mob.id, proto.GoneDied))

	total := 0
	for _, d := range mob.damage {
		total += d
	}
	var looter *Player
	most := 0
	for pid, d := range mob.damage {
		p := w.players[pid]
		if p == nil || p.dead || p.m != mob.m || d <= 0 {
			continue
		}
		base, job := mob.def.BaseExp*d/total, mob.def.JobExp*d/total
		if mob.def.BaseExp > 0 {
			base = max(base, 1)
		}
		if mob.def.JobExp > 0 {
			job = max(job, 1)
		}
		w.giveExp(p, base, job)
		if d > most {
			looter, most = p, d
		}
	}
	var owner uint32
	if looter != nil {
		owner = looter.id
	}
	w.dropLoot(mob, owner, now)
	for _, p := range mob.m.players {
		if p.target == mob.id {
			p.target = 0
		}
	}

	delay := minRespawn + mob.spawn.Delay
	if mob.spawn.Variance > 0 {
		delay += time.Duration(w.rng.Int64N(int64(mob.spawn.Variance)))
	}
	w.respawns = append(w.respawns, respawn{at: now.Add(delay), spawn: mob.spawn})
}

// dropLoot rolls a monster's drops onto the ground around its corpse.
func (w *World) dropLoot(mob *Monster, owner uint32, now time.Time) {
	for _, d := range mob.def.Drops {
		if w.rng.IntN(10000) < d.Rate {
			w.dropItem(mob.m, mob.x, mob.y, d.ItemID, 1, owner, now)
		}
	}
}
