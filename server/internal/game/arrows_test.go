package game

import (
	"errors"
	"testing"
)

func TestArrows(t *testing.T) {
	c := NewChar("town", 1, 1)
	c.Job = 3 // Archer
	c.BaseLevel = 10
	c.Stats[DEX] = 20
	c.add(1701, 1) // Bow
	c.add(1752, 3) // Fire Arrows: one stack
	c.add(1752, 2) // ...grows the same stack
	bow, arrows := c.Inventory[0].Index, c.Inventory[1].Index
	if len(c.Inventory) != 2 || c.Inventory[1].Amount != 5 {
		t.Fatalf("arrows should stack: %+v", c.Inventory)
	}
	if err := c.Equip(bow); err != nil {
		t.Fatal(err)
	}
	unarmed := c.Derive()
	if !c.NeedsArrows() || c.attackElement() != ElemNeutral {
		t.Fatal("a bow with no arrows: needs arrows, shoots neutral")
	}
	if err := c.Equip(arrows); err != nil {
		t.Fatal(err)
	}
	d := c.Derive()
	if d.Atk2 != unarmed.Atk2+30 || c.attackElement() != ElemFire {
		t.Fatalf("Fire Arrows add 30 ATK and fire: atk2 %d (was %d), element %d", d.Atk2, unarmed.Atk2, c.attackElement())
	}
	// Bows use DEX for status ATK: 20 + 2*2 + STR/5 + LUK/5.
	if want := 20 + 4 + c.Stats[STR]/5 + c.Stats[LUK]/5; d.Atk != want {
		t.Fatalf("bow status ATK %d, want %d", d.Atk, want)
	}
	for want := 4; want >= 0; want-- {
		left, err := c.UseArrow()
		if err != nil || left != want {
			t.Fatalf("shot: %d left, %v; want %d", left, err, want)
		}
	}
	if _, _, ok := c.Arrow(); ok || len(c.Inventory) != 1 {
		t.Fatalf("the empty stack should be gone: %+v", c.Inventory)
	}
	if _, err := c.UseArrow(); !errors.Is(err, ErrNoArrows) {
		t.Fatalf("no arrows: %v", err)
	}
}

func TestForgedWeaponElement(t *testing.T) {
	c := NewChar("town", 1, 1)
	c.BaseLevel = 10
	c.Inventory = []InvItem{{Index: 0, ItemID: 1207, Amount: 1, Element: ElemFire}} // Fire Main Gauche
	if err := c.Equip(0); err != nil {
		t.Fatal(err)
	}
	if got := c.attackElement(); got != ElemFire {
		t.Fatalf("a Fire Main Gauche attacks with element %d, want fire", got)
	}
	c.Inventory[0].Element = ElemNeutral
	if got := c.attackElement(); got != ElemNeutral {
		t.Fatalf("an ordinary Main Gauche attacks with element %d", got)
	}
}
