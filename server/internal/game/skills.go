package game

import "errors"

// Job IDs (RO's class numbers).
const (
	JobNovice uint16 = 0
)

// SkillTarget is what a skill is used on.
type SkillTarget string

const (
	TargetPassive SkillTarget = "passive" // always on, can't be used
	TargetSelf    SkillTarget = "self"
	TargetEnemy   SkillTarget = "enemy"
	TargetFriend  SkillTarget = "friend"
	TargetGround  SkillTarget = "ground"
)

// SkillReq is another skill that must be at Level first.
type SkillReq struct {
	ID    int `json:"id"`
	Level int `json:"level"`
}

// SkillDef describes a skill. Display names, descriptions and icons come from
// the client's skillnametable/skilldesctable and icon files, keyed by Name.
type SkillDef struct {
	ID       int         `json:"id"`
	Name     string      `json:"name"` // RO's internal (Aegis) name, e.g. NV_BASIC
	Job      uint16      `json:"job"`  // the job whose skill tree it's in
	MaxLevel int         `json:"maxLevel"`
	Target   SkillTarget `json:"target"`
	SP       []int       `json:"sp,omitempty"`       // SP cost per level
	CastTime []int       `json:"castTime,omitempty"` // ms per level
	Delays   []int       `json:"delays,omitempty"`   // ms before any skill can be used again, per level
	// Range in cells; negative means the weapon's reach plus (-Range - 1), as
	// in eAthena (-1 = melee weapon reach).
	Range     int        `json:"range,omitempty"`
	Element   int        `json:"element,omitempty"` // 0 neutral, 1 water, 3 fire, 4 wind, 5 poison, 6 holy
	Hits      []int      `json:"hits,omitempty"`    // hits per level (bolts hit once per level)
	Knockback int        `json:"knockback,omitempty"`
	Zeny      []int      `json:"zeny,omitempty"`     // zeny cost per level (Mammonite)
	Duration  []int      `json:"duration,omitempty"` // buff length in ms per level
	Weapons   []int      `json:"weapons,omitempty"`  // allowed weapon types; empty = any
	Requires  []SkillReq `json:"requires,omitempty"`
	// Quest skills are granted rather than bought with skill points.
	Quest bool `json:"quest,omitempty"`
}

// Skill IDs used by the game rules.
const (
	SkillBasic    = 1   // NV_BASIC
	SkillFirstAid = 142 // NV_FIRSTAID
)

// Skills is the skill database: the Novice skills below plus the 1st-job skills
// generated from the 2004 data (skills_data.go).
var Skills = map[int]*SkillDef{
	SkillBasic: {ID: SkillBasic, Name: "NV_BASIC", Job: JobNovice, MaxLevel: 9, Target: TargetPassive},
	SkillFirstAid: {
		ID: SkillFirstAid, Name: "NV_FIRSTAID", Job: JobNovice, MaxLevel: 1, Target: TargetSelf,
		SP: []int{3}, Quest: true,
	},
}

func init() {
	for _, s := range firstJobSkills {
		Skills[s.ID] = s
	}
	// The 2004 skill_db lists Magnum Break as targeted; in RO it bursts around
	// the caster, so it's used on yourself here.
	Skills[SkillMagnumBreak].Target = TargetSelf

	// The cities' monsters and items (gen_world.py); hand-written ones win.
	for _, m := range generatedMobs {
		if _, ok := Mobs[m.ID]; !ok {
			Mobs[m.ID] = m
		}
	}
	for _, it := range generatedItems {
		if _, ok := Items[it.ID]; !ok {
			Items[it.ID] = it
		}
	}
}

// 1st-job skill IDs used by the game rules.
const (
	SkillSwordMastery  = 2
	SkillHPRecovery    = 4
	SkillBash          = 5
	SkillMagnumBreak   = 7
	SkillSPRecovery    = 9
	SkillColdBolt      = 14
	SkillFireBolt      = 19
	SkillLightningBolt = 20
	SkillDivineProt    = 22
	SkillHeal          = 28
	SkillIncAgi        = 29
	SkillBlessing      = 34
	SkillEnlargeWeight = 36
	SkillDiscount      = 37
	SkillOvercharge    = 38
	SkillMammonite     = 42
	SkillOwlsEye       = 43
	SkillVulturesEye   = 44
	SkillDoubleStrafe  = 46
	SkillDoubleAttack  = 48
	SkillImproveDodge  = 49
	SkillEnvenom       = 52
)

// FirstAidHeal is the HP First Aid restores.
const FirstAidHeal = 5

var (
	ErrUnknownSkill   = errors.New("unknown skill")
	ErrSkillMaxed     = errors.New("skill is already at its maximum level")
	ErrNoSkillPoints  = errors.New("no skill points left")
	ErrSkillLocked    = errors.New("skill requirements not met")
	ErrNotLearned     = errors.New("skill not learned")
	ErrNotEnoughSP    = errors.New("not enough SP")
	ErrQuestSkill     = errors.New("quest skills can't be learned with skill points")
	ErrNeedBasicSkill = errors.New("Basic Skill level 3 is needed to sit")
)

// SkillLevel is the learned level of a skill (0 if not learned).
func (c *CharData) SkillLevel(id int) int { return c.Skills[id] }

// InTree reports whether a skill belongs to the character's skill tree.
// Later jobs keep the Novice skills, as in RO.
func (c *CharData) InTree(def *SkillDef) bool {
	return def.Job == c.Job || def.Job == JobNovice
}

// CanLearn explains why a skill can't be raised a level, or returns nil.
func (c *CharData) CanLearn(id int) error {
	def, ok := Skills[id]
	if !ok || !c.InTree(def) {
		return ErrUnknownSkill
	}
	if def.Quest {
		return ErrQuestSkill
	}
	if c.SkillLevel(id) >= def.MaxLevel {
		return ErrSkillMaxed
	}
	for _, req := range def.Requires {
		if c.SkillLevel(req.ID) < req.Level {
			return ErrSkillLocked
		}
	}
	if c.SkillPoints <= 0 {
		return ErrNoSkillPoints
	}
	return nil
}

// LearnSkill raises a skill one level, spending a skill point.
func (c *CharData) LearnSkill(id int) error {
	if err := c.CanLearn(id); err != nil {
		return err
	}
	if c.Skills == nil {
		c.Skills = map[int]int{}
	}
	c.Skills[id]++
	c.SkillPoints--
	return nil
}

// CanSit is RO's rule: sitting comes with Basic Skill level 3.
func (c *CharData) CanSit() bool { return c.SkillLevel(SkillBasic) >= 3 }

// SkillSP is the SP a skill costs at a level.
func (d *SkillDef) SkillSP(level int) int {
	if level < 1 || len(d.SP) == 0 {
		return 0
	}
	return d.SP[min(level, len(d.SP))-1]
}

// atLevel picks a per-level value (0 for level 0 or an empty table).
func atLevel(values []int, level int) int {
	if level < 1 || len(values) == 0 {
		return 0
	}
	return values[min(level, len(values))-1]
}

func (d *SkillDef) SkillDelay(level int) int    { return atLevel(d.Delays, level) }
func (d *SkillDef) SkillZeny(level int) int     { return atLevel(d.Zeny, level) }
func (d *SkillDef) SkillDuration(level int) int { return atLevel(d.Duration, level) }

// SkillHits is how many times the skill hits at a level (at least 1).
func (d *SkillDef) SkillHits(level int) int { return max(atLevel(d.Hits, level), 1) }

// UsableWith reports whether the skill can be used with a weapon type.
func (d *SkillDef) UsableWith(weaponType int) bool {
	if len(d.Weapons) == 0 {
		return true
	}
	for _, w := range d.Weapons {
		if w == weaponType {
			return true
		}
	}
	return false
}

// SkillCast is the cast time in ms at a level.
func (d *SkillDef) SkillCast(level int) int {
	if level < 1 || len(d.CastTime) == 0 {
		return 0
	}
	return d.CastTime[min(level, len(d.CastTime))-1]
}

// fillSkillDefaults gives characters saved before skills existed their skill
// points (1 per job level after the first) and the granted quest skills.
func (c *CharData) fillSkillDefaults() {
	if c.Skills != nil {
		return
	}
	c.Skills = map[int]int{SkillFirstAid: 1}
	c.SkillPoints = max(c.JobLevel-1, 0)
}
