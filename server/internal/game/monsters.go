package game

// MobDef is a monster type. Values follow rAthena's pre-renewal mob_db.
type MobDef struct {
	ID     int
	Name   string
	Sprite string // file under data/sprite/몬스터/, without extension
	Level  int
	HP     int

	BaseExp, JobExp int
	Atk1, Atk2      int
	Def, Mdef       int
	Stats           [NumStats]int
	Range           int // attack range in cells
	ChaseRange      int // gives up on a target further than this

	WalkSpeed    int // ms per cell
	AttackDelay  int // ms between attacks
	AttackMotion int // ms of the attack animation
	DamageMotion int // ms of the "hurt" animation

	Mobile bool // wanders and chases; eggs and plants stay put
	Fights bool // hits back when attacked
	Plant  bool // every hit does exactly 1 damage

	Drops []Drop
}

// Drop is a chance (out of 10000, like rAthena) to drop an item on death.
type Drop struct {
	ItemID int
	Rate   int
}

// Mobs used on moc_fild07. Drops are trimmed to items in the item DB.
var Mobs = map[int]*MobDef{
	1047: {
		ID: 1047, Name: "Peco Peco Egg", Sprite: "peco_egg", Level: 3, HP: 420,
		BaseExp: 4, JobExp: 4, Atk1: 1, Atk2: 2, Def: 20, Mdef: 20,
		Stats: [NumStats]int{LUK: 20}, Range: 1, ChaseRange: 12,
		WalkSpeed: 1000, AttackDelay: 1001, AttackMotion: 1, DamageMotion: 1,
		Drops: []Drop{{935, 1500}, {507, 400}, {507, 400}, {713, 1800}, {1010, 250}},
	},
	1049: {
		ID: 1049, Name: "Picky", Sprite: "picky", Level: 3, HP: 80,
		BaseExp: 4, JobExp: 3, Atk1: 9, Atk2: 12,
		Stats: [NumStats]int{AGI: 3, VIT: 3, INT: 5, DEX: 10, LUK: 30}, Range: 1, ChaseRange: 12,
		WalkSpeed: 200, AttackDelay: 988, AttackMotion: 288, DamageMotion: 168,
		Mobile: true, Fights: true,
		Drops: []Drop{{916, 9000}, {949, 700}, {507, 550}, {519, 300}},
	},
	1050: {
		ID: 1050, Name: "Picky", Sprite: "picky_", Level: 4, HP: 83,
		BaseExp: 5, JobExp: 4, Atk1: 8, Atk2: 11, Def: 20,
		Stats: [NumStats]int{AGI: 3, VIT: 3, INT: 10, DEX: 11, LUK: 20}, Range: 1, ChaseRange: 12,
		WalkSpeed: 200, AttackDelay: 988, AttackMotion: 288, DamageMotion: 168,
		Mobile: true, Fights: true,
		Drops: []Drop{{916, 9000}, {949, 700}, {507, 600}, {519, 300}},
	},
	1081: {
		ID: 1081, Name: "Yellow Plant", Sprite: "yellow_plant", Level: 1, HP: 10,
		Atk1: 1, Atk2: 2, Def: 100, Mdef: 99, Range: 1, ChaseRange: 12,
		WalkSpeed: 1000, AttackDelay: 1, AttackMotion: 1, DamageMotion: 1,
		Plant: true,
		Drops: []Drop{{508, 5500}, {712, 1000}, {711, 1000}, {905, 500}, {914, 500}},
	},
	1113: {
		ID: 1113, Name: "Drops", Sprite: "drops", Level: 3, HP: 55,
		BaseExp: 4, JobExp: 3, Atk1: 10, Atk2: 13,
		Stats: [NumStats]int{AGI: 3, VIT: 3, DEX: 12, LUK: 15}, Range: 1, ChaseRange: 12,
		WalkSpeed: 400, AttackDelay: 1372, AttackMotion: 672, DamageMotion: 480,
		Mobile: true, Fights: true,
		Drops: []Drop{{909, 7500}, {938, 500}, {512, 1100}, {713, 1700}, {512, 800}},
	},
}
