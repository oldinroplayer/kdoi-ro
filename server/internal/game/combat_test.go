package game

import (
	"errors"
	"math/rand/v2"
	"testing"
)

func testRand() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func TestAddExpLevelsUp(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.HP = 1
	ups, jobUps := c.AddExp(9, 10) // exactly base 1->2 and job 1->2
	if ups != 1 || jobUps != 1 || c.BaseLevel != 2 || c.JobLevel != 2 || c.BaseExp != 0 || c.JobExp != 0 {
		t.Fatalf("after first level: %+v", c)
	}
	if c.StatusPoints != StartPoints+StatusPointsFor(2) {
		t.Fatalf("status points = %d", c.StatusPoints)
	}
	if d := c.Derive(); c.HP != d.MaxHP {
		t.Fatalf("level up should heal: hp %d/%d", c.HP, d.MaxHP)
	}
	// A big chunk crosses several levels and keeps the remainder.
	ups, _ = c.AddExp(16+25+5, 0)
	if ups != 2 || c.BaseLevel != 4 || c.BaseExp != 5 {
		t.Fatalf("multi-level: ups=%d %+v", ups, c)
	}
}

func TestExpStopsAtCaps(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.JobLevel = Job(JobNovice).MaxJobLevel()
	c.AddExp(0, 1000)
	if c.JobLevel != Job(JobNovice).MaxJobLevel() || c.JobExp != 0 || c.JobNext() != 0 {
		t.Fatalf("job cap: %+v", c)
	}
}

func TestPlantsTakeOneDamage(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.Stats[STR] = 99
	if h := c.AttackMob(Mobs[1081], testRand()); h.Damage != 1 || h.Miss {
		t.Fatalf("plant hit: %+v", h)
	}
}

func TestDefenceReducesDamage(t *testing.T) {
	r := testRand()
	weak, tough := NewChar("morocc", 0, 0), NewChar("morocc", 0, 0)
	tough.Stats[VIT] = 9
	picky := Mobs[1049]
	sum := func(c *CharData) (total int) {
		for range 500 {
			total += picky.AttackPlayer(c, r).Damage
		}
		return total
	}
	if a, b := sum(&weak), sum(&tough); b >= a {
		t.Fatalf("VIT should reduce damage taken: %d vs %d", a, b)
	}
}

func TestWeaponIncreasesDamage(t *testing.T) {
	r := testRand()
	bare, armed := NewChar("morocc", 0, 0), NewChar("morocc", 0, 0)
	armed.add(1201, 1)
	armed.Equip(armed.Inventory[0].Index)
	sum := func(c *CharData) (total int) {
		for range 500 {
			total += c.AttackMob(Mobs[1113], r).Damage
		}
		return total
	}
	if a, b := sum(&bare), sum(&armed); b <= a {
		t.Fatalf("knife should hit harder: %d vs %d", a, b)
	}
	if armed.AttackDelay() <= 0 || armed.AttackMotion() <= 0 {
		t.Fatal("attack timings must be positive")
	}
}

func TestUseItemHeals(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.add(501, 2)
	c.add(909, 1)
	c.HP = 1
	healed, err := c.UseItem(c.Inventory[0].Index, testRand())
	if err != nil || healed < 39 || c.HP != 1+healed || c.amountOf(501) != 1 {
		t.Fatalf("potion: healed=%d err=%v hp=%d", healed, err, c.HP)
	}
	// Healing never exceeds max HP.
	c.HP = c.Derive().MaxHP - 1
	if healed, _ := c.UseItem(c.Inventory[0].Index, testRand()); healed != 1 || c.amountOf(501) != 0 {
		t.Fatalf("capped heal = %d", healed)
	}
	if _, err := c.UseItem(c.Inventory[0].Index, testRand()); !errors.Is(err, ErrNotUsable) {
		t.Fatalf("using Jellopy: %v", err)
	}
}

func TestSellAndLoot(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	for range 5 {
		if err := c.PickUp(909, 1); err != nil {
			t.Fatal(err)
		}
	}
	c.add(1201, 1)
	c.Equip(c.Inventory[1].Index)
	jellopy, knife := c.Inventory[0].Index, c.Inventory[1].Index

	if _, err := c.Sell([]Sale{{knife, 1}}); !errors.Is(err, ErrEquipped) {
		t.Fatalf("selling worn knife: %v", err)
	}
	if _, err := c.Sell([]Sale{{jellopy, 6}}); !errors.Is(err, ErrNoSuchItem) {
		t.Fatalf("selling too many: %v", err)
	}
	earned, err := c.Sell([]Sale{{jellopy, 5}})
	if err != nil || earned != 15 || c.Zeny != StartZeny+15 || c.amountOf(909) != 0 {
		t.Fatalf("sell: earned=%d err=%v zeny=%d", earned, err, c.Zeny)
	}

	c.Stats[STR] = 1
	c.add(1010, 101) // 101 Phracon = 2020 weight, over the 2030 limit with the knife
	if err := c.PickUp(1010, 1); !errors.Is(err, ErrOverweight) {
		t.Fatalf("looting when overweight: %v", err)
	}
}

func TestShelledPickyDropsEggShell(t *testing.T) {
	var rate int
	for _, d := range Mobs[1050].Drops {
		if d.ItemID == 5015 {
			rate = d.Rate
		}
	}
	if rate != 1000 {
		t.Fatalf("Egg Shell drop rate = %d, want 1000 (10%%)", rate)
	}
	c := NewChar("morocc", 0, 0)
	if err := c.PickUp(5015, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.Equip(c.Inventory[0].Index); err != nil || c.Derive().Def != 3 || c.Look().HeadTop != 5015 {
		t.Fatalf("wearing Egg Shell: err=%v def=%d look=%+v", err, c.Derive().Def, c.Look())
	}
}
