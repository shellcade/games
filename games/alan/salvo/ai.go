package main

import "math"

// cpuAim points a CPU tank at the nearest live enemy by simulating shots across
// a grid of angles and powers (the real flight physics, wind included) and
// keeping the one that lands closest — then nudges it off perfect by a little so
// the CPU is a worthy, not unbeatable, opponent.
func cpuAim(t *tank, tanks []*tank, terrain []int, wind float64, jitter func() float64, missMul float64) {
	target := nearestEnemy(t, tanks)
	if target == nil {
		return
	}
	rightward := target.col > t.col

	bestA, bestP, bestD := t.angle, t.power, math.Inf(1)
	for a := 15.0; a <= 165.0; a += 4 {
		if rightward && a > 115 {
			continue // don't lob hard-left at a right-side target
		}
		if !rightward && a < 65 {
			continue
		}
		for p := 28.0; p <= 98.0; p += 4 {
			ix, iy, ok := simImpact(t, a, p, wind, terrain, tanks)
			if !ok {
				continue
			}
			d := math.Hypot(float64(target.col)-ix, target.y-iy)
			if d < bestD {
				bestD, bestA, bestP = d, a, p
			}
		}
	}

	// A touch of inaccuracy — more when the shot is awkward (longer range), and
	// scaled by difficulty (easy lobs wide, hard is nearly dead-on).
	miss := (4.0 + math.Min(8, bestD)) * missMul
	bestA += (jitter()*2 - 1) * miss
	bestP += (jitter()*2 - 1) * (miss + 2)

	t.angle = clampF(bestA, 1, 179)
	t.power = clampF(bestP, 5, 100)
	t.weapon = cpuWeapon(t, bestD)
}

func nearestEnemy(t *tank, tanks []*tank) *tank {
	var best *tank
	bestDist := math.Inf(1)
	for _, e := range tanks {
		if !e.alive || e == t {
			continue
		}
		if d := math.Abs(float64(e.col - t.col)); d < bestDist {
			bestDist, best = d, e
		}
	}
	return best
}

// cpuWeapon spends a HEAVY round only on a shot that looks like it'll land, and
// never wastes the feeble TRACER.
func cpuWeapon(t *tank, bestD float64) int {
	if t.ammo[1] != 0 && bestD < 3 {
		return 1 // HEAVY — go for the kill
	}
	return 0 // SHELL
}

// simImpact mirrors the live shell exactly, returning the impact point and
// whether the shot actually struck something (terrain or a tank) rather than
// sailing off the field.
func simImpact(self *tank, angle, power, wind float64, terrain []int, tanks []*tank) (ix, iy float64, ok bool) {
	spd := minSpeed + power/100*(maxSpeed-minSpeed)
	r := angle * math.Pi / 180
	dx, dy := math.Cos(r), -math.Sin(r)
	x := float64(self.col) + dx*barrelLen
	y := self.y - 0.6 + dy*barrelLen
	vx, vy := spd*math.Cos(r), -spd*math.Sin(r)/aspect

	const dt = 0.03
	for i := 0; i < 800; i++ {
		x, y, vx, vy = integrate(x, y, vx, vy, wind, dt)
		if x < 0 || x >= scrW || y > float64(groundBottom)+2 {
			return x, y, false // sailed off — a dud, no use to the CPU
		}
		for _, e := range tanks {
			if !e.alive || e == self {
				continue
			}
			if math.Hypot(float64(e.col)-x, e.y-y) < 1.2 {
				return x, y, true
			}
		}
		if solidAt(terrain, x, y) {
			return x, y, true
		}
	}
	return x, y, false
}
