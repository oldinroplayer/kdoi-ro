package game

import "errors"

// Cards: equipment has slots (item_db's Slot column), and a card compounded
// into a slot stays there for good, adding its bonuses while the equipment is
// worn. Pre-renewal rules: a card only fits equipment of its kind (its
// CardLoc, e.g. a weapon card in a weapon), and only while not equipped.

// MaxSlots is the most card slots an item can have.
const MaxSlots = 4

// NumRaces is how many monster races there are (mob_db's numbering).
const NumRaces = 10

// Bonus is what a card adds: item_db script bonuses KDoiRo applies (status
// effects, skill grants and resistances aren't in the game yet).
type Bonus struct {
	Stats                [NumStats]int // bStr...bLuk (bAllStats adds to each)
	MaxHP, MaxSP         int           // flat
	MaxHPRate, MaxSPRate int           // percent
	Atk                  int           // bBaseAtk / bAtk
	Def, Mdef            int
	Hit, Flee, Flee2     int // Flee2: perfect dodge
	Crit                 int
	HPRegenRate          int              // percent more natural HP recovery
	SPRegenRate          int              // ...SP
	AddRace              [NumRaces]int    // percent more damage to a race
	AddEle               [NumElements]int // ...to an element
	SubRace              [NumRaces]int    // percent less damage from a race
	SubEle               [NumElements]int // ...from an element
}

func (b *Bonus) add(o *Bonus) {
	for i := range b.Stats {
		b.Stats[i] += o.Stats[i]
	}
	b.MaxHP += o.MaxHP
	b.MaxSP += o.MaxSP
	b.MaxHPRate += o.MaxHPRate
	b.MaxSPRate += o.MaxSPRate
	b.Atk += o.Atk
	b.Def += o.Def
	b.Mdef += o.Mdef
	b.Hit += o.Hit
	b.Flee += o.Flee
	b.Flee2 += o.Flee2
	b.Crit += o.Crit
	b.HPRegenRate += o.HPRegenRate
	b.SPRegenRate += o.SPRegenRate
	for i := range b.AddRace {
		b.AddRace[i] += o.AddRace[i]
		b.SubRace[i] += o.SubRace[i]
	}
	for i := range b.AddEle {
		b.AddEle[i] += o.AddEle[i]
		b.SubEle[i] += o.SubEle[i]
	}
}

var (
	ErrNotACard      = errors.New("that's not a card")
	ErrCardDoesntFit = errors.New("that card doesn't go in that item")
	ErrNoFreeSlot    = errors.New("no free slot")
)

// FitsCard is whether a card can go into an item: the right kind of
// equipment with a free slot. (Being unequipped is checked by Compound.)
func FitsCard(card, equip ItemDef, cardsIn int) bool {
	return card.Type == TypeCard && equip.Equippable() && equip.Type != TypeAmmo &&
		card.CardLoc&equip.Slot != 0 && cardsIn < equip.Slots
}

// Compound puts the card at cardIndex into the equipment at equipIndex,
// using the card up. The equipment mustn't be worn.
func (c *CharData) Compound(cardIndex, equipIndex int) error {
	card, equip := c.entry(cardIndex), c.entry(equipIndex)
	if card == nil || equip == nil {
		return ErrNoSuchItem
	}
	cardDef, equipDef := Items[card.ItemID], Items[equip.ItemID]
	if cardDef.Type != TypeCard {
		return ErrNotACard
	}
	if equip.Equip != 0 {
		return ErrEquipped
	}
	if cardDef.CardLoc&equipDef.Slot == 0 || !equipDef.Equippable() || equipDef.Type == TypeAmmo {
		return ErrCardDoesntFit
	}
	if len(equip.Cards) >= equipDef.Slots {
		return ErrNoFreeSlot
	}
	equip.Cards = append(append([]int(nil), equip.Cards...), card.ItemID)
	c.removeAmount(cardIndex, 1) // after using equip: removal can shift entries
	return nil
}

// cardBonus sums the bonuses of the cards in the equipment worn.
func (c *CharData) cardBonus() Bonus {
	var b Bonus
	for _, it := range c.Inventory {
		if it.Equip == 0 {
			continue
		}
		for _, id := range it.Cards {
			if def, ok := Items[id]; ok && def.Bonus != nil {
				b.add(def.Bonus)
			}
		}
	}
	return b
}

// cardDamage scales a hit on a monster by the attacker's card bonuses against
// its race and element (pre-renewal: each a separate multiplier).
func (b *Bonus) cardDamage(dmg int, m *MobDef) int {
	if r := m.Race; r >= 0 && r < NumRaces {
		dmg = dmg * (100 + b.AddRace[r]) / 100
	}
	if e := m.Element; e >= 0 && e < NumElements {
		dmg = dmg * (100 + b.AddEle[e]) / 100
	}
	return dmg
}

// cardDefense scales a monster's hit on the wearer by the cards that reduce
// damage from its race and its element (its attacks carry its element).
func (b *Bonus) cardDefense(dmg int, m *MobDef) int {
	if r := m.Race; r >= 0 && r < NumRaces {
		dmg = dmg * (100 - b.SubRace[r]) / 100
	}
	if e := m.Element; e >= 0 && e < NumElements {
		dmg = dmg * (100 - b.SubEle[e]) / 100
	}
	return dmg
}
