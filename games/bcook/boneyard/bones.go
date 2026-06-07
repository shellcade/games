package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// The bones of the fallen — the soul of the game. A death leaves a corpse
// record in the world for the week: handle, where, what killed them, their
// gold, and (until the template picker lands in stage 4) the auto
// panic-scrawl. Fresh corpses render `%`; the render cap is 12/floor with
// oldest-first eviction to bone-dust (remembered, not rendered).

type corpse struct {
	handle string
	floor  int
	x, y   int
	killer string
	gold   int
	at     time.Time
	words  string // panic-scrawl for now; the closed-template picker is stage 4

	gaspDir string // the last move direction — "they fled east"
	species string // killer's bestiary slug (the avenge target)
	floor0  int    // unjittered death floor (== floor; kept for clarity)

	respects int  // flowers left at these bones
	looted   bool // gold taken
	devoured bool // marrow taken (blocked once respects >= 3)
	avenged  int  // kills credited against these bones
}

const bonesRenderCap = 12 // per floor (design: render cap with eviction)

// die ends d's run: post the BANKED depth to the board (unbanked progress is
// the Greed Engine's tuition), leave the corpse, and reseat the delver at the
// Gate with a fresh run.
func (rm *room) die(r kit.Room, d *delver, killer string) {
	d.hp = 0
	deathFloor := d.floor

	rm.settleRunScore(r, d)

	// Per-death leaderboard post: BestResult aggregation keeps the weekly
	// max; a resident room never settles, so banking and dying are the
	// posting moments.
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: d.p, Metric: d.banked, Rank: 1, Status: kit.StatusFinished,
	}}})

	// The corpse joins the world (panic-scrawl: closed vocab, no free text),
	// jittered off stairs/shrines and existing corpses so the staircase
	// contract holds and no record shadows another.
	cx, cy := rm.corpseSpot(deathFloor, d.x, d.y)
	gasp := gaspName(d.lastDX, d.lastDY)
	c := &corpse{
		handle: d.p.Handle,
		floor:  deathFloor, x: cx, y: cy,
		killer:  killer,
		species: killer,
		gold:    d.gold,
		at:      r.Now(),
		gaspDir: gasp,
		words:   panicScrawl(killer, gasp),
	}
	// The dying breath: an 8-wake window to pick last words ([1-5]).
	d.dying = c
	d.dyingUntil = r.Now().Add(8 * time.Second)
	rm.bones = append(rm.bones, c)
	rm.evictBones(deathFloor)
	rm.dirtyFloor(deathFloor)

	// A fresh run from the Gate, IN the same delver (allocation-free death:
	// the world keeps the old run's bones, the heap keeps nothing else).
	d.resetRun(rm, r, killer, deathFloor)
	rm.dirtyFloor(1)
}

// corpseSpot spiral-searches the nearest plain-floor tile (never stairs or
// shrine) without a rendered corpse — starting at the death tile itself.
func (rm *room) corpseSpot(floor, x, y int) (int, int) {
	f := rm.world.at(floor)
	for ring := 0; ring <= 4; ring++ {
		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				if maxAbs(dx, dy) != ring {
					continue
				}
				nx, ny := x+dx, y+dy
				if nx < 1 || nx >= floorW-1 || ny < 1 || ny >= floorH-1 {
					continue
				}
				if t := f.tiles[ny][nx]; t != tFloor && t != tWater {
					continue
				}
				if rm.corpseAt(floor, nx, ny) != nil {
					continue
				}
				return nx, ny
			}
		}
	}
	return x, y // pathological crowding: stack rather than lose the record
}

func maxAbs(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}

// evictBones enforces the per-floor render cap, oldest first (evicted bones
// become unrendered dust; the record itself stays for the week's memorial).
func (rm *room) evictBones(floor int) {
	n := 0
	for _, c := range rm.bones {
		if c.floor == floor && !c.dust() {
			n++
		}
	}
	for i := 0; i < len(rm.bones) && n > bonesRenderCap; i++ {
		c := rm.bones[i]
		if c.floor == floor && !c.dust() {
			c.x, c.y = -1, -1 // dust: remembered, not rendered
			n--
		}
	}
}

func (c *corpse) dust() bool { return c.x < 0 }

// corpseAt returns the rendered corpse on (floor,x,y), or nil.
func (rm *room) corpseAt(floor, x, y int) *corpse {
	for i := len(rm.bones) - 1; i >= 0; i-- {
		c := rm.bones[i]
		if c.floor == floor && c.x == x && c.y == y {
			return c
		}
	}
	return nil
}

// inspectBones is the walk-over moment: the dead speak — and the last gasp
// points where they ran. Reading the bones arms the AVENGE vow: kill their
// killer's kind on this floor and the dead are avenged.
func (d *delver) inspectBones(c *corpse) {
	d.say("Here lies " + c.handle + " — " + c.killer + ". \"" + c.words + "\" (fled " + c.gaspDir + ")")
	d.say("[L]oot " + itoa(c.gold) + "g  [R]espect  [D]evour")
	d.vow = c
}

// devourBones takes the marrow: heal = min(8 + floor/2, 20) — but the
// well-mourned are protected (respects >= 3).
func (d *delver) devourBones(rm *room, c *corpse) {
	if c.devoured {
		d.say("The marrow is gone.")
		return
	}
	if c.respects >= 3 {
		d.say("The flowers stay your hand. " + c.handle + " is too well mourned.")
		return
	}
	c.devoured = true
	heal := 8 + c.floor/2
	if heal > 20 {
		heal = 20
	}
	d.hp += heal
	if d.hp > d.maxHP {
		d.hp = d.maxHP
	}
	d.devours++
	d.say("You crack the bones and take the marrow. +" + itoa(heal) + " HP. The dark approves.")
}

// creditAvenge is called when d kills a monster: an armed vow against that
// species on this floor settles it.
func (d *delver) creditAvenge(rm *room, speciesName string) {
	c := d.vow
	if c == nil || c.floor != d.floor || c.species != speciesName {
		return
	}
	c.avenged++
	d.avenges++
	d.vow = nil
	d.say("You avenge " + c.handle + ". The " + speciesName + " answers for its dead.")
}

// lootBones takes the corpse's gold (the graverobber's path).
func (d *delver) lootBones(rm *room, c *corpse) {
	if c.looted || c.gold == 0 {
		d.say("The bones have already been picked clean.")
		return
	}
	d.gold += c.gold
	d.say("You take " + itoa(c.gold) + " gold off " + c.handle + "'s bones.")
	c.gold, c.looted = 0, true
	d.looted++
}

// respectBones leaves a flower: +1 luck (capped, one-floor — full effects
// land in stage 3; the counter and the social signal land NOW).
func (d *delver) respectBones(rm *room, r kit.Room, c *corpse) {
	c.respects++
	d.respects++
	gain := 1
	if d.armor != nil && d.armor.name == "gilded reliquary mail" {
		gain = 2 // the mail honors the dead twice over
	}
	if d.relic != nil && d.relic.power == 2 { // graverobber's ring: no luck from flowers
		gain = 0
	}
	d.luck += gain
	if d.luck > 5 {
		d.luck = 5
	}
	rm.creditRespect(r, d, c)
	d.say("You pay your respects to " + c.handle + ". (+" + itoa(gain) + " luck)")
}
