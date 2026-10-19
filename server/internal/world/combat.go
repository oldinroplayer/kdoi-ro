package world

import (
	"fmt"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/proto"
)

// Attack makes a player auto-attack a monster: it walks into range and keeps
// hitting at its attack speed until the monster dies or the player moves.
func (w *World) Attack(id, targetID uint32) error {
	return w.do(func() {
		p := w.players[id]
		mob := w.monsters[targetID]
		if p == nil || p.dead || p.sitting || p.room != nil || mob == nil || mob.m != p.m {
			return
		}
		p.target = mob.id
		p.pending, p.pickup = nil, 0
		p.nextRepath = time.Time{} // plan the approach on the next tick
	})
}

// Respawn brings a dead player back at the save point with half HP/SP.
func (w *World) Respawn(id uint32) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || !p.dead {
			return
		}
		p.dead = false
		reviveVitals(&p.data)
		w.removeFromMap(p)
		w.place(p, w.maps[w.savePoint.Map], w.savePoint.X, w.savePoint.Y)
		w.sendStatus(p)
	})
}

func (w *World) tickPlayer(p *Player, now time.Time) {
	if p.dead {
		return
	}
	w.regen(p, now)
	w.tickBuffs(p, now)
	w.tickCasting(p, now)
	reach := p.data.Derive().Range

	p.advance(now, func() bool {
		if wp, ok := p.m.warpAt(p.x, p.y); ok {
			w.warp(p, wp)
			return false
		}
		if mob := w.monsters[p.target]; mob != nil && chebyshev(p.x, p.y, mob.x, mob.y) <= reach {
			w.halt(&p.unit)
			return false
		}
		if w.pendingInRange(p) {
			w.halt(&p.unit)
			return false
		}
		if it := p.m.items[p.pickup]; it != nil && chebyshev(p.x, p.y, it.x, it.y) <= pickUpRange {
			w.halt(&p.unit)
			return false
		}
		return true
	})

	if p.pending != nil {
		w.tickPendingSkill(p, now)
		return
	}
	if p.pickup != 0 {
		w.tickPickUp(p, now)
		return
	}
	if p.target == 0 {
		return
	}
	mob := w.monsters[p.target]
	if mob == nil || mob.m != p.m {
		p.target = 0
		return
	}
	if chebyshev(p.x, p.y, mob.x, mob.y) <= reach {
		w.halt(&p.unit)
		if !now.Before(p.nextAttack) {
			w.playerHits(p, mob, now)
		}
		return
	}
	// Chase: re-plan when the monster has moved or we've stopped short.
	if now.Before(p.nextRepath) {
		return
	}
	p.nextRepath = now.Add(300 * time.Millisecond)
	if p.walking() && p.chaseTarget == mob.pos() {
		return
	}
	p.chaseTarget = mob.pos()
	if route := p.routeTo(mob.pos(), reach, maxChaseSteps, now); route != nil {
		w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
	} else if !p.walking() {
		p.target = 0
		p.out.Send(proto.Message("That's too far away."))
	}
}

// Sit sits a player down or stands it up. Sitting stops walking and attacking,
// and doubles natural HP/SP regeneration.
func (w *World) Sit(id uint32, sit bool) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || p.dead || p.sitting == sit {
			return
		}
		if sit && !p.data.CanSit() {
			p.out.Send(proto.Message("You need Basic Skill level 3 to sit."))
			return
		}
		w.cancelCast(p)
		p.sitting = sit
		if sit {
			p.target = 0
			w.halt(&p.unit)
			// Start the faster regen right away instead of waiting out a standing tick.
			now := time.Now()
			if next := now.Add(regenInterval(hpRegenInterval, true)); next.Before(p.nextHPRegen) {
				p.nextHPRegen = next
			}
			if next := now.Add(regenInterval(spRegenInterval, true)); next.Before(p.nextSPRegen) {
				p.nextSPRegen = next
			}
		}
		w.broadcast(p.m, proto.PlayerSit(p.id, sit))
	})
}

// Turn faces a player in a direction without moving it (e.g. while sitting).
// Ignored while walking, since the walk decides the facing.
func (w *World) Turn(id uint32, dir uint8) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil || p.dead || dir > 7 || p.walking() || p.dir == dir {
			return
		}
		p.dir = dir
		w.broadcast(p.m, proto.PlayerTurn(p.id, dir))
	})
}

// regenInterval is how often regen ticks: twice as often while sitting.
func regenInterval(standing time.Duration, sitting bool) time.Duration {
	if sitting {
		return standing / 2
	}
	return standing
}

func (w *World) regen(p *Player, now time.Time) {
	d := p.data.Derive()
	changed := false
	if !now.Before(p.nextHPRegen) {
		p.nextHPRegen = now.Add(regenInterval(hpRegenInterval, p.sitting))
		if p.data.HP < d.MaxHP {
			p.data.HP = min(p.data.HP+p.data.RegenHP(), d.MaxHP)
			changed = true
		}
	}
	if !now.Before(p.nextSPRegen) {
		p.nextSPRegen = now.Add(regenInterval(spRegenInterval, p.sitting))
		if p.data.SP < d.MaxSP {
			p.data.SP = min(p.data.SP+p.data.RegenSP(), d.MaxSP)
			changed = true
		}
	}
	if changed {
		w.sendStatus(p)
	}
}

func (w *World) playerHits(p *Player, mob *Monster, now time.Time) {
	p.nextAttack = now.Add(time.Duration(p.data.AttackDelay()) * time.Millisecond)
	p.dir = dirFor(mob.x-p.x, mob.y-p.y)
	hit := p.data.AttackMob(mob.def, w.rng)
	hits := 1
	// Double Attack: a dagger hit may strike twice.
	if !hit.Miss && w.rng.IntN(100) < p.data.DoubleAttackChance(p.data.Derive().WeaponType) {
		hit.Damage += p.data.AttackMob(mob.def, w.rng).Damage
		hits = 2
	}
	w.damageMonster(p, mob, hit, 0, hits, p.data.AttackMotion(), now)
}

// damageMonster applies a hit from a player (attack or skill) and shows it.
func (w *World) damageMonster(p *Player, mob *Monster, hit game.Hit, skillID, hits, motion int, now time.Time) {
	var flags uint8
	hurt := 0
	switch {
	case hit.Miss:
		flags = proto.DamageMiss
	case hit.Crit:
		flags = proto.DamageCrit
	}
	if !hit.Miss && mob.def.DamageMotion > 1 {
		hurt = mob.def.DamageMotion
	}
	w.broadcast(p.m, proto.Damage(p.id, mob.id, hit.Damage, flags, motion, hurt, skillID, hits))
	if hit.Miss {
		return
	}

	mob.hp -= hit.Damage
	mob.damage[p.id] += hit.Damage
	if mob.hp <= 0 {
		w.killMonster(mob, now)
		return
	}
	// Getting hit makes a monster flinch, and fighters turn on their attacker.
	if hurt > 0 {
		w.halt(&mob.unit)
		mob.busyUntil = now.Add(time.Duration(hurt) * time.Millisecond)
	}
	if mob.def.Fights && mob.target == 0 {
		mob.target = p.id
		mob.nextAttack = now.Add(time.Duration(hurt) * time.Millisecond)
	}
	w.callForHelp(mob, p, now)
}

// assistRange is how far (in cells) assisting monsters notice a fight.
const assistRange = 10

// callForHelp makes idle monsters of the same kind with the Assist flag join
// in when one of them is attacked (eAthena's mode flag 8).
func (w *World) callForHelp(mob *Monster, attacker *Player, now time.Time) {
	for _, other := range mob.m.monsters {
		if other == mob || other.def.ID != mob.def.ID || !other.def.Assist || !other.def.Fights || other.target != 0 {
			continue
		}
		if chebyshev(other.x, other.y, mob.x, mob.y) <= assistRange {
			other.target = attacker.id
			other.nextAttack = now
		}
	}
}

func (w *World) monsterHits(mob *Monster, p *Player, now time.Time) {
	mob.nextAttack = now.Add(time.Duration(mob.def.AttackDelay) * time.Millisecond)
	mob.dir = dirFor(p.x-mob.x, p.y-mob.y)
	hit := mob.def.AttackPlayer(&p.data, w.rng)

	var flags uint8
	hurt := playerHurtMotion
	if hit.Miss {
		flags, hurt = proto.DamageMiss, 0
	}
	w.broadcast(mob.m, proto.Damage(mob.id, p.id, hit.Damage, flags, mob.def.AttackMotion, hurt, 0, 1))
	if hit.Miss {
		return
	}
	w.cancelCast(p) // being hit interrupts casting, as in RO
	p.data.HP -= hit.Damage
	if p.data.HP <= 0 {
		w.killPlayer(p)
		return
	}
	w.sendStatus(p)
}

func (w *World) killPlayer(p *Player) {
	w.cancelCast(p)
	w.clearBuffs(p) // being knocked out ends buffs, as in RO
	w.leaveRoom(p, proto.LeftByChoice)
	p.pending = nil
	p.data.HP = 0
	p.dead = true
	p.sitting = false
	p.target = 0
	p.path = nil
	for _, mob := range p.m.monsters {
		if mob.target == p.id {
			mob.target = 0
		}
	}
	w.broadcast(p.m, proto.UnitDead(p.id))
	w.sendStatus(p)
	p.out.Send(proto.Message("You have been knocked out."))
}

// giveExp grants EXP and announces level ups.
func (w *World) giveExp(p *Player, base, job int) {
	baseUps, jobUps := p.data.AddExp(base, job)
	if baseUps > 0 {
		w.broadcast(p.m, proto.LevelUp(p.id, proto.LevelBase))
		p.out.Send(proto.Message(fmt.Sprintf("Base level up! You are now level %d.", p.data.BaseLevel)))
	}
	if jobUps > 0 {
		w.broadcast(p.m, proto.LevelUp(p.id, proto.LevelJob))
		p.out.Send(proto.Message(fmt.Sprintf("Job level up! Job level %d.", p.data.JobLevel)))
		w.sendSkills(p) // a new skill point
	}
	w.sendStatus(p)
}

// UseItem consumes a healing item.
func (w *World) UseItem(id uint32, index int) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if p.dead {
			p.out.Send(proto.Message("You can't do that while knocked out."))
			return
		}
		healed, err := p.data.UseItem(index, w.rng)
		if err != nil {
			p.out.Send(proto.Message(messageFor(err)))
			return
		}
		w.broadcast(p.m, proto.Heal(p.id, healed))
		w.sendInventory(p)
		w.sendStatus(p)
	})
}
