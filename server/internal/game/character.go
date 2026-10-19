package game

import "errors"

const (
	MaxInventory = 100   // distinct entries
	MaxStack     = 30000 // per stackable entry
	MaxZeny      = 1_000_000_000
	MaxStat      = 99
	StartZeny    = 2000
	StartPoints  = 48 // rAthena start_status_points
)

// Stat indices, in RO's order.
const (
	STR = iota
	AGI
	VIT
	INT
	DEX
	LUK
	NumStats
)

var (
	ErrNotEnoughZeny  = errors.New("not enough zeny")
	ErrOverweight     = errors.New("too heavy")
	ErrInventoryFull  = errors.New("inventory full")
	ErrUnknownItem    = errors.New("unknown item")
	ErrNotEquippable  = errors.New("item can't be equipped")
	ErrLevelTooLow    = errors.New("level too low to equip")
	ErrNoSuchItem     = errors.New("no such inventory item")
	ErrNotEnoughPoint = errors.New("not enough status points")
	ErrStatMaxed      = errors.New("stat is already at its maximum")
	ErrUnknownStat    = errors.New("unknown stat")
	ErrNotUsable      = errors.New("item can't be used")
	ErrEquipped       = errors.New("item is equipped")
)

// InvItem is one inventory entry. Equip is the slot it's worn in (0 = not worn).
type InvItem struct {
	Index  int    `json:"index"`
	ItemID int    `json:"item_id"`
	Amount int    `json:"amount"`
	Equip  uint16 `json:"equip,omitempty"`
}

// CharData is everything about a character that persists and that the world
// mutates while the player is online.
// SavePoint is where a character respawns (set at a Kafra); empty = the default.
type SavePoint struct {
	Map string `json:"map"`
	X   int    `json:"x"`
	Y   int    `json:"y"`
}

type CharData struct {
	Job          uint16        `json:"job"`
	Map          string        `json:"map"`
	X            int           `json:"x"`
	Y            int           `json:"y"`
	BaseLevel    int           `json:"base_level"`
	JobLevel     int           `json:"job_level"`
	BaseExp      int           `json:"base_exp"`
	JobExp       int           `json:"job_exp"`
	HP           int           `json:"hp"`
	SP           int           `json:"sp"`
	Stats        [NumStats]int `json:"stats"`
	StatusPoints int           `json:"status_points"`
	Skills       map[int]int   `json:"skills"` // skill ID -> learned level
	// Buffs are active skill buffs (skill ID -> level). They're not saved;
	// the world removes them when they run out.
	Buffs       map[int]int `json:"-"`
	SkillPoints int         `json:"skill_points"`
	Zeny        int         `json:"zeny"`
	Save        SavePoint   `json:"save,omitempty"`
	// Hair is the hairstyle (1..HairStyles; 0 = not chosen yet, drawn as the
	// default) and HairColor its palette (0..HairColors-1).
	Hair      int `json:"hair,omitempty"`
	HairColor int `json:"hair_color,omitempty"`
	// Female picks the female sprites (characters from before genders are male).
	Female    bool      `json:"female,omitempty"`
	Inventory []InvItem `json:"inventory"`
}

// Clone returns a copy that shares no slices or maps with c.
func (c CharData) Clone() CharData {
	if c.Buffs != nil {
		buffs := make(map[int]int, len(c.Buffs))
		for id, lv := range c.Buffs {
			buffs[id] = lv
		}
		c.Buffs = buffs
	}
	c.Inventory = append([]InvItem(nil), c.Inventory...)
	skills := make(map[int]int, len(c.Skills))
	for id, lv := range c.Skills {
		skills[id] = lv
	}
	c.Skills = skills
	return c
}

// NewChar returns a fresh Novice at the given position.
func NewChar(mapName string, x, y int) CharData {
	c := CharData{Job: 0, Map: mapName, X: x, Y: y}
	c.FillDefaults()
	return c
}

// ClampVitals keeps HP/SP within their maximums; HP 0 (characters saved before
// HP existed) becomes full.
func (c *CharData) ClampVitals() {
	d := c.Derive()
	if c.HP <= 0 {
		c.HP, c.SP = d.MaxHP, d.MaxSP
	}
	c.HP = min(c.HP, d.MaxHP)
	c.SP = min(max(c.SP, 0), d.MaxSP)
}

// FillDefaults initialises fields missing from characters saved before stats
// existed (all-zero stats).
func (c *CharData) FillDefaults() {
	if c.BaseLevel == 0 {
		c.BaseLevel = 1
		c.JobLevel = 1
		for i := range c.Stats {
			c.Stats[i] = 1
		}
		c.StatusPoints = StartPoints
		c.Zeny = StartZeny
	}
	c.fillSkillDefaults()
	c.ClampVitals()
}

// RaiseCost is the status points needed to raise a stat from its current value.
func RaiseCost(current int) int { return (current-1)/10 + 2 }

func (c *CharData) RaiseStat(stat int) error {
	if stat < 0 || stat >= NumStats {
		return ErrUnknownStat
	}
	if c.Stats[stat] >= MaxStat {
		return ErrStatMaxed
	}
	cost := RaiseCost(c.Stats[stat])
	if cost > c.StatusPoints {
		return ErrNotEnoughPoint
	}
	c.StatusPoints -= cost
	c.Stats[stat]++
	return nil
}

// Weight is the total carried weight.
func (c *CharData) Weight() int {
	w := 0
	for _, it := range c.Inventory {
		w += Items[it.ItemID].Weight * it.Amount
	}
	return w
}

// MaxWeight follows RO: 2000 + 30 per STR.
// MaxWeight follows RO: the job's base weight plus 30 per (base) STR, plus
// Enlarge Weight Limit.
func (c *CharData) MaxWeight() int {
	return Job(c.Job).BaseWeight + 30*c.Stats[STR] + 200*c.SkillLevel(SkillEnlargeWeight)
}

// EffectiveStats are the base stats plus job, passive and buff bonuses.
func (c *CharData) EffectiveStats() [NumStats]int {
	s := c.Stats
	b := c.statBonus()
	for i := range s {
		s[i] += b[i]
	}
	return s
}

// Purchase is one line of a shop order.
type Purchase struct {
	ItemID, Amount int
}

// Buy checks the whole order (zeny, weight, inventory space) and then applies
// it, so an order either fully succeeds or changes nothing.
func (c *CharData) Buy(order []Purchase, price func(itemID int) (int, bool)) error {
	total, weight := 0, 0
	newEntries := 0
	stacking := map[int]int{}
	for _, p := range order {
		def, ok := Items[p.ItemID]
		pr, sold := price(p.ItemID)
		if !ok || !sold || p.Amount <= 0 || p.Amount > MaxStack {
			return ErrUnknownItem
		}
		total += pr * p.Amount
		weight += def.Weight * p.Amount
		if def.Stackable() {
			if c.find(p.ItemID) < 0 && stacking[p.ItemID] == 0 {
				newEntries++
			}
			stacking[p.ItemID] += p.Amount
			if c.amountOf(p.ItemID)+stacking[p.ItemID] > MaxStack {
				return ErrInventoryFull
			}
		} else {
			newEntries += p.Amount
		}
	}
	if total > c.Zeny {
		return ErrNotEnoughZeny
	}
	if c.Weight()+weight > c.MaxWeight() {
		return ErrOverweight
	}
	if len(c.Inventory)+newEntries > MaxInventory {
		return ErrInventoryFull
	}

	c.Zeny -= total
	for _, p := range order {
		c.add(p.ItemID, p.Amount)
	}
	return nil
}

func (c *CharData) find(itemID int) int {
	for i, it := range c.Inventory {
		if it.ItemID == itemID {
			return i
		}
	}
	return -1
}

func (c *CharData) amountOf(itemID int) int {
	if i := c.find(itemID); i >= 0 {
		return c.Inventory[i].Amount
	}
	return 0
}

func (c *CharData) add(itemID, amount int) {
	if Items[itemID].Stackable() {
		if i := c.find(itemID); i >= 0 {
			c.Inventory[i].Amount += amount
			return
		}
		c.Inventory = append(c.Inventory, InvItem{Index: c.freeIndex(), ItemID: itemID, Amount: amount})
		return
	}
	for range amount {
		c.Inventory = append(c.Inventory, InvItem{Index: c.freeIndex(), ItemID: itemID, Amount: 1})
	}
}

// freeIndex returns the lowest unused inventory index, so indices stay stable
// for the client while items come and go.
func (c *CharData) freeIndex() int {
	used := map[int]bool{}
	for _, it := range c.Inventory {
		used[it.Index] = true
	}
	for i := 0; ; i++ {
		if !used[i] {
			return i
		}
	}
}

func (c *CharData) entry(index int) *InvItem {
	for i := range c.Inventory {
		if c.Inventory[i].Index == index {
			return &c.Inventory[i]
		}
	}
	return nil
}

// UseItem consumes one healing item and returns the HP it restored.
func (c *CharData) UseItem(index int, r Rand) (int, error) {
	it := c.entry(index)
	if it == nil {
		return 0, ErrNoSuchItem
	}
	def := Items[it.ItemID]
	if def.Type != TypeHealing {
		return 0, ErrNotUsable
	}
	c.removeAmount(index, 1)
	before := c.HP
	c.HP = min(c.HP+between(r, def.HealMin, def.HealMax), c.Derive().MaxHP)
	return c.HP - before, nil
}

func (c *CharData) removeAmount(index, amount int) {
	for i := range c.Inventory {
		if c.Inventory[i].Index != index {
			continue
		}
		c.Inventory[i].Amount -= amount
		if c.Inventory[i].Amount <= 0 {
			c.Inventory = append(c.Inventory[:i], c.Inventory[i+1:]...)
		}
		return
	}
}

// Sale is one line of a sell order: an inventory index and how many.
type Sale struct {
	Index, Amount int
}

// Sell sells items to an NPC at half price; all-or-nothing like Buy.
func (c *CharData) Sell(order []Sale) (earned int, err error) {
	wanted := map[int]int{}
	for _, s := range order {
		it := c.entry(s.Index)
		if it == nil || s.Amount <= 0 {
			return 0, ErrNoSuchItem
		}
		if it.Equip != 0 {
			return 0, ErrEquipped
		}
		wanted[s.Index] += s.Amount
		if wanted[s.Index] > it.Amount {
			return 0, ErrNoSuchItem
		}
		earned += c.SellPrice(Items[it.ItemID]) * s.Amount
	}
	if c.Zeny+earned > MaxZeny {
		return 0, ErrInventoryFull
	}
	for index, amount := range wanted {
		c.removeAmount(index, amount)
	}
	c.Zeny += earned
	return earned, nil
}

// PickUp adds items from the ground if there's room and they aren't too heavy.
func (c *CharData) PickUp(itemID, amount int) error {
	def, ok := Items[itemID]
	if !ok || amount <= 0 {
		return ErrUnknownItem
	}
	if c.Weight()+def.Weight*amount > c.MaxWeight() {
		return ErrOverweight
	}
	switch {
	case def.Stackable() && c.find(itemID) >= 0:
		if c.amountOf(itemID)+amount > MaxStack {
			return ErrInventoryFull
		}
	case def.Stackable():
		if len(c.Inventory) >= MaxInventory {
			return ErrInventoryFull
		}
	case len(c.Inventory)+amount > MaxInventory:
		return ErrInventoryFull
	}
	c.add(itemID, amount)
	return nil
}

// Drop takes items out of the inventory to put them on the ground. Worn
// equipment has to be taken off first.
func (c *CharData) Drop(index, amount int) (itemID int, err error) {
	it := c.entry(index)
	if it == nil || amount <= 0 || amount > it.Amount {
		return 0, ErrNoSuchItem
	}
	if it.Equip != 0 {
		return 0, ErrEquipped
	}
	itemID = it.ItemID
	c.removeAmount(index, amount)
	return itemID, nil
}

// Equip wears the item at index, taking off whatever occupied its slot.
func (c *CharData) Equip(index int) error {
	it := c.entry(index)
	if it == nil {
		return ErrNoSuchItem
	}
	def := Items[it.ItemID]
	if !def.Equippable() {
		return ErrNotEquippable
	}
	if c.BaseLevel < def.EquipLevel {
		return ErrLevelTooLow
	}
	if !def.CanUse(c.Job) {
		return ErrWrongJob
	}
	for i := range c.Inventory {
		if c.Inventory[i].Equip&def.Slot != 0 {
			c.Inventory[i].Equip = 0
		}
	}
	it.Equip = def.Slot
	return nil
}

func (c *CharData) Unequip(index int) error {
	it := c.entry(index)
	if it == nil || it.Equip == 0 {
		return ErrNoSuchItem
	}
	it.Equip = 0
	return nil
}

// Derived are the computed battle stats shown in the status window.
type Derived struct {
	WeaponType, Range int           // equipped weapon class and attack reach in cells
	Bonus             [NumStats]int // job level stat bonuses
	MaxHP, MaxSP      int
	Atk, Atk2         int // status ATK + equipment ATK
	MatkMin, MatkMax  int
	Def, Def2         int // equipment DEF + VIT DEF
	Mdef, Mdef2       int
	Hit, Flee, Flee2  int
	Crit              int
	Aspd              int
	Weight, MaxWeight int
}

// Derive computes battle stats with pre-renewal formulas (Novice job values).
func (c *CharData) Derive() Derived {
	job := Job(c.Job)
	bonus := c.statBonus()
	s := c.Stats
	for i := range s {
		s[i] += bonus[i]
	}
	lv := c.BaseLevel
	d := Derived{
		Bonus:     bonus,
		MaxHP:     job.MaxHP(lv, s[VIT]),
		MaxSP:     job.MaxSP(lv, s[INT]),
		Atk:       s[STR] + (s[STR]/10)*(s[STR]/10) + s[DEX]/5 + s[LUK]/5,
		MatkMin:   s[INT] + (s[INT]/7)*(s[INT]/7),
		MatkMax:   s[INT] + (s[INT]/5)*(s[INT]/5),
		Def2:      s[VIT],
		Mdef2:     s[INT],
		Hit:       lv + s[DEX],
		Flee:      lv + s[AGI],
		Flee2:     1 + s[LUK]/10,
		Crit:      1 + s[LUK]*3/10,
		Weight:    c.Weight(),
		MaxWeight: c.MaxWeight(),
	}
	weapon, reach := WeaponFist, 1
	for _, it := range c.Inventory {
		if it.Equip == 0 {
			continue
		}
		def := Items[it.ItemID]
		d.Atk2 += def.Atk
		d.Def += def.Def
		if def.Type == TypeWeapon {
			weapon = def.WeaponType
			reach = max(def.Range, 1)
		}
	}
	d.WeaponType, d.Range = weapon, reach
	c.applyPassives(&d)
	// Attack speed from the job's base delay for the weapon type (eAthena pc.c).
	baseDelay := job.AttackDelay[weapon]
	delay := baseDelay - baseDelay*(4*s[AGI]+s[DEX])/1000
	d.Aspd = 200 - delay/10
	return d
}

// Look is the equipment visible on a character's sprite, as item IDs (0 = none).
type Look struct {
	Weapon, Shield            int
	HeadTop, HeadMid, HeadLow int
	Hair, HairColor           int
	Female                    bool
}

// The 2004 client's hairstyles (sprite/인간족/머리통/남|여/1..19_남|여) and
// their hair colours (palette/머리/머리<style>_남|여_0..8.pal), for either gender.
const (
	HairStyles = 19
	HairColors = 9
)

var ErrBadHair = errors.New("no such hairstyle or colour")

// SetAppearance changes the hairstyle, hair colour and gender.
func (c *CharData) SetAppearance(style, color int, female bool) error {
	if style < 1 || style > HairStyles || color < 0 || color >= HairColors {
		return ErrBadHair
	}
	c.Hair, c.HairColor, c.Female = style, color, female
	return nil
}

// Look returns what shows on the character sprite: worn items and hair.
func (c *CharData) Look() Look {
	l := Look{Hair: c.Hair, HairColor: c.HairColor, Female: c.Female}
	for _, it := range c.Inventory {
		switch {
		case it.Equip&SlotHandR != 0:
			l.Weapon = it.ItemID
		case it.Equip&SlotHandL != 0:
			l.Shield = it.ItemID
		case it.Equip&SlotHeadTop != 0:
			l.HeadTop = it.ItemID
		case it.Equip&SlotHeadMid != 0:
			l.HeadMid = it.ItemID
		case it.Equip&SlotHeadLow != 0:
			l.HeadLow = it.ItemID
		}
	}
	return l
}
