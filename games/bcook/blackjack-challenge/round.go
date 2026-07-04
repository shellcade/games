package main

import "math/rand"

// outcome is a single player hand's result against the dealer.
type outcome int

const (
	outLose outcome = iota
	outPush
	outWin
	outBlackjack // player blackjack, dealer without blackjack
)

// dealerPlay draws for the dealer until the hand totals 17 or more, standing on
// all 17 including soft 17 (S17). Callers skip this entirely when no player hand
// is live. The rng feeds a possible mid-round discard recycle in draw.
func dealerPlay(d hand, s *shoe, rng *rand.Rand) hand {
	for {
		if total, _ := d.value(); total >= 17 {
			return d
		}
		d = append(d, s.draw(rng))
	}
}

// settleHandEx compares a player hand against the dealer with explicit blackjack
// flags, so a caller can declare a hand non-blackjack (e.g. a two-card 21 formed
// by splitting is a plain 21). A busted player always loses; otherwise blackjack
// beats non-blackjack, then totals are compared, with equal totals a push.
func settleHandEx(p hand, pBlackjack bool, d hand, dBlackjack bool) outcome {
	switch {
	case p.isBust():
		return outLose
	case pBlackjack && dBlackjack:
		return outPush
	case pBlackjack:
		return outBlackjack
	case dBlackjack:
		return outLose
	case d.isBust():
		return outWin
	case p.total() > d.total():
		return outWin
	case p.total() < d.total():
		return outLose
	default:
		return outPush
	}
}

// settleHand compares a player hand against the dealer using natural blackjack
// detection for both.
func settleHand(p, d hand) outcome {
	return settleHandEx(p, p.isBlackjack(), d, d.isBlackjack())
}

// halfUp halves n in integer chips with the half-chip rounded UP — the single
// owner of the player-favorable rounding policy used by the 3:2 blackjack
// payout and the surrender return (so the two sites can never drift apart and
// silently break chip conservation on odd bets).
func halfUp(n int) int { return (n + 1) / 2 }

// creditFor is the amount returned to a player's chips at settlement for a hand
// of the given bet. The stake was deducted at deal time, so a loss credits 0, a
// push returns the stake, a win pays even money, and a blackjack pays 3:2 —
// with an odd bet's half-chip rounded UP (the player-favorable integer-chip
// reading of 3:2; never underpay the headline payout).
func creditFor(o outcome, bet int) int {
	switch o {
	case outWin:
		return 2 * bet
	case outBlackjack:
		return bet + halfUp(bet*3)
	case outPush:
		return bet
	default: // outLose
		return 0
	}
}

// insuranceCredit returns the chips paid back on an insurance side bet of `ins`.
// Insurance pays 2:1 when the dealer has blackjack (stake + 2×stake), else 0.
func insuranceCredit(dealerBlackjack bool, ins int) int {
	if dealerBlackjack {
		return ins * 3
	}
	return 0
}

// perfectPairsOutcome classifies a player's first two cards for the Perfect
// Pairs side bet and returns the result kind and its payout multiplier (the X
// in X:1), or ("", 0) when the cards are not a pair. A perfect pair is the same
// rank and suit (25:1), a colored pair is the same rank and colour but different
// suit (12:1), and a mixed pair is the same rank in different colours (6:1).
func perfectPairsOutcome(a, b card) (kind string, mult int) {
	switch {
	case a.r != b.r:
		return "", 0
	case a.s == b.s:
		return "perfect", 25
	case a.s.red() == b.s.red():
		return "colored", 12
	default:
		return "mixed", 6
	}
}

// pairsCreditFor is the chips returned to a player on a Perfect Pairs side bet of
// `bet` at multiplier `mult` (the X in X:1). The stake was deducted when the bet
// was placed, so a winning pair returns stake + mult×stake and a loss (mult 0)
// returns nothing.
func pairsCreditFor(mult, bet int) int {
	if mult <= 0 {
		return 0
	}
	return bet * (mult + 1)
}
