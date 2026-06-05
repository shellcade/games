package main

import (
	"math/rand"
	"testing"
)

func TestHandValue(t *testing.T) {
	cases := []struct {
		name  string
		h     hand
		total int
		soft  bool
	}{
		{"soft 18", hand{{rankAce, suitSpade}, {7, suitHeart}}, 18, true},
		{"hard 13 after a third card", hand{{rankAce, suitSpade}, {7, suitHeart}, {5, suitClub}}, 13, false},
		{"pair of aces is soft 12", hand{{rankAce, suitSpade}, {rankAce, suitHeart}}, 12, true},
		{"two faces", hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}, 20, false},
		{"three sevens is a hard 21", hand{{7, suitSpade}, {7, suitHeart}, {7, suitClub}}, 21, false},
		{"ten and ace is 21 soft", hand{{10, suitSpade}, {rankAce, suitHeart}}, 21, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			total, soft := c.h.value()
			if total != c.total || soft != c.soft {
				t.Fatalf("value() = (%d, soft=%v), want (%d, soft=%v)", total, soft, c.total, c.soft)
			}
		})
	}
}

func TestBlackjack(t *testing.T) {
	if !(hand{{rankAce, suitSpade}, {rankKing, suitHeart}}).isBlackjack() {
		t.Error("A,K should be a blackjack")
	}
	if (hand{{7, suitSpade}, {7, suitHeart}, {7, suitClub}}).isBlackjack() {
		t.Error("7,7,7 (21 in three cards) is not a blackjack")
	}
	if (hand{{10, suitSpade}, {9, suitHeart}}).isBlackjack() {
		t.Error("a two-card 19 is not a blackjack")
	}
}

func TestBust(t *testing.T) {
	if !(hand{{rankKing, suitSpade}, {rankQueen, suitHeart}, {5, suitClub}}).isBust() {
		t.Error("K,Q,5 (25) should bust")
	}
	if (hand{{rankAce, suitSpade}, {rankKing, suitHeart}, {5, suitClub}}).isBust() {
		t.Error("A,K,5 reduces to 16 and must not bust")
	}
}

func TestShoeIsSixDecks(t *testing.T) {
	s := newShoe(rand.New(rand.NewSource(1)))
	if len(s.cards) != numDecks*52 {
		t.Fatalf("shoe has %d cards, want %d", len(s.cards), numDecks*52)
	}
	// every (rank,suit) appears exactly numDecks times
	counts := map[card]int{}
	for _, c := range s.cards {
		counts[c]++
	}
	if len(counts) != 52 {
		t.Fatalf("shoe has %d distinct cards, want 52", len(counts))
	}
	for c, n := range counts {
		if n != numDecks {
			t.Fatalf("card %v appears %d times, want %d", c, n, numDecks)
		}
	}
}

func TestSeededShoeReproducesOrder(t *testing.T) {
	a := newShoe(rand.New(rand.NewSource(42)))
	b := newShoe(rand.New(rand.NewSource(42)))
	for i := 0; i < 100; i++ {
		if a.draw() != b.draw() {
			t.Fatalf("same-seed shoes diverged at draw %d", i)
		}
	}
}

func TestShoeReshufflePastCut(t *testing.T) {
	s := newShoe(rand.New(rand.NewSource(7)))
	if s.needsReshuffle() {
		t.Fatal("a fresh shoe should not need a reshuffle")
	}
	for !s.needsReshuffle() {
		s.draw()
	}
	if s.pos < s.cut {
		t.Fatalf("needsReshuffle true at pos %d but cut is %d", s.pos, s.cut)
	}
}
