package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shellcade/kit/v2/kittest"
)

func TestFreeSpinsAwardedDeterministically(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // loads the config default; we override h.variant below
	// One scatter on the strip: stopping all three reels on it puts the scatter
	// in each reel's window center -> 3 scatters -> trigger. Sparse enough to pass
	// the convergence gate.
	h.variant = mustCompile(t, oddsVariant{
		Name: "scatterland", Weights: map[string]int{"7": 5, "C": 15, "S": 1},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, startBalance-10
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{idx, idx, idx}, final: [3]symbol{symScatter, symScatter, symScatter}}
	h.settleSpin(r, p.AccountID)
	if m.freeSpins != 8 {
		t.Fatalf("freeSpins = %d, want 8", m.freeSpins)
	}
	if m.freeBet != 10 {
		t.Fatalf("freeBet = %d, want the triggering bet 10", m.freeBet)
	}
}

func TestFreeSpinWinCreditsAtLockedBetNoCharge(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // loads the config default; we override h.variant below
	// No scatter on this strip: the 777 free spin credits and decrements without
	// any chance of a retrigger.
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 5, "C": 15},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	// Enter free spins by hand: 3 spins left, locked bet 50.
	m.freeSpins, m.freeBet, m.freeVar = 3, 50, h.variant
	m.balance = 1000
	i7 := firstIdx(t, h.variant.strip, sym7)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{i7, i7, i7}, final: [3]symbol{sym7, sym7, sym7}} // 20x
	h.settleSpin(r, p.AccountID)

	if m.balance != 1000+50*20 { // credited at the LOCKED bet, no deduction
		t.Fatalf("balance = %d, want %d", m.balance, 1000+50*20)
	}
	if m.freeSpins != 2 {
		t.Fatalf("freeSpins = %d, want 2 (decremented)", m.freeSpins)
	}
	if m.gamble != nil {
		t.Fatal("gamble must not be offered during free spins")
	}
}

func TestFreeSpinsAutoPlayToCompletion(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // loads the config default; we override h.variant below
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 9}, // no scatter -> no retrigger
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.freeSpins, m.freeBet, m.freeVar = 3, 10, h.variant
	m.balance = 1000

	for i := 0; i < 30; i++ {
		r.Advance(300 * time.Millisecond)
		h.OnWake(r)
	}
	if m.freeSpins != 0 {
		t.Fatalf("freeSpins = %d, want 0 after auto-play", m.freeSpins)
	}
	if m.spin != nil {
		t.Fatal("no spin should be in flight after the feature ends")
	}
}

func TestFreeSpinTriggerAnnouncesRoomWide(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // loads the config default; we override h.variant below
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 5, "C": 15, "S": 1},
		Paytable: []payEntry{{Faces: "7", Multiplier: 20}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, 990
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: [3]int{idx, idx, idx}, final: [3]symbol{symScatter, symScatter, symScatter}}
	h.settleSpin(r, p.AccountID)
	if !h.tickerActive(r.Now()) {
		t.Fatal("expected an active ticker on a free-spin trigger")
	}
	if got := h.ticker.text; !strings.Contains(got, "FREE SPINS") {
		t.Fatalf("ticker = %q, want a FREE SPINS announcement", got)
	}
}
