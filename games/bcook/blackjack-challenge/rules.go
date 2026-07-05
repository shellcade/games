package main

import "math/rand"

// The Blackjack Challenge rules layer: every payout table, comparison, and
// rule gate of the variant lives here (spec: docs/superpowers/specs/
// 2026-07-04-blackjack-challenge-design.md).

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

// starPairsOutcome classifies a seat's first two cards for the Star Pairs
// side bet, returning the result kind and its payout multiplier (the X in
// X:1), or ("", 0) when the cards are not a same-rank pair. Highest event
// only: a pair of aces (30:1) outranks the suit tiers; then a perfect pair
// (same rank and suit, 20:1), a coloured pair (same rank and colour, 8:1),
// and a mixed pair (same rank, 5:1). Ten-value cards pair by RANK: 10+J is
// not a pair.
func starPairsOutcome(a, b card) (kind string, mult int) {
	switch {
	case a.r != b.r:
		return "", 0
	case a.r == rankAce:
		return "aces", 30
	case a.s == b.s:
		return "perfect", 20
	case a.s.red() == b.s.red():
		return "colored", 8
	default:
		return "mixed", 5
	}
}

// bjTen returns the ten-value card's rank of a two-card blackjack, or 0 when
// the hand is not one. Ranks compare directly for the ranked payout
// (10 < J(11) < Q(12) < K(13)).
func bjTen(h hand) rank {
	if !h.isBlackjack() {
		return 0
	}
	if h[0].r == rankAce {
		return h[1].r
	}
	return h[0].r
}

// blackjackMult is the X in the X:1 blackjack payout: 2 against a dealer
// without blackjack; against a dealer blackjack, by the rank of the player's
// ten-card vs the dealer's (K>Q>J>10) — higher 5, equal 4, lower 3. A player
// blackjack NEVER loses on this table.
func blackjackMult(playerTen, dealerTen rank, dealerBJ bool) int {
	if !dealerBJ {
		return 2
	}
	switch {
	case playerTen > dealerTen:
		return 5
	case playerTen == dealerTen:
		return 4
	default:
		return 3
	}
}

// grossMult is the stake-inclusive settlement multiplier for a played hand
// against the dealer's final hand: credit = grossMult × bet. Challenge
// order: busts lose outright; an auto-win (Player 21 / Five Card Trick) pays
// even money no matter what the dealer holds; a blackjack pays its ranked
// odds; any other hand loses to a dealer blackjack (the seat-level clawback
// caps what the house collects — see settle); a dealer bust pays even money;
// otherwise beat the dealer's total outright or lose — TIES LOSE.
func grossMult(h *phand, d hand, dBJ bool) int {
	switch {
	case h.cards.isBust():
		return 0
	case h.autoWon:
		return 2
	case h.cards.isBlackjack():
		return 1 + blackjackMult(bjTen(h.cards), bjTen(d), dBJ)
	case dBJ:
		return 0
	case d.isBust():
		return 2
	case h.cards.total() > d.total():
		return 2
	default:
		return 0
	}
}

// pairsCreditFor is the chips returned to a player on a Star Pairs side bet of
// `bet` at multiplier `mult` (the X in X:1). The stake was deducted when the bet
// was placed, so a winning pair returns stake + mult×stake and a loss (mult 0)
// returns nothing.
func pairsCreditFor(mult, bet int) int {
	if mult <= 0 {
		return 0
	}
	return bet * (mult + 1)
}
