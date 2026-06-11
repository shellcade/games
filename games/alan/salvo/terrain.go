package main

import "math"

// The battlefield ground is a per-column heightmap: terrain[c] is the row of the
// topmost ground cell in column c, and ground fills from there down to
// groundBottom. A column whose surface is pushed past groundBottom is a pit —
// no ground at all, and a tank standing there falls off the map.

type rng interface {
	Intn(n int) int
	Float64() float64
}

// genTerrain builds rolling hills from a few layered waves with random phase and
// amplitude, so every battle has fresh, natural-looking ground.
func genTerrain(rand rng) []int {
	mid := float64(surfMin+surfMax) / 2
	amp := float64(surfMax-surfMin) / 2

	type wave struct{ k, phase, a float64 }
	ws := make([]wave, 3)
	for i := range ws {
		ws[i] = wave{
			k:     (1 + float64(i)) * (1.0 + rand.Float64()),
			phase: rand.Float64() * 2 * math.Pi,
			a:     (1.0 / float64(i+1)) * (0.6 + rand.Float64()*0.6),
		}
	}

	t := make([]int, scrW)
	for c := 0; c < scrW; c++ {
		x := float64(c) / scrW * 2 * math.Pi
		h := 0.0
		for _, w := range ws {
			h += w.a * math.Sin(w.k*x+w.phase)
		}
		row := mid - h*amp*0.45
		t[c] = clampI(int(math.Round(row)), surfMin, surfMax)
	}
	return t
}

// surfaceAt is the row a tank in column c rests on (one above the ground).
func surfaceAt(terrain []int, c int) float64 { return float64(terrain[c]) - 1 }

// grounded reports whether column c still has ground to stand on.
func grounded(terrain []int, c int) bool { return terrain[c] <= groundBottom }

// solidAt reports whether the point (x,y) is inside the ground — the shell's
// terrain test.
func solidAt(terrain []int, x, y float64) bool {
	c := int(math.Round(x))
	if c < 0 || c >= scrW {
		return false
	}
	return terrain[c] <= groundBottom && y >= float64(terrain[c])
}

// craterAt scoops a circular bite out of the ground centred at (ex,ey): in each
// column the blast reaches, the surface drops to the bottom of the circle (or
// the whole column blows through to a pit).
func craterAt(terrain []int, ex, ey, radius float64) {
	r := int(math.Ceil(radius))
	cx := int(math.Round(ex))
	for c := cx - r; c <= cx+r; c++ {
		if c < 0 || c >= scrW {
			continue
		}
		dx := float64(c) - ex
		if math.Abs(dx) > radius {
			continue
		}
		bottom := ey + math.Sqrt(radius*radius-dx*dx)
		if bottom < float64(terrain[c]) {
			continue // the blast doesn't reach this column's ground
		}
		newSurf := int(math.Round(bottom)) + 1
		if newSurf > groundBottom+1 {
			newSurf = groundBottom + 1 // blown clean through — a pit
		}
		if newSurf > terrain[c] {
			terrain[c] = newSurf
		}
	}
}
