package game

// MaxBaseLevel is 99; EXP tables come from the 2004 exp.txt (jobs_data.go).
var MaxBaseLevel = len(baseExpTable) + 1

// BaseNext is the EXP needed for the next base level (0 at the cap).
func (c *CharData) BaseNext() int {
	if c.BaseLevel < 1 || c.BaseLevel >= MaxBaseLevel {
		return 0
	}
	return baseExpTable[c.BaseLevel-1]
}

// JobNext is the job EXP needed for the next job level (0 at the cap: 10 for
// Novices, 50 for 1st jobs).
func (c *CharData) JobNext() int {
	table := Job(c.Job).jobExp()
	if c.JobLevel < 1 || c.JobLevel > len(table) {
		return 0
	}
	return table[c.JobLevel-1]
}

// StatusPointsFor is the status points gained on reaching base level lv.
func StatusPointsFor(lv int) int { return lv/5 + 3 }

// AddExp grants EXP, applying any level ups (each fully restores HP and SP,
// as in RO). It returns how many base and job levels were gained.
func (c *CharData) AddExp(base, job int) (baseUps, jobUps int) {
	if next := c.BaseNext(); next > 0 {
		c.BaseExp += base
		for next > 0 && c.BaseExp >= next {
			c.BaseExp -= next
			c.BaseLevel++
			c.StatusPoints += StatusPointsFor(c.BaseLevel)
			baseUps++
			next = c.BaseNext()
		}
		if next == 0 {
			c.BaseExp = 0
		}
	}
	if next := c.JobNext(); next > 0 {
		c.JobExp += job
		for next > 0 && c.JobExp >= next {
			c.JobExp -= next
			c.JobLevel++
			c.SkillPoints++ // one skill point per job level, as in RO
			jobUps++
			next = c.JobNext()
		}
		if next == 0 {
			c.JobExp = 0
		}
	}
	if baseUps > 0 {
		d := c.Derive()
		c.HP, c.SP = d.MaxHP, d.MaxSP
	}
	return baseUps, jobUps
}
