package main

import "math/rand"

// rank is 1..13 with 1 = Ace, 11 = Jack, 12 = Queen, 13 = King.
type rank uint8

const (
	rankAce   rank = 1
	rankJack  rank = 11
	rankQueen rank = 12
	rankKing  rank = 13
)

// points is the blackjack value of a rank; an Ace counts 11 here and is reduced
// to 1 by hand.value when needed. Faces are worth 10.
func (r rank) points() int {
	switch {
	case r == rankAce:
		return 11
	case r >= 10:
		return 10
	default:
		return int(r)
	}
}

// label is the short display token: A, 2..10, J, Q, K.
func (r rank) label() string {
	switch r {
	case rankAce:
		return "A"
	case rankJack:
		return "J"
	case rankQueen:
		return "Q"
	case rankKing:
		return "K"
	default:
		// A small switch (not a map literal) keeps rendering free of
		// map-iteration-order surprises and allocation in the steady state.
		switch r {
		case 2:
			return "2"
		case 3:
			return "3"
		case 4:
			return "4"
		case 5:
			return "5"
		case 6:
			return "6"
		case 7:
			return "7"
		case 8:
			return "8"
		case 9:
			return "9"
		case 10:
			return "10"
		}
		return "?"
	}
}

// boxLabel is the one-character rank for a fixed-width card box (ten is "T", so
// every card face is exactly two cells: rank + suit pip).
func (r rank) boxLabel() string {
	if r == 10 {
		return "T"
	}
	return r.label()
}

// suit is one of the four suits. The pip is a single-width Unicode glyph.
type suit uint8

const (
	suitSpade suit = iota
	suitHeart
	suitDiamond
	suitClub
)

func (s suit) pip() rune { return [...]rune{'♠', '♥', '♦', '♣'}[s] }

// red reports whether the suit is drawn red (hearts/diamonds).
func (s suit) red() bool { return s == suitHeart || s == suitDiamond }

// card is a single playing card.
type card struct {
	r rank
	s suit
}

// hand is a set of cards held by a player or the dealer.
type hand []card

// value returns the best total and whether the hand is soft (an Ace still
// counts as 11).
func (h hand) value() (total int, soft bool) {
	aces := 0
	for _, c := range h {
		total += c.r.points()
		if c.r == rankAce {
			aces++
		}
	}
	for total > 21 && aces > 0 {
		total -= 10 // count one Ace as 1 instead of 11
		aces--
	}
	return total, aces > 0
}

// total is the hand value without the soft flag.
func (h hand) total() int { t, _ := h.value(); return t }

// isBlackjack reports a two-card 21.
func (h hand) isBlackjack() bool { return len(h) == 2 && h.total() == 21 }

// isBust reports a value over 21.
func (h hand) isBust() bool { return h.total() > 21 }

// --- shoe ------------------------------------------------------------------

const (
	numDecks       = 6
	cutPenetration = 0.75 // reshuffle once this fraction of the shoe is dealt
)

// shoe is a multi-deck stack dealt from front to back, reshuffled past a cut
// card. It draws from the room-seeded RNG so a seeded room reproduces every
// deal (and survives hibernation, since the shoe lives entirely in guest
// memory and the RNG is reconstructed from the room seed).
type shoe struct {
	cards      []card
	pos        int
	cut        int
	roundStart int  // pos when the current round began; cards before it are settled-round discards
	recycled   bool // drained mid-round and refilled from discards; reshuffle at the next round boundary
}

func newShoe(rng *rand.Rand) *shoe {
	cards := make([]card, 0, numDecks*52)
	for d := 0; d < numDecks; d++ {
		for s := suit(0); s < 4; s++ {
			for r := rank(1); r <= 13; r++ {
				cards = append(cards, card{r, s})
			}
		}
	}
	s := &shoe{cards: cards, cut: int(float64(len(cards)) * cutPenetration)}
	s.shuffle(rng)
	return s
}

// shuffle Fisher–Yates shuffles the whole shoe and resets the draw cursor.
func (s *shoe) shuffle(rng *rand.Rand) {
	for i := len(s.cards) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		s.cards[i], s.cards[j] = s.cards[j], s.cards[i]
	}
	s.pos = 0
	s.roundStart = 0
	s.recycled = false
}

// beginRound marks the draw cursor at a round boundary: everything before it
// is a settled round's discards, available to recycle if this round drains
// the shoe.
func (s *shoe) beginRound() { s.roundStart = s.pos }

// draw deals the next card. A shoe drained MID-round (an extreme run of splits
// and hits at a full table can outrun the ~78 cards behind the cut) recycles
// the settled rounds' discards rather than repeating the last card — the
// casino procedure, so a card already in play this round is never duplicated.
func (s *shoe) draw(rng *rand.Rand) card {
	if s.pos >= len(s.cards) {
		s.recycle(rng)
	}
	c := s.cards[s.pos]
	s.pos++
	return c
}

// recycle shuffles the settled-round discards (everything before roundStart)
// back behind the cards already dealt this round and continues from them. The
// whole shoe then reshuffles at the next round boundary. With nothing to
// recycle (one round cannot hold all 312 cards) the old defensive clamp
// remains.
func (s *shoe) recycle(rng *rand.Rand) {
	if s.roundStart == 0 {
		s.pos = len(s.cards) - 1
		return
	}
	discards := s.cards[:s.roundStart]
	for i := len(discards) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		discards[i], discards[j] = discards[j], discards[i]
	}
	rebuilt := make([]card, 0, len(s.cards))
	rebuilt = append(rebuilt, s.cards[s.roundStart:]...) // this round's dealt cards
	rebuilt = append(rebuilt, discards...)               // then the recycled discards
	s.pos = len(s.cards) - s.roundStart
	s.cards = rebuilt
	s.roundStart = 0
	s.recycled = true
}

// needsReshuffle reports whether the shoe must be reshuffled before the next
// round: the cut card has been reached, or a drained round recycled discards.
func (s *shoe) needsReshuffle() bool { return s.pos >= s.cut || s.recycled }
