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
	centiburn   int // fractional burn accumulator (kit/relic multipliers)
	lastPassive time.Time

	// moveCD (design §5): clamp(200 - 5*Dex, 90, 200) ms between moves.
	nextMoveAt time.Time

	banked        int // deepest banked depth (the leaderboard metric)
	deepest       int // deepest floor reached this run (display)
	turns         int // actions this run (the rush-bonus clock)
	firstBankTurn int

	kills    int
	luck     int // RESPECT luck: +1/corpse, cap +5, one-floor (resets on stairs)
	respects int
	looted   int

	// explored is the fog-of-war memory, per visited floor.
	explored map[int]*[floorH][floorW]bool

	weapon, armor, relic       *itemDef
	cursedW, cursedA, cursedR  bool // looted-cursed slots, equip-locked till cleansed
	heals                      int
	recalls, necros, tokens    int  // scrolls of recall/necromancer; avenge bone-tokens
	keys                       int  // crypt keys
	lastBankX, lastBankY, lastBankFloor int // recall anchor (last banked shrine)
	torchMul             int // percent of baseline burn (LANTERN 60)
	kit                  *kitDef

	lastDX, lastDY int // last move direction (the corpse's last gasp)

	vow        *corpse // armed avenge target (read bones to arm)
	devours    int
	avenges    int
	dying      *corpse   // last-words modal target (this run's fresh corpse)
	dyingUntil time.Time // modal window

	heldUntil   time.Time // gelatinous engulf: movement locked
	rotUntilFloor int     // plague rot active while deepest <= this
	knownHeal   bool      // identification: draughts are murky until first quaff
	viewingWall bool // the memorial overlay ([m])

	online bool // connected (offline delvers persist but are not targets)
	rng    uint64 // per-actor combat PRNG (week-seed derived; never wall-clock)
	runs   int    // lifetime runs this week (re-seeds the PRNG per run)

	msg   [2]string // the two message-log lines
	dirty bool      // re-render this view on the next wake
}

// Starting line (stage 3 brings the BLADE/LANTERN/FLASK kits; until then the
// baseline delver is LANTERN-shaped: balanced stats, standard torch).
func newDelver(p kit.Player, w *world, r kit.Room) *delver {
	f := w.at(1) // B1 always exists by the time anyone joins (OnJoin gens it)
	d := &delver{
		p: p, floor: 1, x: f.upX, y: f.upY,
		hp: 30, maxHP: 30,
		str: 14, dex: 14,
		torch:       600,
		lastPassive: r.Now(),
		explored:    map[int]*[floorH][floorW]bool{},
		dirty:       true,
	}
	d.online = true
	d.rng = actorSeed(w.seed, fnvHash(p.AccountID), 0)
	d.applyKit(&kits[1]) // LANTERN until chosen; [1][2][3] at the Gate swap it
	d.say("The Boneyard. [1]BLADE [2]LANTERN [3]FLASK — choose your kit.")
	d.reveal(f)
	return d
}

// resetRun starts a fresh run IN PLACE: under -gc=leaking every allocation is
// permanent, so the dead delver's fog arrays are zeroed and reused, never
// replaced (the death-alloc budget gate enforces this).
func (d *delver) resetRun(rm *room, r kit.Room, killer string, restFloor int) {
	for _, mem := range d.explored {
		*mem = [floorH][floorW]bool{}
	}
	f := rm.world.at(1)
	d.floor, d.x, d.y = 1, f.upX, f.upY
	d.gold, d.kills, d.luck, d.respects, d.looted = 0, 0, 0, 0, 0
	d.devours, d.avenges, d.vow = 0, 0, nil
	d.lastDX, d.lastDY = 0, 0
	d.recalls, d.necros, d.tokens, d.keys = 0, 0, 0, 0
	d.rotUntilFloor = 0
	d.cursedW, d.cursedA, d.cursedR = false, false, false
	d.lastBankFloor = 0
	d.turns, d.firstBankTurn = 0, 0
	d.banked, d.deepest = 0, 1
	d.applyKit(d.kit) // same kit, fresh loadout (swap with [1-3] at the Gate)
	d.lastPassive = r.Now()
	d.runs++
	d.rng = actorSeed(rm.world.seed, fnvHash(d.p.AccountID), uint64(d.runs))
	// The death frame: both log lines carry the moment (the full YOU DIED
	// card is the stage-4/6 death-flow pass).
	d.msg[0] = "You die. " + killer + " got you on B" + itoa(restFloor) + "."
	d.msg[1] = "You wake at the Gate. Your bones rest on B" + itoa(restFloor) + "."
	d.reveal(f)
	d.dirty = true
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

// msgWidth is the message log's column budget: row 23 shares with the
// compact hint bar (layout invariant: hint <= 17 runes, right-aligned).
const msgWidth = 62

// clampCols clamps s to n display columns, rune-aware, with an ellipsis.
func clampCols(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// say pushes a message-log line (two-line memory, newest last), clamped to
// the log's column budget — every player-visible string passes through here.
func (d *delver) say(s string) {
	d.msg[0] = d.msg[1]
	d.msg[1] = clampCols(s, msgWidth)
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
// The kit multiplier applies to BOTH active and passive components (design
// §7) via a centitorch accumulator, so LANTERN's 0.6x is exact.
func (d *delver) burn(t int) {
	if d.torch <= 0 {
		return
	}
	mul := d.torchMul
	if mul == 0 {
		mul = 100
	}
	if d.relic != nil && d.relic.power == 1 { // amulet of the deep: -30%
		mul = mul * 70 / 100
	}
	d.centiburn += t * mul
	t = d.centiburn / 100
	d.centiburn %= 100
	if t == 0 {
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
	// The dying breath: [1-5] rewrites the fresh corpse's last words from
	// the closed templates; anything else (or the window lapsing) keeps the
	// panic-scrawl and gets on with the next run.
	if d.dying != nil {
		if r.Now().Before(d.dyingUntil) && in.Kind == kit.InputRune && in.Rune >= '1' && in.Rune <= '5' {
			t := lwTemplates[in.Rune-'1']
			c := d.dying
			c.words = t.fill(c.species, c.floor, c.gaspDir)
			d.say("Your last words are carved: \"" + c.words + "\"")
			d.dying = nil
			rm.dirtyFloor(c.floor)
			return
		}
		d.dying = nil
	}
	dx, dy := 0, 0
	switch {
	case in.Kind == kit.InputRune:
		switch in.Rune {
		case 'a':
			dx = -1
		case 'd':
			dx = 1
		case 'w':
			dy = -1
		case 's':
			dy = 1
		case '>':
			d.descend(rm, r)
			return
		case '<':
			d.ascend(rm, r)
			return
		case 'l':
			if m := rm.mimicAt(d.floor, d.x, d.y); m != nil {
				// The bones bite back.
				m.hidden = false
				d.say("The corpse SPRINGS — a tomb mimic!")
				rm.dirtyWitnesses(d.floor, m.x, m.y, nil)
				d.dirty = true
				rm.monsterAttack(r, m, d)
				return
			}
			if c := rm.corpseAt(d.floor, d.x, d.y); c != nil {
				d.lootBones(rm, c)
			}
			return
		case 'f':
			if c := rm.corpseAt(d.floor, d.x, d.y); c != nil {
				d.respectBones(rm, r, c)
			}
			return
		case 'q':
			d.quaff()
			return
		case 'r':
			d.readRecall(rm, r)
			return
		case 'g':
			d.readNecro(rm, r)
			return
		case 'c':
			d.cleanse(rm)
			return
		case 'm':
			d.viewingWall = !d.viewingWall
			d.dirty = true
			return
		case 'e':
			if c := rm.corpseAt(d.floor, d.x, d.y); c != nil {
				d.devourBones(rm, c)
			}
			return
		case 'b':
			d.bank(rm, r)
			return
		case '7', '8', '9':
			d.shop(rm, r, in.Rune)
			return
		case '1', '2', '3':
			f := rm.world.at(d.floor)
			if d.floor == 1 && f.tiles[d.y][d.x] == tUp {
				d.applyKit(&kits[in.Rune-'1'])
			}
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
	if now.Before(d.nextMoveAt) || now.Before(d.heldUntil) {
		return // cooldown, or held fast in the cube
	}
	f := rm.world.at(d.floor)
	nx, ny := d.x+dx, d.y+dy
	if nx == f.cryptX && ny == f.cryptY && f.tiles[ny][nx] == tCrypt {
		d.openCrypt(rm, f)
		return
	}
	if !f.open(nx, ny) {
		return
	}
	d.nextMoveAt = now.Add(d.moveCD())
	d.lastDX, d.lastDY = dx, dy
	d.turns++
	if m := rm.monsterAt(d.floor, nx, ny); m != nil {
		// Bump attack: the move becomes a swing.
		d.burn(1)
		d.attackMonster(rm, r, m)
		d.dirty = true
		rm.dirtyWitnesses(d.floor, nx, ny, d)
		return
	}
	d.x, d.y = nx, ny
	d.burn(1)
	d.reveal(f)
	d.dirty = true
	rm.dirtyWitnesses(d.floor, nx, ny, d)

	if dr := rm.dropAt(d.floor, nx, ny); dr != nil {
		d.pickup(rm, dr)
		return
	}
	if c := rm.corpseAt(d.floor, nx, ny); c != nil {
		d.inspectBones(c)
		return
	}
	switch f.tiles[ny][nx] {
	case tDown:
		d.say("Stairs down. [>] to descend.")
	case tShrine:
		d.say("Shrine: [b]ank  [7]torch " + itoa(shopPrice(80, d.floor)) + "g [8]draught " + itoa(shopPrice(120, d.floor)) + "g [9]oil " + itoa(shopPrice(60, d.floor)) + "g")
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
	nf := rm.floorAt(d.floor)
	d.x, d.y = nf.upX, nf.upY
	d.lastPassive = r.Now()
	d.luck = 0 // RESPECT luck is one-floor
	d.reveal(nf)
	d.say("Down. B" + itoa(d.floor) + ".")
	// Skeleton-rise (design): 1-in-8 on floor entry, a corpse node RISES.
	if d.floor >= 4 && roll(&d.rng, 8) == 8 {
		for _, c := range rm.bones {
			if c.floor == d.floor && !c.dust() {
				rm.monsters = append(rm.monsters, &monster{
					sp: speciesByName("skeleton"), floor: d.floor, x: c.x, y: c.y,
					hp:  scaled(14, hpScalar(d.floor)),
					rng: actorSeed(rm.world.seed, uint64(d.floor), uint64(len(rm.monsters))),
				})
				c.x, c.y = -1, -1 // the bones walk now
				d.say("The bones of " + c.name() + " RISE.")
				break
			}
		}
	}
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
	nf := rm.floorAt(d.floor)
	d.x, d.y = nf.downX, nf.downY
	d.lastPassive = r.Now()
	d.luck = 0
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


// openCrypt spends a key to break a sealed crypt, spilling a guaranteed prize.
func (d *delver) openCrypt(rm *room, f *floor) {
	if d.keys == 0 {
		d.say("A sealed crypt. You need a crypt key.")
		return
	}
	d.keys--
	f.tiles[f.cryptY][f.cryptX] = tFloor
	// A guaranteed good drop spills onto the crypt tile.
	def := pick(newGenRNG(rm.world.seed, f.depth+0x600), iWeapon, iArmor, f.depth)
	if def == nil {
		def = &catalog[2] // bone cleaver fallback
	}
	rm.drops = append(rm.drops, &drop{def: def, floor: f.depth, x: f.cryptX, y: f.cryptY})
	rm.drops = append(rm.drops, &drop{floor: f.depth, x: f.cryptX, y: f.cryptY - 1, gold: 200 + f.depth*40})
	d.say("The crypt grinds open. Treasure, and the smell of the long dead.")
	rm.dirtyFloor(f.depth)
}
