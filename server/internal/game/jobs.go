package game

import "errors"

// Job IDs (RO's class numbers). JobNovice is in skills.go.
const (
	JobSwordman uint16 = 1
	JobMage     uint16 = 2
	JobArcher   uint16 = 3
	JobAcolyte  uint16 = 4
	JobMerchant uint16 = 5
	JobThief    uint16 = 6
)

// FirstJobs are the jobs a Novice can change into, in RO's order.
var FirstJobs = []uint16{JobSwordman, JobMage, JobArcher, JobAcolyte, JobMerchant, JobThief}

// Weapon types, in job_db1's attack delay column order (eAthena's weapon "View").
const (
	WeaponFist   = 0
	WeaponDagger = 1
	WeaponSword  = 2 // one-handed sword
	WeaponBow    = 11
	WeaponTypes  = 17
)

// JobDef is a job's numbers from the 2004 eAthena job_db1/job_db2 (see jobs_data.go).
type JobDef struct {
	ID           uint16
	Name         string
	BaseWeight   int // weight limit before STR
	HPFactor     int
	HPMultiplier int
	SPFactor     int
	// AttackDelay is the base attack animation (ms) per weapon type; 2000 means
	// the job can't use that weapon type.
	AttackDelay [WeaponTypes]int
	// JobBonus[i] is the stat gained at job level i+1 (0 none, 1 STR ... 6 LUK).
	JobBonus []int
	hpSigma  []int
}

// Job returns the definition of a job (Novice for unknown IDs).
func Job(id uint16) *JobDef {
	if j, ok := jobTable[id]; ok {
		return j
	}
	return jobTable[JobNovice]
}

// MaxJobLevel is 10 for Novices and 50 for 1st jobs.
func (j *JobDef) MaxJobLevel() int {
	if j.ID == JobNovice {
		return len(noviceJobExp) + 1
	}
	return len(firstJobExp) + 1
}

func (j *JobDef) jobExp() []int {
	if j.ID == JobNovice {
		return noviceJobExp
	}
	return firstJobExp
}

// MaxHP follows eAthena's status calculation:
// (3500 + lv*HPMultiplier + sigma(lv)) / 100 * (100 + VIT) / 100,
// where sigma adds HPFactor*j + 50 per level j, rounded down to hundreds.
func (j *JobDef) MaxHP(lv, vit int) int {
	if j.hpSigma == nil {
		k := 0
		j.hpSigma = make([]int, len(baseExpTable)+2)
		for l := 2; l < len(j.hpSigma); l++ {
			k += j.HPFactor*l + 50
			k -= k % 100
			j.hpSigma[l-1] = k
		}
	}
	sigma := j.hpSigma[min(max(lv-1, 0), len(j.hpSigma)-1)]
	return (3500 + lv*j.HPMultiplier + sigma) / 100 * (100 + vit) / 100
}

func (j *JobDef) MaxSP(lv, intel int) int {
	return (j.SPFactor*lv + 1000) / 100 * (100 + intel) / 100
}

// Bonus is the job's stat bonus at a job level.
func (j *JobDef) Bonus(jobLevel int) [NumStats]int {
	var b [NumStats]int
	for i := 0; i < jobLevel && i < len(j.JobBonus); i++ {
		if s := j.JobBonus[i]; s >= 1 && s <= NumStats {
			b[s-1]++
		}
	}
	return b
}

// CanUse reports whether a job may equip an item (2004 item_db Job bitmask).
func (d ItemDef) CanUse(job uint16) bool {
	return d.Jobs == 0 || d.Jobs&(1<<job) != 0
}

var (
	ErrNotNovice      = errors.New("already has a job")
	ErrJobLevelTooLow = errors.New("job level 10 needed")
	ErrUnspentPoints  = errors.New("skill points left to spend")
	ErrUnknownJob     = errors.New("unknown job")
	ErrWrongJob       = errors.New("your job can't use that")
)

// CanChangeJob explains why a Novice can't become a 1st job yet (2004 rule:
// Job Level 10 with every skill point spent).
func (c *CharData) CanChangeJob() error {
	if c.Job != JobNovice {
		return ErrNotNovice
	}
	if c.JobLevel < Job(JobNovice).MaxJobLevel() {
		return ErrJobLevelTooLow
	}
	if c.SkillPoints > 0 {
		return ErrUnspentPoints
	}
	return nil
}

// ChangeJob makes a Novice a 1st job: job level 1, and anything the new job
// can't wear is taken off.
func (c *CharData) ChangeJob(job uint16) error {
	if err := c.CanChangeJob(); err != nil {
		return err
	}
	if _, ok := jobTable[job]; !ok || job == JobNovice {
		return ErrUnknownJob
	}
	c.Job, c.JobLevel, c.JobExp = job, 1, 0
	for i := range c.Inventory {
		if c.Inventory[i].Equip != 0 && !Items[c.Inventory[i].ItemID].CanUse(job) {
			c.Inventory[i].Equip = 0
		}
	}
	d := c.Derive()
	c.HP, c.SP = min(c.HP, d.MaxHP), min(c.SP, d.MaxSP)
	return nil
}
