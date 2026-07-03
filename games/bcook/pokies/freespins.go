package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// freespins.go is the scatter feature: 3+ scatters anywhere in the 3x3 window
// award free spins that auto-play at no cost (paying at the triggering bet under
// the variant pinned at trigger), and retrigger when more scatters land.

// scatterCount returns the scatter count in the machine's last settled window.
func (rm *room) scatterCount(m *machine) int {
	if len(m.lastStrip) == 0 {
		return 0
	}
	w := scatterWindow(m.lastStrip, m.lastIdx)
	n := 0
	for reel := 0; reel < 3; reel++ {
		for row := 0; row < 3; row++ {
			if w[reel][row] == symScatter {
				n++
			}
		}
	}
	return n
}

// triggerFreeSpins awards free spins from the just-settled window under variant v
// (the spin's pinned variant), returning the spins awarded (0 if none). When
// fresh (a base-game trigger) it locks the bet and variant and zeroes the win
// accumulator; a retrigger (fresh=false, called from inside the feature) only
// adds spins and MUST keep the accumulator — the caller owns the fresh/retrigger
// distinction so a retrigger on the last spin never wipes the pending settle.
func (rm *room) triggerFreeSpins(m *machine, v *variant, bet int, fresh bool) int {
	award := v.scatterAward(scatterWindow(m.lastStrip, m.lastIdx))
	if award == 0 {
		return 0
	}
	if fresh {
		m.freeBet = bet
		m.freeVar = v
		m.freeWin = 0
	}
	m.freeSpins += award
	return award
}

// scheduleNextFree sets the earliest time the next auto free spin may begin.
func (rm *room) scheduleNextFree(r kit.Room, m *machine) {
	m.nextFree = r.Now().Add(freeSpinGap)
}

// endFreeSpins finalizes a feature: it is the SINGLE Settle for the whole
// feature, closing the one open stake with the accumulated (stake-clamped)
// win — the triggering line win folded in plus every free-spin win. Flashes
// the total and releases the pinned variant.
func (rm *room) endFreeSpins(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	gross := capGross(m.freeWin, m.stake)
	if gross > 0 {
		m.flash = fmt.Sprintf("FEATURE +%d", gross)
		m.flashUntil = r.Now().Add(flashDur)
	}
	m.freeVar = nil
	rm.settle(r, id, gross)
	m.freeWin = 0
}

// autoFreeSpin rolls one free spin (no bet charged) under the pinned free-spin
// variant. The OnWake landing loop settles it via settleSpin's free path.
func (rm *room) autoFreeSpin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.spin != nil || m.freeSpins <= 0 {
		return
	}
	v := m.freeVar
	if v == nil {
		v = rm.variant
	}
	s := &spinState{startedAt: r.Now(), variant: v}
	for i := range s.final {
		s.stopIdx[i] = r.Rand().Intn(len(v.strip))
		s.final[i] = v.strip[s.stopIdx[i]]
	}
	m.spin = s
}
