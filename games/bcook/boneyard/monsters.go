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

// stealthy reports the crypt stalker's invisible-until-near behavior.
func (sp *species) stealthy() bool { return sp.name == "crypt stalker" }

var bestiary = []species{
	{"cave rat", 'r', kit.Style{FG: kit.DimGray}, 1, 3, 3, 1, 2, 0, 10, 400 * time.Millisecond, true, false},
	{"kobold", 'd', kit.Style{FG: kit.Red, Attr: kit.AttrBold}, 1, 5, 6, 1, 4, 3, 12, 400 * time.Millisecond, false, false},
	{"bloat", 'b', kit.Style{FG: kit.Green}, 2, 6, 4, 0, 0, 0, 10, time.Second, false, true},
	{"jackal", 'j', kit.Style{FG: kit.Red}, 2, 7, 5, 1, 4, 4, 13, 280 * time.Millisecond, false, false},
	{"goblin", 'g', kit.Style{FG: kit.Green}, 3, 8, 12, 1, 6, 4, 14, 400 * time.Millisecond, false, false},
	{"gnome sapper", 'n', kit.Style{FG: kit.Cyan}, 3, 8, 9, 1, 6, 2, 13, 400 * time.Millisecond, false, false},
	{"skeleton", 's', kit.Style{FG: kit.White}, 4, 9, 14, 1, 8, 5, 15, 650 * time.Millisecond, false, false},
	// The cube keeps the design's '#' glyph: cyan-green ON PURPOSE — color is
	// the wall/cube distinction (walls are DimGray), exactly the corpse/mimic
	// kind of ambiguity the Boneyard trades in.
	{"gelatinous cube", '#', kit.Style{FG: kit.RGB(0x40, 0xd0, 0xa0), Attr: kit.AttrBold}, 6, 9, 40, 2, 4, 2, 11, time.Second, false, false},
	{"cursed wraith", 'W', kit.Style{FG: kit.RGB(0xc0, 0x60, 0xc0), Attr: kit.AttrBold}, 5, 11, 16, 1, 6, 0, 16, 400 * time.Millisecond, false, false},
	{"crypt stalker", 'S', kit.Style{FG: kit.DimGray, Attr: kit.AttrBold}, 7, 13, 18, 2, 4, 6, 17, 280 * time.Millisecond, false, false},
	{"plague ghoul", 'z', kit.Style{FG: kit.Green, Attr: kit.AttrBold}, 8, 14, 22, 1, 8, 0, 16, 400 * time.Millisecond, false, false},
	{"bone golem", 'G', kit.Style{FG: kit.White, Attr: kit.AttrBold}, 10, 18, 55, 2, 8, 7, 18, 650 * time.Millisecond, false, false},
	{"marrow fiend", 'M', kit.Style{FG: kit.Red, Attr: kit.AttrBold}, 11, 17, 28, 2, 6, 0, 17, 400 * time.Millisecond, false, false},
	{"lich acolyte", 'L', kit.Style{FG: kit.RGB(0xc0, 0x60, 0xc0), Attr: kit.AttrBold}, 13, 19, 30, 1, 8, 0, 18, 400 * time.Millisecond, false, false},
	{"ossuary tyrant", 'T', kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}, 16, 25, 80, 3, 6, 9, 20, 650 * time.Millisecond, false, false},
	{"hollow king", 'K', kit.Style{FG: kit.White, Attr: kit.AttrBold}, 20, 25, 60, 2, 10, 0, 19, 400 * time.Millisecond, false, false},
	// The tomb mimic renders EXACTLY like a fresh corpse (the one sanctioned
	// glyph+color overlap) and springs when looted.
	{"tomb mimic", '%', kit.Style{FG: kit.Gray(0xb8)}, 4, 9, 8, 1, 8, 0, 14, 400 * time.Millisecond, false, false},
}

// monster is one live spawn.
type monster struct {
	sp     *species
	floor  int
	x, y   int
	hp     int
	rng    uint64 // per-actor combat PRNG: (week_seed, floor, spawn_index)
	nextAt time.Time
	fuse   int  // bloat: consecutive acts adjacent to a delver (bursts at 2)
	hidden bool // tomb mimic: disguised until sprung
	ally   bool // raised by the necromancer scroll
	allyUntil time.Time
}

// spawnFloor populates a freshly generated floor with its band's species —
// positions and picks from the SAME deterministic stream as the floor itself
// (gen-time), so every room of the week agrees where the kobolds started.
func (rm *room) spawnFloor(f *floor) {
	// An INDEPENDENT sub-stream from the floor's gen stream: a fresh PRNG at
	// the same (seed, depth) with a domain tag XORed in. (Do NOT "advance"
	// this stream to separate it — any change to the draw count would
	// silently reshuffle every spawn of every week.)
	g := newGenRNG(rm.world.seed, f.depth)
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
			hp:     scaled(sp.hp, hpScalar(f.depth)),
			rng:    actorSeed(rm.world.seed, uint64(f.depth), uint64(i)),
			hidden: sp.glyph == '%',
		})
	}
}

// openTile finds a random walkable floor tile. Rejection-sampling the interior
// is near-instant on a normal floor (rooms leave it ~30%+ open), but a
// pathologically sparse floor — or one whose open tiles were nearly all
// flooded into water pools — would starve the sampler and spin FOREVER,
// burning a full core into the 100ms per-callback deadline (and tripping the
// platform's fault watchdog, which quarantines the game). So bound the sampler
// and fall back to a deterministic wrap-around scan that always terminates.
func (rm *room) openTile(g *genRNG, f *floor) (int, int) {
	// A generous cap: a normal floor wins on the first few draws, so the cap is
	// never reached and byte-for-byte determinism with the old sampler holds.
	// Only a starved floor (which used to hang) reaches the scan below.
	const maxSample = 256
	for i := 0; i < maxSample; i++ {
		x, y := 1+g.intn(floorW-2), 1+g.intn(floorH-2)
		if f.tiles[y][x] == tFloor {
			return x, y
		}
	}
	// Sampler starved: scan the interior from a seed-derived offset for the
	// first floor tile. Deterministic (one g draw), O(area), guaranteed to find
	// any floor tile that exists.
	interior := (floorW - 2) * (floorH - 2)
	start := g.intn(interior)
	for k := 0; k < interior; k++ {
		idx := (start + k) % interior
		x, y := 1+idx%(floorW-2), 1+idx/(floorW-2)
		if f.tiles[y][x] == tFloor {
			return x, y
		}
	}
	// Degenerate floor with no floor tile at all: land on the up-stairs, which
	// genFloor always carves. Never spin.
	return f.upX, f.upY
}

// tickMonsters advances every live monster whose actPeriod elapsed: chase the
// nearest visible delver on the floor (8-tile chebyshev), bump-attack when
// adjacent, wander otherwise. Movement dirties witnesses.
func (rm *room) tickMonsters(r kit.Room, now time.Time) {
	// Floor throttle: AI runs only on floors with an ONLINE delver — an empty
	// (or abandoned) floor's monsters sleep, so a quiet resident world burns
	// nothing (the kit GUIDE's idle-throttle rule).
	var active [maxMVP + 1]bool
	any := false
	for _, d := range rm.delvers {
		if d.online && d.floor >= 1 && d.floor <= maxMVP {
			active[d.floor] = true
			any = true
		}
	}
	if !any {
		return
	}
	for _, m := range rm.monsters {
		if m.hp <= 0 || m.hidden || !active[m.floor] {
			continue
		}
		if m.ally && now.After(m.allyUntil) {
			m.hp = 0 // the raised dead crumble when their minute is up
			rm.dirtyWitnesses(m.floor, m.x, m.y, nil)
			continue
		}
		// Catch-up loop (spec): act on the actor's OWN cadence, stepping
		// nextAt by actPeriod — capped at 4 per wake, snapping after a long
		// gap (hibernation resume, floor reactivation).
		if m.nextAt.IsZero() || now.Sub(m.nextAt) > 2*time.Second {
			m.nextAt = now
		}
		for steps := 0; steps < 4 && !now.Before(m.nextAt); steps++ {
			m.nextAt = m.nextAt.Add(m.sp.period)
			rm.actMonster(r, m)
		}
		if !now.Before(m.nextAt) {
			m.nextAt = now.Add(m.sp.period) // still behind after the cap: snap
		}
	}
}

// actMonster is one action on the monster's own clock.
func (rm *room) actMonster(r kit.Room, m *monster) {
	if m.ally {
		rm.actAlly(r, m)
		return
	}
	target := rm.nearestDelver(m)

	if m.sp.burst {
		// The bloat: two consecutive acts adjacent to a delver and it blows.
		if target != nil && cheb(m.x-target.x, m.y-target.y) == 1 {
			m.fuse++
			if m.fuse >= 2 {
				m.hp = 0
				rm.dirtyWitnesses(m.floor, m.x, m.y, nil)
				rm.burst(r, m)
				return
			}
		} else {
			m.fuse = 0
		}
	}

	// Signature behaviors of the deep.
	switch m.sp.name {
	case "bone golem":
		if m.hp > 0 && m.hp <= 10 {
			if c := rm.nearestCorpse(m); c != nil {
				m.hp += 15
				c.x, c.y = -1, -1
				rm.dirtyWitnesses(m.floor, m.x, m.y, nil)
			}
		}
	case "marrow fiend":
		if c := rm.nearestCorpse(m); c != nil && cheb(c.x-m.x, c.y-m.y) <= 1 {
			m.hp += 4
			c.x, c.y = -1, -1 // it eats the dead and grows
			rm.dirtyWitnesses(m.floor, m.x, m.y, nil)
		}
	case "lich acolyte":
		if roll(&m.rng, 40) == 1 {
			if c := rm.nearestCorpse(m); c != nil {
				rm.monsters = append(rm.monsters, &monster{sp: speciesByName("skeleton"),
					floor: m.floor, x: c.x, y: c.y, hp: scaled(14, hpScalar(m.floor)),
					rng: actorSeed(rm.world.seed, uint64(m.floor), uint64(len(rm.monsters)))})
				c.x, c.y = -1, -1
				rm.dirtyWitnesses(m.floor, c.x, c.y, nil)
			}
		}
	case "ossuary tyrant":
		if target != nil && roll(&m.rng, 30) == 1 {
			rm.monsters = append(rm.monsters, &monster{sp: speciesByName("jackal"),
				floor: m.floor, x: m.x, y: m.y, hp: scaled(5, hpScalar(m.floor)),
				rng: actorSeed(rm.world.seed, uint64(m.floor), uint64(len(rm.monsters)))})
		}
	}

	if target != nil && m.sp.flees && m.hp == 1 {
		rm.moveMonster(m, m.x+sign(m.x-target.x), m.y+sign(m.y-target.y))
		return
	}
	if target == nil || cheb(m.x-target.x, m.y-target.y) > 8 {
		dx, dy := wanderDir(rm.wakes + m.x*7 + m.y*13)
		rm.moveMonster(m, m.x+dx, m.y+dy)
		return
	}
	if cheb(m.x-target.x, m.y-target.y) == 1 {
		rm.monsterAttack(r, m, target)
		return
	}
	rm.moveMonster(m, m.x+sign(target.x-m.x), m.y+sign(target.y-m.y))
}

// mimicAt returns the unsprung tomb mimic on a tile, if any.
func (rm *room) mimicAt(floor, x, y int) *monster {
	for _, m := range rm.monsters {
		if m.hp > 0 && m.hidden && m.floor == floor && m.x == x && m.y == y {
			return m
		}
	}
	return nil
}

// nearestDelver returns the closest LIVING delver on m's floor, or nil.
func (rm *room) nearestDelver(m *monster) *delver {
	var best *delver
	bd := 1 << 30
	for _, d := range rm.delvers {
		if d.floor != m.floor || d.hp <= 0 || !d.online {
			continue // an offline run persists but is never a target
		}
		_ = d
		if c := cheb(m.x-d.x, m.y-d.y); c < bd {
			bd, best = c, d
		}
	}
	return best
}

func (rm *room) moveMonster(m *monster, nx, ny int) {
	f := rm.world.at(m.floor)
	if !f.open(nx, ny) || rm.monsterAt(m.floor, nx, ny) != nil || rm.mimicAt(m.floor, nx, ny) != nil {
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
		if m.hp > 0 && !m.hidden && m.floor == floor && m.x == x && m.y == y {
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

// speciesByName finds a bestiary entry (skeleton-rise, tests).
func speciesByName(n string) *species {
	for i := range bestiary {
		if bestiary[i].name == n {
			return &bestiary[i]
		}
	}
	return &bestiary[0]
}

// actAlly: a raised skeleton hunts the nearest enemy monster on its floor.
func (rm *room) actAlly(r kit.Room, m *monster) {
	var tgt *monster
	bd := 1 << 30
	for _, o := range rm.monsters {
		if o == m || o.ally || o.hp <= 0 || o.hidden || o.floor != m.floor {
			continue
		}
		if dd := cheb(o.x-m.x, o.y-m.y); dd < bd {
			bd, tgt = dd, o
		}
	}
	if tgt == nil {
		return
	}
	if cheb(tgt.x-m.x, tgt.y-m.y) == 1 {
		tgt.hp -= roll(&m.rng, 6) + 2 // the dead hit back
		if tgt.hp <= 0 {
			rm.dirtyWitnesses(tgt.floor, tgt.x, tgt.y, nil)
		}
		return
	}
	rm.moveMonster(m, m.x+sign(tgt.x-m.x), m.y+sign(tgt.y-m.y))
}

// nearestCorpse finds the closest rendered corpse on m's floor.
func (rm *room) nearestCorpse(m *monster) *corpse {
	var best *corpse
	bd := 1 << 30
	for _, c := range rm.bones {
		if c.floor == m.floor && !c.dust() {
			if dd := cheb(c.x-m.x, c.y-m.y); dd < bd {
				bd, best = dd, c
			}
		}
	}
	return best
}
