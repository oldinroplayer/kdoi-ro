package world

import (
	"fmt"
	"time"

	"kdoiro/server/internal/proto"
)

// meleeRange is a Novice's attack range in cells.
const meleeRange = 1

// Attack makes a player auto-attack a monster: it walks into range and keeps
// hitting at its attack speed until the monster dies or the player moves.
func (w *World) Attack(id, targetID uint32) error {
	return w.do(func() {
		p := w.players[id]
		mob := w.monsters[targetID]
		if p == nil || p.dead || mob == nil || mob.m != p.m {
			return
		}
		p.target = mob.id
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

	p.advance(now, func() bool {
		if wp, ok := p.m.warpAt(p.x, p.y); ok {
			w.warp(p, wp)
			return false
		}
		if mob := w.monsters[p.target]; mob != nil && chebyshev(p.x, p.y, mob.x, mob.y) <= meleeRange {
			w.halt(&p.unit)
			return false
		}
		return true
	})

	if p.target == 0 {
		return
	}
	mob := w.monsters[p.target]
	if mob == nil || mob.m != p.m {
		p.target = 0
		return
	}
	if chebyshev(p.x, p.y, mob.x, mob.y) <= meleeRange {
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
	if route := p.routeTo(mob.pos(), meleeRange, maxChaseSteps, now); route != nil {
		w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
	} else if !p.walking() {
		p.target = 0
		p.out.Send(proto.Message("That's too far away."))
	}
}

func (w *World) regen(p *Player, now time.Time) {
	d := p.data.Derive()
	changed := false
	if !now.Before(p.nextHPRegen) {
		p.nextHPRegen = now.Add(hpRegenInterval)
		if p.data.HP < d.MaxHP {
			p.data.HP = min(p.data.HP+p.data.RegenHP(), d.MaxHP)
			changed = true
		}
	}
	if !now.Before(p.nextSPRegen) {
		p.nextSPRegen = now.Add(spRegenInterval)
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
	w.broadcast(p.m, proto.Damage(p.id, mob.id, hit.Damage, flags, p.data.AttackMotion(), hurt))
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
	w.broadcast(mob.m, proto.Damage(mob.id, p.id, hit.Damage, flags, mob.def.AttackMotion, hurt))
	if hit.Miss {
		return
	}
	p.data.HP -= hit.Damage
	if p.data.HP <= 0 {
		w.killPlayer(p)
		return
	}
	w.sendStatus(p)
}

func (w *World) killPlayer(p *Player) {
	p.data.HP = 0
	p.dead = true
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
