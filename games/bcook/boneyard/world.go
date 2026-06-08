package main

// The weekly world: floors generated lazily from the week seed, byte-identical
// for everyone (and across any future regeneration from the same seed). Floor
// indices are 1-based "B<n>" depths; the MVP band is B1..B9.

const (
	floorW = 140 // floor width  (design §1: ~140x40 scrolling floors)
	floorH = 40  // floor height
	maxMVP = 18  // the Drowned Reliquary opens: B10-B18 (design bands 1-4)

	mapRows = 21 // viewport rows 0..20; 21 HUD, 22 status, 23 hints/log
)

// Tile glyphs double as the tile enum: rendering and collision read the same
// byte. Everything not in this set is an entity (player/monster/bones/item)
// drawn OVER the tile.
const (
	tWall   = '#'
	tFloor  = '.'
	tDoor   = '+'
	tDown   = '>'
	tUp     = '<'
	tShrine = '_' // stairwell shrine (banking + shop), B3/B6/B9...
	tWater  = '~' // flavor hazard tiles (the Sunken Ossuary is damp)
	tCrypt  = 'C' // a sealed crypt (rendered ▒): opens with a crypt key, guards good loot
)

type floor struct {
	depth int // 1-based: B<depth>
	tiles [floorH][floorW]byte

	upX, upY     int // arrival stairs (B1's up-stairs is the Gate)
	downX, downY int
	shrineX, shrineY int // 0,0 = no shrine on this floor
	cryptX, cryptY   int // 0,0 = no sealed crypt
}

type world struct {
	seed   int64
	floors map[int]*floor // lazily generated on first entry
}

func newWorld(seed int64) *world {
	return &world{seed: seed, floors: map[int]*floor{}}
}

// at returns floor B<depth>, generating it deterministically on first entry
// (collapse-spec: floors lazy-regen on first descent).
func (w *world) at(depth int) *floor {
	if f, ok := w.floors[depth]; ok {
		return f
	}
	f := genFloor(w.seed, depth)
	w.floors[depth] = f
	return f
}

// open reports whether (x,y) is walkable on f.
func (f *floor) open(x, y int) bool {
	if x < 0 || x >= floorW || y < 0 || y >= floorH {
		return false
	}
	switch f.tiles[y][x] {
	case tWall, tCrypt:
		return false
	}
	return true
}

// hasShrine reports whether this depth carries a stairwell shrine (every
// third floor: B3, B6, B9, ... — the Greed Engine's banking points).
func hasShrine(depth int) bool { return depth%3 == 0 }
