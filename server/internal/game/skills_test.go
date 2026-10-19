package game

import (
	"errors"
	"testing"
)

func TestSkillPointsAndLearning(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	if c.SkillPoints != 0 || c.SkillLevel(SkillFirstAid) != 1 || c.CanSit() {
		t.Fatalf("new Novice: points=%d firstAid=%d", c.SkillPoints, c.SkillLevel(SkillFirstAid))
	}
	if err := c.LearnSkill(SkillBasic); !errors.Is(err, ErrNoSkillPoints) {
		t.Fatalf("learning without points: %v", err)
	}
	// Each job level grants a skill point.
	c.AddExp(0, 10+18+28)
	if c.JobLevel != 4 || c.SkillPoints != 3 {
		t.Fatalf("after 3 job levels: lv=%d points=%d", c.JobLevel, c.SkillPoints)
	}
	for range 3 {
		if err := c.LearnSkill(SkillBasic); err != nil {
			t.Fatal(err)
		}
	}
	if !c.CanSit() || c.SkillPoints != 0 {
		t.Fatalf("Basic Skill 3 should allow sitting: %+v", c.Skills)
	}
	if err := c.LearnSkill(SkillFirstAid); !errors.Is(err, ErrQuestSkill) {
		t.Fatalf("First Aid is a quest skill: %v", err)
	}
	c.Skills[SkillBasic], c.SkillPoints = 9, 1
	if err := c.LearnSkill(SkillBasic); !errors.Is(err, ErrSkillMaxed) {
		t.Fatalf("past max level: %v", err)
	}
	if err := c.LearnSkill(9999); !errors.Is(err, ErrUnknownSkill) {
		t.Fatalf("unknown skill: %v", err)
	}
}

func TestLegacyCharactersGetSkillPoints(t *testing.T) {
	c := CharData{BaseLevel: 5, JobLevel: 6, Stats: [NumStats]int{1, 1, 1, 1, 1, 1}}
	c.FillDefaults()
	if c.SkillPoints != 5 || c.SkillLevel(SkillFirstAid) != 1 {
		t.Fatalf("legacy job lv 6: points=%d skills=%v", c.SkillPoints, c.Skills)
	}
}

func TestCloneSharesNothing(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	c.add(501, 1)
	d := c.Clone()
	d.Skills[SkillBasic] = 5
	d.Inventory[0].Amount = 99
	if c.SkillLevel(SkillBasic) != 0 || c.Inventory[0].Amount != 1 {
		t.Fatal("Clone must not share skills or inventory")
	}
}
