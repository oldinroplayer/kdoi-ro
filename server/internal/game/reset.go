package game

import (
	"errors"
	"slices"
)

// Resets, as rAthena's (pre-renewal) stat and skill reset: everything spent
// comes back to be spent again.

// TotalStatusPoints is every status point a character of this base level has
// earned: the starting points plus those from each level up.
func TotalStatusPoints(baseLevel int) int {
	total := StartPoints
	for lv := 2; lv <= baseLevel; lv++ {
		total += StatusPointsFor(lv)
	}
	return total
}

// ResetStats puts every stat back to 1 and returns all status points.
func (c *CharData) ResetStats() {
	for i := range c.Stats {
		c.Stats[i] = 1
	}
	c.StatusPoints = TotalStatusPoints(c.BaseLevel)
	c.ClampVitals() // max HP/SP may have dropped
}

// ResetSkills unlearns every skill bought with skill points and returns the
// points. Quest skills (First Aid) stay, and so does Basic Skill for anyone
// past Novice: sitting and job changes rely on it (rAthena keeps it too).
// It returns how many points came back.
func (c *CharData) ResetSkills() int {
	refunded := 0
	for id, lv := range c.Skills {
		def, ok := Skills[id]
		if !ok || def.Quest || (id == SkillBasic && c.Job != JobNovice) {
			continue
		}
		refunded += lv
		delete(c.Skills, id)
	}
	c.SkillPoints += refunded
	c.ClampVitals() // passives like SP Recovery or HP Recovery no longer count
	return refunded
}

// ErrNotFirstJob means a job switch was asked of a Novice (they take the Job
// Guide's test instead) or towards a job that isn't a 1st job.
var ErrNotFirstJob = errors.New("only 1st jobs can switch to another 1st job")

// SwitchJob turns a 1st job into another 1st job, keeping its job level and
// job EXP, and resets stats and skills so every point can be spent again on
// the new job (Basic Skill and quest skills stay). Gear the new job can't use
// comes off.
func (c *CharData) SwitchJob(job uint16) error {
	if !slices.Contains(FirstJobs, c.Job) || !slices.Contains(FirstJobs, job) {
		return ErrNotFirstJob
	}
	if job == c.Job {
		return ErrSameJob
	}
	c.ResetSkills() // the old job's skills, while it's still the old job
	c.Job = job
	for i := range c.Inventory {
		if c.Inventory[i].Equip != 0 && !Items[c.Inventory[i].ItemID].CanUse(job) {
			c.Inventory[i].Equip = 0
		}
	}
	c.ResetStats()
	return nil
}

// ErrSameJob means a switch to the job the character already has.
var ErrSameJob = errors.New("that's already your job")
