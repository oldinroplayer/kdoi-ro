package game

import "testing"

func TestResetStats(t *testing.T) {
	c := NewChar("town", 1, 1)
	c.AddExp(1_000_000, 0) // several base levels
	if c.BaseLevel < 5 {
		t.Fatalf("base level %d", c.BaseLevel)
	}
	c.Stats[STR], c.Stats[AGI], c.StatusPoints = 30, 20, 7
	c.ResetStats()
	if c.Stats != [NumStats]int{1, 1, 1, 1, 1, 1} || c.StatusPoints != TotalStatusPoints(c.BaseLevel) {
		t.Fatalf("after reset: stats %v, %d points, want %d", c.Stats, c.StatusPoints, TotalStatusPoints(c.BaseLevel))
	}
	if TotalStatusPoints(1) != StartPoints || TotalStatusPoints(2) != StartPoints+StatusPointsFor(2) {
		t.Fatal("TotalStatusPoints counts the start points plus each level's")
	}
}

func TestResetSkills(t *testing.T) {
	c := NewChar("town", 1, 1)
	c.Job = JobNovice
	c.Skills = map[int]int{SkillBasic: 9, SkillFirstAid: 1}
	c.SkillPoints = 0
	if got := c.ResetSkills(); got != 9 || c.SkillLevel(SkillBasic) != 0 || c.SkillLevel(SkillFirstAid) != 1 {
		t.Fatalf("Novice: refunded %d, basic %d, first aid %d", got, c.SkillLevel(SkillBasic), c.SkillLevel(SkillFirstAid))
	}

	c.Job = 1 // Swordman
	c.Skills = map[int]int{SkillBasic: 9, SkillFirstAid: 1, SkillBash: 5, SkillMagnumBreak: 2}
	c.SkillPoints = 1
	if got := c.ResetSkills(); got != 7 || c.SkillPoints != 8 || c.SkillLevel(SkillBasic) != 9 || c.SkillLevel(SkillBash) != 0 {
		t.Fatalf("Swordman: refunded %d, points %d, basic %d, bash %d", got, c.SkillPoints, c.SkillLevel(SkillBasic), c.SkillLevel(SkillBash))
	}
}
