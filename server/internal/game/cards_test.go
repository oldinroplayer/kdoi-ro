package game

import (
	"errors"
	"testing"
)

func TestCompoundCards(t *testing.T) {
	if Items[1201].Slots != 3 || Items[4002].Type != TypeCard || Items[4002].CardLoc != SlotHandR {
		t.Fatalf("data: Knife slots %d, Fabre Card %+v", Items[1201].Slots, Items[4002])
	}
	c := NewChar("town", 1, 1)
	c.add(1201, 1) // Knife [3]
	c.add(4002, 4) // Fabre Card (weapon): VIT +1, Max HP +100
	c.add(4001, 1) // Poring Card (armor)
	knife, fabre, poring := c.Inventory[0].Index, c.Inventory[1].Index, c.Inventory[2].Index

	if err := c.Compound(poring, knife); !errors.Is(err, ErrCardDoesntFit) {
		t.Fatalf("an armor card in a weapon: %v", err)
	}
	if err := c.Compound(knife, knife); !errors.Is(err, ErrNotACard) {
		t.Fatalf("a knife isn't a card: %v", err)
	}
	if err := c.Equip(knife); err != nil {
		t.Fatal(err)
	}
	if err := c.Compound(fabre, knife); !errors.Is(err, ErrEquipped) {
		t.Fatalf("worn equipment: %v", err)
	}
	if err := c.Unequip(knife); err != nil {
		t.Fatal(err)
	}
	before := c.Derive()
	for range 3 {
		if err := c.Compound(fabre, knife); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Compound(fabre, knife); !errors.Is(err, ErrNoFreeSlot) {
		t.Fatalf("a fourth card in a [3] knife: %v", err)
	}
	if got := c.entry(knife).Cards; len(got) != 3 || c.amountOf(4002) != 1 {
		t.Fatalf("cards in the knife %v, Fabre Cards left %d", got, c.amountOf(4002))
	}
	// Bonuses count only while worn.
	if d := c.Derive(); d.MaxHP != before.MaxHP {
		t.Fatalf("unworn cards changed Max HP: %d -> %d", before.MaxHP, d.MaxHP)
	}
	if err := c.Equip(knife); err != nil {
		t.Fatal(err)
	}
	d := c.Derive()
	if d.Cards.MaxHP != 300 || c.EffectiveStats()[VIT] != c.Stats[VIT]+Job(c.Job).Bonus(c.JobLevel)[VIT]+3 {
		t.Fatalf("worn: card Max HP %d, VIT %d", d.Cards.MaxHP, c.EffectiveStats()[VIT])
	}
}

func TestCardDamage(t *testing.T) {
	b := Bonus{AddRace: [NumRaces]int{RaceFormless: 20}, AddEle: [NumElements]int{ElemWater: 20}, SubEle: [NumElements]int{ElemWater: 10}}
	m := &MobDef{MobTraits: MobTraits{Race: RaceFormless, Element: ElemWater}}
	if got := b.cardDamage(100, m); got != 144 { // 100 * 1.2 * 1.2
		t.Fatalf("damage %d, want 144", got)
	}
	if got := b.cardDefense(100, m); got != 90 {
		t.Fatalf("damage taken %d, want 90", got)
	}
}
