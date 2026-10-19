package game

// Rand is the randomness combat needs; *math/rand/v2.Rand satisfies it.
type Rand interface {
	IntN(n int) int
}

// between returns a random int in [lo, hi].
func between(r Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.IntN(hi-lo+1)
}

// Hit is the outcome of one attack.
type Hit struct {
	Damage int
	Miss   bool
	Crit   bool
}

// hitChance is RO's pre-renewal 80 + HIT - FLEE, kept within 5-95%.
func hitChance(hit, flee int) int {
	return min(max(80+hit-flee, 5), 95)
}

// AttackMob resolves a player's melee attack on a monster.
func (c *CharData) AttackMob(m *MobDef, r Rand) Hit {
	return c.attackMob(m, r, 100, 0, 0)
}

// attackMob is a physical hit scaled to pct% plus flat damage, with hitBonus
// added to the hit chance (skills like Bash are more accurate).
func (c *CharData) attackMob(m *MobDef, r Rand, pct, flat, hitBonus int) Hit {
	d := c.Derive()
	if m.Plant {
		return Hit{Damage: 1}
	}

	// Critical hits skip the flee check and DEF, and roll maximum weapon damage.
	critChance := max(d.Crit-m.Stats[LUK]/5, 0) // percent
	crit := r.IntN(100) < critChance
	if !crit && r.IntN(100) >= min(hitChance(d.Hit, m.Level+m.Stats[AGI])+hitBonus, 100) {
		return Hit{Miss: true}
	}

	// Weapon damage rolls between a DEX-based minimum and the weapon's ATK.
	weapon := d.Atk2
	weaponMin := min(c.EffectiveStats()[DEX], weapon)
	dmg := d.Atk
	if crit {
		dmg += weapon
	} else {
		dmg += between(r, weaponMin, weapon)
		// Hard DEF reduces by percentage, then VIT soft DEF subtracts.
		dmg = dmg*(100-min(m.Def, 99))/100 - m.Stats[VIT]
	}
	dmg = dmg*pct/100 + flat
	return Hit{Damage: max(dmg, 1), Crit: crit}
}

// AttackPlayer resolves a monster's attack on a player.
func (m *MobDef) AttackPlayer(c *CharData, r Rand) Hit {
	d := c.Derive()
	if r.IntN(100) >= hitChance(m.Level+m.Stats[DEX], d.Flee) {
		return Hit{Miss: true}
	}
	dmg := between(r, m.Atk1, m.Atk2)
	dmg = dmg*(100-min(d.Def, 99))/100 - d.Def2
	return Hit{Damage: max(dmg, 1)}
}

// AttackDelay is the time between a player's attacks, from ASPD (pre-renewal:
// amotion = (200 - ASPD) * 10 ms, and attacks repeat every 2 * amotion).
func (c *CharData) AttackDelay() int { return 2 * c.AttackMotion() }

// AttackMotion is the length of the player's attack animation in ms.
func (c *CharData) AttackMotion() int { return (200 - c.Derive().Aspd) * 10 }

// Regen returns HP and SP restored by one natural regeneration tick
// (every 6 s for HP and 8 s for SP, standing).
func (c *CharData) RegenHP() int {
	d := c.Derive()
	return max(1, d.MaxHP/200) + c.EffectiveStats()[VIT]/5 + c.skillRegenHP(d.MaxHP)
}

func (c *CharData) RegenSP() int {
	d := c.Derive()
	return 1 + d.MaxSP/100 + c.EffectiveStats()[INT]/6 + c.skillRegenSP(d.MaxSP)
}
