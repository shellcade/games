package main

import "strconv"

// The hint card (toggled with ?): what the book says to do with the hand on
// turn, adapted to THIS table's rules. The base is six-deck basic strategy
// (dealer stands on all 17), adjusted for the Challenge variant:
//
//   - no surrender and no insurance exist here, so those cells fall back to
//     their hit lines;
//   - doubling is open on two OR three cards (legalActions), so the double
//     cells stay live one card longer;
//   - pairs split by POINT VALUE (K+10 is a pair of 10s);
//   - the Five Card Trick dominates four-card hands: a soft four-card hand
//     can never bust, so hitting is a GUARANTEED instant even-money win, and
//     a hard four-card 15 or less wins on the spot often enough that hitting
//     beats standing on every cell;
//   - ties lose, which drags down standing on made hands against strong up
//     cards — enough to flip the four-card hard 17 vs 9/10/A cell to HIT
//     (standing loses the frequent dealer ties; a safe hit wins instantly).
const (
	sayHit    = "HIT"
	sayStand  = "STAND"
	sayDouble = "DOUBLE"
	saySplit  = "SPLIT"
)

// recommend returns the book play for hand h against the dealer's face-up
// card, plus the situation it read ("hard 14 vs 10"), mirroring legalActions'
// availability rules so it never suggests a move the table would reject.
func recommend(s *seat, h *phand, up card) (act, why string) {
	canDouble := !h.doubled && len(h.cards) <= 3 && s.bal >= h.bet
	canSplit := len(h.cards) == 2 && h.cards[0].r.points() == h.cards[1].r.points() &&
		s.bal >= h.bet && len(s.hands) < maxHands
	u := up.r.points() // 2..10, A = 11
	vs := " vs " + upLabel(up)
	total, soft := h.cards.value()

	// Four cards: the Five Card Trick takes over (see the header note). An
	// unresolved hand never holds five (a safe fifth card auto-wins).
	if len(h.cards) >= 4 {
		switch {
		case soft:
			return sayHit, "soft 4 cards - the trick is safe"
		case total <= 15:
			return sayHit, "4 cards - a safe hit wins"
		case total == 17 && u >= 9:
			return sayHit, "4-card 17" + vs + " - ties lose"
		}
		// 4-card 16 and made 17+ hands: fall through to the total tables.
	}

	if canSplit {
		if a, ok := pairPlay(h.cards[0].r.points(), h.cards[0].r == rankAce, u); ok {
			return a, "pair of " + pairName(h.cards[0]) + vs
		}
	}
	if soft {
		return softPlay(total, u, canDouble), "soft " + strconv.Itoa(total) + vs
	}
	return hardPlay(total, u, canDouble), "hard " + strconv.Itoa(total) + vs
}

// pairName names a split pair by what this table pairs on: rank for A-9, the
// shared point VALUE for the ten cards (K+10 reads "pair of 10s").
func pairName(c card) string {
	if c.r >= 10 {
		return "10s"
	}
	return c.r.label() + "s"
}

// pairPlay is the split table, keyed by the pair's point value (aces flagged
// apart from other 11-point math). ok=false hands the pair back to the total
// tables (5,5 plays as hard 10; 10-value pairs and non-split cells stand/hit
// by total).
func pairPlay(v int, aces bool, u int) (string, bool) {
	switch {
	case aces:
		return saySplit, true
	case v == 9:
		if u == 7 || u >= 10 {
			return sayStand, true
		}
		return saySplit, true
	case v == 8:
		return saySplit, true
	case v == 7:
		if u <= 7 {
			return saySplit, true
		}
	case v == 6:
		if u <= 6 {
			return saySplit, true
		}
	case v == 4:
		if u == 5 || u == 6 {
			return saySplit, true
		}
	case v == 2 || v == 3:
		if u <= 7 {
			return saySplit, true
		}
	}
	return "", false // 10s, 5s, and the chart's non-split cells: play the total
}

// softPlay is the soft-total table (an ace still counting 11).
func softPlay(total, u int, canDouble bool) string {
	switch {
	case total >= 19:
		return sayStand
	case total == 18:
		switch {
		case u <= 6:
			if canDouble {
				return sayDouble
			}
			return sayStand
		case u <= 8:
			return sayStand
		default:
			return sayHit
		}
	case total == 17:
		if canDouble && u >= 3 && u <= 6 {
			return sayDouble
		}
	case total >= 15: // soft 15-16
		if canDouble && u >= 4 && u <= 6 {
			return sayDouble
		}
	default: // soft 13-14
		if canDouble && (u == 5 || u == 6) {
			return sayDouble
		}
	}
	return sayHit
}

// hardPlay is the hard-total table. The base chart's surrender cells (16 vs
// 9/10/A, 15 vs 10) read HIT here — this table has no surrender.
func hardPlay(total, u int, canDouble bool) string {
	switch {
	case total >= 17:
		return sayStand
	case total >= 13: // 13-16
		if u <= 6 {
			return sayStand
		}
		return sayHit
	case total == 12:
		if u >= 4 && u <= 6 {
			return sayStand
		}
		return sayHit
	case total == 11:
		if canDouble && u <= 10 {
			return sayDouble
		}
		return sayHit
	case total == 10:
		if canDouble && u <= 9 {
			return sayDouble
		}
		return sayHit
	case total == 9:
		if canDouble && u >= 3 && u <= 6 {
			return sayDouble
		}
		return sayHit
	default:
		return sayHit
	}
}

// upLabel names the dealer's face-up card the way the chart reads: A, or the
// point value (every face reads 10).
func upLabel(up card) string {
	if up.r == rankAce {
		return "A"
	}
	return strconv.Itoa(up.r.points())
}
