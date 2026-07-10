package main

import "strconv"

// The hint card (toggled with ?): what the book says to do with the hand on
// turn. The table is standard six-deck basic strategy for THIS table's rules —
// dealer stands on all 17, double after split allowed, late surrender offered
// — and recommend degrades correctly when the ideal action is unavailable (no
// double after a hit, no surrender past the first decision, no split past the
// hand cap or the bankroll).

const (
	sayHit       = "HIT"
	sayStand     = "STAND"
	sayDouble    = "DOUBLE"
	saySplit     = "SPLIT"
	saySurrender = "SURRENDER"
)

// recommend returns the book play for hand h against the dealer up card, plus
// the situation it read ("hard 14 vs 10"), mirroring legalActions' availability
// rules so it never suggests a move the table would reject.
func recommend(s *seat, h *phand, up card) (act, why string) {
	first := len(h.cards) == 2 && !h.doubled
	canDouble := first && s.bal >= h.bet
	canSplit := first && h.cards[0].r == h.cards[1].r && s.bal >= h.bet && len(s.hands) < maxHands
	canSurrender := first && len(s.hands) == 1
	u := up.r.points() // 2..10, A = 11
	vs := " vs " + upLabel(up)

	if canSplit {
		if a, ok := pairPlay(h.cards[0].r.points(), h.cards[0].r == rankAce, u); ok {
			return a, "pair of " + h.cards[0].r.label() + "s" + vs
		}
	}

	total, soft := h.cards.value()
	if soft {
		return softPlay(total, u, canDouble), "soft " + strconv.Itoa(total) + vs
	}
	return hardPlay(total, u, canDouble, canSurrender), "hard " + strconv.Itoa(total) + vs
}

// pairPlay is the split table, keyed by the pair's point value (aces flagged
// apart from other 11-point math). ok=false hands the pair back to the total
// tables (5,5 plays as hard 10; 10s and off-chart pairs stand/hit by total).
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

// hardPlay is the hard-total table, with the late-surrender cells falling back
// to their hit lines once the surrender window has passed.
func hardPlay(total, u int, canDouble, canSurrender bool) string {
	switch {
	case total >= 17:
		return sayStand
	case total == 16:
		if u >= 9 && canSurrender {
			return saySurrender
		}
		if u <= 6 {
			return sayStand
		}
		return sayHit
	case total == 15:
		if u == 10 && canSurrender {
			return saySurrender
		}
		if u <= 6 {
			return sayStand
		}
		return sayHit
	case total >= 13: // 13-14
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

// upLabel names the dealer up card the way the chart reads: A, or the point
// value (every face reads 10).
func upLabel(up card) string {
	if up.r == rankAce {
		return "A"
	}
	return strconv.Itoa(up.r.points())
}
