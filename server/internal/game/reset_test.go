package game

import (
	"errors"
	"testing"
)

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

func TestSwitchJob(t *testing.T) {
	c := NewChar("town", 1, 1)
	if err := c.SwitchJob(JobMage); !errors.Is(err, ErrNotFirstJob) {
		t.Fatalf("a Novice can't switch: %v", err)
	}
	c.Job, c.JobLevel, c.JobExp, c.BaseLevel = JobSwordman, 20, 123, 20
	c.Skills = map[int]int{SkillBasic: 9, SkillFirstAid: 1, SkillBash: 10, SkillMagnumBreak: 5}
	c.SkillPoints = 4
	c.Stats[STR], c.Stats[VIT], c.StatusPoints = 40, 20, 0
	c.add(1101, 1) // Sword: a Mage can't use it
	sword := c.Inventory[len(c.Inventory)-1].Index
	if err := c.Equip(sword); err != nil {
		t.Fatal(err)
	}

	if err := c.SwitchJob(JobSwordman); !errors.Is(err, ErrSameJob) {
		t.Fatalf("same job: %v", err)
	}
	if err := c.SwitchJob(JobMage); err != nil {
		t.Fatal(err)
	}
	if c.Job != JobMage || c.JobLevel != 20 || c.JobExp != 123 {
		t.Fatalf("job %d level %d exp %d: the job changes, its level and EXP stay", c.Job, c.JobLevel, c.JobExp)
	}
	if c.SkillPoints != 19 || c.SkillLevel(SkillBash) != 0 || c.SkillLevel(SkillBasic) != 9 || c.SkillLevel(SkillFirstAid) != 1 {
		t.Fatalf("skills: %d points, bash %d, basic %d, first aid %d", c.SkillPoints, c.SkillLevel(SkillBash), c.SkillLevel(SkillBasic), c.SkillLevel(SkillFirstAid))
	}
	if c.Stats[STR] != 1 || c.StatusPoints != TotalStatusPoints(c.BaseLevel) {
		t.Fatalf("stats: STR %d, %d points", c.Stats[STR], c.StatusPoints)
	}
	if it := c.entry(sword); it.Equip != 0 {
		t.Fatal("the Sword should come off a Mage")
	}
	if err := c.SwitchJob(JobNovice); !errors.Is(err, ErrNotFirstJob) {
		t.Fatalf("back to Novice isn't a switch: %v", err)
	}
}
