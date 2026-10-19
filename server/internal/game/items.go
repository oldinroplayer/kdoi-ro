// Package game holds the rules shared by the world and persistence: items,
// inventories, equipment and character stats (modelled on RO pre-renewal).
package game

// ItemType classifies how an item behaves.
type ItemType string

const (
	TypeHealing ItemType = "healing"
	TypeWeapon  ItemType = "weapon"
	TypeArmor   ItemType = "armor"
	TypeEtc     ItemType = "etc"  // loot, only worth selling
	TypeAmmo    ItemType = "ammo" // arrows: worn as a stack, one used per bow shot
	TypeWarp    ItemType = "warp" // wings: Warp says where they take you
	TypeCard    ItemType = "card" // compounded into a slot of equipment (CardLoc)
)

// Equip locations, as RO's bitmask.
const (
	SlotHeadLow uint16 = 0x0001
	SlotHandR   uint16 = 0x0002 // weapon
	SlotGarment uint16 = 0x0004
	SlotAccL    uint16 = 0x0008
	SlotArmor   uint16 = 0x0010
	SlotHandL   uint16 = 0x0020 // shield
	SlotShoes   uint16 = 0x0040
	SlotAccR    uint16 = 0x0080
	SlotHeadTop uint16 = 0x0100
	SlotHeadMid uint16 = 0x0200
	SlotAmmo    uint16 = 0x8000 // arrows
)

// ItemDef is one entry of the item database. Display names, descriptions and
// icons come from the client's own data tables, keyed by ID.
type ItemDef struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"` // for logs and tests; clients use their own table
	Type       ItemType `json:"type"`
	Slot       uint16   `json:"slot,omitempty"` // equip location bitmask
	Atk        int      `json:"atk,omitempty"`
	Def        int      `json:"def,omitempty"`
	Weight     int      `json:"weight"`
	Price      int      `json:"price"`
	EquipLevel int      `json:"equipLevel,omitempty"`
	HealMin    int      `json:"healMin,omitempty"` // healing items restore a random
	HealMax    int      `json:"healMax,omitempty"` // amount of HP in [HealMin, HealMax]
	// Jobs is the 2004 item_db job bitmask (bit n = RO class n); 0 = every job.
	Jobs uint32 `json:"jobs,omitempty"`
	// WeaponType is eAthena's weapon class (1 dagger, 2 one-handed sword, 11 bow ...).
	WeaponType int `json:"weaponType,omitempty"`
	Range      int `json:"range,omitempty"` // weapon reach in cells (bows: 5)
	// Element is the attack element arrows give bow shots (item_db bAtkEle);
	// Crit adds critical rate (Sharp Arrow).
	Element int `json:"element,omitempty"`
	Crit    int `json:"crit,omitempty"`
	// Warp is where a wing takes its user: WarpRandom or WarpSavePoint.
	Warp string `json:"warp,omitempty"`
	// Slots is how many cards the equipment takes (0-4).
	Slots int `json:"slots,omitempty"`
	// CardLoc is the equipment a card goes into (an equip location mask, e.g.
	// SlotHandR for weapon cards); Bonus is what it adds while worn.
	CardLoc uint16 `json:"cardLoc,omitempty"`
	Bonus   *Bonus `json:"-"`
	// Sprite names the equipment's sprite layer, for items shown on the
	// character (weapons, shields, headgear): e.g. "단검" for a weapon's
	// 초보자_남_단검.spr. Empty for items that don't change the sprite.
	Sprite string `json:"sprite,omitempty"`
}

// Where wings (TypeWarp items) take you: item_db's warp "Random" / "SavePoint".
const (
	WarpRandom    = "random"     // Fly Wing: a random spot on the same map
	WarpSavePoint = "save point" // Butterfly Wing: the save point
)

// SellPrice is what NPC shops pay: half the buy price, like RO.
func (d ItemDef) SellPrice() int { return d.Price / 2 }

func (d ItemDef) Equippable() bool { return d.Slot != 0 }

// Stackable items share one inventory entry; equipment never stacks, except
// arrows, which are worn as a whole stack.
func (d ItemDef) Stackable() bool { return !d.Equippable() || d.Type == TypeAmmo }

// Items is the item database. Values follow rAthena's pre-renewal item_db.
var Items = map[int]ItemDef{
	501: {ID: 501, Name: "Red Potion", Type: TypeHealing, Weight: 7, Price: 50, HealMin: 45, HealMax: 65},
	507: {ID: 507, Name: "Red Herb", Type: TypeHealing, Weight: 3, Price: 18, HealMin: 18, HealMax: 28},
	508: {ID: 508, Name: "Yellow Herb", Type: TypeHealing, Weight: 5, Price: 40, HealMin: 38, HealMax: 58},
	512: {ID: 512, Name: "Apple", Type: TypeHealing, Weight: 2, Price: 15, HealMin: 16, HealMax: 22},
	519: {ID: 519, Name: "Milk", Type: TypeHealing, Weight: 3, Price: 25, HealMin: 27, HealMax: 37},

	711:  {ID: 711, Name: "Shoot", Type: TypeEtc, Weight: 1, Price: 16},
	712:  {ID: 712, Name: "Flower", Type: TypeEtc, Weight: 1, Price: 2},
	713:  {ID: 713, Name: "Empty Bottle", Type: TypeEtc, Weight: 2, Price: 6},
	905:  {ID: 905, Name: "Stem", Type: TypeEtc, Weight: 1, Price: 46},
	909:  {ID: 909, Name: "Jellopy", Type: TypeEtc, Weight: 1, Price: 6},
	914:  {ID: 914, Name: "Fluff", Type: TypeEtc, Weight: 1, Price: 8},
	916:  {ID: 916, Name: "Feather of Birds", Type: TypeEtc, Weight: 1, Price: 10},
	935:  {ID: 935, Name: "Shell", Type: TypeEtc, Weight: 1, Price: 14},
	938:  {ID: 938, Name: "Sticky Mucus", Type: TypeEtc, Weight: 1, Price: 70},
	949:  {ID: 949, Name: "Feather", Type: TypeEtc, Weight: 1, Price: 20},
	1010: {ID: 1010, Name: "Phracon", Type: TypeEtc, Weight: 20, Price: 200},

	// moc_fild06 loot, from the November 2004 eAthena item_db
	// (reference/eathena-2004/db/item_db.txt).
	517:  {ID: 517, Name: "Meat", Type: TypeHealing, Weight: 15, Price: 50, HealMin: 70, HealMax: 99},
	511:  {ID: 511, Name: "Green Herb", Type: TypeEtc, Weight: 3, Price: 10}, // cures poison; no poison yet
	715:  {ID: 715, Name: "Yellow Gemstone", Type: TypeEtc, Weight: 3, Price: 600},
	737:  {ID: 737, Name: "Black Ladle", Type: TypeEtc, Weight: 5, Price: 400},
	757:  {ID: 757, Name: "Rough Elunium", Type: TypeEtc, Weight: 20, Price: 648},
	910:  {ID: 910, Name: "Garlet", Type: TypeEtc, Weight: 1, Price: 40},
	911:  {ID: 911, Name: "Scell", Type: TypeEtc, Weight: 1, Price: 160},
	912:  {ID: 912, Name: "Zargon", Type: TypeEtc, Weight: 1, Price: 480},
	917:  {ID: 917, Name: "Talon", Type: TypeEtc, Weight: 1, Price: 20},
	925:  {ID: 925, Name: "Bill of Birds", Type: TypeEtc, Weight: 1, Price: 64},
	952:  {ID: 952, Name: "Cactus Needle", Type: TypeEtc, Weight: 1, Price: 82},
	955:  {ID: 955, Name: "Worm Peeling", Type: TypeEtc, Weight: 1, Price: 52},
	990:  {ID: 990, Name: "Red Blood", Type: TypeEtc, Weight: 5, Price: 1000},
	992:  {ID: 992, Name: "Wind of Verdure", Type: TypeEtc, Weight: 5, Price: 1000},
	993:  {ID: 993, Name: "Green Live", Type: TypeEtc, Weight: 5, Price: 1000},
	1001: {ID: 1001, Name: "Star Dust", Type: TypeEtc, Weight: 1, Price: 1500},
	1002: {ID: 1002, Name: "Iron Ore", Type: TypeEtc, Weight: 15, Price: 50},
	7031: {ID: 7031, Name: "Old Frying Pan", Type: TypeEtc, Weight: 1, Price: 196},

	1101: {ID: 1101, Name: "Sword", Type: TypeWeapon, Slot: SlotHandR, Atk: 25, Weight: 50, Price: 100, EquipLevel: 2, Sprite: "검", Jobs: 8803555, WeaponType: WeaponSword},
	1201: {ID: 1201, Name: "Knife", Type: TypeWeapon, Slot: SlotHandR, Atk: 17, Weight: 40, Price: 50, Sprite: "단검", Jobs: 10444527, WeaponType: WeaponDagger},
	1207: {ID: 1207, Name: "Main Gauche", Type: TypeWeapon, Slot: SlotHandR, Atk: 43, Weight: 60, Price: 2400, Sprite: "1207", Jobs: 10444527, WeaponType: WeaponDagger},
	1701: {ID: 1701, Name: "Bow", Type: TypeWeapon, Slot: SlotHandR, Atk: 15, Weight: 50, Price: 1000, EquipLevel: 4,
		Sprite: "활", Jobs: 1706056, WeaponType: WeaponBow, Range: 5},
	2101: {ID: 2101, Name: "Guard", Type: TypeArmor, Slot: SlotHandL, Def: 1, Weight: 30, Price: 500, Sprite: "가드"},
	2220: {ID: 2220, Name: "Hat", Type: TypeArmor, Slot: SlotHeadTop, Def: 2, Weight: 20, Price: 1000, Sprite: "햇"},
	2301: {ID: 2301, Name: "Cotton Shirt", Type: TypeArmor, Slot: SlotArmor, Def: 1, Weight: 10, Price: 10},
	2401: {ID: 2401, Name: "Sandals", Type: TypeArmor, Slot: SlotShoes, Def: 1, Weight: 20, Price: 400},
	5015: {ID: 5015, Name: "Egg Shell", Type: TypeArmor, Slot: SlotHeadTop, Def: 3, Weight: 20, Price: 20, Sprite: "장식용알껍질"},
	2501: {ID: 2501, Name: "Hood", Type: TypeArmor, Slot: SlotGarment, Def: 1, Weight: 20, Price: 1000},
}
