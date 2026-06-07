package main

import (
	"context"

	kit "github.com/shellcade/kit/v2"
)

// The Greed Engine (design §8/§11): depth only counts when BANKED at a
// stairwell shrine — every shrine is a cash-out-or-push-deeper moment. The
// Soul Score is the bones-engager's composite (secondary board feeder), and
// the per-account KV keys power the weekly social boards.

// bank cashes the current depth in at a shrine: the leaderboard metric moves,
// the board post fires (BestResult keeps the weekly max), and the turn count
// to first bank is frozen for the rush bonus.
func (d *delver) bank(rm *room, r kit.Room) {
	f := rm.world.at(d.floor)
	if f.tiles[d.y][d.x] != tShrine {
		d.say("No shrine here. The deep keeps what you haven't banked.")
		return
	}
	if d.floor <= d.banked {
		d.say("B" + itoa(d.banked) + " is already banked. Deeper, delver.")
		return
	}
	d.banked = d.floor
	if d.firstBankTurn == 0 {
		d.firstBankTurn = d.turns
	}
	d.say("BANKED. B" + itoa(d.banked) + " is yours whatever happens down there.")
	r.Post(kit.Result{Rankings: []kit.PlayerResult{{
		Player: d.p, Metric: d.banked, Rank: 1, Status: kit.StatusFinished,
	}}})
	kvAdd(r, d.p, "banks_wk", 1)
}

// soulScore computes the run's composite (design §8, caps included; floors
// at zero). The rush bonus is the minor tiebreaker it was rebalanced to be.
func (d *delver) soulScore() int {
	tier := 0
	switch {
	case d.banked >= 21:
		tier = 5
	case d.banked >= 16:
		tier = 4
	case d.banked >= 10:
		tier = 3
	case d.banked >= 4:
		tier = 2
	case d.banked >= 1:
		tier = 1
	}
	s := 100*d.banked + 250*min2(tier, 1)*0 // tier bonus applied below as one-time band value
	s = 100 * d.banked
	if tier > 0 {
		s += 250
	}
	s += 8 * cap2(d.kills, 200)
	s += cap2(d.gold, 5000) / 10
	s += 200 * cap2(d.respects, 30)
	s += 150 * cap2(d.avenges, 30)
	s += 60 * cap2(d.looted, 5)
	s -= 40 * d.devours
	if d.firstBankTurn > 0 && d.firstBankTurn < 600 {
		s += (600 - d.firstBankTurn) / 10
	}
	if s < 0 {
		s = 0
	}
	return s
}

func cap2(v, c int) int {
	if v > c {
		return c
	}
	return v
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// settleRunScore writes the run's score and counters to the per-account KV —
// the weekly boards' raw material (design §11 keys). MergeMax keeps bests;
// MergeSum accumulates the social counters.
func (rm *room) settleRunScore(r kit.Room, d *delver) {
	kvMax(r, d.p, "soulscore_best_wk", d.soulScore())
	kvAdd(r, d.p, "avenges_done_wk", d.avenges)
	kvAdd(r, d.p, "gear_looted", d.looted)
	kvAdd(r, d.p, "devours_wk", d.devours)
}

// creditRespect records the mourner AND the mourned (respects_received is
// MOST MOURNED's metric; the dead's account is found by handle on the roster
// — an offline author's flowers still count when they reconnect this week).
func (rm *room) creditRespect(r kit.Room, mourner *delver, c *corpse) {
	kvAdd(r, mourner.p, "respects_given_wk", 1)
	for _, o := range rm.delvers {
		if o.p.Handle == c.handle {
			kvAdd(r, o.p, "respects_received", 1)
			return
		}
	}
}

// kvAdd / kvMax are allocation-light KV helpers on the per-account store.
func kvAdd(r kit.Room, p kit.Player, key string, n int) {
	if n == 0 {
		return
	}
	st := r.Services().Accounts.For(p).Store()
	_ = st.Set(context.Background(), key, []byte(itoa(n)), kit.MergeSum)
}

func kvMax(r kit.Room, p kit.Player, key string, n int) {
	st := r.Services().Accounts.For(p).Store()
	_ = st.Set(context.Background(), key, []byte(itoa(n)), kit.MergeMax)
}
