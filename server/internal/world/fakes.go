package world

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"kdoiro/server/internal/game"
	"kdoiro/server/internal/path"
	"kdoiro/server/internal/proto"
)

// Fake players make maps feel lived in: while fakes are on (the console's
// fakeplayers, or Config.FakePlayers) and a real player wakes a map with
// monsters, 15-20 of them join it, with a level and job suited to its
// monsters, and wander and fight like players would. They leave when the map
// goes back to sleep. They're Players with no connection behind them: never
// saved, never counted as online, can't be whispered, and earn no EXP.

const (
	fakeMin, fakeMax = 15, 20
	fakeSight        = 12 // cells: how far they look for monsters
	fakeThinkMin     = 600 * time.Millisecond
	fakeThinkMax     = 1600 * time.Millisecond
	fakeRevive       = 8 * time.Second // knocked out fakes get back up after this
	fakeWanderMin    = 5
	fakeWanderMax    = 14
	// Fakes are this many levels above the map's average monster, plus up to
	// fakeLevelSpread more, so they hold their own.
	fakeLevelAbove  = 5
	fakeLevelSpread = 3
)

// fakeSkillChance is how often (percent of decisions in a fight) a fake of a
// melee job uses its attack skill, so its SP lasts.
const fakeSkillChance = 35

// fakeAttackSkills are the attack skills fake 1st jobs use on monsters.
var fakeAttackSkills = map[uint16]int{
	game.JobSwordman: game.SkillBash,
	game.JobArcher:   game.SkillDoubleStrafe,
	game.JobMerchant: game.SkillMammonite,
	game.JobThief:    game.SkillEnvenom,
}

// fakeBolts are the bolts fake Mages know, each with its element.
var fakeBolts = []struct{ skill, element int }{
	{game.SkillFireBolt, game.ElemFire},
	{game.SkillColdBolt, game.ElemWater},
	{game.SkillLightningBolt, game.ElemWind},
}

// nobody is the Outbox of a fake player: nothing listens.
type nobody struct{}

func (nobody) Send([]byte) {}

// FakePlayers reports whether fake players are on.
func (w *World) FakePlayers() bool {
	var on bool
	w.do(func() { on = w.fakesOn })
	return on
}

// SetFakePlayers turns fake players on (they join maps with real players as
// those maps are next ticked) or off (they all leave now).
func (w *World) SetFakePlayers(on bool) error {
	return w.do(func() {
		w.fakesOn = on
		if !on {
			for _, m := range w.maps {
				w.removeFakes(m)
			}
		}
	})
}

// realPlayers counts the players on a map who aren't fakes.
func (m *mapInstance) realPlayers() int {
	n := 0
	for _, p := range m.players {
		if !p.fake {
			n++
		}
	}
	return n
}

// tickFakes fills awake maps that have real players and monsters with fakes,
// and clears them from maps gone to sleep.
func (w *World) tickFakes(m *mapInstance, now time.Time) {
	switch {
	case !w.fakesOn || m.asleep:
		w.removeFakes(m)
	case len(m.fakes) == 0 && len(m.monsters) > 0 && m.realPlayers() > 0:
		w.spawnFakes(m, now)
	}
}

func (w *World) spawnFakes(m *mapInstance, now time.Time) {
	level := mobLevel(m)
	for range fakeMin + w.rng.IntN(fakeMax-fakeMin+1) {
		x, y, ok := w.randomCell(m)
		if !ok {
			return
		}
		p := w.newFake(m, level, x, y, now)
		w.players[p.id] = p
		m.fakes = append(m.fakes, p.id)
		w.place(p, m, x, y)
	}
}

func (w *World) removeFakes(m *mapInstance) {
	for _, id := range m.fakes {
		if p := w.players[id]; p != nil {
			w.removeFromMap(p)
			delete(w.players, id)
		}
	}
	m.fakes = nil
}

// mobLevel is the average level of a map's monsters.
func mobLevel(m *mapInstance) int {
	total, n := 0, 0
	for _, mob := range m.monsters {
		total += mob.def.Level
		n++
	}
	if n == 0 {
		return 1
	}
	return total / n
}

// newFake makes a character fit for a map whose monsters average `level`:
// 5-8 levels above it, a Novice below base level 10 and otherwise a random
// 1st job, with stats raised the way that job usually raises them and gear
// to match. Mages also know the three bolts.
func (w *World) newFake(m *mapInstance, level, x, y int, now time.Time) *Player {
	c := game.NewChar(m.name, x, y)
	c.BaseLevel = min(max(level+fakeLevelAbove+w.rng.IntN(fakeLevelSpread+1), 1), 60)
	if c.BaseLevel >= 10 {
		c.Job = game.FirstJobs[w.rng.IntN(len(game.FirstJobs))]
		c.JobLevel = 1 + w.rng.IntN(min(c.BaseLevel-8, game.Job(c.Job).MaxJobLevel()-1))
	} else {
		c.JobLevel = min(c.BaseLevel, game.Job(game.JobNovice).MaxJobLevel())
	}
	c.Skills[game.SkillBasic] = 9 // so they can sit to rest
	// Their job's skills, about as high as their job level's points allow.
	lv := min(max((c.JobLevel+2)/3, 1), 10)
	switch c.Job {
	case game.JobMage:
		for _, b := range fakeBolts {
			c.Skills[b.skill] = lv
		}
		c.Skills[game.SkillSPRecovery] = lv
	case game.JobAcolyte:
		c.Skills[game.SkillHeal] = lv
		c.Skills[game.SkillIncAgi] = lv
		c.Skills[game.SkillBlessing] = lv
	default:
		if id, ok := fakeAttackSkills[c.Job]; ok {
			c.Skills[id] = min(lv, game.Skills[id].MaxLevel)
		}
	}
	c.Hair = 1 + w.rng.IntN(game.HairStyles)
	c.HairColor = w.rng.IntN(game.HairColors)
	c.Female = w.rng.IntN(2) == 0
	c.StatusPoints = game.TotalStatusPoints(c.BaseLevel)
	raiseLike(&c, c.Job, w.rng.IntN)
	c.Inventory = nil
	for _, id := range fakeGear(c.Job, w.rng.IntN) {
		if _, ok := game.Items[id]; !ok {
			continue
		}
		amount := 1
		if game.Items[id].Type == game.TypeAmmo {
			amount = 5000
		}
		c.Inventory = append(c.Inventory, game.InvItem{Index: len(c.Inventory), ItemID: id, Amount: amount})
		_ = c.Equip(len(c.Inventory) - 1) // too low a level or the wrong job: carried, not worn
	}
	c.HP = 0
	c.ClampVitals() // full HP and SP

	w.nextID++
	return &Player{
		unit: unit{id: w.nextID, dir: uint8(w.rng.IntN(8)), speed: WalkSpeed},
		Name: w.fakeName(), data: c, out: nobody{}, fake: true,
		nextHPRegen: now.Add(hpRegenInterval), nextSPRegen: now.Add(spRegenInterval),
		fakeThink: now.Add(time.Duration(w.rng.Int64N(int64(fakeThinkMax)))),
	}
}

// statPlans are the stats each job usually raises, most wanted first.
var statPlans = map[uint16][]int{
	game.JobNovice:   {game.STR, game.AGI, game.VIT, game.DEX},
	game.JobSwordman: {game.STR, game.VIT, game.AGI, game.DEX},
	game.JobMage:     {game.INT, game.DEX, game.VIT},
	game.JobArcher:   {game.DEX, game.AGI, game.LUK},
	game.JobAcolyte:  {game.INT, game.VIT, game.DEX},
	game.JobMerchant: {game.STR, game.VIT, game.DEX},
	game.JobThief:    {game.AGI, game.STR, game.DEX, game.LUK},
}

// raiseLike spends status points along a job's plan: mostly its first stat,
// sometimes the others, until no raise is affordable.
func raiseLike(c *game.CharData, job uint16, intn func(int) int) {
	plan := statPlans[job]
	for range 400 {
		stat := plan[0]
		if r := intn(10); r >= 5 {
			stat = plan[1+r%(len(plan)-1)]
		}
		if c.RaiseStat(stat) != nil && c.RaiseStat(plan[0]) != nil {
			return
		}
	}
}

// fakeGear is a job's usual weapon and some armor (picked at random).
func fakeGear(job uint16, intn func(int) int) []int {
	pick := func(ids ...int) int { return ids[intn(len(ids))] }
	var weapon []int
	switch job {
	case game.JobSwordman:
		weapon = []int{pick(1101, 1104, 1107, 1116)}
	case game.JobMage:
		weapon = []int{pick(1601, 1604)}
	case game.JobArcher:
		weapon = []int{pick(1701, 1704), 1750} // and arrows
	case game.JobAcolyte:
		weapon = []int{pick(1501, 1504)}
	case game.JobMerchant:
		weapon = []int{pick(1301, 1351, 1501)}
	case game.JobThief:
		weapon = []int{pick(1201, 1204, 1207, 1210)}
	default:
		weapon = []int{pick(1201, 1207, 1101)}
	}
	gear := append(weapon, pick(2301, 2303, 2305, 2312), pick(2401, 2403), pick(2501, 2503))
	if job != game.JobArcher && intn(2) == 0 {
		gear = append(gear, pick(2101, 2103))
	}
	if intn(3) > 0 {
		gear = append(gear, pick(2220, 2226, 2211, 2208, 5015, 2201))
	}
	return gear
}

// fakeName makes up a name like a player's, unused by anyone on the server.
func (w *World) fakeName() string {
	starts := []string{"Ka", "Ri", "Mo", "Su", "Lu", "Ze", "Ny", "Ar", "El", "Vi", "To", "Ha", "Ji", "Ro", "Mi", "Sa", "Ky", "Ne", "Ra", "Yu", "Da", "Fe", "Gi", "Po"}
	middles := []string{"ra", "ko", "ne", "ta", "ri", "ro", "no", "mi", "za", "ru", "ki", "ya", "len", "vin", "sha", "dor"}
	tails := []string{"", "", "", "x", "o", "a", "_", "99", "kun", "chan", "Z", "XD", "07", "ie"}
	for range 50 {
		name := starts[w.rng.IntN(len(starts))] + middles[w.rng.IntN(len(middles))] + tails[w.rng.IntN(len(tails))]
		if w.rng.IntN(4) == 0 {
			name = strings.ToLower(name)
		}
		if len(name) < 4 || w.nameTaken(name) {
			continue
		}
		return name
	}
	w.nextID++
	return fmt.Sprintf("Adventurer%d", w.nextID%100000)
}

// nameTaken is whether a real or fake player is using a name.
func (w *World) nameTaken(name string) bool {
	if _, ok := w.online[strings.ToLower(name)]; ok {
		return true
	}
	for _, p := range w.players {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// tickFake is a fake player's mind: get up when knocked out, rest when hurt,
// fight a monster nearby, or wander off to find one.
func (w *World) tickFake(p *Player, now time.Time) {
	if now.Before(p.fakeThink) {
		return
	}
	p.fakeThink = now.Add(fakeThinkMin + time.Duration(w.rng.Int64N(int64(fakeThinkMax-fakeThinkMin))))
	if p.dead {
		if p.deadSince.IsZero() {
			p.deadSince = now
		} else if now.Sub(p.deadSince) > fakeRevive {
			w.reviveFake(p)
		}
		return
	}
	d := p.data.Derive()
	if p.sitting {
		if p.data.HP*10 >= d.MaxHP*9 {
			w.setSitting(p, false)
		}
		return
	}
	if p.casting != nil || p.pending != nil {
		return // casting, or walking up to cast
	}
	if mob := w.monsters[p.target]; mob != nil && mob.m == p.m {
		w.fakeCast(p, mob, now) // Mages keep bolting while they fight
		return
	}
	p.target = 0
	// Casters rest for SP too.
	tired := (p.data.Job == game.JobMage || p.data.Job == game.JobAcolyte) && p.data.SP*100 < d.MaxSP*20
	if p.data.HP*100 < d.MaxHP*35 || tired {
		w.setSitting(p, true)
		return
	}
	if mob := w.fakePrey(p); mob != nil {
		p.target = mob.id
		p.nextRepath = time.Time{}
		w.fakeCast(p, mob, now)
		return
	}
	if !p.walking() && w.rng.IntN(2) == 0 {
		w.wanderFake(p)
	}
}

// fakeCast has a fake use its job's skills in a fight (the auto-attack
// carries on after each cast):
//   - Mages always bolt, with the bolt the monster is weakest to;
//   - Acolytes heal themselves when hurt and keep Blessing and Increase AGI up;
//   - Swordmen, Archers, Merchants and Thieves sometimes use their attack
//     skill (Bash, Double Strafe, Mammonite, Envenom).
func (w *World) fakeCast(p *Player, mob *Monster, now time.Time) {
	if now.Before(p.nextSkill) {
		return
	}
	switch p.data.Job {
	case game.JobMage:
		best, bestFix := 0, -1
		for _, b := range fakeBolts {
			if fix := mob.def.ElementFix(b.element); fix > bestFix {
				best, bestFix = b.skill, fix
			}
		}
		w.fakeUse(p, best, mob.id, now)
	case game.JobAcolyte:
		d := p.data.Derive()
		switch {
		case p.data.HP*100 < d.MaxHP*60:
			w.fakeUse(p, game.SkillHeal, p.id, now)
		case p.data.Buffs[game.SkillBlessing] == 0:
			w.fakeUse(p, game.SkillBlessing, p.id, now)
		case p.data.Buffs[game.SkillIncAgi] == 0:
			w.fakeUse(p, game.SkillIncAgi, p.id, now)
		}
	default:
		if id, ok := fakeAttackSkills[p.data.Job]; ok && w.rng.IntN(100) < fakeSkillChance {
			w.fakeUse(p, id, mob.id, now)
		}
	}
}

// fakeUse casts a skill at the highest level the fake's SP affords.
func (w *World) fakeUse(p *Player, skillID int, target uint32, now time.Time) {
	def := game.Skills[skillID]
	if def == nil {
		return
	}
	for lv := p.data.SkillLevel(skillID); lv >= 1; lv-- {
		if def.SkillSP(lv) <= p.data.SP {
			_ = w.startSkill(p, skillID, lv, target, now)
			return
		}
	}
}

// fakePrey picks a monster near a fake to fight: the closest few, at random,
// skipping plants and ones other fakes are already on.
func (w *World) fakePrey(p *Player) *Monster {
	taken := map[uint32]bool{}
	for _, id := range p.m.fakes {
		if other := w.players[id]; other != nil && other != p {
			taken[other.target] = true
		}
	}
	var near []*Monster
	for _, mob := range p.m.monsters {
		if mob.def.Plant || taken[mob.id] || chebyshev(p.x, p.y, mob.x, mob.y) > fakeSight {
			continue
		}
		near = append(near, mob)
	}
	if len(near) == 0 {
		return nil
	}
	slices.SortFunc(near, func(a, b *Monster) int {
		return chebyshev(p.x, p.y, a.x, a.y) - chebyshev(p.x, p.y, b.x, b.y)
	})
	return near[w.rng.IntN(min(len(near), 3))]
}

// wanderFake walks a fake to a random cell a little way off (never onto a warp).
func (w *World) wanderFake(p *Player) {
	for range 10 {
		dist := fakeWanderMin + w.rng.IntN(fakeWanderMax-fakeWanderMin+1)
		x, y := p.x+w.rng.IntN(2*dist+1)-dist, p.y+w.rng.IntN(2*dist+1)-dist
		if !p.m.grid.Walkable(x, y) {
			continue
		}
		if _, onWarp := p.m.warpAt(x, y); onWarp {
			continue
		}
		if route := p.routeTo(path.Point{X: x, Y: y}, 0, MaxWalkSteps, time.Now()); route != nil {
			w.broadcast(p.m, proto.PlayerMove(p.id, p.x, p.y, cells(route)))
			return
		}
	}
}

// reviveFake gets a knocked out fake back up somewhere else on its map.
func (w *World) reviveFake(p *Player) {
	x, y, ok := w.randomCell(p.m)
	if !ok {
		return
	}
	p.dead, p.deadSince = false, time.Time{}
	p.data.HP = 0
	p.data.ClampVitals()
	m := p.m
	w.removeFromMap(p)
	w.place(p, m, x, y)
}
