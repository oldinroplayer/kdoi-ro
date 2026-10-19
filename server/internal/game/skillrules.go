package game

// Skill effects that live in the game rules: passives, buffs, damage and
// healing formulas, and shop price changes (RO pre-renewal values).

// statBonus is the stat bonus from the job level, passive skills and buffs.
func (c *CharData) statBonus() [NumStats]int {
	b := Job(c.Job).Bonus(c.JobLevel)
	b[DEX] += c.SkillLevel(SkillOwlsEye)
	if lv := c.Buffs[SkillIncAgi]; lv > 0 {
		b[AGI] += 2 + lv
	}
	if lv := c.Buffs[SkillBlessing]; lv > 0 {
		b[STR] += lv
		b[DEX] += lv
		b[INT] += lv
	}
	return b
}

// applyPassives adds passive skills to derived stats.
func (c *CharData) applyPassives(d *Derived) {
	if d.WeaponType == WeaponDagger || d.WeaponType == WeaponSword {
		d.Atk2 += 4 * c.SkillLevel(SkillSwordMastery)
	}
	d.Flee += 3 * c.SkillLevel(SkillImproveDodge)
	if lv := c.SkillLevel(SkillVulturesEye); lv > 0 {
		d.Hit += lv
		if d.WeaponType == WeaponBow {
			d.Range += lv
		}
	}
}

// skillRegenHP/SP are the extra regen from Increase HP/SP Recovery per tick.
func (c *CharData) skillRegenHP(maxHP int) int {
	lv := c.SkillLevel(SkillHPRecovery)
	return 5*lv + maxHP*lv/500
}

func (c *CharData) skillRegenSP(maxSP int) int {
	lv := c.SkillLevel(SkillSPRecovery)
	return 3*lv + maxSP*lv/500
}

// SkillAttackMob resolves a physical attack skill of an element: damage is
// pct% of a normal hit plus flat, and accuracy gets hitBonus percent.
func (c *CharData) SkillAttackMob(m *MobDef, r Rand, pct, flat, hitBonus, element int) Hit {
	return c.attackMob(m, r, pct, flat, hitBonus, element)
}

// MagicAttackMob is one hit of a bolt of an element: MATK reduced by the
// target's MDEF (percent) and INT, at least 1, then the element's effect.
func (c *CharData) MagicAttackMob(m *MobDef, r Rand, element int) Hit {
	if m.Plant {
		return Hit{Damage: 1}
	}
	d := c.Derive()
	dmg := between(r, d.MatkMin, d.MatkMax)
	dmg = dmg*(100-min(m.Mdef, 99))/100 - m.Stats[INT]
	return Hit{Damage: applyElement(max(dmg, 1), m.ElementFix(element))}
}

// HealDamage is what Heal does to an undead-element monster: half the heal,
// as holy damage (RO pre-renewal). 0 for anything not undead.
func (c *CharData) HealDamage(m *MobDef, level int) int {
	if m.Element != ElemUndead {
		return 0
	}
	return applyElement(c.HealAmount(level)/2, m.ElementFix(ElemHoly))
}

// DivineProtection is the damage Divine Protection blocks from undead and
// demon monsters: 3 per level (RO pre-renewal).
func (c *CharData) DivineProtection(m *MobDef) int {
	if m.Race != RaceUndead && m.Race != RaceDemon {
		return 0
	}
	return 3 * c.SkillLevel(SkillDivineProt)
}

// HealAmount is Heal's pre-renewal formula: (BaseLv + INT) / 8 * (4 + 8 * level).
func (c *CharData) HealAmount(level int) int {
	return (c.BaseLevel + c.EffectiveStats()[INT]) / 8 * (4 + 8*level)
}

// shopRate is Discount/Overcharge: 7, 9 ... 23, 24 percent.
func shopRate(level int) int {
	if level <= 0 {
		return 0
	}
	return min(5+2*level, 24)
}

// BuyPrice applies Discount to a shop price.
func (c *CharData) BuyPrice(price int) int {
	return max(price*(100-shopRate(c.SkillLevel(SkillDiscount)))/100, 1)
}

// SellPrice is what a shop pays for an item: half its price, plus Overcharge.
func (c *CharData) SellPrice(d ItemDef) int {
	return d.SellPrice() * (100 + shopRate(c.SkillLevel(SkillOvercharge))) / 100
}

// SellRate is the Overcharge percentage shops pay (100 = plain half price).
func (c *CharData) SellRate() int { return 100 + shopRate(c.SkillLevel(SkillOvercharge)) }

// DoubleAttackChance is the percent chance a dagger hit strikes twice.
func (c *CharData) DoubleAttackChance(weaponType int) int {
	if weaponType != WeaponDagger {
		return 0
	}
	return 5 * c.SkillLevel(SkillDoubleAttack)
}
