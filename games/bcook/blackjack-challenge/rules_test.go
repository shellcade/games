package main

import "testing"

func TestStarPairsOutcome(t *testing.T) {
	cases := []struct {
		a, b card
		kind string
		mult int
	}{
		{card{rankAce, suitSpade}, card{rankAce, suitHeart}, "aces", 30},
		{card{rankAce, suitSpade}, card{rankAce, suitClub}, "aces", 30}, // aces outrank perfect/colored
		{card{8, suitDiamond}, card{8, suitDiamond}, "perfect", 20},
		{card{8, suitDiamond}, card{8, suitHeart}, "colored", 8},
		{card{8, suitDiamond}, card{8, suitClub}, "mixed", 5},
		{card{10, suitSpade}, card{rankJack, suitSpade}, "", 0}, // same points, different rank: not a pair
		{card{rankKing, suitSpade}, card{rankQueen, suitSpade}, "", 0},
	}
	for _, c := range cases {
		kind, mult := starPairsOutcome(c.a, c.b)
		if kind != c.kind || mult != c.mult {
			t.Errorf("starPairsOutcome(%v,%v) = %q,%d want %q,%d", c.a, c.b, kind, mult, c.kind, c.mult)
		}
	}
}

func TestBjTen(t *testing.T) {
	if r := bjTen(hand{{rankAce, suitSpade}, {rankKing, suitHeart}}); r != rankKing {
		t.Errorf("bjTen(A,K) = %v want K", r)
	}
	if r := bjTen(hand{{10, suitSpade}, {rankAce, suitHeart}}); r != 10 {
		t.Errorf("bjTen(10,A) = %v want 10", r)
	}
	if r := bjTen(hand{{10, suitSpade}, {9, suitHeart}}); r != 0 {
		t.Errorf("bjTen(non-blackjack) = %v want 0", r)
	}
}

func TestBlackjackMult(t *testing.T) {
	cases := []struct {
		p, d rank
		dBJ  bool
		want int
	}{
		{rankKing, 0, false, 2},          // dealer no blackjack: 2:1
		{rankKing, rankQueen, true, 5},   // player ten outranks: 5:1
		{rankJack, rankJack, true, 4},    // same rank: 4:1
		{10, rankJack, true, 3},          // outranked: 3:1
	}
	for _, c := range cases {
		if got := blackjackMult(c.p, c.d, c.dBJ); got != c.want {
			t.Errorf("blackjackMult(%v,%v,%v) = %d want %d", c.p, c.d, c.dBJ, got, c.want)
		}
	}
}

func TestGrossMult(t *testing.T) {
	d19 := hand{{10, suitClub}, {9, suitDiamond}}
	d22 := hand{{10, suitClub}, {6, suitDiamond}, {6, suitSpade}}
	dBJhand := hand{{rankAce, suitClub}, {rankJack, suitDiamond}}
	cases := []struct {
		name string
		h    *phand
		d    hand
		dBJ  bool
		want int
	}{
		{"bust loses even vs dealer bust", &phand{cards: hand{{10, suitSpade}, {9, suitHeart}, {5, suitClub}}}, d22, false, 0},
		{"auto-won pays 2 even vs dealer blackjack", &phand{cards: hand{{7, suitSpade}, {7, suitHeart}, {7, suitClub}}, autoWon: true}, dBJhand, true, 2},
		{"blackjack vs no dealer blackjack pays 1+2", &phand{cards: hand{{rankAce, suitSpade}, {rankKing, suitHeart}}}, d19, false, 3},
		{"blackjack K vs dealer blackjack J pays 1+5", &phand{cards: hand{{rankAce, suitSpade}, {rankKing, suitHeart}}}, dBJhand, true, 6},
		{"ordinary hand loses to dealer blackjack", &phand{cards: hand{{10, suitSpade}, {9, suitHeart}}}, dBJhand, true, 0},
		{"dealer bust pays even", &phand{cards: hand{{10, suitSpade}, {2, suitHeart}}}, d22, false, 2},
		{"higher total wins even", &phand{cards: hand{{10, suitSpade}, {10, suitHeart}}}, d19, false, 2},
		{"TIE LOSES", &phand{cards: hand{{10, suitSpade}, {9, suitHeart}}}, d19, false, 0},
		{"lower total loses", &phand{cards: hand{{10, suitSpade}, {8, suitHeart}}}, d19, false, 0},
	}
	for _, c := range cases {
		if got := grossMult(c.h, c.d, c.dBJ); got != c.want {
			t.Errorf("%s: grossMult = %d want %d", c.name, got, c.want)
		}
	}
}
