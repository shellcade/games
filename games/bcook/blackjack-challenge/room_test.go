package main

import (
	"strings"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

func mkPlayer(h string) kit.Player {
	return kit.Player{AccountID: h, Handle: h, Kind: kit.KindMember, Conn: "conn-" + h}
}

// newGame returns a started room driven by an in-memory kittest.Room. The
// credits double is set to clamp at the game's declared MaxPayoutMultiplier so
// the settlement ceiling is exercised exactly as the host would apply it.
func newGame(t *testing.T, players ...kit.Player) (*room, *kittest.Room) {
	t.Helper()
	tr := kittest.NewRoom(players...)
	tr.CreditsMaxPayoutMultiplier = maxPayoutMult
	rm := newRoom(tr.Config(), tr.Services())
	rm.OnStart(tr)
	return rm, tr
}

// fund sets the seat's account-wide credits balance in the double AND the seat's
// cached balance to n, so a subsequent Wager (deal/double/split) draws against a
// known bankroll.
func fund(tr *kittest.Room, s *seat, n int) {
	if tr.Credits == nil {
		tr.Credits = map[string]int64{}
	}
	tr.Credits[s.p.AccountID] = int64(n)
	s.bal = n
}

// staked models a seat that has already opened its stake this round: `bal` is the
// bankroll left after `stake` credits were escrowed. It seeds both the double
// (balance + open stake) and the seat's cache (bal + roundStake) so a direct
// settle() call reproduces a post-deal position without replaying the deal.
func staked(tr *kittest.Room, s *seat, bal, stake int) {
	if tr.Credits == nil {
		tr.Credits = map[string]int64{}
	}
	if tr.CreditsStakes == nil {
		tr.CreditsStakes = map[string]int64{}
	}
	tr.Credits[s.p.AccountID] = int64(bal)
	tr.CreditsStakes[s.p.AccountID] = int64(stake)
	s.bal = bal
	s.roundStake = int64(stake)
}

// pump advances the virtual clock by d in heartbeat-sized steps, waking on each,
// so deadlines land exactly as they would under the host heartbeat.
func pump(rm *room, tr *kittest.Room, d time.Duration) {
	const beat = 50 * time.Millisecond
	for elapsed := time.Duration(0); elapsed < d; elapsed += beat {
		tr.Advance(beat)
		rm.OnWake(tr)
	}
}

func runeInput(r rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: r} }

func keyInput(k kit.Key) kit.Input { return kit.Input{Kind: kit.InputKey, Key: k} }

func TestPairsSideBetLoopsOnP(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	if s.pairsBet != 0 {
		t.Fatalf("pairs side bet defaults to %d, want 0 (off)", s.pairsBet)
	}
	s.bal = 100000                    // deep enough to afford every tier, so the loop wraps only at the top
	rm.OnInput(tr, a, runeInput('p')) // P advances one tier
	if s.pairsBet != pairsTiers[1] {
		t.Fatalf("after P, pairsBet = %d, want %d", s.pairsBet, pairsTiers[1])
	}
	for i := 0; i < len(pairsTiers)-1; i++ { // loop the rest of the way round
		rm.OnInput(tr, a, runeInput('p'))
	}
	if s.pairsBet != 0 {
		t.Fatalf("after a full loop, pairsBet = %d, want reset to 0 at the end", s.pairsBet)
	}
	// B is the behind bet, not pairs — it must not touch your own pairs.
	rm.OnInput(tr, a, runeInput('p')) // -> 10
	rm.OnInput(tr, a, runeInput('b'))
	if s.pairsBet != pairsTiers[1] {
		t.Fatalf("B changed own pairs (B should be behind-only): %d", s.pairsBet)
	}
}

func TestPairsSideBetClampedToChips(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.bet = 100
	s.bal = 105 // can afford the 100 main bet + at most a 5-chip side bet, so only "off"
	for i := 0; i < len(pairsTiers); i++ {
		rm.OnInput(tr, a, runeInput('p'))
	}
	if s.bet+s.pairsBet > s.bal {
		t.Fatalf("main %d + pairs %d exceeds chips %d (clamp failed)", s.bet, s.pairsBet, s.bal)
	}
}

func TestBackFocusCyclesSeatsOnLeftRight(t *testing.T) {
	a, b, c := mkPlayer("a"), mkPlayer("b"), mkPlayer("c")
	rm, tr := newGame(t, a, b, c)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.OnJoin(tr, c)
	sa := rm.seats[a.AccountID]
	if sa.focus != "" {
		t.Fatalf("focus starts %q, want self (empty)", sa.focus)
	}
	rm.OnInput(tr, a, keyInput(kit.KeyRight))
	if sa.focus != b.AccountID {
		t.Fatalf("after Right, focus = %q, want b", sa.focus)
	}
	rm.OnInput(tr, a, keyInput(kit.KeyRight))
	if sa.focus != c.AccountID {
		t.Fatalf("after Right Right, focus = %q, want c", sa.focus)
	}
	rm.OnInput(tr, a, keyInput(kit.KeyRight))
	if sa.focus != "" {
		t.Fatalf("after cycling through all seats, focus = %q, want back to self", sa.focus)
	}
	// Left walks the other way: from self, wrap to the last seat.
	rm.OnInput(tr, a, keyInput(kit.KeyLeft))
	if sa.focus != c.AccountID {
		t.Fatalf("after Left from self, focus = %q, want c (wrap backward)", sa.focus)
	}
}

func TestBackFocusSoloIsNoOp(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	rm.OnInput(tr, a, keyInput(kit.KeyRight))
	if rm.seats[a.AccountID].focus != "" {
		t.Fatal("with no other seats, Left/Right must stay on self")
	}
}

func TestBackBetAdjustsWhenFocused(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.bet = 25
	sa.bal = 1000
	rm.OnInput(tr, a, keyInput(kit.KeyRight)) // focus seat b
	rm.OnInput(tr, a, runeInput('b'))         // behind loops 0 -> 10
	rm.OnInput(tr, a, runeInput('p'))         // their-pairs loops 0 -> 10
	bb := sa.backs[b.AccountID]
	if bb == nil || bb.behind != 10 || bb.pairs != 10 {
		t.Fatalf("back on b = %+v, want behind 10 / pairs 10", bb)
	}
	// The viewer's own bet must be untouched while editing a back.
	if sa.bet != 25 || sa.pairsBet != 0 {
		t.Fatalf("own bet changed while editing a back: bet=%d pairs=%d", sa.bet, sa.pairsBet)
	}
}

func TestBackBetBudgetClamped(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.bet = 100
	sa.bal = 105 // only 5 chips beyond the main bet — no back tier fits
	rm.OnInput(tr, a, keyInput(kit.KeyRight))
	for i := 0; i < len(pairsTiers); i++ {
		rm.OnInput(tr, a, runeInput('b')) // try to raise the behind stake
		rm.OnInput(tr, a, runeInput('p')) // and the their-pairs stake
	}
	committed := sa.bet + sa.pairsBet
	if bb := sa.backs[b.AccountID]; bb != nil {
		committed += bb.behind + bb.pairs
	}
	if committed > sa.bal {
		t.Fatalf("total commitment %d exceeds chips %d (budget clamp failed)", committed, sa.bal)
	}
}

func TestDealResolvesPerfectPairsSideBet(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.bet = 50
	s.placed = true
	s.pairsBet = 10
	fund(tr, s, 1000)
	// Stack the shoe: the dealer's one card, then the seat's two cards — a mixed pair.
	rm.sh.cards = hand{
		{10, suitClub},                 // dealer
		{8, suitSpade}, {8, suitHeart}, // seat: mixed pair of 8s
		{2, suitClub}, {3, suitClub}, {4, suitClub}, // filler draws
	}
	rm.sh.pos = 0
	rm.sh.roundStart = 0
	rm.deal(tr)

	if s.pairsKind != "mixed" {
		t.Fatalf("pairsKind = %q, want mixed", s.pairsKind)
	}
	if s.pairsWin != 70 { // mixed 6:1 on 10 -> 10 + 60
		t.Fatalf("pairsWin = %d, want 70", s.pairsWin)
	}
	// The deal Wagers the main bet (50) + the pairs stake (10) onto the seat's
	// open stake: bankroll 1000 -> 940, roundStake 60. The mixed pair's 70 gross
	// folds into grossThisRound (paid only at the single settle), NOT the balance.
	if s.bal != 940 {
		t.Fatalf("bal = %d, want 940 (bet + pairs Wagered at deal)", s.bal)
	}
	if s.roundStake != 60 {
		t.Fatalf("roundStake = %d, want 60 (main 50 + pairs 10)", s.roundStake)
	}
	if s.grossThisRound != 70 {
		t.Fatalf("grossThisRound = %d, want 70 (mixed pair folded into the open stake)", s.grossThisRound)
	}
}

func TestDealResolvesBackPairs(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa, sb := rm.seats[a.AccountID], rm.seats[b.AccountID]
	sa.bet, sa.placed = 25, true
	sb.bet, sb.placed = 25, true
	fund(tr, sa, 1000)
	fund(tr, sb, 1000)
	sa.backs = map[string]*backBet{b.AccountID: {pairs: 10}} // a backs b's pairs
	// the dealer's one card, then seat a's two cards, then seat b's two cards (a mixed pair).
	rm.sh.cards = hand{
		{10, suitClub},                 // dealer
		{2, suitSpade}, {7, suitHeart}, // seat a (no pair)
		{8, suitSpade}, {8, suitHeart}, // seat b: mixed pair of 8s
		{3, suitClub}, {4, suitClub}, {5, suitClub}, // filler
	}
	rm.sh.pos, rm.sh.roundStart = 0, 0
	rm.deal(tr)

	bb := sa.backs[b.AccountID]
	if bb.pairsKind != "mixed" || bb.pairsWin != 70 {
		t.Fatalf("back-pairs on b = kind %q win %d, want mixed/70", bb.pairsKind, bb.pairsWin)
	}
	// a Wagered main 25 + back-pairs 10 (bal 1000 -> 965, roundStake 35); the 70
	// back-pairs gross folds into a's open stake, paid at settle not now.
	if sa.bal != 965 {
		t.Fatalf("a bal = %d, want 965 (main 25 + back-pairs 10 Wagered)", sa.bal)
	}
	if sa.grossThisRound != 70 {
		t.Fatalf("a grossThisRound = %d, want 70 (back-pairs win folded in)", sa.grossThisRound)
	}
}

func TestDealVoidsBackOnSatOutTarget(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa, sb := rm.seats[a.AccountID], rm.seats[b.AccountID]
	sa.bet, sa.placed = 25, true
	fund(tr, sa, 1000)
	sb.placed = false // b sits this round out
	sa.backs = map[string]*backBet{b.AccountID: {behind: 50, pairs: 10}}
	rm.sh.cards = hand{{10, suitClub}, {2, suitSpade}, {7, suitHeart}, {3, suitClub}, {4, suitClub}}
	rm.sh.pos, rm.sh.roundStart = 0, 0
	rm.deal(tr)

	if sa.bal != 975 { // only a's own 25 bet Wagered; the back on a sat-out seat is voided
		t.Fatalf("a bal = %d, want 975 (back on sat-out target voided, not Wagered)", sa.bal)
	}
	if sa.roundStake != 25 {
		t.Fatalf("a roundStake = %d, want 25 (only the main bet opened)", sa.roundStake)
	}
	if bb := sa.backs[b.AccountID]; bb != nil && (bb.behind != 0 || bb.pairs != 0) {
		t.Fatalf("back on sat-out target not voided: %+v", bb)
	}
}

func TestSettleBehindBetWinFolds(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa, sb := rm.seats[a.AccountID], rm.seats[b.AccountID]
	sa.placed, sa.bet = true, 25
	staked(tr, sa, 925, 75)                                                     // a's open stake: main 25 + behind 50 escrowed
	sa.hands = []*phand{{cards: hand{{2, suitSpade}, {3, suitHeart}}, bet: 25}} // a: 5, loses
	sb.placed, sb.bet = true, 25
	sb.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 25}} // b: 19, beats dealer
	sa.backs = map[string]*backBet{b.AccountID: {behind: 50}}                    // a backs b's hand
	rm.dealer = hand{{10, suitClub}, {7, suitDiamond}}                           // 17

	rm.settle(tr)

	bb := sa.backs[b.AccountID]
	if bb.behindWin != 100 { // even money: 50 stake + 50
		t.Fatalf("behindWin = %d, want 100 (behind paid even money on b's win)", bb.behindWin)
	}
	// a's own hand loses (gross 0); the behind grosses 100 on a 75 stake: net +25.
	if sa.result != "WIN +25" {
		t.Fatalf("a result = %q, want WIN +25 (behind win folded into net)", sa.result)
	}
}

func TestSettleBehindRefundsWhenTargetLeft(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.placed, sa.bet = true, 25
	staked(tr, sa, 900, 75)                                                      // main 25 + behind 50 escrowed; 900 bankroll left
	sa.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 25}} // 19 vs 19: a tie LOSES on this table
	sa.backs = map[string]*backBet{b.AccountID: {behind: 50}}
	delete(rm.seats, b.AccountID) // b left mid-round
	rm.order = []string{a.AccountID}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // 19

	rm.settle(tr)

	if bb := sa.backs[b.AccountID]; bb.behindWin != 50 {
		t.Fatalf("behindWin = %d, want 50 (behind refunded when target left)", bb.behindWin)
	}
	// own hand loses the tie (gross 0), behind refunded (gross 50): Settle(50) on a
	// 75 stake nets -25 against the 900 bankroll -> 950.
	if sa.bal != 950 {
		t.Fatalf("a bal = %d, want 950 (tie loss + behind refund settled once)", sa.bal)
	}
}

func TestSettleFoldsPairsResultIntoNet(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 50
	s.pairsBet = 10
	staked(tr, s, 940, 60)                                                      // main 50 + pairs 10 escrowed
	s.pairsWin = 70                                                             // a mixed pair already resolved at deal...
	s.grossThisRound = 70                                                       // ...and folded into the open stake's gross
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 50}} // 19
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}                          // 19 vs 19 -> hand LOSES the tie
	rm.settle(tr)
	// Hand loses the tie (gross 0) atop the pairs gross 70 = 70 on a 60 stake: net +10.
	if s.result != "WIN +10" {
		t.Fatalf("result = %q, want WIN +10 (pairs win folded into the round net)", s.result)
	}
}

func TestJoinSeatsPlayer(t *testing.T) {
	p := mkPlayer("alice")
	rm, tr := newGame(t, p)
	rm.OnJoin(tr, p)
	s := rm.seats[p.AccountID]
	// A fresh account seeds the credits double's default balance (1000); the seat
	// caches it and seeds its peak from it.
	if s == nil || s.bal != 1000 || s.highScore != 1000 || s.bet != betTiers[1] {
		t.Fatalf("bad seat after join: %+v", s)
	}
}

func TestBettingDeductsAndSitsOut(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.seats[a.AccountID].bet = 50
	rm.OnInput(tr, a, runeInput(' ')) // a places a bet; b does not

	pump(rm, tr, bettingDur+time.Second) // betting closes -> deal

	if len(rm.seats[a.AccountID].hands) == 0 {
		t.Fatal("a placed a bet but was not dealt a hand")
	}
	if rm.seats[a.AccountID].bal != 1000-50 {
		t.Errorf("a bal = %d, want %d (bet Wagered at deal)", rm.seats[a.AccountID].bal, 1000-50)
	}
	if len(rm.seats[b.AccountID].hands) != 0 {
		t.Error("b placed no bet but was dealt in")
	}
}

func TestEmptyBettingReopensWithoutDealing(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a) // nobody places a bet

	pump(rm, tr, bettingDur+time.Second)

	if rm.phase != phBetting {
		t.Errorf("phase = %q, want betting (no bet should not deal)", rm.phase)
	}
	if len(rm.dealer) != 0 {
		t.Error("no cards should have been dealt")
	}
}

func TestNoWinnerLifecycle(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	rm.OnInput(tr, a, runeInput(' ')) // place a bet
	pump(rm, tr, 150*time.Second)     // a full round via auto-stands, loop back to betting

	if tr.Ended != nil {
		t.Error("a no-winner table must never settle via End()")
	}
	rm.OnLeave(tr, a)
	if tr.Ended != nil {
		t.Error("leaving must not settle a ranked result")
	}
}

// turnsSetup joins two players, both placed with the given hands, and puts the
// table into the player-turns phase with the turn pointer at the first seat.
func turnsSetup(t *testing.T, ah, bh hand) (*room, *kittest.Room, kit.Player, kit.Player) {
	t.Helper()
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.what = pendNone // drop the pending betting one-shot; we drive turns directly
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	rm.seats[a.AccountID].placed = true
	rm.seats[b.AccountID].placed = true
	rm.seats[a.AccountID].hands = []*phand{{cards: ah, bet: 50}}
	rm.seats[b.AccountID].hands = []*phand{{cards: bh, bet: 50}}
	rm.dealer = hand{{10, suitSpade}, {7, suitHeart}}
	rm.phase = phTurns
	return rm, tr, a, b
}

func TestPublishesContextPerPhase(t *testing.T) {
	// Betting is a navigation screen.
	_, tr := newGame(t, mkPlayer("a"))
	if tr.InputCtx != kit.CtxNav {
		t.Errorf("betting InputCtx = %v, want CtxNav", tr.InputCtx)
	}
	// Player turns bind h/s/d/p/r as domain commands.
	rm, tr2, _, _ := turnsSetup(t, hand{{10, suitSpade}, {5, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	rm.beginTurn(tr2)
	if tr2.InputCtx != kit.CtxCommand {
		t.Errorf("turns InputCtx = %v, want CtxCommand", tr2.InputCtx)
	}
}

func TestOnlyActiveSeatActs(t *testing.T) {
	rm, tr, _, b := turnsSetup(t, hand{{10, suitSpade}, {5, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	rm.OnInput(tr, b, runeInput('s')) // b is not on turn
	if rm.seats[b.AccountID].hands[0].resolved {
		t.Error("a non-active seat must not be able to act")
	}
}

func TestDoubleDrawsOneCardAndResolves(t *testing.T) {
	rm, tr, a, _ := turnsSetup(t, hand{{5, suitSpade}, {6, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	fund(tr, rm.seats[a.AccountID], 950)
	rm.OnInput(tr, a, runeInput('d'))

	h := rm.seats[a.AccountID].hands[0]
	if rm.seats[a.AccountID].bal != 900 {
		t.Errorf("bal = %d, want 900 (double Wagered the second bet)", rm.seats[a.AccountID].bal)
	}
	if h.bet != 100 || len(h.cards) != 3 || !h.resolved {
		t.Errorf("after double: bet=%d cards=%d resolved=%v, want 100/3/true", h.bet, len(h.cards), h.resolved)
	}
}

func TestSplitFormsTwoHands(t *testing.T) {
	rm, tr, a, _ := turnsSetup(t, hand{{8, suitSpade}, {8, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	fund(tr, rm.seats[a.AccountID], 950)
	rm.OnInput(tr, a, runeInput('p'))

	s := rm.seats[a.AccountID]
	if len(s.hands) != 2 {
		t.Fatalf("after split: %d hands, want 2", len(s.hands))
	}
	if s.bal != 900 {
		t.Errorf("bal = %d, want 900 (split Wagered the second bet)", s.bal)
	}
	for i, h := range s.hands {
		if len(h.cards) != 2 {
			t.Errorf("split hand %d has %d cards, want 2", i, len(h.cards))
		}
	}
}

func TestHitTo21AutoWins(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {6, suitHeart}}, bet: 100}}
	rm.sh.cards[rm.sh.pos] = card{5, suitClub} // next draw: 16 + 5 = 21
	rm.act(tr, a, 'h')
	h := s.hands[0]
	if !h.autoWon || !h.resolved {
		t.Errorf("hit to 21: autoWon=%v resolved=%v, want true/true (Player 21)", h.autoWon, h.resolved)
	}
}

func TestFiveCardTrickAutoWins(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{2, suitSpade}, {3, suitHeart}, {4, suitClub}, {5, suitDiamond}}, bet: 100}}
	rm.sh.cards[rm.sh.pos] = card{2, suitClub} // fifth card, total 16 <= 21
	rm.act(tr, a, 'h')
	h := s.hands[0]
	if !h.autoWon || !h.resolved {
		t.Errorf("five cards under 21: autoWon=%v resolved=%v, want true/true", h.autoWon, h.resolved)
	}
}

func TestDoubleAllowedOnThreeCards(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{2, suitSpade}, {3, suitHeart}, {4, suitClub}}, bet: 100}}
	rm.sh.cards[rm.sh.pos] = card{10, suitClub} // doubled draw: 9 + 10 = 19
	rm.act(tr, a, 'd')
	h := s.hands[0]
	if !h.doubled || h.bet != 200 || len(h.cards) != 4 || !h.resolved {
		t.Errorf("3-card double: doubled=%v bet=%d cards=%d resolved=%v", h.doubled, h.bet, len(h.cards), h.resolved)
	}
}

func TestSplitOnEqualPointValue(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{rankKing, suitSpade}, {10, suitHeart}}, bet: 100}} // K+10: same points, different rank
	rm.act(tr, a, 'p')
	if len(s.hands) != 2 {
		t.Fatalf("K+10 should split on this table: hands = %d, want 2", len(s.hands))
	}
}

func TestResplitCapsAtThreeHands(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 10000)
	rm.phase = phTurns
	s.hands = []*phand{
		{cards: hand{{8, suitSpade}, {8, suitHeart}}, bet: 100},
		{cards: hand{{9, suitSpade}, {9, suitHeart}}, bet: 100},
		{cards: hand{{7, suitSpade}, {7, suitHeart}}, bet: 100},
	}
	if rm.split(tr, s, s.hands[0]) {
		t.Error("fourth hand formed: split must cap at 3 hands per seat")
	}
}

func TestSplitAcesPlayOn(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{rankAce, suitSpade}, {rankAce, suitHeart}}, bet: 100}}
	rm.sh.cards[rm.sh.pos] = card{5, suitClub}   // first split hand's draw: A+5, playable
	rm.sh.cards[rm.sh.pos+1] = card{7, suitClub} // second: A+7, playable
	rm.act(tr, a, 'p')
	for i, h := range s.hands {
		if h.resolved {
			t.Errorf("split-ace hand %d frozen; Challenge split hands play on", i)
		}
	}
}

func TestSplitHandBlackjackResolves(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{rankAce, suitSpade}, {rankAce, suitHeart}}, bet: 100}}
	// Split draw order: the NEW hand (inserted after the original, at hands[1])
	// draws first; the original hand (hands[0]) draws second. Stuff accordingly
	// so hands[0] becomes the A+K blackjack and hands[1] the playable A+5.
	rm.sh.cards[rm.sh.pos] = card{5, suitClub}          // first draw -> new hand (hands[1]): A+5
	rm.sh.cards[rm.sh.pos+1] = card{rankKing, suitClub} // second draw -> original hand (hands[0]): A+K IS blackjack here
	rm.act(tr, a, 'p')
	if !s.hands[0].resolved || !s.hands[0].cards.isBlackjack() {
		t.Errorf("split A+K should resolve as blackjack: resolved=%v", s.hands[0].resolved)
	}
	if s.hands[1].resolved {
		t.Error("A+5 split hand should still be playable")
	}
}

func TestSurrenderKeyIgnored(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	fund(tr, s, 900)
	rm.phase = phTurns
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {6, suitHeart}}, bet: 100}}
	rm.OnInput(tr, a, runeInput('r'))
	if s.hands[0].resolved {
		t.Error("no surrender on this table: R must be a no-op on a turn")
	}
}

func TestTurnTimeoutAutoStands(t *testing.T) {
	rm, tr, a, _ := turnsSetup(t, hand{{10, suitSpade}, {5, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	rm.beginTurn(tr) // arm the per-turn deadline

	pump(rm, tr, turnDur+time.Second)

	if !rm.seats[a.AccountID].hands[0].resolved {
		t.Error("a timed-out turn should auto-stand the active hand")
	}
}

func TestBustRebuysAndKeepsHighScore(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	staked(tr, s, 0, 50) // bet 50 already escrowed; broke bankroll; this hand loses
	s.highScore = 2500
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {10, suitHeart}, {5, suitClub}}, bet: 50}} // 25, bust
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}

	rm.settle(tr)

	// The losing stake Settles for 0, leaving the seat broke, so the platform
	// broke-relief Buyback tops the balance up to the double's rebuy amount (1000).
	if s.bal != 1000 {
		t.Errorf("bal = %d, want re-buy to 1000", s.bal)
	}
	if s.highScore != 2500 {
		t.Errorf("highScore = %d, want 2500 (a bust must not lower it)", s.highScore)
	}
}

func TestRebuyWhenBelowMinimumBet(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	staked(tr, s, 5, 5) // 5 left, above zero but under the 10-credit minimum bet
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {8, suitHeart}}, bet: 5}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // dealer 19 beats 18

	rm.settle(tr)

	// After the loss settles, 5 credits is under the minimum bet, so Buyback tops
	// the balance up to the double's rebuy amount (1000) rather than soft-locking.
	if s.bal != 1000 {
		t.Errorf("bal = %d, want re-buy to 1000 (a stack under the minimum bet must re-buy)", s.bal)
	}
}

func TestBehindBetsAreStickyAcrossRounds(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.bal = 1000
	sa.backs = map[string]*backBet{b.AccountID: {behind: 50, pairs: 10}}

	rm.enterBetting(tr) // a fresh betting window must carry a's back on b

	back := sa.backs[b.AccountID]
	if back == nil {
		t.Fatalf("back on b was dropped; behind bets should be sticky")
	}
	if back.behind != 50 || back.pairs != 10 {
		t.Fatalf("carried back = behind %d pairs %d, want behind 50 pairs 10", back.behind, back.pairs)
	}
}

func TestStickyBackPrunedWhenTargetLeaves(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.bal = 1000
	sa.backs = map[string]*backBet{b.AccountID: {behind: 50}}

	rm.OnLeave(tr, b)   // b leaves the table
	rm.enterBetting(tr) // a's carried back on the now-absent b must be pruned

	if _, ok := sa.backs[b.AccountID]; ok {
		t.Fatalf("a back on a departed target should be pruned, not carried")
	}
}

func TestDoubledHandTagged(t *testing.T) {
	doubled := &phand{cards: hand{{5, suitSpade}, {6, suitHeart}, {10, suitClub}}, bet: 100, doubled: true}
	if got := dblTag(doubled); got != " DBL" {
		t.Errorf("dblTag(doubled) = %q, want %q", got, " DBL")
	}
	plain := &phand{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 50}
	if got := dblTag(plain); got != "" {
		t.Errorf("dblTag(plain) = %q, want empty", got)
	}
}

func TestBlackjackRankedPayouts(t *testing.T) {
	cases := []struct {
		name   string
		player hand
		dealer hand
		want   int // final balance from 900 after settling a 100 stake
	}{
		{"K beats dealer J: 5:1", hand{{rankAce, suitSpade}, {rankKing, suitHeart}}, hand{{rankAce, suitClub}, {rankJack, suitDiamond}}, 1500},
		{"same rank: 4:1", hand{{rankAce, suitSpade}, {rankJack, suitHeart}}, hand{{rankAce, suitClub}, {rankJack, suitDiamond}}, 1400},
		{"10 loses rank to J: 3:1", hand{{rankAce, suitSpade}, {10, suitHeart}}, hand{{rankAce, suitClub}, {rankJack, suitDiamond}}, 1300},
		{"no dealer blackjack: 2:1", hand{{rankAce, suitSpade}, {rankKing, suitHeart}}, hand{{10, suitClub}, {9, suitDiamond}}, 1200},
	}
	for _, c := range cases {
		a := mkPlayer("a")
		rm, tr := newGame(t, a)
		rm.OnJoin(tr, a)
		s := rm.seats[a.AccountID]
		s.placed = true
		s.bet = 100
		staked(tr, s, 900, 100)
		s.hands = []*phand{{cards: c.player, bet: 100}}
		rm.dealer = c.dealer
		rm.settle(tr)
		if s.bal != c.want {
			t.Errorf("%s: bal = %d, want %d", c.name, s.bal, c.want)
		}
	}
}

func TestTiesLose(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 100
	staked(tr, s, 900, 100)
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 100}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // 19 vs 19
	rm.settle(tr)
	if s.bal != 900 {
		t.Errorf("bal = %d, want 900 (a tie LOSES the stake on this table)", s.bal)
	}
	if s.result != "LOSE -100" {
		t.Errorf("result = %q, want LOSE -100", s.result)
	}
}

func TestAutoWinPaysEvenMoneyRegardlessOfDealer(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 100
	staked(tr, s, 900, 100)
	s.hands = []*phand{{cards: hand{{7, suitSpade}, {7, suitHeart}, {7, suitClub}}, bet: 100, autoWon: true, resolved: true}}
	rm.dealer = hand{{rankAce, suitClub}, {rankJack, suitDiamond}} // even a dealer blackjack
	rm.settle(tr)
	if s.bal != 1100 {
		t.Errorf("bal = %d, want 1100 (Player 21 pays even money vs anything)", s.bal)
	}
}

func TestDealerBlackjackClawback(t *testing.T) {
	// Split into two hands, one doubled: 300 total staked, but a dealer
	// blackjack collects only the ORIGINAL 100 bet — 200 comes back.
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 100
	staked(tr, s, 700, 300)
	s.hands = []*phand{
		{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 200, doubled: true, resolved: true},
		{cards: hand{{10, suitDiamond}, {8, suitHeart}}, bet: 100, resolved: true},
	}
	rm.dealer = hand{{rankAce, suitClub}, {rankJack, suitDiamond}}
	rm.settle(tr)
	if s.bal != 900 {
		t.Errorf("bal = %d, want 900 (700 + 200 clawback refund; net -100)", s.bal)
	}
}

func TestClawbackSkipsBustedHands(t *testing.T) {
	// The busted hand lost before the dealer's blackjack existed: its doubled
	// 200 stake is NOT shielded. Only the live 100 hand faces the dealer BJ,
	// and it alone is within the original-bet collection — no refund due.
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 100
	staked(tr, s, 700, 300)
	s.hands = []*phand{
		{cards: hand{{10, suitSpade}, {9, suitHeart}, {5, suitClub}}, bet: 200, doubled: true, resolved: true}, // bust 24
		{cards: hand{{10, suitDiamond}, {8, suitHeart}}, bet: 100, resolved: true},
	}
	rm.dealer = hand{{rankAce, suitClub}, {rankJack, suitDiamond}}
	rm.settle(tr)
	if s.bal != 700 {
		t.Errorf("bal = %d, want 700 (busted stake stays lost; live loss within original bet)", s.bal)
	}
}

func TestBehindBetRidesTheChallengeOdds(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa, sb := rm.seats[a.AccountID], rm.seats[b.AccountID]
	sa.placed, sb.placed = true, true
	sa.bet, sb.bet = 100, 100
	staked(tr, sa, 900, 100)
	staked(tr, sb, 850, 150) // 100 main + 50 behind on a
	sb.backs = map[string]*backBet{a.AccountID: {behind: 50}}
	sa.hands = []*phand{{cards: hand{{rankAce, suitSpade}, {rankKing, suitHeart}}, bet: 100, resolved: true}}
	sb.hands = []*phand{{cards: hand{{10, suitDiamond}, {9, suitHeart}}, bet: 100, resolved: true}}
	rm.dealer = hand{{10, suitClub}, {8, suitDiamond}} // 18: b wins even, a has blackjack 2:1
	rm.settle(tr)
	// b: main 19 beats 18 -> 200, behind rides a's blackjack at 2:1 -> 150. 850+350 = 1200.
	if sb.bal != 1200 {
		t.Errorf("backer bal = %d, want 1200 (behind pays the ranked blackjack odds)", sb.bal)
	}
}

// TestDealingOrderIsDeterministic asserts the deal ranges the join-ordered slice,
// never Go's map iteration order: two identically-seeded rooms deal identical
// hands to the same seats.
func TestDealingOrderIsDeterministic(t *testing.T) {
	deal := func() (hand, hand, hand) {
		a, b := mkPlayer("a"), mkPlayer("b")
		rm, tr := newGame(t, a, b)
		rm.OnJoin(tr, a)
		rm.OnJoin(tr, b)
		rm.OnInput(tr, a, runeInput(' '))
		rm.OnInput(tr, b, runeInput(' '))
		pump(rm, tr, bettingDur+gracePeriod+time.Second)
		return rm.seats[a.AccountID].hands[0].cards, rm.seats[b.AccountID].hands[0].cards, rm.dealer
	}
	a1, b1, d1 := deal()
	a2, b2, d2 := deal()
	eq := func(x, y hand) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
		return true
	}
	if !eq(a1, a2) || !eq(b1, b2) || !eq(d1, d2) {
		t.Fatalf("same-seed rooms dealt differently:\n a:%v/%v b:%v/%v d:%v/%v", a1, a2, b1, b2, d1, d2)
	}
}

// TestBalanceSeedsAndPostsPeak covers the account-wide credits path: a fresh
// seat caches the default balance, and a winning settle raises the peak
// (sourced from the post-Settle balance) and Posts it to the leaderboard.
func TestBalanceSeedsAndPostsPeak(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)

	// A fresh seat seeds the credits double's default balance into the cache.
	s := rm.seats[a.AccountID]
	if s.bal != 1000 || s.highScore != 1000 {
		t.Fatalf("seeded seat: bal=%d high=%d, want 1000/1000", s.bal, s.highScore)
	}

	// A winning settle raises the peak from the fresh balance and posts it.
	s.placed = true
	staked(tr, s, 1900, 100) // 100 escrowed, hand will gross 200
	s.hands = []*phand{{cards: hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}, bet: 100}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // dealer 19, player 20 wins
	rm.settle(tr)

	if s.bal != 2100 || s.highScore != 2100 {
		t.Fatalf("after win: bal=%d high=%d, want 2100/2100", s.bal, s.highScore)
	}
	if got := tr.Credits[a.AccountID]; got != 2100 {
		t.Errorf("settled account balance = %d, want 2100", got)
	}
	if len(tr.Posted) == 0 {
		t.Fatal("a new peak should Post to the leaderboard")
	}
	last := tr.Posted[len(tr.Posted)-1]
	if len(last.Rankings) != 1 || last.Rankings[0].Metric != 2100 {
		t.Errorf("posted ranking = %+v, want metric 2100", last.Rankings)
	}
}

// TestTopPrizeDoesNotClamp asserts the declared MaxPayoutMultiplier (26) covers
// the game's largest single-stake outcome: a Perfect Pairs "perfect" pair pays
// 25:1, grossing stake×(25+1) = stake×26 — exactly the ceiling, so it settles in
// full and is never clamped.
func TestTopPrizeDoesNotClamp(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	// A lone 100 Perfect Pairs stake hitting a "perfect" pair: gross 2600.
	staked(tr, s, 0, 100)
	s.grossThisRound = 100 * (25 + 1) // 2600, the top pairs payout
	net := rm.settleOpenStake(s)
	if net != 2500 {
		t.Fatalf("net = %d, want 2500 (top pairs prize settles unclamped)", net)
	}
	if got := tr.Credits[a.AccountID]; got != 2600 {
		t.Fatalf("settled balance = %d, want 2600 (full 26x top prize paid)", got)
	}
}

// colIndex returns the COLUMN (rune) index of sub in row, or -1 — unlike
// strings.Index, whose byte offsets drift once a character glyph (e.g. λ)
// occupies a cell to the left.
func colIndex(row, sub string) int {
	rs, ss := []rune(row), []rune(sub)
	for i := 0; i+len(ss) <= len(rs); i++ {
		match := true
		for j := range ss {
			if rs[i+j] != ss[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// TestSeatRendersCharacterTile asserts the arcade character tile (kit v2.9.0)
// lands on the seat rail immediately before the player's name — one styled
// cell plus one space — on the frames BOTH players receive.
func TestSeatRendersCharacterTile(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	b.Character = kit.Character{Glyph: "@", InkR: 1, InkG: 2, InkB: 3, BgR: 4, BgG: 5, BgB: 6, Fallback: '@'}
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	for _, viewer := range []kit.Player{a, b} {
		f := tr.LastFrame(viewer)
		if f == nil {
			t.Fatalf("no frame for %s", viewer.Handle)
		}
		row := kittest.String(f, seatNameRow)
		for _, p := range []kit.Player{a, b} {
			idx := colIndex(row, p.Handle)
			if idx < 2 {
				t.Fatalf("%s's seat name not on row %d: %q", p.Handle, seatNameRow, row)
			}
			want := kit.CharacterCell(p.Character)
			got := f.Cells[seatNameRow][idx-2]
			if got != want {
				t.Errorf("viewer %s: cell before %q = %+v, want character tile %+v", viewer.Handle, p.Handle, got, want)
			}
			if sp := f.Cells[seatNameRow][idx-1].Rune; sp != ' ' && sp != 0 {
				t.Errorf("viewer %s: no space between %q's tile and name (got %q)", viewer.Handle, p.Handle, sp)
			}
		}
	}
}

// TestWaitLineRendersCharacterTile asserts the turn-wait line carries the
// active player's character tile right before their name.
func TestWaitLineRendersCharacterTile(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	a.Character = kit.Character{Glyph: "λ", InkR: 9, Fallback: 'L'}
	rm, tr, _, _ := func() (*room, *kittest.Room, kit.Player, kit.Player) {
		rm, tr := newGame(t, a, b)
		rm.what = pendNone
		rm.OnJoin(tr, a)
		rm.OnJoin(tr, b)
		rm.seats[a.AccountID].placed = true
		rm.seats[b.AccountID].placed = true
		rm.seats[a.AccountID].hands = []*phand{{cards: hand{{10, suitSpade}, {5, suitHeart}}, bet: 50}}
		rm.seats[b.AccountID].hands = []*phand{{cards: hand{{10, suitClub}, {6, suitDiamond}}, bet: 50}}
		rm.dealer = hand{{10, suitSpade}, {7, suitHeart}}
		rm.phase = phTurns
		return rm, tr, a, b
	}()
	rm.render(tr) // a is first unresolved: b's frame shows "waiting on alice..."

	f := tr.LastFrame(b)
	row := kittest.String(f, actionRow)
	idx := colIndex(row, "waiting on")
	if idx < 0 {
		t.Fatalf("no wait line on row %d: %q", actionRow, row)
	}
	nameIdx := colIndex(row, a.Handle)
	if nameIdx < 0 {
		t.Fatalf("active player's name missing from wait line: %q", row)
	}
	if got, want := f.Cells[actionRow][nameIdx-2], kit.CharacterCell(a.Character); got != want {
		t.Errorf("cell before active name = %+v, want character tile %+v", got, want)
	}
}

// TestHibernationStableDealReplays asserts the deal is reconstructable from guest
// memory + the room clock: a deal recorded, then re-composed after the schedule
// settles, draws every card settled (no RNG re-consult).
func TestHibernationStableDealReplays(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	rm.OnInput(tr, a, runeInput(' '))
	pump(rm, tr, bettingDur+gracePeriod+time.Second)
	dealt := append(hand(nil), rm.seats[a.AccountID].hands[0].cards...)

	// Let any animation schedule fully settle, then the dealt cards are fixed.
	pump(rm, tr, 5*time.Second)
	after := rm.seats[a.AccountID].hands[0].cards
	if len(after) != len(dealt) {
		t.Fatalf("card count changed across waking: %d -> %d", len(dealt), len(after))
	}
	for i := range dealt {
		if after[i] != dealt[i] {
			t.Fatalf("card %d changed across waking: %v -> %v", i, dealt[i], after[i])
		}
	}
}

// TestDoubledHandRendersDBL confirms a doubled hand's value line carries the DBL
// flag on the frame, so onlookers can see who doubled down.
func TestDoubledHandRendersDBL(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.hands = []*phand{{cards: hand{{5, suitSpade}, {6, suitHeart}, {10, suitClub}}, bet: 200, doubled: true, resolved: true}} // 21 after doubling
	rm.phase = phTurns
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}

	rm.render(tr)

	if row := kittest.String(tr.LastFrame(a), seatValRow); !strings.Contains(row, "DBL") {
		t.Fatalf("doubled hand value row = %q, want it to contain DBL", row)
	}
}

// dealerReady puts the table at the moment the last player has resolved, with a
// live (non-bust) player so the dealer will draw out. The dealer's single dealt
// card and the shoe's next draws are caller-supplied so the draw-out is
// deterministic.
func dealerReady(t *testing.T, dealer hand, nextDraws ...card) (*room, *kittest.Room, kit.Player) {
	t.Helper()
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.what = pendNone // drop the betting one-shot; we drive the dealer directly
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 50, resolved: true}} // 19, live
	rm.phase = phTurns
	rm.dealer = dealer
	if len(nextDraws) > 0 {
		rm.sh.cards = append(hand(nil), nextDraws...) // stack the shoe's top
		rm.sh.pos = 0
		rm.sh.roundStart = 0
	}
	return rm, tr, a
}

// TestDealerBustWaitsForTheCardToLand is the heart of the reveal-UX fix: the
// dealer's BUST verdict must not appear until the busting card has animated in.
// Before the fix the label keyed off the authoritative (already complete) hand,
// so BUST flashed up the instant the dealer's turn began.
func TestDealerBustWaitsForTheCardToLand(t *testing.T) {
	// Dealer starts on 10, draws a 6 (16, keeps drawing) then a ten -> 26, a
	// deterministic bust on the second draw.
	rm, tr, a := dealerReady(t, hand{{10, suitClub}}, card{6, suitDiamond}, card{10, suitClub})

	rm.enterDealer(tr) // schedules the draw-out, including the busting hit
	rm.render(tr)

	if !rm.dealingActive() {
		t.Fatal("dealer draw-out should still be animating right after enterDealer")
	}
	if row := kittest.String(tr.LastFrame(a), dealerValRow); strings.Contains(row, "BUST") {
		t.Fatalf("dealer BUST shown before the hit landed: %q", row)
	}

	// Once the whole draw-out has played out the bust shows and the round settles
	// (stay inside the results window, before the next betting round clears it).
	pump(rm, tr, 5*time.Second)
	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results once the draw-out finished", rm.phase)
	}
	if row := kittest.String(tr.LastFrame(a), dealerValRow); !strings.Contains(row, "BUST") {
		t.Fatalf("dealer BUST not shown after the hit landed: %q", row)
	}
}

// TestDealerRevealIsPaced asserts the draw-out is unhurried: settlement is
// deferred by the full per-card cadence, not just the lead-in beat. This
// scenario stages exactly one draw, so the settle deadline must cover the
// lead-in, that card's slide and flip, and the done-hold on the made hand.
func TestDealerRevealIsPaced(t *testing.T) {
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}}, card{8, suitDiamond}) // draws to 18, stands

	start := tr.Now()
	rm.enterDealer(tr)

	if rm.what != pendSettle {
		t.Fatalf("dealer draw-out should defer settlement, what = %v", rm.what)
	}
	// One drawn card at minimum: the lead-in beat, then the card slides in and
	// flips face up, then the final hold before results (dealerDrawGap only
	// separates consecutive draws, so it does not appear on a single draw).
	minDelay := dealerLeadIn + slideDur + flipDur + dealerDoneHold
	if delay := rm.pendAt.Sub(start); delay < minDelay {
		t.Fatalf("settle deferred only %v, want at least the one-draw cadence %v", delay, minDelay)
	}
}

// TestDealerTotalTicksUpAsCardsLand checks the displayed total reflects only the
// face-up cards: just the one dealt card before the draw-out, then each drawn
// card in step with the animation rather than the full hand up front.
func TestDealerTotalTicksUpAsCardsLand(t *testing.T) {
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}}, card{7, suitDiamond}) // draws to 17, stands

	// Before the dealer's turn, only the one dealt card counts.
	if got := rm.dealerShownCount(); got != 1 {
		t.Fatalf("before the draw-out, shown count = %d, want 1", got)
	}

	rm.enterDealer(tr) // schedules the draw; still mid lead-in
	rm.render(tr)
	if got := rm.dealerShownCount(); got != 1 {
		t.Fatalf("mid lead-in, shown count = %d, want 1 (drawn card not yet arrived)", got)
	}

	// After the draw-out settles (still within the results window) both cards
	// count and the total is complete.
	pump(rm, tr, 3*time.Second)
	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results once the draw-out finished", rm.phase)
	}
	if got := rm.dealerShownCount(); got != 2 {
		t.Fatalf("after the draw-out, shown count = %d, want 2 (both cards face up)", got)
	}
	if got := rm.dealer.total(); got != 17 {
		t.Fatalf("dealer total = %d, want 17", got)
	}
}

// TestResultLabelDoesNotLeakChips guards the results chip line: the settlement
// summary is drawn instead of the stack, so a result narrower than the stack
// (e.g. "PUSH" over "$1000") leaves no stray digit peeking out beside it.
func TestResultLabelDoesNotLeakChips(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bal = 1000 // "$1000" is wider than "PUSH"
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 50}}
	s.result = "PUSH"
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}
	rm.phase = phResults
	rm.render(tr)

	row := []rune(kittest.String(tr.LastFrame(a), seatChipRow))
	slot := (kit.Cols - slotW) / 2 // single seat: the group is one centred slot
	if got := strings.TrimSpace(string(row[slot : slot+slotW])); got != "PUSH" {
		t.Fatalf("chip-row slot = %q, want exactly %q (no chips bleeding through)", got, "PUSH")
	}
}

// TestReadyUpSkipsTheResultsWait covers the results-phase ready-up: a single
// seated player confirming (Enter/Space) starts the next betting round at once
// rather than waiting out the full results flash.
func TestReadyUpSkipsTheResultsWait(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	staked(tr, s, 950, 50)
	s.hands = []*phand{{cards: hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}, bet: 50}} // 20
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}                                        // 19, player wins
	rm.settle(tr)

	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results after settle", rm.phase)
	}

	// Confirm readies up; the only seated player being ready deals the next hand.
	rm.OnInput(tr, a, runeInput(' '))
	if rm.phase != phBetting {
		t.Fatalf("phase = %q, want betting (an all-ready table skips the wait)", rm.phase)
	}
}

// cellCol returns the column of the first cell on row matching want, or -1.
func cellCol(f *kit.Frame, row int, want kit.Cell) int {
	for c := range f.Cells[row] {
		if f.Cells[row][c] == want {
			return c
		}
	}
	return -1
}

// TestBackersLineShowsBackerTile asserts that a seat being backed shows, on its
// dedicated backers line, the backing player's character tile and stake — so you
// can see who is backing whom.
func TestBackersLineShowsBackerTile(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, Fallback: 'L'}
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.placed = true
	sa.backs = map[string]*backBet{b.AccountID: {behind: 25}}
	rm.render(tr)
	f := tr.LastFrame(b)

	if cellCol(f, seatBackRow, kit.CharacterCell(a.Character)) < 0 {
		t.Fatalf("backer alice's character tile not on the backers line (row %d): %q",
			seatBackRow, kittest.String(f, seatBackRow))
	}
	if row := kittest.String(f, seatBackRow); !strings.Contains(row, "25") {
		t.Fatalf("backers line does not show the behind stake: %q", row)
	}
}

// TestFocusedTargetShowsBackDetail asserts that while a viewer is focused on a
// seat they are backing, the action bar spells out their behind/their-pairs
// stakes on that seat.
func TestFocusedTargetShowsBackDetail(t *testing.T) {
	a, b := mkPlayer("alice"), mkPlayer("bob")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	sa := rm.seats[a.AccountID]
	sa.focus = b.AccountID
	sa.backs = map[string]*backBet{b.AccountID: {behind: 25, pairs: 10}}
	rm.render(tr)

	row := kittest.String(tr.LastFrame(a), actionRow)
	if !strings.Contains(row, "BACKING") || !strings.Contains(row, "bob") {
		t.Fatalf("focused action bar does not name the backed seat: %q", row)
	}
	if !strings.Contains(row, "25") || !strings.Contains(row, "10") {
		t.Fatalf("focused action bar does not show behind/their-pairs stakes: %q", row)
	}
}

// TestBettingShowsPairsSideBet asserts a seat's selected Perfect Pairs side
// stake is shown during betting directly beneath that seat's main bet — so the
// two lines form one contiguous per-seat block and it's unambiguous whose side
// bet is whose at a multi-seat table.
func TestBettingShowsPairsSideBet(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.bet = 50
	s.pairsBet = 25
	rm.render(tr)
	f := tr.LastFrame(a)

	betRow := kittest.String(f, seatCardRow+1)  // where the "bet N" status sits
	pairRow := kittest.String(f, seatCardRow+2) // pairs must sit immediately below it
	if !strings.Contains(betRow, "bet 50") {
		t.Fatalf("expected the bet status on row %d: %q", seatCardRow+1, betRow)
	}
	if !strings.Contains(pairRow, "+pairs 25") {
		t.Fatalf("expected the pairs side bet directly below the bet on row %d: %q", seatCardRow+2, pairRow)
	}
	// The two lines must align under the same seat slot.
	if colIndex(betRow, "bet 50") < 0 || colIndex(pairRow, "+pairs 25") < 0 {
		t.Fatalf("bet and pairs lines not aligned in the seat slot:\n%q\n%q", betRow, pairRow)
	}
}

// TestPairsLineCarriesCharacterTile asserts the Perfect Pairs side-bet line is
// prefixed with the placing player's arcade character tile, so whose side bet is
// whose reads from the face beside it, not just the column.
func TestPairsLineCarriesCharacterTile(t *testing.T) {
	a := mkPlayer("alice")
	a.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, Fallback: 'L'}
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	rm.seats[a.AccountID].pairsBet = 25
	rm.render(tr)
	f := tr.LastFrame(a)

	row := kittest.String(f, seatCardRow+2)
	idx := colIndex(row, "+pairs 25")
	if idx < 2 {
		t.Fatalf("pairs line not found (or no room for a tile) on row %d: %q", seatCardRow+2, row)
	}
	if got, want := f.Cells[seatCardRow+2][idx-2], kit.CharacterCell(a.Character); got != want {
		t.Errorf("cell before the pairs bet = %+v, want the character tile %+v", got, want)
	}
	if sp := f.Cells[seatCardRow+2][idx-1].Rune; sp != ' ' && sp != 0 {
		t.Errorf("no space between the character tile and the pairs bet (got %q)", sp)
	}
}

// TestResultsShowsPerfectPairsWin asserts a winning Perfect Pairs side bet is
// surfaced on the seat with its category and multiplier during results.
func TestResultsShowsPerfectPairsWin(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 50
	staked(tr, s, 1000, 75) // main 50 + pairs 25 escrowed
	s.pairsBet = 25
	s.pairsKind = "colored"
	s.pairsWin = 325
	s.grossThisRound = 325 // the colored pair already folded into the open stake
	s.hands = []*phand{{cards: hand{{8, suitHeart}, {8, suitDiamond}}, bet: 50}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}
	rm.settle(tr) // -> results phase
	rm.render(tr)

	row := kittest.String(tr.LastFrame(a), seatPairRow)
	if !strings.Contains(row, "COLORED 12:1") {
		t.Fatalf("results row %d does not show the pairs win: %q", seatPairRow, row)
	}
}

// TestRulesTaglineFlanksTheDealer asserts the rules signage moved out of the
// mid-felt row up to the dealer's label row, split into a left and a right
// label, leaving the old mid-felt tagline row clear.
func TestRulesTaglineFlanksTheDealer(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	rm.render(tr)
	f := tr.LastFrame(a)

	// The title on row 0 is the live render path's (not the economy-off
	// branch's) — it must carry the full variant name.
	if top := kittest.String(f, 0); !strings.Contains(top, "♠♥♦♣ BLACKJACK CHALLENGE") {
		t.Errorf("title on the normal render path is not BLACKJACK CHALLENGE: %q", top)
	}

	dealerLabelRow := kittest.String(f, dealerRow-1)
	if !strings.Contains(dealerLabelRow, "blackjack pays 2:1 - ties lose") {
		t.Errorf("payout rule not on the dealer label row: %q", dealerLabelRow)
	}
	if !strings.Contains(dealerLabelRow, "dealer stands on 17") {
		t.Errorf("dealer rule not on the dealer label row: %q", dealerLabelRow)
	}
	if !strings.Contains(dealerLabelRow, "D E A L E R") {
		t.Errorf("DEALER label should remain centred between the rules: %q", dealerLabelRow)
	}
	// The old mid-felt tagline (row 9) must be clear now.
	if mid := kittest.String(f, 9); strings.Contains(mid, "blackjack pays") {
		t.Errorf("rules tagline still sits mid-felt on row 9: %q", mid)
	}
}

// TestSplitSeatShowsEveryHandsCards is the regression guard for the reported
// bug: a seat split into two hands must render BOTH hands' cards (each on its
// own compact line), not collapse the second hand to a bare "+". The active
// hand is marked so the player can see which one they are acting on.
func TestSplitSeatShowsEveryHandsCards(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bal = 800
	s.hands = []*phand{
		{cards: hand{{8, suitSpade}, {3, suitHeart}}, bet: 50},
		{cards: hand{{8, suitClub}, {10, suitDiamond}}, bet: 50},
	}
	rm.dealer = hand{{10, suitSpade}, {7, suitHeart}}
	rm.phase = phTurns
	rm.render(tr)

	f := tr.LastFrame(a)
	// Gather the seat's content rows into one blob.
	var blob string
	for _, row := range []int{seatCardRow, seatCardRow + 1, seatCardRow + 2, seatCardRow + 3} {
		blob += kittest.String(f, row) + "\n"
	}
	// Both hands' cards must be present — second hand included.
	for _, tok := range []string{"8♠", "3♥", "8♣", "T♦"} {
		if !strings.Contains(blob, tok) {
			t.Fatalf("split seat is missing card %q from its hands:\n%s", tok, blob)
		}
	}
	if strings.Contains(blob, "+") {
		t.Fatalf("split seat collapsed a hand to \"+\" instead of showing its cards:\n%s", blob)
	}
}

// TestReadyUpWaitsOnOtherPlayers asserts one player readying up does not skip
// the wait while another seated player is still not ready.
func TestReadyUpWaitsOnOtherPlayers(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	for _, p := range []kit.Player{a, b} {
		s := rm.seats[p.AccountID]
		s.placed = true
		staked(tr, s, 950, 50)
		s.hands = []*phand{{cards: hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}, bet: 50}}
	}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}
	rm.settle(tr)

	rm.OnInput(tr, a, runeInput(' ')) // only a readies up
	if !rm.seats[a.AccountID].ready {
		t.Fatal("a should be marked ready after confirming")
	}
	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results still (b has not readied)", rm.phase)
	}

	rm.OnInput(tr, b, runeInput(' ')) // now both ready -> next hand
	if rm.phase != phBetting {
		t.Fatalf("phase = %q, want betting once everyone is ready", rm.phase)
	}
}

// TestPairsVerdictHeldUntilCardsLand is the regression guard for the reported
// bug: the Perfect Pairs result must not appear while the seat's second card is
// still animating in. The outcome is fixed at the deal, but revealing it early
// spoils the reveal — so "pairs lost" (or a win) waits for both first cards to
// land face up.
func TestPairsVerdictHeldUntilCardsLand(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 50
	s.bal = 1000
	s.pairsBet = 25
	// A non-pair hand: the side bet loses, and that must stay hidden mid-deal.
	s.hands = []*phand{{cards: hand{{9, suitSpade}, {4, suitHeart}}, bet: 50}}
	s.pairsKind = "" // resolved at the deal: no pair
	rm.dealer = hand{{10, suitClub}, {7, suitDiamond}}
	rm.phase = phTurns
	rm.recordDeal(tr) // stagger the initial deal's slide + flip
	rm.render(tr)

	if row := kittest.String(tr.LastFrame(a), seatPairRow); strings.Contains(row, "pairs lost") {
		t.Fatalf("pairs verdict shown before the second card landed: %q", row)
	}

	pump(rm, tr, 5*time.Second) // let the whole deal settle
	rm.render(tr)
	if row := kittest.String(tr.LastFrame(a), seatPairRow); !strings.Contains(row, "pairs lost") {
		t.Fatalf("pairs verdict not shown after the cards landed: %q", row)
	}
}

// TestManyCardHandStaysInItsSlot is the regression guard for the reported bug: a
// hit-heavy hand of five or more cards has a card box wider than the 15-col seat
// slot, so it used to spill past the slot's right edge into the neighbouring
// seat. It must fall back to the compact one-line rendering and stay in its slot.
func TestManyCardHandStaysInItsSlot(t *testing.T) {
	a := mkPlayer("alice")
	rm, tr := newGame(t, a)
	rm.what = pendNone
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bal = 800
	s.hands = []*phand{{cards: hand{{2, suitSpade}, {2, suitHeart}, {3, suitClub}, {4, suitDiamond}, {5, suitSpade}}, bet: 50}}
	rm.dealer = hand{{10, suitClub}, {7, suitDiamond}}
	rm.phase = phTurns
	rm.render(tr)

	// One centred seat: anything drawn at or past its right edge is an overflow
	// into where the neighbouring seat's slot would sit.
	slot := (kit.Cols - slotW) / 2
	f := tr.LastFrame(a)
	for _, row := range []int{seatCardRow, seatCardRow + 1, seatCardRow + 2} {
		line := []rune(kittest.String(f, row))
		for c := slot + slotW; c < kit.Cols-1; c++ {
			if c < len(line) && line[c] != ' ' && line[c] != 0 {
				t.Fatalf("seat card row %d spills past its slot at col %d: %q", row, c, string(line))
			}
		}
	}
}

// A seat that lands in betting below the minimum stake can re-buy with R rather
// than being soft-locked (Confirm no-ops while broke, and the auto-rebuy only
// fires post-hand). Guards the betting-screen broke-relief affordance.
func TestBrokeSeatRebuysOnRInBetting(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]

	// Broke: below the min bet (betTiers[0]) and the buyback floor.
	fund(tr, s, 5)
	// Confirm cannot place a bet while broke — the seat is stuck without a rebuy.
	rm.OnInput(tr, a, keyInput(kit.KeyEnter))
	if s.placed {
		t.Fatal("broke seat placed a bet it cannot afford")
	}

	// R triggers the broke-relief re-buy, topping the seat back up.
	rm.OnInput(tr, a, runeInput('r'))
	if s.bal != 1000 {
		t.Fatalf("after re-buy bal = %d, want 1000", s.bal)
	}
	if tr.CreditsRebuys[a.AccountID] != 1 {
		t.Fatalf("re-buy count = %d, want 1", tr.CreditsRebuys[a.AccountID])
	}
	// Now solvent: a further R is a no-op (no phantom extra rebuy).
	rm.OnInput(tr, a, runeInput('r'))
	if tr.CreditsRebuys[a.AccountID] != 1 {
		t.Fatalf("solvent R triggered a re-buy: count = %d, want 1", tr.CreditsRebuys[a.AccountID])
	}
	// And the seat can now place the minimum bet.
	rm.OnInput(tr, a, keyInput(kit.KeyEnter))
	if !s.placed {
		t.Fatal("re-bought seat still cannot place a bet")
	}
}
