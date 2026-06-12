package main

import (
	"math/rand"
	"strconv"
	"time"
)

// order is one crewmate's current demand. It may target a control anywhere on
// the ship — the owner of the ORDER and the owner of the CONTROL are usually
// different people, and that gap is the game.
type order struct {
	seq      int    // serial, links hails to their order
	targetID string // account id of the crew whose panel holds the control
	ctrlIdx  int
	want     int    // demanded state; -1 = a button press
	text     string // full order text, built once at issue time
	issuedAt time.Time
	expires  time.Time
	paused   time.Duration // remaining time banked while a meteor storm runs
	active   bool
}

// otherPanelBias is how often (with 2+ crew) an order is routed off the
// owner's own panel — the Spaceteam number: most orders are for someone else.
const otherPanelBias = 0.72

// issueOrder deals a fresh order to crew c: pick an untargeted control
// (biased to other panels), a demanded state that is NOT already true, and
// build the text once. Returns false when the ship has no free control.
func (rm *room) issueOrder(rng *rand.Rand, c *crew, now time.Time) bool {
	type cand struct {
		id string
		ci int
	}
	var own, others []cand
	for _, cw := range rm.crews {
		if !cw.boarded {
			continue
		}
		for ci := range cw.panel {
			if rm.targeted(cw.id, ci) {
				continue
			}
			if cw.id == c.id {
				own = append(own, cand{cw.id, ci})
			} else {
				others = append(others, cand{cw.id, ci})
			}
		}
	}
	pool := append(others, own...)
	if len(pool) == 0 {
		c.ord.active = false
		return false
	}
	var pick cand
	if len(others) > 0 && len(own) > 0 {
		if rng.Float64() < otherPanelBias {
			pick = others[rng.Intn(len(others))]
		} else {
			pick = own[rng.Intn(len(own))]
		}
	} else {
		pick = pool[rng.Intn(len(pool))]
	}

	tc := rm.controlAt(pick.id, pick.ci)
	want := -1
	if lo, hi := tc.statesOf(); hi >= lo {
		// pick uniformly among the states that are not already true
		want = lo + rng.Intn(hi-lo)
		if want >= tc.state {
			want++
		}
	}

	rm.orderSeq++
	c.ord = order{
		seq:      rm.orderSeq,
		targetID: pick.id,
		ctrlIdx:  pick.ci,
		want:     want,
		text:     orderText(rng, tc, want, rm.sector),
		issuedAt: now,
		expires:  now.Add(rm.orderDur()),
		active:   true,
	}
	return true
}

// orderDur is the per-order countdown: difficulty sets the base, each sector
// shaves half a second, floored at 5s.
func (rm *room) orderDur() time.Duration {
	base := [...]time.Duration{13 * time.Second, 11 * time.Second, 9 * time.Second}[rm.difficulty]
	d := base - time.Duration(rm.sector-1)*500*time.Millisecond
	if d < 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// setVerbs gains synonyms at sector 4+ — same meaning, more careful reading.
var setVerbs = []string{"SET", "CRANK", "EASE"}

func orderText(rng *rand.Rand, c *control, want, sector int) string {
	name := c.adj + " " + c.jot
	switch c.kind {
	case ckSwitch:
		if want == 1 {
			return "ENGAGE THE " + name
		}
		return "DISENGAGE THE " + name
	case ckButton:
		return "PLUCK THE " + name
	}
	verb := "SET"
	if sector >= 4 {
		verb = setVerbs[rng.Intn(len(setVerbs))]
	}
	return verb + " THE " + name + " TO " + strconv.Itoa(want)
}

// targeted reports whether any active order already demands this control —
// at most one pending order per control, so completions are unambiguous.
func (rm *room) targeted(id string, ci int) bool {
	for _, cw := range rm.crews {
		if cw.ord.active && cw.ord.targetID == id && cw.ord.ctrlIdx == ci {
			return true
		}
	}
	return false
}

func (rm *room) controlAt(id string, ci int) *control {
	for _, cw := range rm.crews {
		if cw.id == id {
			return &cw.panel[ci]
		}
	}
	return nil
}
