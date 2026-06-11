package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Playfield geometry. The canvas is 80x24; row 0 is the scoreboard and row 23
// is the controls/status bar, leaving rows 1..22 for the arena. The arena is a
// flat grid of tiles — one tile per cell — stacked into three independent
// layers. A player only ever sees the layer they are standing on.
const (
	cols   = 80
	top    = 1  // first arena row
	bottom = 22 // last arena row (inclusive)
	arenaH = bottom - top + 1
	arenaW = cols // 80 columns of tiles

	layers = 3 // number of stacked floors; falling through the last eliminates
)

// Tile decay stages. A solid tile, once a player steps off it, ages a beat
// later through these stages and then vanishes; a player on a missing tile
// falls to the same cell on the layer below.
const (
	tileSolid   = 0 // █
	tileCracked = 1 // ▓
	tileWorn    = 2 // ░
	tileGone    = 3 // hole — step here and you drop a layer
)

// Timing. Tiles degrade a beat AFTER you step off them (decayDelay), then walk
// one stage every decayStep. The fall flash lingers briefly so a drop reads.
const (
	decayDelay   = 450 * time.Millisecond // grace before a stepped-off tile starts aging
	decayStep    = 380 * time.Millisecond // time between successive decay stages
	moveEvery    = 90 * time.Millisecond  // min time between accepted moves (per player)
	fallFlash    = 500 * time.Millisecond // how long a fall flash shows on the new layer
	intermission = 3 * time.Second        // pause between rounds
)

// Ambient crumble wave. Solo play is survive-as-long-as-you-can: the arena
// decays random tiles on its own and the rate accelerates over the round. In
// multiplayer the same wave exists but far gentler — footsteps do most of the
// damage. craterTarget()'s solo/pvp split (see room.go) is the analogue of the
// exemplar's approach.
const (
	soloCrumbleBase = 6.0  // tiles/sec crumbled at the very start of a solo round
	soloCrumbleGrow = 1.4  // extra tiles/sec added per second elapsed (acceleration)
	soloCrumbleMax  = 90.0 // ceiling so the very end is frantic but not instant
	pvpCrumbleBase  = 1.5  // gentle ambient decay with 2+ players
	pvpCrumbleGrow  = 0.25
	pvpCrumbleMax   = 18.0
)

// player is one contestant. State is keyed by Player.AccountID so it survives
// room hibernation (connections change across a freeze; accounts don't).
type player struct {
	col, row   int  // position within the arena (absolute canvas coords)
	layer      int  // which floor they stand on (0 = top, layers-1 = bottom)
	alive      bool // still in the round
	lastMove   time.Time
	fellAt     time.Time // when they last dropped a layer (for the fall flash)
	roundStart time.Time // when the current run began (for survival time)
	bestSecs   int       // all-time best survival seconds (seeded from durable KV)
	lastSecs   int       // survival seconds of the most recent run (HUD)

	glyph rune      // the player's character, or '@' when they have none
	color kit.Color // the player's colour: character BG colour, or a palette pick
}

// decayEvent schedules a single tile's next decay tick. The crumble system
// keeps a small set of pending events rather than scanning every tile each
// wake.
type decayEvent struct {
	layer, row, col int
	at              time.Time // when this tile advances one stage
}
