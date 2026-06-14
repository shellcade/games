package main

import (
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

func TestGambleColorIsFair(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("alice"))
	red, n := 0, 20000
	for i := 0; i < n; i++ {
		if suitIsRed(dealCardSuit(r.Rand())) {
			red++
		}
	}
	if frac := float64(red) / float64(n); frac < 0.47 || frac > 0.53 {
		t.Fatalf("red fraction = %.3f, want ≈0.5", frac)
	}
}

func TestGambleSuitIsQuarter(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("alice"))
	hits, n := 0, 20000
	for i := 0; i < n; i++ {
		if dealCardSuit(r.Rand()) == suitHearts {
			hits++
		}
	}
	if frac := float64(hits) / float64(n); frac < 0.22 || frac > 0.28 {
		t.Fatalf("hearts fraction = %.3f, want ≈0.25", frac)
	}
}

func TestGambleWinDoublesAndLadders(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 100)
	if m.gamble == nil || m.gamble.atRisk != 100 {
		t.Fatalf("enterGamble did not hold the win: %+v", m.gamble)
	}
	m.gamble.sel = selRed
	rm.resolveGuess(r, p.AccountID, suitHearts) // hearts = red -> RED wins, x2
	if m.gamble == nil || m.gamble.atRisk != 200 || m.gamble.rungs != 1 {
		t.Fatalf("after red win: %+v, want atRisk 200 rung 1", m.gamble)
	}
	m.gamble.sel = selSpades
	rm.resolveGuess(r, p.AccountID, suitSpades) // suit hit -> x4
	if m.gamble == nil || m.gamble.atRisk != 800 {
		t.Fatalf("after suit win: %+v, want atRisk 800", m.gamble)
	}
}

func TestGambleTakeBanksWin(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 250)
	m.gamble.sel = selTake
	rm.gambleConfirm(r, p.AccountID)
	if m.gamble != nil {
		t.Fatal("take should clear the gamble")
	}
	if m.balance != 1250 {
		t.Fatalf("balance = %d, want 1250 (win banked)", m.balance)
	}
}

func TestGambleLossForfeits(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	rm.enterGamble(r, m, 100)
	m.gamble.sel = selRed
	rm.resolveGuess(r, p.AccountID, suitSpades) // spades = black -> RED loses
	if m.gamble != nil {
		t.Fatal("a loss should clear the gamble")
	}
	if m.balance != 1000 {
		t.Fatalf("balance = %d, want 1000 (win forfeited, nothing credited)", m.balance)
	}
}

func TestGambleAutoTakesAtRungCap(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 1000
	// lastVar drives the cap; default is 5 rungs.
	m.lastVar = defaultVariant()
	rm.enterGamble(r, m, 1)
	for i := 0; i < 5; i++ {
		if m.gamble == nil {
			t.Fatalf("gamble cleared early at rung %d", i)
		}
		m.gamble.sel = selRed
		rm.resolveGuess(r, p.AccountID, suitHearts) // always win
	}
	if m.gamble != nil {
		t.Fatal("ladder should auto-take at the rung cap (5)")
	}
	if m.balance != 1000+32 { // 1 doubled five times = 32
		t.Fatalf("balance = %d, want %d", m.balance, 1000+32)
	}
}
