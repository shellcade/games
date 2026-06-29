package main

import (
	"math/rand"
	"testing"
)

func TestDealerStandsOnSoft17(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	d := dealerPlay(hand{{rankAce, suitSpade}, {6, suitHeart}}, newShoe(rng), rng) // soft 17
	if len(d) != 2 {
		t.Fatalf("dealer drew on soft 17 (S17 should stand): %v", d)
	}
}

func TestDealerDrawsTo17(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	d := dealerPlay(hand{{10, suitSpade}, {6, suitHeart}}, newShoe(rng), rng) // hard 16
	if total := d.total(); total < 17 && !d.isBust() {
		t.Fatalf("dealer stopped at %d, must reach 17+ or bust", total)
	}
}

func TestSettleHand(t *testing.T) {
	bj := hand{{rankAce, suitSpade}, {rankKing, suitHeart}}
	twenty := hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}
	nineteen := hand{{10, suitSpade}, {9, suitHeart}}
	bust := hand{{rankKing, suitSpade}, {rankQueen, suitHeart}, {5, suitClub}}

	cases := []struct {
		name   string
		player hand
		dealer hand
		want   outcome
	}{
		{"player blackjack beats 20", bj, twenty, outBlackjack},
		{"both blackjack push", bj, bj, outPush},
		{"dealer blackjack beats 20", twenty, bj, outLose},
		{"higher beats lower", twenty, nineteen, outWin},
		{"equal totals push", twenty, twenty, outPush},
		{"player bust loses", bust, nineteen, outLose},
		{"player bust loses even if dealer busts", bust, bust, outLose},
		{"dealer bust, player stands", nineteen, bust, outWin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := settleHand(c.player, c.dealer); got != c.want {
				t.Fatalf("settleHand = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSplit21IsNotBlackjack(t *testing.T) {
	// A two-card 21 formed by splitting is a plain 21, not a 3:2 blackjack.
	twentyOne := hand{{rankAce, suitSpade}, {rankKing, suitHeart}}
	twenty := hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}
	if got := settleHandEx(twentyOne, false /* fromSplit */, twenty, false); got != outWin {
		t.Fatalf("split 21 vs dealer 20 = %v, want outWin (not outBlackjack)", got)
	}
}

func TestCreditFor(t *testing.T) {
	cases := []struct {
		o    outcome
		bet  int
		want int
	}{
		{outLose, 100, 0},
		{outPush, 100, 100},
		{outWin, 100, 200},
		{outBlackjack, 100, 250}, // stake 100 + 3:2 (150)
		{outBlackjack, 10, 25},   // stake 10 + 3:2 (15)
		{outBlackjack, 25, 63},   // stake 25 + 3:2 (37.5 -> 38, half-chip rounds UP to the player)
		{outBlackjack, 50, 125},  // stake 50 + 3:2 (75)
	}
	for _, c := range cases {
		if got := creditFor(c.o, c.bet); got != c.want {
			t.Errorf("creditFor(%v, %d) = %d, want %d", c.o, c.bet, got, c.want)
		}
	}
}

func TestInsuranceCredit(t *testing.T) {
	if got := insuranceCredit(true, 50); got != 150 { // stake 50 + 2:1 (100)
		t.Errorf("insurance on dealer blackjack = %d, want 150", got)
	}
	if got := insuranceCredit(false, 50); got != 0 {
		t.Errorf("insurance with no dealer blackjack = %d, want 0", got)
	}
}

func TestPerfectPairsOutcome(t *testing.T) {
	cases := []struct {
		name     string
		a, b     card
		wantKind string
		wantMult int
	}{
		{"perfect: same rank and suit", card{8, suitHeart}, card{8, suitHeart}, "perfect", 25},
		{"colored: same rank, both red", card{8, suitHeart}, card{8, suitDiamond}, "colored", 12},
		{"colored: same rank, both black", card{rankKing, suitSpade}, card{rankKing, suitClub}, "colored", 12},
		{"mixed: same rank, different colour", card{8, suitSpade}, card{8, suitHeart}, "mixed", 6},
		{"no pair: different rank", card{8, suitSpade}, card{9, suitSpade}, "", 0},
		{"no pair: ten and face are not a pair", card{10, suitSpade}, card{rankKing, suitSpade}, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, mult := perfectPairsOutcome(c.a, c.b)
			if kind != c.wantKind || mult != c.wantMult {
				t.Fatalf("perfectPairsOutcome = (%q, %d), want (%q, %d)", kind, mult, c.wantKind, c.wantMult)
			}
		})
	}
}

func TestAffordTier(t *testing.T) {
	tiers := []int{0, 10, 25, 50, 100}
	cases := []struct {
		want, budget, out int
	}{
		{100, 1000, 100}, // wanted tier fits the budget
		{100, 30, 25},    // capped to the highest tier within budget
		{50, 5, 0},       // nothing but "off" fits
		{25, 25, 25},     // exact fit
		{100, 0, 0},      // no budget -> off
		{100, -5, 0},     // negative budget -> off (never panics)
	}
	for _, c := range cases {
		if got := affordTier(tiers, c.want, c.budget); got != c.out {
			t.Errorf("affordTier(want=%d, budget=%d) = %d, want %d", c.want, c.budget, got, c.out)
		}
	}
}

func TestPairsCreditFor(t *testing.T) {
	cases := []struct {
		mult int
		bet  int
		want int
	}{
		{0, 10, 0},     // no pair: side stake lost
		{6, 10, 70},    // mixed 6:1: stake 10 + 60
		{12, 25, 325},  // colored 12:1: stake 25 + 300
		{25, 50, 1300}, // perfect 25:1: stake 50 + 1250
	}
	for _, c := range cases {
		if got := pairsCreditFor(c.mult, c.bet); got != c.want {
			t.Errorf("pairsCreditFor(%d, %d) = %d, want %d", c.mult, c.bet, got, c.want)
		}
	}
}
