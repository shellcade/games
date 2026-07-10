package main

import (
	"strings"
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

// hintSeat builds a seat holding one hand with enough bankroll for any
// double/split, so availability defaults to the chart's ideal cells.
func hintSeat(cards ...card) (*seat, *phand) {
	h := &phand{cards: cards, bet: 50}
	return &seat{bal: 10000, hands: []*phand{h}}, h
}

func TestRecommendChallengeCells(t *testing.T) {
	up := func(r rank) card { return card{r, suitSpade} }
	cases := []struct {
		name  string
		cards hand
		up    rank
		want  string
		why   string
	}{
		// Variant twists first: value pairs, no surrender, 3-card doubles.
		{"K+10 pairs by value, stands", hand{{rankKing, suitHeart}, {10, suitSpade}}, 6, sayStand, "hard 20 vs 6"},
		{"K+Q splits like 8s? no - stands as 20", hand{{rankKing, suitHeart}, {rankQueen, suitSpade}}, 6, sayStand, "hard 20 vs 6"},
		{"hard 16 vs 10 hits (no surrender)", hand{{10, suitHeart}, {6, suitSpade}}, 10, sayHit, "hard 16 vs 10"},
		{"3-card 11 still doubles", hand{{2, suitHeart}, {4, suitSpade}, {5, suitClub}}, 6, sayDouble, "hard 11 vs 6"},
		// Five Card Trick cells.
		{"4-card soft always hits", hand{{rankAce, suitHeart}, {2, suitSpade}, {2, suitClub}, {4, suitDiamond}}, 10, sayHit, "soft 4 cards - the trick is safe"},
		{"4-card hard 14 chases the trick", hand{{2, suitHeart}, {3, suitSpade}, {4, suitClub}, {5, suitDiamond}}, 5, sayHit, "4 cards - a safe hit wins"},
		{"4-card hard 17 vs 10 hits (ties lose)", hand{{2, suitHeart}, {3, suitSpade}, {4, suitClub}, {8, suitDiamond}}, 10, sayHit, "4-card 17 vs 10 - ties lose"},
		{"4-card hard 17 vs 6 stands", hand{{2, suitHeart}, {3, suitSpade}, {4, suitClub}, {8, suitDiamond}}, 6, sayStand, "hard 17 vs 6"},
		{"4-card hard 16 vs 5 keeps the book stand", hand{{2, suitHeart}, {3, suitSpade}, {4, suitClub}, {7, suitDiamond}}, 5, sayStand, "hard 16 vs 5"},
		// Shared basic-strategy spine.
		{"aces always split", hand{{rankAce, suitHeart}, {rankAce, suitSpade}}, 10, saySplit, "pair of As vs 10"},
		{"eights always split", hand{{8, suitHeart}, {8, suitSpade}}, 10, saySplit, "pair of 8s vs 10"},
		{"soft 18 doubles vs 5", hand{{rankAce, suitHeart}, {7, suitSpade}}, 5, sayDouble, "soft 18 vs 5"},
		{"hard 12 vs 4 stands", hand{{10, suitHeart}, {2, suitSpade}}, 4, sayStand, "hard 12 vs 4"},
	}
	for _, c := range cases {
		s, h := hintSeat(c.cards...)
		act, why := recommend(s, h, up(c.up))
		if act != c.want || why != c.why {
			t.Errorf("%s: recommend = %s (%s), want %s (%s)", c.name, act, why, c.want, c.why)
		}
	}
}

// TestRecommendDegradesWithAvailability asserts the fallback rules: the ideal
// cell is only suggested while the table would actually accept it.
func TestRecommendDegradesWithAvailability(t *testing.T) {
	up := func(r rank) card { return card{r, suitSpade} }

	// A fourth card closes the double (challenge doubles on 2-3 cards only):
	// hard 11 with four cards chases the trick instead.
	s, h := hintSeat(card{2, suitHeart}, card{2, suitSpade}, card{3, suitClub}, card{4, suitDiamond})
	if act, why := recommend(s, h, up(6)); act != sayHit || why != "4 cards - a safe hit wins" {
		t.Fatalf("4-card 11 vs 6 = %s (%s), want the trick HIT", act, why)
	}

	// A thin bankroll closes the double: soft 18 vs 5 falls back to STAND.
	s, h = hintSeat(card{rankAce, suitHeart}, card{7, suitSpade})
	s.bal = 0
	if act, _ := recommend(s, h, up(5)); act != sayStand {
		t.Fatalf("broke soft 18 vs 5 = %s, want STAND (can't double)", act)
	}

	// The hand cap closes the split: 8,8 at maxHands plays as hard 16.
	s, h = hintSeat(card{8, suitHeart}, card{8, suitSpade})
	for len(s.hands) < maxHands {
		s.hands = append(s.hands, &phand{cards: hand{{2, suitClub}, {3, suitClub}}, bet: 50})
	}
	if act, _ := recommend(s, h, up(6)); act != sayStand {
		t.Fatalf("capped 8,8 vs 6 = %s, want STAND (plays as hard 16)", act)
	}
}

// TestHintToggleAndRender asserts ? flips the seat's hint card and the book
// line renders for the active viewer on their turn.
func TestHintToggleAndRender(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	rm.OnInput(tr, a, runeInput('?'))
	if !s.hint {
		t.Fatal("? did not toggle the hint card on")
	}
	// Hand-craft a turn: hard 16 vs the dealer's face-up 10 says HIT here
	// (this table has no surrender).
	s.placed = true
	s.hands = []*phand{{cards: hand{{10, suitHeart}, {6, suitSpade}}, bet: 50}}
	rm.dealer = hand{{10, suitClub}}
	rm.phase = phTurns
	rm.render(tr)
	row := kittest.String(tr.LastFrame(a), hintRow)
	if !strings.Contains(row, "hint: HIT - hard 16 vs 10") {
		t.Fatalf("hint line not rendered on row %d: %q", hintRow, row)
	}
	// Toggling off clears it.
	rm.OnInput(tr, a, runeInput('/')) // the unshifted alias
	if s.hint {
		t.Fatal("/ did not toggle the hint card off")
	}
	rm.render(tr)
	if row := kittest.String(tr.LastFrame(a), hintRow); strings.Contains(row, "hint:") {
		t.Fatalf("hint line still rendered after toggle-off: %q", row)
	}
}
