package main

import (
	"context"
	"strconv"
	"strings"

	kit "github.com/shellcade/kit/v2"
)

// --- durable wallet ----------------------------------------------------------
//
// The casino pattern over kv (shared with pokies): balance (merge rule sum, the
// carryable bankroll) and peak (merge rule max, the high-water mark and
// leaderboard metric).

const (
	keyBalance   = "balance"
	keyPeak      = "peak"
	startBalance = 1000 // credits a fresh wallet starts with
	rebuyAmount  = 1000 // balance restored on a bust
)

func kvInt(store kit.KVStore, key string) (int, bool) {
	v, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(v)))
	if err != nil {
		return 0, false
	}
	return n, true
}

// seedWallet returns the joining player's durable (balance, peak): balance
// defaults to startBalance for a first-ever player (or a non-positive stored
// balance), and peak is raised to at least the balance. A nil/guest account
// returns the defaults.
func seedWallet(r kit.Room, p kit.Player) (int, int) {
	acct := r.Services().Accounts.For(p)
	if acct == nil {
		return startBalance, startBalance
	}
	store := acct.Store()
	bal, ok := kvInt(store, keyBalance)
	if !ok || bal <= 0 {
		bal = startBalance
	}
	peak, ok := kvInt(store, keyPeak)
	if !ok || peak < bal {
		peak = bal
	}
	return bal, peak
}

// persistWallet writes the current balance (summed) and raises peak (max). peak
// uses a monotonic max-on-write, so out-of-order or concurrent same-account
// writes can never regress the leaderboard metric. Delegates to the kit's
// ScoreKeeper.PersistWallet, which writes the identical keys + merge rules,
// replacing the duplicated casino-wallet helper.
func (rm *room) persistWallet(r kit.Room, p kit.Player, bal, peak int) {
	rm.sk.PersistWallet(r, p, keyBalance, bal, keyPeak, peak)
}
