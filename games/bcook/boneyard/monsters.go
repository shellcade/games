package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// The MVP bestiary (design §2, B1-B9 species). Stats here are BASE values;
// every spawn applies the canonical per-floor lethality scaling. Spawn
// positions and species are seed-deterministic per floor; monster RUNTIME
// state (positions, HP) is world state and lives for the week.

type species struct {
	name   string
	glyph  rune
	style  kit.Style
	minB   int // band floor
	maxB   int
	hp     int
	dmgN   int // damage dice: dmgN d dmgD
	dmgD   int
	atk    int           // attack bonus
	armor  int           // hit DC (design: "Armor" = the full to-hit target)
	period time.Duration // actPeriod: time between actions
	flees  bool          // flees at 1 HP (cave rat)
	burst  bool          // bloat: explodes on death, 2d4 to all 8 neighbors
}

var bestiary = []species{
	{"cave rat", 'r', kit.Style{FG: kit.DimGray}, 1, 3, 3, 1, 2, 0, 10, 400 * time.Millisecond, true, false},
	{"kobold", 'd', kit.Style{FG: kit.Red, Attr: kit.AttrBold}, 1, 5, 6, 1, 4, 3, 12, 400 * time.Millisecond, false, false},
	{"bloat", 'b', kit.Style{FG: kit.Green}, 2, 6, 4, 0, 0, 0, 10, time.Second, false, true},
	{"jackal", 'j', kit.Style{FG: kit.Red}, 2, 7, 5, 1, 4, 4, 13, 280 * time.Millisecond, false, false},
	{"goblin", 'g', kit.Style{FG: kit.Green}, 3, 8, 12, 1, 6, 4, 14, 400 * time.Millisecond, false, false},
	{"gnome sapper", 'n', kit.Style{FG: kit.Cyan}, 3, 8, 9, 1, 6, 2, 13, 400 * time.Millisecond, false, false},
	{"skeleton", 's', kit.Style{FG: kit.White}, 4, 9, 14, 1, 8, 5, 15, 650 * time.Millisecond, false, false},
	{"gelatinous cube", '#', kit.Style{FG: kit.Cyan}, 6, 9, 40, 2, 4, 2, 11, time.Second, false, false},
}

// monster is one live spawn.
type monster struct {
	sp     *species
	floor  int
	x, y   int
	hp     int
	dmgN   int // scaled damage is applied to the ROLLED total (design §1);
	nextAt time.Time
}

// spawnFloor populates a freshly generated floor with its band's species —
// positions and picks from the SAME deterministic stream as the floor itself
// (gen-time), so every room of the week agrees where the kobolds started.
func (rm *room) spawnFloor(f *floor) {
	g := newGenRNG(rm.world.seed, f.depth)
	for i := 0; i < 4096; i++ { // re-sync past the gen draws: independent sub-stream
		_ = i
	}
	g.s ^= 0xB0E5 // monster sub-stream tag

	var band []*species
	for i := range bestiary {
		if f.depth >= bestiary[i].minB && f.depth <= bestiary[i].maxB {
			band = append(band, &bestiary[i])
		}
	}
	if len(band) == 0 {
		return
	}
	n := 6 + g.intn(4) + f.depth/3 // gentle density ramp
	for i := 0; i < n; i++ {
		sp := band[g.intn(len(band))]
		x, y := rm.openTile(g, f)
		rm.monsters = append(rm.monsters, &monster{
			sp: sp, floor: f.depth, x: x, y: y,
			hp: scaled(sp.hp, hpScalar(f.depth)),
		})
	}
}

// openTile finds a random walkable, non-stairs tile.
func (rm *room) openTile(g *genRNG, f *floor) (int, int) {
	for {
		x, y := 1+g.intn(floorW-2), 1+g.intn(floorH-2)
		if f.tiles[y][x] == tFloor {
			return x, y
		}
	}
}

// tickMonsters advances every live monster whose actPeriod elapsed: chase the
// nearest visible delver on the floor (8-tile chebyshev), bump-attack when
// adjacent, wander otherwise. Movement dirties witnesses.
func (rm *room) tickMonsters(r kit.Room, now time.Time) {
	for _, m := range rm.monsters {
		if m.hp <= 0 || now.Before(m.nextAt) {
			continue
		}
		m.nextAt = now.Add(m.sp.period)

		target := rm.nearestDelver(m)
		if target != nil && m.sp.flees && m.hp == 1 {
			rm.moveMonster(m, m.x+sign(m.x-target.x), m.y+sign(m.y-target.y))
			continue
		}
		if target == nil || cheb(m.x-target.x, m.y-target.y) > 8 {
			// Wander: a lazy drunken step keeps floors feeling alive.
			dx, dy := wanderDir(rm.wakes+m.x*7+m.y*13)
			rm.moveMonster(m, m.x+dx, m.y+dy)
			continue
		}
		if cheb(m.x-target.x, m.y-target.y) == 1 {
			rm.monsterAttack(r, m, target)
			continue
		}
		rm.moveMonster(m, m.x+sign(target.x-m.x), m.y+sign(target.y-m.y))
	}
}

// nearestDelver returns the closest LIVING delver on m's floor, or nil.
func (rm *room) nearestDelver(m *monster) *delver {
	var best *delver
	bd := 1 << 30
	for _, d := range rm.delvers {
		if d.floor != m.floor || d.hp <= 0 {
			continue
		}
		if c := cheb(m.x-d.x, m.y-d.y); c < bd {
			bd, best = c, d
		}
	}
	return best
}

func (rm *room) moveMonster(m *monster, nx, ny int) {
	f := rm.world.at(m.floor)
	if !f.open(nx, ny) || rm.monsterAt(m.floor, nx, ny) != nil {
		return
	}
	ox, oy := m.x, m.y
	m.x, m.y = nx, ny
	rm.dirtyWitnesses(m.floor, ox, oy, nil)
	rm.dirtyWitnesses(m.floor, nx, ny, nil)
}

// monsterAt returns the live monster on (floor,x,y), or nil.
func (rm *room) monsterAt(floor, x, y int) *monster {
	for _, m := range rm.monsters {
		if m.hp > 0 && m.floor == floor && m.x == x && m.y == y {
			return m
		}
	}
	return nil
}

func cheb(dx, dy int) int {
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

func sign(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

// wanderDir picks a lazy pseudo-direction (most ticks: stand still).
func wanderDir(h int) (int, int) {
	switch h % 9 {
	case 0:
		return 1, 0
	case 1:
		return -1, 0
	case 2:
		return 0, 1
	case 3:
		return 0, -1
	default:
		return 0, 0
	}
}
