package world

import "time"

// Maps sleep while no real player is on them: their monsters don't think,
// wander or fight, which saves the server's work on empty maps. A map wakes
// as soon as a player arrives (before it is sent the map's contents, so
// everything is simply there) and stays awake for mapGrace after the last
// player leaves, so warping out and straight back finds it as it was.
const mapGrace = time.Minute

// awake reports whether a map's monsters should run this tick.
func (m *mapInstance) awake(now time.Time) bool {
	return len(m.players) > 0 || now.Before(m.sleepAt)
}

// emptied starts a map's grace period when its last player leaves.
func (m *mapInstance) emptied(now time.Time) {
	m.sleepAt = now.Add(mapGrace)
}

// tickMaps runs the monsters of awake maps and puts newly idle maps to sleep.
func (w *World) tickMaps(now time.Time) {
	for _, m := range w.maps {
		if !m.awake(now) {
			if !m.asleep {
				m.asleep = true
				freeze(m)
			}
			continue
		}
		m.asleep = false
		for _, mob := range m.monsters {
			w.tickMonster(mob, now)
		}
	}
}

// freeze stops a falling-asleep map's monsters where they stand, so they don't
// replay a stale walk when it wakes. Nobody is there to tell.
func freeze(m *mapInstance) {
	for _, mob := range m.monsters {
		mob.path = nil
		mob.target = 0
	}
}
