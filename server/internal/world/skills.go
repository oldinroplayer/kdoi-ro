package world

import (
	"errors"
	"slices"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

// cast is a skill on its way: waiting for the player to walk into range
// (Player.pending) or being cast (Player.casting). It takes effect when the
// cast time is up.
type cast struct {
	skill  *game.SkillDef
	level  int
	target uint32 // a monster for enemy skills, a player for self/friend skills
	until  time.Time
	resume uint32 // monster to go back to auto-attacking afterwards
	// onMonster: a friend skill aimed at a monster (Heal on the undead).
	onMonster bool
}

// LearnSkill spends a skill point on a skill.
func (w *World) LearnSkill(id uint32, skillID int) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if err := p.data.LearnSkill(skillID); err != nil {
			p.out.Send(proto.Message(skillMessage(err)))
			return
		}
		w.sendSkills(p)
		w.sendStatus(p) // passives change derived stats
	})
}

// UseSkill starts using a skill. level 0 means the highest learned level.
// Self skills ignore targetID; friend skills with targetID 0 target the user.
func (w *World) UseSkill(id uint32, skillID, level int, targetID uint32) error {
	return w.do(func() {
		p := w.players[id]
		if p == nil {
			return
		}
		if err := w.startSkill(p, skillID, level, targetID, time.Now()); err != nil {
			p.out.Send(proto.Message(skillMessage(err)))
		}
	})
}

var (
	errSkillBusy   = errors.New("still recovering from the last skill")
	errCantUseNow  = errors.New("can't use skills right now")
	errUnsupported = errors.New("that skill isn't available yet")
	errWrongWeapon = errors.New("wrong weapon for this skill")
	errNoZeny      = errors.New("not enough zeny")
	errBadTarget   = errors.New("invalid target")
	errNotUndead   = errors.New("heal only hurts undead monsters")
)

// skillReach is how far (in cells) a skill reaches: a negative skill range is
// relative to the weapon's reach, as in eAthena.
func skillReach(def *game.SkillDef, weaponReach int) int {
	if def.Range < 0 {
		return weaponReach + (-def.Range - 1)
	}
	return def.Range
}

func (w *World) startSkill(p *Player, skillID, level int, targetID uint32, now time.Time) error {
	def, ok := game.Skills[skillID]
	if !ok || !p.data.InTree(def) {
		return game.ErrUnknownSkill
	}
	learned := p.data.SkillLevel(skillID)
	if learned == 0 {
		return game.ErrNotLearned
	}
	if level <= 0 || level > learned {
		level = learned
	}
	if def.Target == game.TargetPassive || def.Target == game.TargetGround {
		return errUnsupported
	}
	if p.dead || p.sitting || p.casting != nil || p.room != nil {
		return errCantUseNow
	}
	if now.Before(p.nextSkill) {
		return errSkillBusy
	}
	if p.data.SP < def.SkillSP(level) {
		return game.ErrNotEnoughSP
	}
	if !def.UsableWith(p.data.Derive().WeaponType) {
		return errWrongWeapon
	}
	if shootsArrow(p, def) {
		if _, _, ok := p.data.Arrow(); !ok {
			return game.ErrNoArrows
		}
	}
	if p.data.Zeny < def.SkillZeny(level) {
		return errNoZeny
	}

	c := &cast{skill: def, level: level, target: p.id, resume: p.target}
	switch def.Target {
	case game.TargetEnemy:
		mob := w.monsters[targetID]
		if mob == nil || mob.m != p.m {
			return errBadTarget
		}
		c.target = mob.id
	case game.TargetFriend:
		if other := w.players[targetID]; other != nil && other.m == p.m && !other.dead {
			c.target = other.id
		} else if mob := w.monsters[targetID]; mob != nil && mob.m == p.m && def.ID == game.SkillHeal {
			// Heal burns the undead, as in RO; it does nothing to anything else.
			if mob.def.Element != game.ElemUndead {
				return errNotUndead
			}
			c.target, c.onMonster = mob.id, true
		}
	}
	p.target = 0 // a skill replaces auto-attacking
	p.pending, p.pickup = nil, 0

	if !w.skillInRange(p, c) {
		// Walk into range first, then cast (like clicking a far monster).
		p.pending = c
		p.nextRepath = time.Time{}
		return nil
	}
	w.beginCast(p, c, now)
	return nil
}

// skillTarget returns where a cast's target stands, or false if it's gone.
func (w *World) skillTarget(p *Player, c *cast) (x, y int, ok bool) {
	if c.skill.Target == game.TargetEnemy || c.onMonster {
		if mob := w.monsters[c.target]; mob != nil && mob.m == p.m {
			return mob.x, mob.y, true
		}
		return 0, 0, false
	}
	if t := w.players[c.target]; t != nil && t.m == p.m && !t.dead {
		return t.x, t.y, true
	}
	return 0, 0, false
}

func (w *World) skillInRange(p *Player, c *cast) bool {
	x, y, ok := w.skillTarget(p, c)
	return ok && chebyshev(p.x, p.y, x, y) <= skillReach(c.skill, p.data.Derive().Range)
}

// pendingInRange stops the walk toward a pending skill's target once in range.
func (w *World) pendingInRange(p *Player) bool {
	return p.pending != nil && w.skillInRange(p, p.pending)
}

// tickPendingSkill walks toward a pending skill's target and casts in range.
func (w *World) tickPendingSkill(p *Player, now time.Time) {
	c := p.pending
	x, y, ok := w.skillTarget(p, c)
	if !ok || p.dead {
		p.pending = nil
		return
	}
	reach := skillReach(c.skill, p.data.Derive().Range)
	if chebyshev(p.x, p.y, x, y) <= reach {
		p.pending = nil
		w.halt(&p.unit)
		if err := w.beginCastChecked(p, c, now); err != nil {
			p.out.Send(proto.Message(skillMessage(err)))
		}
		return
	}
	if now.Before(p.nextRepath) {
		return
	}
	p.nextRepath = now.Add(400 * time.Millisecond)
	if route := p.routeTo(path.Point{X: x, Y: y}, reach, maxChaseSteps, now); route != nil {
		w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
	} else if !p.walking() {
		p.pending = nil
		p.out.Send(proto.Message("The target is too far away."))
	}
}

// beginCastChecked re-checks what may have changed while walking.
func (w *World) beginCastChecked(p *Player, c *cast, now time.Time) error {
	switch {
	case p.sitting || p.casting != nil:
		return errCantUseNow
	case now.Before(p.nextSkill):
		return errSkillBusy
	case p.data.SP < c.skill.SkillSP(c.level):
		return game.ErrNotEnoughSP
	}
	w.beginCast(p, c, now)
	return nil
}

// beginCast faces the target and starts the cast (or uses the skill at once).
func (w *World) beginCast(p *Player, c *cast, now time.Time) {
	w.halt(&p.unit)
	if x, y, ok := w.skillTarget(p, c); ok && (x != p.x || y != p.y) {
		p.dir = dirFor(x-p.x, y-p.y)
	}
	ms := c.skill.SkillCast(c.level)
	if ms == 0 {
		w.finishSkill(p, c, now)
		return
	}
	c.until = now.Add(time.Duration(ms) * time.Millisecond)
	p.casting = c
	w.castSensed(p, now)
	w.broadcast(p.m, proto.SkillCast(p.id, c.target, c.skill.ID, ms))
}

// tickCasting finishes a cast once its time is up.
func (w *World) tickCasting(p *Player, now time.Time) {
	if c := p.casting; c != nil && !now.Before(c.until) {
		p.casting = nil
		w.finishSkill(p, c, now)
	}
}

// cancelCast interrupts a cast in progress (moving, being hit, knocked out...).
func (w *World) cancelCast(p *Player) {
	if p.casting == nil {
		return
	}
	p.casting = nil
	w.broadcast(p.m, proto.SkillCast(p.id, p.id, 0, 0)) // skill 0 = cast cancelled
}

// finishSkill pays the costs and applies the skill's effect. The effects use
// the pre-renewal formulas; see game/skillrules.go.
func (w *World) finishSkill(p *Player, c *cast, now time.Time) {
	def, lv := c.skill, c.level
	sp, zeny := def.SkillSP(lv), def.SkillZeny(lv)
	if p.dead || p.data.SP < sp || p.data.Zeny < zeny {
		return
	}
	x, y, ok := w.skillTarget(p, c)
	// A target that walked a little out of range while the cast finished still
	// gets hit (RO is lenient too); one that's gone or far away doesn't.
	if !ok || chebyshev(p.x, p.y, x, y) > skillReach(def, p.data.Derive().Range)+2 {
		p.out.Send(proto.Message(skillMessage(errBadTarget)))
		return
	}
	if shootsArrow(p, def) && !w.useArrow(p) {
		return
	}
	p.data.SP -= sp
	p.data.Zeny -= zeny
	p.nextSkill = now.Add(time.Duration(def.SkillDelay(lv)) * time.Millisecond)
	w.broadcast(p.m, proto.SkillUse(p.id, c.target, def.ID, lv))

	motion := p.data.AttackMotion()
	mob := w.monsters[c.target]
	// Physical skills hit with their own element, or the weapon's (neutral) when
	// skill_db gives none.
	physical := func(pct, flat, hitBonus int) {
		hit := p.data.SkillAttackMob(mob.def, w.rng, pct, flat, hitBonus, def.Element)
		p.nextAttack = now.Add(time.Duration(p.data.AttackDelay()) * time.Millisecond)
		w.damageMonster(p, mob, hit, def.ID, 1, motion, now)
	}

	switch def.ID {
	case game.SkillFirstAid:
		w.heal(p, game.FirstAidHeal)
	case game.SkillBash:
		physical(100+30*lv, 0, 5*lv)
	case game.SkillMammonite:
		physical(100+50*lv, 0, 0)
	case game.SkillEnvenom:
		physical(100, 15*lv, 0)
	case game.SkillDoubleStrafe:
		// Two arrows, each rolled separately.
		hit := p.data.SkillAttackMob(mob.def, w.rng, 100+10*lv, 0, 0, def.Element)
		if second := p.data.SkillAttackMob(mob.def, w.rng, 100+10*lv, 0, 0, def.Element); !second.Miss {
			if hit.Miss {
				hit = second
			} else {
				hit.Damage += second.Damage
			}
		}
		p.nextAttack = now.Add(time.Duration(p.data.AttackDelay()) * time.Millisecond)
		w.damageMonster(p, mob, hit, def.ID, 2, motion, now)
	case game.SkillColdBolt, game.SkillFireBolt, game.SkillLightningBolt:
		// One bolt per level, each a separate magic hit; bolts never miss.
		hits := def.SkillHits(lv)
		var hit game.Hit
		for range hits {
			hit.Damage += p.data.MagicAttackMob(mob.def, w.rng, def.Element).Damage
		}
		w.damageMonster(p, mob, hit, def.ID, hits, 0, now)
	case game.SkillMagnumBreak:
		w.magnumBreak(p, lv, motion, now)
	case game.SkillHeal:
		if c.onMonster && mob != nil {
			hit := game.Hit{Damage: p.data.HealDamage(mob.def, lv)}
			w.damageMonster(p, mob, hit, def.ID, 1, 0, now)
		} else if t := w.players[c.target]; t != nil {
			w.heal(t, p.data.HealAmount(lv))
		}
	case game.SkillIncAgi, game.SkillBlessing:
		if t := w.players[c.target]; t != nil {
			w.applyBuff(t, def.ID, lv, def.SkillDuration(lv), now)
		}
	}
	w.sendStatus(p)

	// Fighters go back to swinging at what they were attacking.
	if c.resume != 0 && w.monsters[c.resume] != nil {
		p.target = c.resume
		p.nextRepath = time.Time{}
	}
}

// magnumBreakRadius is the fire burst's reach around the caster (5x5 cells).
const magnumBreakRadius = 2

// magnumBreak hits every monster around the caster with fire and knocks them back.
func (w *World) magnumBreak(p *Player, lv, motion int, now time.Time) {
	var near []*Monster
	for _, mob := range p.m.monsters {
		if chebyshev(p.x, p.y, mob.x, mob.y) <= magnumBreakRadius {
			near = append(near, mob)
		}
	}
	slices.SortFunc(near, func(a, b *Monster) int { return int(a.id) - int(b.id) })
	knockback := game.Skills[game.SkillMagnumBreak].Knockback
	for _, mob := range near {
		hit := p.data.SkillAttackMob(mob.def, w.rng, 100+20*lv, 0, 10*lv, game.ElemFire)
		w.damageMonster(p, mob, hit, game.SkillMagnumBreak, 1, motion, now)
		if !hit.Miss && w.monsters[mob.id] != nil {
			w.knockBack(&mob.unit, p.x, p.y, knockback)
		}
	}
	p.nextAttack = now.Add(time.Duration(p.data.AttackDelay()) * time.Millisecond)
}

// knockBack pushes a unit up to cells away from (fromX, fromY), stopping at walls.
func (w *World) knockBack(u *unit, fromX, fromY, cells int) {
	dx, dy := sign(u.x-fromX), sign(u.y-fromY)
	if dx == 0 && dy == 0 {
		dx = -1 // standing on the caster: pushed the way RO does (west)
	}
	u.path = nil
	for range cells {
		if !u.m.grid.Walkable(u.x+dx, u.y+dy) {
			break
		}
		u.x, u.y = u.x+dx, u.y+dy
	}
	w.broadcast(u.m, proto.PlayerStop(u.id, u.x, u.y))
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// heal restores HP to a player and shows the number.
func (w *World) heal(t *Player, amount int) {
	before := t.data.HP
	t.data.HP = min(t.data.HP+amount, t.data.Derive().MaxHP)
	w.broadcast(t.m, proto.Heal(t.id, t.data.HP-before))
	w.sendStatus(t)
}

// applyBuff starts (or refreshes) a buff on a player for ms milliseconds.
func (w *World) applyBuff(t *Player, skillID, level, ms int, now time.Time) {
	if t.data.Buffs == nil {
		t.data.Buffs = map[int]int{}
	}
	if t.buffEnds == nil {
		t.buffEnds = map[int]time.Time{}
	}
	t.data.Buffs[skillID] = level
	t.buffEnds[skillID] = now.Add(time.Duration(ms) * time.Millisecond)
	t.out.Send(proto.BuffIcon(t.id, skillID, level, ms))
	w.updateSpeed(t)
	w.sendStatus(t)
}

// tickBuffs ends buffs whose time is up.
func (w *World) tickBuffs(p *Player, now time.Time) {
	ended := false
	for id, end := range p.buffEnds {
		if !now.Before(end) {
			w.endBuff(p, id)
			ended = true
		}
	}
	if ended {
		w.sendStatus(p)
	}
}

func (w *World) endBuff(p *Player, skillID int) {
	delete(p.buffEnds, skillID)
	delete(p.data.Buffs, skillID)
	p.out.Send(proto.BuffIcon(p.id, skillID, 0, 0))
	w.updateSpeed(p)
}

// clearBuffs ends every buff (when knocked out).
func (w *World) clearBuffs(p *Player) {
	for id := range p.buffEnds {
		w.endBuff(p, id)
	}
}

// sendBuffs re-sends the icons of running buffs (after a map change).
func (w *World) sendBuffs(p *Player, now time.Time) {
	for id, end := range p.buffEnds {
		p.out.Send(proto.BuffIcon(p.id, id, p.data.Buffs[id], int(end.Sub(now).Milliseconds())))
	}
}

// sendSkills sends the skill tree (learned levels and skill points).
func (w *World) sendSkills(p *Player) {
	var list [][2]int
	for id, def := range game.Skills {
		if p.data.InTree(def) {
			list = append(list, [2]int{id, p.data.SkillLevel(id)})
		}
	}
	slices.SortFunc(list, func(a, b [2]int) int { return a[0] - b[0] })
	p.out.Send(proto.SkillList(p.data.SkillPoints, list))
}

func skillMessage(err error) string {
	switch {
	case errors.Is(err, game.ErrNotEnoughSP):
		return "Not enough SP."
	case errors.Is(err, game.ErrNoSkillPoints):
		return "You have no skill points left."
	case errors.Is(err, game.ErrSkillMaxed):
		return "That skill is already at its maximum level."
	case errors.Is(err, game.ErrSkillLocked):
		return "Learn the required skills first."
	case errors.Is(err, game.ErrQuestSkill):
		return "That skill can't be learned with skill points."
	case errors.Is(err, game.ErrNotLearned):
		return "You haven't learned that skill."
	case errors.Is(err, errSkillBusy):
		return "You can't use a skill again yet."
	case errors.Is(err, errCantUseNow):
		return "You can't use skills right now."
	case errors.Is(err, errWrongWeapon):
		return "You need a different weapon for that skill."
	case errors.Is(err, game.ErrNoArrows):
		return arrowsMessage
	case errors.Is(err, errNoZeny):
		return "Not enough zeny."
	case errors.Is(err, errBadTarget):
		return "Invalid target."
	case errors.Is(err, errNotUndead):
		return "Heal only hurts undead monsters."
	default:
		return "You can't use that skill."
	}
}

// incAgiSpeedup is how much faster Increase AGI makes walking (eAthena 2004:
// walk time per cell minus 25%).
const incAgiSpeedup = 25

// walkSpeed is a player's ms per straight cell, with buffs applied.
func walkSpeed(c *game.CharData) int {
	s := WalkSpeed
	if c.Buffs[game.SkillIncAgi] > 0 {
		s -= s * incAgiSpeedup / 100
	}
	return s
}

// updateSpeed applies buff changes to a player's walk speed and tells the map.
// A walk in progress continues at the new pace from its next step.
func (w *World) updateSpeed(p *Player) {
	if s := walkSpeed(&p.data); s != p.speed {
		p.speed = s
		w.broadcast(p.m, proto.PlayerSpeed(p.id, s))
	}
}
