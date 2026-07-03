package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shellcade/kit/v2/kittest"
)

// allReels returns a [numReels]int with every reel stopped at idx.
func allReels(idx int) (out [numReels]int) {
	for i := range out {
		out[i] = idx
	}
	return out
}

// faceRow returns a [numReels]symbol with every reel's center face = s.
func faceRow(s symbol) (out [numReels]symbol) {
	for i := range out {
		out[i] = s
	}
	return out
}

func TestFreeSpinsAwardedDeterministically(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	// One scatter on the strip: stopping all five reels on it puts the scatter in
	// each reel's window centre -> 5 scatters -> trigger. Sparse enough to converge.
	h.variant = mustCompile(t, oddsVariant{
		Name: "scatterland", Weights: map[string]int{"7": 4, "C": 30, "S": 1},
		Paytable: []payEntry{{Faces: "7", Pay3: 100, Pay4: 300, Pay5: 1000}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	openStake(h, r, p) // wager the base bet; the trigger keeps this one stake open
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: allReels(idx), final: faceRow(symScatter)}
	h.settleSpin(r, p.AccountID)
	if m.freeSpins != 8 {
		t.Fatalf("freeSpins = %d, want 8", m.freeSpins)
	}
	if m.freeBet != 10 {
		t.Fatalf("freeBet = %d, want the triggering bet 10", m.freeBet)
	}
	if m.stake != 10 {
		t.Fatalf("stake = %d, want the base stake held open through the feature", m.stake)
	}
}

// A free spin never wagers and never settles mid-feature: it only accumulates
// its win onto the one open stake (balance unchanged until the feature ends).
func TestFreeSpinAccumulatesNoMidCharge(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	r.CreditsMaxPayoutMultiplier = maxPayoutMult
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	// No scatter on this strip: the 7-run free spin pays without a retrigger.
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 30},
		Paytable: []payEntry{{Faces: "7", Pay3: 50, Pay4: 150, Pay5: 500}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 50
	openStake(h, r, p) // the original base stake, held open through the feature
	m.freeSpins, m.freeBet, m.freeVar = 3, 50, h.variant
	before := m.credits
	i7 := firstIdx(t, h.variant.strip, sym7)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: allReels(i7), final: faceRow(sym7)}
	win := 50 * h.variant.waysPayout(scatterWindow(h.variant.strip, allReels(i7))) / wayScale
	h.settleSpin(r, p.AccountID)

	if m.credits != before {
		t.Fatalf("balance changed mid-feature: %d, want unchanged %d (no per-spin settle)", m.credits, before)
	}
	if m.freeWin != win {
		t.Fatalf("freeWin = %d, want the accumulated %d", m.freeWin, win)
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
	r.CreditsMaxPayoutMultiplier = maxPayoutMult
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 30}, // no scatter -> no retrigger
		Paytable: []payEntry{{Faces: "7", Pay3: 50, Pay4: 150, Pay5: 500}},
	})
	h.OnJoin(r, p)
	seatAt0(t, h, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	openStake(h, r, p) // the base stake, held open until the feature settles
	m.freeSpins, m.freeBet, m.freeVar = 3, 10, h.variant

	for i := 0; i < 40; i++ {
		r.Advance(300 * time.Millisecond)
		h.OnWake(r)
	}
	if m.freeSpins != 0 {
		t.Fatalf("freeSpins = %d, want 0 after auto-play", m.freeSpins)
	}
	if m.spin != nil {
		t.Fatal("no spin should be in flight after the feature ends")
	}
	if m.stake != 0 {
		t.Fatalf("stake = %d, want 0 (the feature settled the one open stake exactly once)", m.stake)
	}
}

func TestFreeSpinTriggerAnnouncesRoomWide(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 4, "C": 30, "S": 1},
		Paytable: []payEntry{{Faces: "7", Pay3: 100, Pay4: 300, Pay5: 1000}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	openStake(h, r, p)
	idx := firstIdx(t, h.variant.strip, symScatter)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: allReels(idx), final: faceRow(symScatter)}
	h.settleSpin(r, p.AccountID)
	if !h.tickerActive(r.Now()) {
		t.Fatal("expected an active ticker on a free-spin trigger")
	}
	if got := h.ticker.text; !strings.Contains(got, "FREE SPINS") {
		t.Fatalf("ticker = %q, want a FREE SPINS announcement", got)
	}
}
