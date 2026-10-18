// Package game holds the rules shared by the world and persistence: items,
// inventories, equipment and character stats (modelled on RO pre-renewal).
package game

// ItemType classifies how an item behaves.
type ItemType string

const (
	TypeHealing ItemType = "healing"
	TypeWeapon  ItemType = "weapon"
	TypeArmor   ItemType = "armor"
	TypeEtc     ItemType = "etc" // loot, only worth selling
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
	// Sprite names the equipment's sprite layer, for items shown on the
	// character (weapons, shields, headgear): e.g. "단검" for a weapon's
	// 초보자_남_단검.spr. Empty for items that don't change the sprite.
	Sprite string `json:"sprite,omitempty"`
}

// SellPrice is what NPC shops pay: half the buy price, like RO.
func (d ItemDef) SellPrice() int { return d.Price / 2 }

func (d ItemDef) Equippable() bool { return d.Slot != 0 }

// Stackable items share one inventory entry; equipment never stacks.
func (d ItemDef) Stackable() bool { return !d.Equippable() }

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

	1101: {ID: 1101, Name: "Sword", Type: TypeWeapon, Slot: SlotHandR, Atk: 25, Weight: 50, Price: 100, EquipLevel: 2, Sprite: "검"},
	1201: {ID: 1201, Name: "Knife", Type: TypeWeapon, Slot: SlotHandR, Atk: 17, Weight: 40, Price: 50, Sprite: "단검"},
	1207: {ID: 1207, Name: "Main Gauche", Type: TypeWeapon, Slot: SlotHandR, Atk: 43, Weight: 60, Price: 2400, Sprite: "1207"},
	2101: {ID: 2101, Name: "Guard", Type: TypeArmor, Slot: SlotHandL, Def: 1, Weight: 30, Price: 500, Sprite: "가드"},
	2220: {ID: 2220, Name: "Hat", Type: TypeArmor, Slot: SlotHeadTop, Def: 2, Weight: 20, Price: 1000, Sprite: "햇"},
	2301: {ID: 2301, Name: "Cotton Shirt", Type: TypeArmor, Slot: SlotArmor, Def: 1, Weight: 10, Price: 10},
	2401: {ID: 2401, Name: "Sandals", Type: TypeArmor, Slot: SlotShoes, Def: 1, Weight: 20, Price: 400},
	2501: {ID: 2501, Name: "Hood", Type: TypeArmor, Slot: SlotGarment, Def: 1, Weight: 20, Price: 1000},
}
