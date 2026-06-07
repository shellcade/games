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
	ra := rand.New(rand.NewSource(42))
	rb := rand.New(rand.NewSource(42))
	a := newShoe(ra)
	b := newShoe(rb)
	for i := 0; i < 100; i++ {
		if a.draw(ra) != b.draw(rb) {
			t.Fatalf("same-seed shoes diverged at draw %d", i)
		}
	}
}

func TestShoeReshufflePastCut(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	s := newShoe(rng)
	if s.needsReshuffle() {
		t.Fatal("a fresh shoe should not need a reshuffle")
	}
	for !s.needsReshuffle() {
		s.draw(rng)
	}
	if s.pos < s.cut {
		t.Fatalf("needsReshuffle true at pos %d but cut is %d", s.pos, s.cut)
	}
}

func TestShoeDrainMidRoundRecyclesDiscards(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	s := newShoe(rng)
	// Burn most of the shoe as settled rounds, then start a round with only a
	// few cards left: the round must outrun the shoe.
	s.pos = len(s.cards) - 5
	s.beginRound()
	seen := map[card]int{}
	// 40 draws in one round: a clamping shoe would repeat its last card ~36
	// times; a recycling shoe deals from the reshuffled discards instead.
	for i := 0; i < 40; i++ {
		seen[s.draw(rng)]++
	}
	for c, n := range seen {
		if n > numDecks {
			t.Fatalf("card %v dealt %d times within one round; only %d exist in the shoe", c, n, numDecks)
		}
	}
	if !s.recycled {
		t.Fatal("draining the round should have recycled the discards")
	}
	if !s.needsReshuffle() {
		t.Fatal("a recycled shoe must reshuffle at the next round boundary")
	}
	s.shuffle(rng)
	if s.needsReshuffle() || s.recycled || s.roundStart != 0 {
		t.Fatal("shuffle must clear the recycle state")
	}
	if len(s.cards) != numDecks*52 {
		t.Fatalf("recycle changed the shoe size to %d, want %d", len(s.cards), numDecks*52)
	}
}
