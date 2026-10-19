package game

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
