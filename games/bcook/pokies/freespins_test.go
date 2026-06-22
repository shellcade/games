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
		Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 100}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, startBalance-10
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
}

func TestFreeSpinWinCreditsAtLockedBetNoCharge(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	// No scatter on this strip: the 7-run free spin credits without a retrigger.
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 30},
		Paytable: []payEntry{{Faces: "7", Pay3: 5, Pay4: 15, Pay5: 50}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 10
	m.freeSpins, m.freeBet, m.freeVar = 3, 50, h.variant
	m.balance = 1000
	i7 := firstIdx(t, h.variant.strip, sym7)
	m.spin = &spinState{startedAt: r.Now(), variant: h.variant,
		stopIdx: allReels(i7), final: faceRow(sym7)}
	want := 1000 + 50*h.variant.waysPayout(scatterWindow(h.variant.strip, allReels(i7)))
	h.settleSpin(r, p.AccountID)

	if m.balance != want {
		t.Fatalf("balance = %d, want %d (credited at the locked bet, no deduction)", m.balance, want)
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
	h.OnStart(r)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 3, "C": 30}, // no scatter -> no retrigger
		Paytable: []payEntry{{Faces: "7", Pay3: 5, Pay4: 15, Pay5: 50}},
	})
	h.OnJoin(r, p)
	seatAt0(t, h, p)
	m := h.machines[p.AccountID]
	m.freeSpins, m.freeBet, m.freeVar = 3, 10, h.variant
	m.balance = 1000

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
}

func TestFreeSpinTriggerAnnouncesRoomWide(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	h.variant = mustCompile(t, oddsVariant{
		Name: "fs", Weights: map[string]int{"7": 4, "C": 30, "S": 1},
		Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 100}},
		Scatter:  []scatterEntry{{Count: 3, Spins: 8}},
	})
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet, m.balance = 10, 990
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
