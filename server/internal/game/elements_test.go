package game

import "testing"

// fixedRand always rolls the same value (0 = best hit chance, low damage roll).
type fixedRand int

func (r fixedRand) IntN(n int) int { return min(int(r), n-1) }

func TestElementTable(t *testing.T) {
	for _, c := range []struct{ atk, def, lvl, want int }{
		{ElemFire, ElemUndead, 1, 125}, // fire burns the undead
		{ElemHoly, ElemUndead, 1, 150}, // so does holy
		{ElemWater, ElemFire, 1, 150},  // water puts out fire
		{ElemFire, ElemWater, 1, 50},
		{ElemNeutral, ElemGhost, 1, 25},    // ghosts shrug off plain hits
		{ElemShadow, ElemUndead, 2, -50},   // shadow heals level 2 undead
		{ElemWind, ElemWind, 1, 25},        // wind barely hurts wind
		{ElemWind, ElemWind, 4, -50},       // and heals strong wind monsters
		{ElemNeutral, ElemNeutral, 0, 100}, // level 0 (unknown) counts as 1
	} {
		if got := AttrFix(c.atk, c.def, c.lvl); got != c.want {
			t.Errorf("AttrFix(%d, %d, %d) = %d, want %d", c.atk, c.def, c.lvl, got, c.want)
		}
	}
}

func TestMonsterTraitsLoaded(t *testing.T) {
	zombie, poring := Mobs[1015], Mobs[1002]
	if zombie == nil || zombie.Element != ElemUndead || zombie.Race != RaceUndead || !zombie.Aggressive {
		t.Fatalf("zombie traits: %+v", zombie)
	}
	if poring == nil || poring.Element != ElemWater || poring.Aggressive {
		t.Fatalf("poring traits: %+v", poring)
	}
	for id, m := range Mobs {
		if m.ElementLevel == 0 && id < 9000 { // test monsters are 9000+
			t.Errorf("monster %d (%s) has no element data", id, m.Name)
		}
	}
}

func TestElementsAndRacesInCombat(t *testing.T) {
	c := NewChar("payon", 1, 1)
	c.Stats[INT] = 50
	undead := &MobDef{Name: "Undead", HP: 100, MobTraits: MobTraits{Element: ElemUndead, ElementLevel: 1, Race: RaceUndead}}
	plain := &MobDef{Name: "Plain", HP: 100, MobTraits: MobTraits{Element: ElemNeutral, ElementLevel: 1, Race: RaceBrute}}

	// The same bolt roll does 125% to the undead as fire, 100% as neutral.
	fire := c.MagicAttackMob(undead, fixedRand(0), ElemFire).Damage
	neutral := c.MagicAttackMob(undead, fixedRand(0), ElemNeutral).Damage
	if fire != neutral*125/100 {
		t.Fatalf("fire bolt on undead %d, neutral %d", fire, neutral)
	}

	// Heal burns only the undead: half the heal, as holy (150%).
	if got, want := c.HealDamage(undead, 3), c.HealAmount(3)/2*150/100; got != want {
		t.Fatalf("heal on undead = %d, want %d", got, want)
	}
	if c.HealDamage(plain, 3) != 0 {
		t.Fatal("heal shouldn't hurt the living")
	}

	// Divine Protection only blocks undead and demon hits.
	c.Skills[SkillDivineProt] = 5
	if c.DivineProtection(undead) != 15 || c.DivineProtection(plain) != 0 {
		t.Fatalf("divine protection: %d vs undead, %d vs brute", c.DivineProtection(undead), c.DivineProtection(plain))
	}
}
