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
	// Assist: joins in when a nearby monster of the same kind is attacked
	// (eAthena mode flag 8).
	Assist bool

	// Element, race, size and aggression, filled in from mob_db at start-up.
	MobTraits

	Drops []Drop
}

// Drop is a chance (out of 10000, like rAthena) to drop an item on death.
type Drop struct {
	ItemID int
	Rate   int
}

// Mobs used on moc_fild07 and moc_fild06. Drops are trimmed to items in the
// item DB (weapons, arrows and cards are left out for now).
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
		// rAthena has Egg Shell at 0.1% (10); raised to 10% here.
		Drops: []Drop{{916, 9000}, {949, 700}, {507, 600}, {519, 300}, {5015, 1000}},
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

	// moc_fild06 monsters, from the November 2004 eAthena mob_db
	// (reference/eathena-2004/db/mob_db.txt). Mode 139 = assist; 131 = looter.
	1009: {
		ID: 1009, Name: "Condor", Sprite: "condor", Level: 5, HP: 92,
		BaseExp: 6, JobExp: 5, Atk1: 11, Atk2: 14,
		Stats: [NumStats]int{STR: 1, AGI: 13, VIT: 5, DEX: 13, LUK: 10}, Range: 1, ChaseRange: 12,
		WalkSpeed: 150, AttackDelay: 1148, AttackMotion: 648, DamageMotion: 480,
		Mobile: true, Fights: true, Assist: true,
		Drops: []Drop{{917, 6500}, {715, 80}, {517, 400}, {916, 2000}},
	},
	1019: {
		ID: 1019, Name: "PecoPeco", Sprite: "pecopeco", Level: 13, HP: 531,
		BaseExp: 85, JobExp: 36, Atk1: 35, Atk2: 46,
		Stats: [NumStats]int{STR: 1, AGI: 13, VIT: 13, INT: 25, DEX: 27, LUK: 9}, Range: 1, ChaseRange: 12,
		WalkSpeed: 200, AttackDelay: 1564, AttackMotion: 864, DamageMotion: 576,
		Mobile: true, Fights: true, Assist: true,
		Drops: []Drop{{925, 5500}, {508, 50}, {507, 900}},
	},
	1055: {
		ID: 1055, Name: "Muka", Sprite: "muka", Level: 15, HP: 570,
		BaseExp: 72, JobExp: 48, Atk1: 40, Atk2: 49, Def: 5, Mdef: 5,
		Stats: [NumStats]int{STR: 15, AGI: 15, VIT: 30, INT: 5, DEX: 20, LUK: 10}, Range: 1, ChaseRange: 12,
		WalkSpeed: 300, AttackDelay: 1960, AttackMotion: 960, DamageMotion: 384,
		Mobile: true, Fights: true,
		Drops: []Drop{{993, 70}, {952, 5500}, {713, 2000}, {511, 400}, {507, 1000}, {1002, 350}},
	},
	1095: {
		ID: 1095, Name: "Andre", Sprite: "andre", Level: 17, HP: 688,
		BaseExp: 109, JobExp: 71, Atk1: 60, Atk2: 71, Def: 10,
		Stats: [NumStats]int{STR: 1, AGI: 17, VIT: 24, INT: 20, DEX: 26, LUK: 20}, Range: 1, ChaseRange: 12,
		WalkSpeed: 300, AttackDelay: 1288, AttackMotion: 288, DamageMotion: 384,
		Mobile: true, Fights: true, Assist: true,
		Drops: []Drop{{955, 5500}, {910, 1000}, {938, 500}, {993, 40}, {1001, 4}, {1002, 450}, {757, 28}},
	},
	1097: {
		ID: 1097, Name: "Ant Egg", Sprite: "ant_egg", Level: 4, HP: 420,
		BaseExp: 5, JobExp: 4, Atk1: 1, Atk2: 2, Def: 20, Mdef: 20,
		Stats: [NumStats]int{STR: 1, AGI: 1, VIT: 1, DEX: 1, LUK: 20}, Range: 1, ChaseRange: 12,
		WalkSpeed: 1000, AttackDelay: 1001, AttackMotion: 1, DamageMotion: 1,
		Drops: []Drop{{1010, 320}, {935, 2500}, {909, 2000}, {938, 650}, {713, 2000}, {1002, 300}},
	},
	1105: {
		ID: 1105, Name: "Deniro", Sprite: "deniro", Level: 19, HP: 760,
		BaseExp: 135, JobExp: 85, Atk1: 68, Atk2: 79, Def: 15,
		Stats: [NumStats]int{STR: 1, AGI: 19, VIT: 30, INT: 20, DEX: 43, LUK: 10}, Range: 1, ChaseRange: 12,
		WalkSpeed: 150, AttackDelay: 1288, AttackMotion: 288, DamageMotion: 576,
		Mobile: true, Fights: true, Assist: true,
		Drops: []Drop{{955, 6000}, {910, 3000}, {938, 1200}, {990, 45}, {1001, 8}, {1002, 550}, {757, 34}},
	},
	1138: {
		ID: 1138, Name: "Magnolia", Sprite: "magnolia", Level: 26, HP: 3195,
		BaseExp: 393, JobExp: 248, Atk1: 120, Atk2: 151, Def: 5, Mdef: 30,
		Stats: [NumStats]int{STR: 1, AGI: 26, VIT: 26, DEX: 39, LUK: 5}, Range: 1, ChaseRange: 12,
		WalkSpeed: 250, AttackDelay: 1560, AttackMotion: 360, DamageMotion: 360,
		Mobile: true, Fights: true,
		Drops: []Drop{{7031, 5500}, {910, 800}, {911, 100}, {912, 10}, {737, 20}, {508, 250}},
	},
	1160: {
		ID: 1160, Name: "Piere", Sprite: "piere", Level: 18, HP: 733,
		BaseExp: 122, JobExp: 78, Atk1: 64, Atk2: 75, Def: 15,
		Stats: [NumStats]int{STR: 1, AGI: 18, VIT: 26, INT: 20, DEX: 27, LUK: 15}, Range: 1, ChaseRange: 12,
		WalkSpeed: 200, AttackDelay: 1288, AttackMotion: 288, DamageMotion: 576,
		Mobile: true, Fights: true, Assist: true,
		Drops: []Drop{{955, 5700}, {910, 1100}, {938, 600}, {992, 15}, {1001, 5}, {1002, 500}, {757, 31}},
	},
}
