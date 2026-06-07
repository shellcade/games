package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// delver is one player's run: position, vitals, torch, and the per-floor
// explored memory their fog of war renders from. The run persists across
// disconnects (the world is persistent); only rendering stops.
type delver struct {
	p     kit.Player
	floor int
	x, y  int

	hp, maxHP int
	gold      int
	str, dex  int

	// Torch (design §7, hybrid drain): 1t per action plus 1t per 2s of
	// wall-clock on a floor. At 0 the dark closes in.
	torch       int
	lastPassive time.Time

	// moveCD (design §5): clamp(200 - 5*Dex, 90, 200) ms between moves.
	nextMoveAt time.Time

	banked  int // deepest banked depth (the leaderboard metric)
	deepest int // deepest floor reached this run (display)

	// explored is the fog-of-war memory, per visited floor.
	explored map[int]*[floorH][floorW]bool

	msg   [2]string // the two message-log lines
	dirty bool      // re-render this view on the next wake
}

// Starting line (stage 3 brings the BLADE/LANTERN/FLASK kits; until then the
// baseline delver is LANTERN-shaped: balanced stats, standard torch).
func newDelver(p kit.Player, w *world, r kit.Room) *delver {
	f := w.at(1)
	d := &delver{
		p: p, floor: 1, x: f.upX, y: f.upY,
		hp: 30, maxHP: 30,
		str: 14, dex: 14,
		torch:       600,
		lastPassive: r.Now(),
		explored:    map[int]*[floorH][floorW]bool{},
		dirty:       true,
	}
	d.say("The Boneyard. The bones of the week's dead are down here somewhere.")
	d.reveal(f)
	return d
}

func (d *delver) moveCD() time.Duration {
	ms := 200 - 5*d.dex
	if ms < 90 {
		ms = 90
	}
	if ms > 200 {
		ms = 200
	}
	return time.Duration(ms) * time.Millisecond
}

// sightRadius is the torch-lit visibility (the dark collapses it to 2 — the
// design's 5x5 keyhole).
func (d *delver) sightRadius() int {
	if d.torch <= 0 {
		return 2
	}
	return 8
}

// say pushes a message-log line (two-line memory, newest last).
func (d *delver) say(s string) {
	d.msg[0] = d.msg[1]
	d.msg[1] = s
	d.dirty = true
}

// reveal marks the delver's current sight into the floor's explored memory.
func (d *delver) reveal(f *floor) {
	mem, ok := d.explored[d.floor]
	if !ok {
		mem = &[floorH][floorW]bool{}
		d.explored[d.floor] = mem
	}
	r := d.sightRadius()
	for y := d.y - r; y <= d.y+r; y++ {
		for x := d.x - r; x <= d.x+r; x++ {
			if x >= 0 && x < floorW && y >= 0 && y < floorH {
				mem[y][x] = true
			}
		}
	}
}

// camera returns the (clamped, centered) viewport origin.
func (d *delver) camera() (ox, oy int) {
	ox = clamp(d.x-kit.Cols/2, 0, floorW-kit.Cols)
	oy = clamp(d.y-mapRows/2, 0, floorH-mapRows)
	return
}

// burn spends torch (clamped at 0) and dirties the HUD when the gauge moves.
func (d *delver) burn(t int) {
	if d.torch <= 0 {
		return
	}
	d.torch -= t
	if d.torch < 0 {
		d.torch = 0
	}
	d.dirty = true
	if d.torch == 0 {
		d.say("Your torch gutters out. The dark presses in.")
	}
}

// tick is the delver's share of the 100ms world wake: the passive torch
// component (1t per 2s on a floor).
func (d *delver) tick(rm *room, now time.Time) {
	if now.Sub(d.lastPassive) >= 2*time.Second {
		d.lastPassive = now
		d.burn(1)
	}
}

// handleInput routes a key/rune to the run.
func (d *delver) handleInput(rm *room, r kit.Room, in kit.Input) {
	dx, dy := 0, 0
	switch {
	case in.Kind == kit.InputRune:
		switch in.Rune {
		case 'h':
			dx = -1
		case 'l':
			dx = 1
		case 'k':
			dy = -1
		case 'j':
			dy = 1
		case 'y':
			dx, dy = -1, -1
		case 'u':
			dx, dy = 1, -1
		case 'b':
			dx, dy = -1, 1
		case 'n':
			dx, dy = 1, 1
		case '>':
			d.descend(rm, r)
			return
		case '<':
			d.ascend(rm, r)
			return
		default:
			return
		}
	case in.Kind == kit.InputKey:
		switch in.Key {
		case kit.KeyUp:
			dy = -1
		case kit.KeyDown:
			dy = 1
		case kit.KeyLeft:
			dx = -1
		case kit.KeyRight:
			dx = 1
		default:
			return
		}
	default:
		return
	}
	d.step(rm, r, dx, dy)
}

// step is one real-time move: gated by moveCD, blocked by walls, burning 1t.
func (d *delver) step(rm *room, r kit.Room, dx, dy int) {
	now := r.Now()
	if now.Before(d.nextMoveAt) {
		return
	}
	f := rm.world.at(d.floor)
	nx, ny := d.x+dx, d.y+dy
	if !f.open(nx, ny) {
		return
	}
	d.nextMoveAt = now.Add(d.moveCD())
	d.x, d.y = nx, ny
	d.burn(1)
	d.reveal(f)
	d.dirty = true
	rm.dirtyWitnesses(d.floor, nx, ny, d)

	switch f.tiles[ny][nx] {
	case tDown:
		d.say("Stairs down. [>] to descend.")
	case tShrine:
		d.say("A stairwell shrine. The deep can't touch what you bank here.")
	case tWater:
		// flavor only (the Ossuary is sinking)
	}
}

// descend takes the down-stairs underfoot.
func (d *delver) descend(rm *room, r kit.Room) {
	f := rm.world.at(d.floor)
	if f.tiles[d.y][d.x] != tDown {
		d.say("No stairs down here.")
		return
	}
	if d.floor >= maxMVP {
		d.say("The way down is choked with rubble. (The deep opens soon.)")
		return
	}
	rm.dirtyFloor(d.floor) // departure is visible to witnesses
	d.floor++
	if d.floor > d.deepest {
		d.deepest = d.floor
	}
	nf := rm.world.at(d.floor)
	d.x, d.y = nf.upX, nf.upY
	d.lastPassive = r.Now()
	d.reveal(nf)
	d.say("Down. B" + itoa(d.floor) + ".")
	rm.dirtyFloor(d.floor)
}

// ascend climbs the up-stairs underfoot (B1's lead back to the Gate — stage 5).
func (d *delver) ascend(rm *room, r kit.Room) {
	f := rm.world.at(d.floor)
	if f.tiles[d.y][d.x] != tUp {
		d.say("No stairs up here.")
		return
	}
	if d.floor == 1 {
		d.say("Daylight is up there. Not yet.")
		return
	}
	rm.dirtyFloor(d.floor)
	d.floor--
	nf := rm.world.at(d.floor)
	d.x, d.y = nf.downX, nf.downY
	d.lastPassive = r.Now()
	d.reveal(nf)
	d.say("Up. B" + itoa(d.floor) + ".")
	rm.dirtyFloor(d.floor)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// itoa is an allocation-light int formatter for message lines (TinyGo +
// gc=leaking: avoid fmt in steady-state paths).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
