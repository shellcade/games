package main

import (
	"math/rand"
	"testing"
)

func TestDealerStandsOnSoft17(t *testing.T) {
	s := newShoe(rand.New(rand.NewSource(1)))
	d := dealerPlay(hand{{rankAce, suitSpade}, {6, suitHeart}}, s) // soft 17
	if len(d) != 2 {
		t.Fatalf("dealer drew on soft 17 (S17 should stand): %v", d)
	}
}

func TestDealerDrawsTo17(t *testing.T) {
	s := newShoe(rand.New(rand.NewSource(2)))
	d := dealerPlay(hand{{10, suitSpade}, {6, suitHeart}}, s) // hard 16
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
