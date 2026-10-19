package game

import (
	"errors"
	"testing"
)

// readyNovice is a Novice that meets the 2004 job change rule.
func readyNovice() CharData {
	c := NewChar("morocc", 0, 0)
	c.JobLevel, c.SkillPoints = 10, 0
	c.Skills[SkillBasic] = 9
	return c
}

func TestJobChangeRequirements(t *testing.T) {
	c := NewChar("morocc", 0, 0)
	if err := c.ChangeJob(JobSwordman); !errors.Is(err, ErrJobLevelTooLow) {
		t.Fatalf("job level 1: %v", err)
	}
	c.JobLevel, c.SkillPoints = 10, 2
	if err := c.ChangeJob(JobSwordman); !errors.Is(err, ErrUnspentPoints) {
		t.Fatalf("unspent points: %v", err)
	}
	c = readyNovice()
	if err := c.ChangeJob(JobNovice); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("to Novice: %v", err)
	}
	if err := c.ChangeJob(JobSwordman); err != nil {
		t.Fatal(err)
	}
	if c.Job != JobSwordman || c.JobLevel != 1 || c.JobExp != 0 || c.SkillLevel(SkillBasic) != 9 {
		t.Fatalf("after change: %+v", c)
	}
	if err := c.ChangeJob(JobMage); !errors.Is(err, ErrNotNovice) {
		t.Fatalf("changing twice: %v", err)
	}
}

func TestJobChangeTakesOffForbiddenGear(t *testing.T) {
	c := readyNovice()
	c.add(1101, 1) // Sword: no Mages
	c.add(1201, 1) // Knife: Mages fine
	c.BaseLevel = 2
	c.Equip(c.Inventory[0].Index)
	if err := c.ChangeJob(JobMage); err != nil {
		t.Fatal(err)
	}
	if c.Inventory[0].Equip != 0 {
		t.Fatal("a Mage can't keep a Sword equipped")
	}
	if err := c.Equip(c.Inventory[0].Index); !errors.Is(err, ErrWrongJob) {
		t.Fatalf("Mage equipping a Sword: %v", err)
	}
	if err := c.Equip(c.Inventory[1].Index); err != nil {
		t.Fatalf("Mage equipping a Knife: %v", err)
	}

	a := readyNovice()
	a.add(1201, 1)
	a.ChangeJob(JobAcolyte)
	if err := a.Equip(a.Inventory[0].Index); !errors.Is(err, ErrWrongJob) {
		t.Fatalf("Acolyte equipping a Knife: %v", err)
	}
}

func TestJobStats(t *testing.T) {
	c := readyNovice()
	novice := c.Derive()
	c.ChangeJob(JobSwordman)
	sw := c.Derive()
	if sw.MaxWeight != 2800+30*c.Stats[STR] {
		t.Fatalf("Swordman weight = %d", sw.MaxWeight)
	}
	if sw.MaxHP < novice.MaxHP || sw.MaxSP < novice.MaxSP {
		t.Fatalf("Swordman HP/SP should beat a Novice's: %+v vs %+v", sw, novice)
	}
	// Swordman job level 2 grants STR +1 (job_db2).
	c.JobLevel = 2
	if b := c.Derive().Bonus; b[STR] != 1 {
		t.Fatalf("job bonus at Lv 2 = %v", b)
	}
	// 1st jobs use the 1st-job EXP table and cap at job level 50.
	c.JobLevel = 1
	if c.JobNext() != 30 || Job(JobSwordman).MaxJobLevel() != 50 {
		t.Fatalf("1st job EXP: next=%d max=%d", c.JobNext(), Job(JobSwordman).MaxJobLevel())
	}
	// Base EXP now runs to level 99.
	c.BaseLevel = 98
	if c.BaseNext() == 0 || MaxBaseLevel != 99 {
		t.Fatalf("base cap: next=%d max=%d", c.BaseNext(), MaxBaseLevel)
	}
}

func TestSwordmanSwingsDaggersFaster(t *testing.T) {
	n := readyNovice()
	n.add(1201, 1)
	n.Equip(n.Inventory[0].Index)
	s := n.Clone()
	s.ChangeJob(JobSwordman)
	s.Equip(s.Inventory[0].Index)
	if s.Derive().Aspd <= n.Derive().Aspd {
		t.Fatalf("Swordman dagger ASPD %d should beat Novice %d", s.Derive().Aspd, n.Derive().Aspd)
	}
}
