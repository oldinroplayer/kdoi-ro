package game

import (
	"errors"
	"testing"
)

func sellAll(itemID int) (int, bool) { return Items[itemID].Price, true }

func TestNewCharDefaults(t *testing.T) {
	c := NewChar("morocc", 1, 2)
	if c.BaseLevel != 1 || c.Stats != [NumStats]int{1, 1, 1, 1, 1, 1} || c.StatusPoints != 48 || c.Zeny != StartZeny {
		t.Fatalf("defaults: %+v", c)
	}
	if d := c.Derive(); d.MaxHP != 40 || d.MaxWeight != 2030 || d.Atk2 != 0 {
		t.Fatalf("derived: %+v", d)
	}
}

func TestBuyStacksAndCharges(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	if err := c.Buy([]Purchase{{1201, 2}, {501, 3}}, sellAll); err != nil {
		t.Fatal(err)
	}
	if err := c.Buy([]Purchase{{501, 2}}, sellAll); err != nil {
		t.Fatal(err)
	}
	// Two knives (no stacking) + one potion stack of 5.
	if len(c.Inventory) != 3 || c.amountOf(501) != 5 {
		t.Fatalf("inventory: %+v", c.Inventory)
	}
	if want := StartZeny - 2*50 - 5*50; c.Zeny != want {
		t.Fatalf("zeny = %d, want %d", c.Zeny, want)
	}
	if c.Weight() != 2*40+5*7 {
		t.Fatalf("weight = %d", c.Weight())
	}
}

func TestBuyIsAllOrNothing(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	cases := map[string]struct {
		order []Purchase
		want  error
	}{
		"zeny":   {[]Purchase{{1201, 1}, {1207, 1}}, ErrNotEnoughZeny}, // 50 + 2400 > 2000
		"weight": {[]Purchase{{501, 300}}, ErrOverweight},              // 300 * 7 = 2100 > 2030 max weight
		"bad":    {[]Purchase{{9999, 1}}, ErrUnknownItem},
		"zero":   {[]Purchase{{501, 0}}, ErrUnknownItem},
	}
	c.Zeny = 1_000_000
	for name, tc := range cases {
		before := c.Zeny
		if name == "zeny" {
			c.Zeny = 2000
		}
		err := c.Buy(tc.order, sellAll)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if len(c.Inventory) != 0 {
			t.Fatalf("%s: inventory changed: %+v", name, c.Inventory)
		}
		c.Zeny = before
	}
}

func TestEquipSwapsSlotAndAddsStats(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.Zeny = 100_000
	if err := c.Buy([]Purchase{{1201, 1}, {1207, 1}, {2301, 1}, {501, 1}}, sellAll); err != nil {
		t.Fatal(err)
	}
	knife, gauche, shirt, potion := c.Inventory[0].Index, c.Inventory[1].Index, c.Inventory[2].Index, c.Inventory[3].Index

	for _, idx := range []int{knife, shirt} {
		if err := c.Equip(idx); err != nil {
			t.Fatal(err)
		}
	}
	if d := c.Derive(); d.Atk2 != 17 || d.Def != 1 {
		t.Fatalf("after knife+shirt: %+v", d)
	}
	// Main Gauche replaces the knife in the weapon slot.
	if err := c.Equip(gauche); err != nil {
		t.Fatal(err)
	}
	if c.entry(knife).Equip != 0 || c.entry(gauche).Equip != SlotHandR || c.Derive().Atk2 != 43 {
		t.Fatalf("swap failed: %+v", c.Inventory)
	}
	if err := c.Equip(potion); !errors.Is(err, ErrNotEquippable) {
		t.Fatalf("equip potion: %v", err)
	}
	if err := c.Unequip(gauche); err != nil || c.Derive().Atk2 != 0 {
		t.Fatalf("unequip: %v", err)
	}

	c.add(1101, 1) // Sword needs base level 2
	if err := c.Equip(c.Inventory[len(c.Inventory)-1].Index); !errors.Is(err, ErrLevelTooLow) {
		t.Fatalf("equip sword at level 1: %v", err)
	}
}

func TestRaiseStat(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	if err := c.RaiseStat(STR); err != nil || c.Stats[STR] != 2 || c.StatusPoints != 46 {
		t.Fatalf("raise: %v %+v", err, c)
	}
	// Costs grow every 10 points: 2 for 1-10, 3 for 11-20, ...
	if RaiseCost(10) != 2 || RaiseCost(11) != 3 || RaiseCost(21) != 4 {
		t.Fatal("RaiseCost table wrong")
	}
	c.StatusPoints = 1
	if err := c.RaiseStat(AGI); !errors.Is(err, ErrNotEnoughPoint) {
		t.Fatalf("raise without points: %v", err)
	}
	if err := c.RaiseStat(7); !errors.Is(err, ErrUnknownStat) {
		t.Fatalf("bad stat: %v", err)
	}
	if d := c.Derive(); d.MaxWeight != 2060 {
		t.Fatalf("STR should raise max weight: %d", d.MaxWeight)
	}
}

func TestFreeIndexReusesGaps(t *testing.T) {
	c := CharData{Inventory: []InvItem{{Index: 0}, {Index: 2}}}
	if got := c.freeIndex(); got != 1 {
		t.Fatalf("freeIndex = %d", got)
	}
}
