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

func TestRecommendBasicStrategyCells(t *testing.T) {
	up := func(r rank) card { return card{r, suitSpade} }
	cases := []struct {
		name  string
		cards hand
		up    rank
		want  string
		why   string
	}{
		{"aces always split", hand{{rankAce, suitHeart}, {rankAce, suitSpade}}, 6, saySplit, "pair of As vs 6"},
		{"eights always split", hand{{8, suitHeart}, {8, suitSpade}}, 10, saySplit, "pair of 8s vs 10"},
		{"nines stand vs seven", hand{{9, suitHeart}, {9, suitSpade}}, 7, sayStand, "pair of 9s vs 7"},
		{"tens never split", hand{{rankKing, suitHeart}, {rankKing, suitSpade}}, 6, sayStand, "hard 20 vs 6"},
		{"fives play as hard ten", hand{{5, suitHeart}, {5, suitSpade}}, 6, sayDouble, "hard 10 vs 6"},
		{"hard 16 vs 10 surrenders", hand{{10, suitHeart}, {6, suitSpade}}, 10, saySurrender, "hard 16 vs 10"},
		{"hard 15 vs 10 surrenders", hand{{10, suitHeart}, {5, suitSpade}}, 10, saySurrender, "hard 15 vs 10"},
		{"hard 16 vs 6 stands", hand{{10, suitHeart}, {6, suitSpade}}, 6, sayStand, "hard 16 vs 6"},
		{"hard 12 vs 2 hits", hand{{10, suitHeart}, {2, suitSpade}}, 2, sayHit, "hard 12 vs 2"},
		{"hard 12 vs 4 stands", hand{{10, suitHeart}, {2, suitSpade}}, 4, sayStand, "hard 12 vs 4"},
		{"eleven doubles vs 10", hand{{6, suitHeart}, {5, suitSpade}}, 10, sayDouble, "hard 11 vs 10"},
		{"eleven hits vs ace", hand{{6, suitHeart}, {5, suitSpade}}, rankAce, sayHit, "hard 11 vs A"},
		{"soft 18 doubles vs 5", hand{{rankAce, suitHeart}, {7, suitSpade}}, 5, sayDouble, "soft 18 vs 5"},
		{"soft 18 stands vs 8", hand{{rankAce, suitHeart}, {7, suitSpade}}, 8, sayStand, "soft 18 vs 8"},
		{"soft 18 hits vs 10", hand{{rankAce, suitHeart}, {7, suitSpade}}, 10, sayHit, "soft 18 vs 10"},
		{"soft 17 doubles vs 4", hand{{rankAce, suitHeart}, {6, suitSpade}}, 4, sayDouble, "soft 17 vs 4"},
		{"faces read as 10", hand{{rankQueen, suitHeart}, {7, suitSpade}}, rankJack, sayStand, "hard 17 vs 10"},
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

	// A third card closes doubling and surrendering: hard 16 vs 10 hits.
	s, h := hintSeat(card{10, suitHeart}, card{4, suitSpade}, card{2, suitClub})
	if act, _ := recommend(s, h, up(10)); act != sayHit {
		t.Fatalf("3-card 16 vs 10 = %s, want HIT (surrender window passed)", act)
	}

	// A thin bankroll closes the double: soft 18 vs 5 falls back to STAND.
	s, h = hintSeat(card{rankAce, suitHeart}, card{7, suitSpade})
	s.bal = 0
	if act, _ := recommend(s, h, up(5)); act != sayStand {
		t.Fatalf("broke soft 18 vs 5 = %s, want STAND (can't double)", act)
	}

	// Split hands (2+) lose the surrender cell: hard 16 vs 10 hits.
	s, h = hintSeat(card{10, suitHeart}, card{6, suitSpade})
	s.hands = append(s.hands, &phand{cards: hand{{2, suitClub}, {3, suitClub}}, bet: 50})
	if act, _ := recommend(s, h, up(10)); act != sayHit {
		t.Fatalf("split-table 16 vs 10 = %s, want HIT (no surrender)", act)
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
	// Hand-craft a turn: hard 14 vs dealer 10 says HIT.
	s.placed = true
	s.hands = []*phand{{cards: hand{{10, suitHeart}, {4, suitSpade}}, bet: 50}}
	rm.dealer = hand{{10, suitClub}, {7, suitDiamond}}
	rm.dealerHole = true
	rm.phase = phTurns
	rm.render(tr)
	row := kittest.String(tr.LastFrame(a), hintRow)
	if !strings.Contains(row, "hint: HIT - hard 14 vs 10") {
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
