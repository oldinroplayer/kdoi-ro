package game

// baseNext[lv] is the base EXP needed to go from lv to lv+1 (pre-renewal table).
var baseNext = []int{0,
	9, 16, 25, 36, 77, 112, 153, 200, 253, 320,
	385, 490, 585, 700, 830, 970, 1120, 1260, 1420, 1620,
	1860, 1990, 2240, 2504, 2950, 3426, 3934, 4474, 6889,
}

// noviceJobNext[lv] is the Novice job EXP from lv to lv+1.
var noviceJobNext = []int{0, 10, 18, 28, 40, 91, 151, 205, 268, 340}

// Level caps: the base table above covers 1-30 for now; Novice job tops out at 10.
var (
	MaxBaseLevel = len(baseNext)
	MaxJobLevel  = len(noviceJobNext)
)

// BaseNext is the EXP needed for the next base level (0 at the cap).
func (c *CharData) BaseNext() int {
	if c.BaseLevel >= MaxBaseLevel {
		return 0
	}
	return baseNext[c.BaseLevel]
}

func (c *CharData) JobNext() int {
	if c.JobLevel >= MaxJobLevel {
		return 0
	}
	return noviceJobNext[c.JobLevel]
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
