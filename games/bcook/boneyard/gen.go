package main

// Deterministic floor generation: rooms + corridors carved from a PRNG seeded
// by (weekSeed, depth) ONLY, so every room/process/regeneration of the same
// week produces byte-identical floors — the property the whole shared-bones
// fantasy rests on ("everyone is descending the SAME staircases").

// genRNG is a splitmix64 — tiny, allocation-free, and stable across builds
// (math/rand's stream is version-dependent; gen determinism cannot be).
type genRNG struct{ s uint64 }

func newGenRNG(seed int64, depth int) *genRNG {
	// Mix the depth in via splitmix's own avalanche so adjacent floors share
	// nothing structurally.
	g := &genRNG{s: uint64(seed)}
	for i := 0; i <= depth; i++ {
		g.next()
	}
	g.s ^= uint64(depth) * 0x9E3779B97F4A7C15
	return g
}

func (g *genRNG) next() uint64 {
	g.s += 0x9E3779B97F4A7C15
	z := g.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// intn returns a uniform int in [0,n).
func (g *genRNG) intn(n int) int { return int(g.next() % uint64(n)) }

// rect is a generated room.
type rect struct{ x, y, w, h int }

func (r rect) cx() int { return r.x + r.w/2 }
func (r rect) cy() int { return r.y + r.h/2 }

func (r rect) overlaps(o rect, pad int) bool {
	return r.x-pad < o.x+o.w && o.x-pad < r.x+r.w &&
		r.y-pad < o.y+o.h && o.y-pad < r.y+r.h
}

// genFloor carves B<depth> from the week seed: bordered walls, 10-16 rooms,
// L-corridors between successive rooms, doors where corridors meet rooms,
// water pools for flavor, the up-stairs in the first room, the down-stairs at
// the (approximately) FARTHEST room center — with the shrine, on shrine
// depths, adjacent to the down-stairs (the Greed Engine's cash-out-or-descend
// moment lives at the staircase).
func genFloor(seed int64, depth int) *floor {
	g := newGenRNG(seed, depth)
	f := &floor{depth: depth}

	for y := 0; y < floorH; y++ {
		for x := 0; x < floorW; x++ {
			f.tiles[y][x] = tWall
		}
	}

	// Rooms: place without overlap (padded), give up after enough attempts.
	var rooms []rect
	want := 10 + g.intn(7)
	for try := 0; try < 220 && len(rooms) < want; try++ {
		r := rect{w: 5 + g.intn(10), h: 4 + g.intn(5)}
		r.x = 1 + g.intn(floorW-r.w-2)
		r.y = 1 + g.intn(floorH-r.h-2)
		ok := true
		for _, o := range rooms {
			if r.overlaps(o, 2) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		rooms = append(rooms, r)
		for y := r.y; y < r.y+r.h; y++ {
			for x := r.x; x < r.x+r.w; x++ {
				f.tiles[y][x] = tFloor
			}
		}
	}

	// Corridors: L-shaped between successive room centers (elbow direction by
	// coin flip), with a door where the corridor pierces a room wall.
	carve := func(x, y int) {
		if x <= 0 || x >= floorW-1 || y <= 0 || y >= floorH-1 {
			return
		}
		if f.tiles[y][x] == tWall {
			f.tiles[y][x] = tFloor
		}
	}
	for i := 1; i < len(rooms); i++ {
		ax, ay := rooms[i-1].cx(), rooms[i-1].cy()
		bx, by := rooms[i].cx(), rooms[i].cy()
		if g.intn(2) == 0 {
			for x := min(ax, bx); x <= max(ax, bx); x++ {
				carve(x, ay)
			}
			for y := min(ay, by); y <= max(ay, by); y++ {
				carve(bx, y)
			}
		} else {
			for y := min(ay, by); y <= max(ay, by); y++ {
				carve(ax, y)
			}
			for x := min(ax, bx); x <= max(ax, bx); x++ {
				carve(x, by)
			}
		}
	}

	// Water pools: a few damp patches (flavor; walkable).
	for i := 0; i < 3+g.intn(4); i++ {
		px, py := 2+g.intn(floorW-4), 2+g.intn(floorH-4)
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 3; dx++ {
				if f.tiles[py+dy][px+dx] == tFloor {
					f.tiles[py+dy][px+dx] = tWater
				}
			}
		}
	}

	// Stairs: up in the first room; down at the farthest room center
	// (straight-line metric — the BFS-farthest refinement can come with the
	// deep-floor pass; rooms are corridor-connected in chain order so every
	// room is reachable). On shrine depths the shrine takes a tile ADJACENT
	// to the down-stairs, with a guaranteed-open neighbor (the room interior
	// around a center is floor by construction).
	f.upX, f.upY = rooms[0].cx(), rooms[0].cy()
	f.tiles[f.upY][f.upX] = tUp

	far, fd := 0, -1
	for i, r := range rooms {
		dx, dy := r.cx()-f.upX, r.cy()-f.upY
		if d := dx*dx + dy*dy; d > fd {
			fd, far = d, i
		}
	}
	f.downX, f.downY = rooms[far].cx(), rooms[far].cy()
	f.tiles[f.downY][f.downX] = tDown
	// A sealed crypt on the belt and below: a tCrypt tile bordering the first
	// room, holding a guaranteed prize behind a crypt key.
	if depth >= 4 {
		r0 := rooms[1%len(rooms)]
		f.cryptX, f.cryptY = r0.cx(), r0.y-1
		if f.cryptY < 1 {
			f.cryptY = r0.y + r0.h
		}
		// Only seal a WALL tile — never a carved corridor (that could sever
		// the path to the down-stairs).
		if f.cryptY >= 1 && f.cryptY < floorH-1 && f.tiles[f.cryptY][f.cryptX] == tWall {
			f.tiles[f.cryptY][f.cryptX] = tCrypt
		} else {
			f.cryptX, f.cryptY = 0, 0
		}
	}
	if hasShrine(depth) {
		f.shrineX, f.shrineY = f.downX+1, f.downY
		if f.shrineX >= rooms[far].x+rooms[far].w {
			f.shrineX = f.downX - 1
		}
		f.tiles[f.shrineY][f.shrineX] = tShrine
	}

	return f
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// scaled applies the canonical lethality scaling (design §1): the BASE stat
// times the scalar, rounded HALF-UP — `floor(x + 0.5)` — the determinism-
// critical operator (byte-identical across rooms for the same week).
func scaled(base int, scalar float64) int {
	x := float64(base) * scalar
	return int(x + 0.5)
}

// hpScalar / dmgScalar are the per-floor lethality multipliers.
func hpScalar(depth int) float64  { return 1.0 + 0.06*float64(depth-1) }
func dmgScalar(depth int) float64 { return 1.0 + 0.05*float64(depth-1) }
