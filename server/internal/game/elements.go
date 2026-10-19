package game

// Elements, as numbered in RO (skill_db, mob_db and attr_fix.txt).
const (
	ElemNeutral = iota
	ElemWater
	ElemEarth
	ElemFire
	ElemWind
	ElemPoison
	ElemHoly
	ElemShadow
	ElemGhost
	ElemUndead
	NumElements
)

// Races, as numbered in mob_db.
const (
	RaceFormless = iota
	RaceUndead
	RaceBrute
	RacePlant
	RaceInsect
	RaceFish
	RaceDemon
	RaceDemiHuman
	RaceAngel
	RaceDragon
)

// MobTraits are what a monster is made of and how it behaves (mob_db; see
// elements_data.go).
type MobTraits struct {
	Element      int // ElemNeutral..ElemUndead
	ElementLevel int // 1..4
	Race         int
	Size         int // 0 small, 1 medium, 2 large
	// ViewRange is how far (cells) the monster notices players (mob_db Range2).
	ViewRange int
	// Aggressive monsters attack players they see (mode flag 4).
	Aggressive bool
	// CastSensor monsters attack players casting nearby (mode flag 16).
	CastSensor bool
}

// AttrFix is the damage percent an attack of element atk does to a target of
// element def at level lvl (1..4). Negative values would heal; callers treat
// them as no damage.
func AttrFix(atk, def, lvl int) int {
	if atk < 0 || atk >= NumElements || def < 0 || def >= NumElements {
		return 100
	}
	return attrFix[min(max(lvl, 1), 4)-1][atk][def]
}

// ElementFix is the damage percent an attack of element atk does to this monster.
func (m *MobDef) ElementFix(atk int) int { return AttrFix(atk, m.Element, m.ElementLevel) }

// applyElement scales damage by an element's effect on the monster; a target
// immune to (or healed by) the element takes none.
func applyElement(dmg, fix int) int {
	if fix <= 0 {
		return 0
	}
	return max(dmg*fix/100, 1)
}
