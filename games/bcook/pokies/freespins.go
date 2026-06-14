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
// (the spin's pinned variant), returning the spins awarded (0 if none). On a
// fresh feature it locks the bet and variant; a trigger during free spins
// retriggers, adding to the running count.
func (rm *room) triggerFreeSpins(m *machine, v *variant, bet int) int {
	award := v.scatterAward(scatterWindow(m.lastStrip, m.lastIdx))
	if award == 0 {
		return 0
	}
	if m.freeSpins == 0 { // fresh feature
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

// endFreeSpins finalizes a feature: flash the accumulated total and release the
// pinned variant.
func (rm *room) endFreeSpins(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	if m.freeWin > 0 {
		m.flash = fmt.Sprintf("FEATURE +%d", m.freeWin)
		m.flashUntil = r.Now().Add(flashDur)
	}
	m.freeVar = nil
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
