package main

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
// is live.
func dealerPlay(d hand, s *shoe) hand {
	for {
		if total, _ := d.value(); total >= 17 {
			return d
		}
		d = append(d, s.draw())
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

// creditFor is the amount returned to a player's chips at settlement for a hand
// of the given bet. The stake was deducted at deal time, so a loss credits 0, a
// push returns the stake, a win pays even money, and a blackjack pays 3:2.
func creditFor(o outcome, bet int) int {
	switch o {
	case outWin:
		return 2 * bet
	case outBlackjack:
		return bet + bet*3/2
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
