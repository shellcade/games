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

// newGame returns a started room driven by an in-memory kittest.Room.
func newGame(t *testing.T, players ...kit.Player) (*room, *kittest.Room) {
	t.Helper()
	tr := kittest.NewRoom(players...)
	rm := newRoom(tr.Config(), tr.Services())
	rm.OnStart(tr)
	return rm, tr
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

func TestPairsSideBetAdjustsOnLeftRight(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	if s.pairsBet != 0 {
		t.Fatalf("pairs side bet defaults to %d, want 0 (off)", s.pairsBet)
	}
	rm.OnInput(tr, a, keyInput(kit.KeyRight)) // raise to the first tier
	if s.pairsBet != pairsTiers[1] {
		t.Fatalf("after Right, pairsBet = %d, want %d", s.pairsBet, pairsTiers[1])
	}
	rm.OnInput(tr, a, keyInput(kit.KeyLeft)) // back to off
	if s.pairsBet != 0 {
		t.Fatalf("after Left, pairsBet = %d, want 0 (off)", s.pairsBet)
	}
}

func TestPairsSideBetClampedToChips(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.bet = 100
	s.chips = 105 // can afford the 100 main bet + at most a 5-chip side bet, so only "off"
	for i := 0; i < len(pairsTiers); i++ {
		rm.OnInput(tr, a, keyInput(kit.KeyRight))
	}
	if s.bet+s.pairsBet > s.chips {
		t.Fatalf("main %d + pairs %d exceeds chips %d (clamp failed)", s.bet, s.pairsBet, s.chips)
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
	s.chips = 1000
	// Stack the shoe: dealer up + hole, then the seat's two cards — a mixed pair.
	rm.sh.cards = hand{
		{10, suitClub}, {9, suitDiamond}, // dealer 19
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
	// 1000 - 50 (bet) - 10 (pairs stake) + 70 (mixed payout) = 1010.
	if s.chips != 1010 {
		t.Fatalf("chips = %d, want 1010 (bet + pairs deducted at deal, mixed pair paid 70)", s.chips)
	}
}

func TestSettleFoldsPairsResultIntoNet(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 50
	s.chips = 1000
	s.pairsBet = 10
	s.pairsWin = 70 // a mixed pair already paid at deal
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {9, suitHeart}}, bet: 50}} // 19
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}                          // 19 -> hand pushes
	rm.settle(tr)
	// Hand pushes (net 0); the pairs win folds in: net = (70 - 10) = +60.
	if s.result != "WIN +60" {
		t.Fatalf("result = %q, want WIN +60 (pairs win folded into the round net)", s.result)
	}
}

func TestJoinSeatsPlayer(t *testing.T) {
	p := mkPlayer("alice")
	rm, tr := newGame(t, p)
	rm.OnJoin(tr, p)
	s := rm.seats[p.AccountID]
	if s == nil || s.chips != startChips || s.highScore != startChips || s.bet != betTiers[1] {
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
	if rm.seats[a.AccountID].chips != startChips-50 {
		t.Errorf("a chips = %d, want %d (bet deducted at deal)", rm.seats[a.AccountID].chips, startChips-50)
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
	rm.seats[a.AccountID].chips = 950
	rm.OnInput(tr, a, runeInput('d'))

	h := rm.seats[a.AccountID].hands[0]
	if rm.seats[a.AccountID].chips != 900 {
		t.Errorf("chips = %d, want 900 (second bet deducted)", rm.seats[a.AccountID].chips)
	}
	if h.bet != 100 || len(h.cards) != 3 || !h.resolved {
		t.Errorf("after double: bet=%d cards=%d resolved=%v, want 100/3/true", h.bet, len(h.cards), h.resolved)
	}
}

func TestSplitFormsTwoHands(t *testing.T) {
	rm, tr, a, _ := turnsSetup(t, hand{{8, suitSpade}, {8, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	rm.seats[a.AccountID].chips = 950
	rm.OnInput(tr, a, runeInput('p'))

	s := rm.seats[a.AccountID]
	if len(s.hands) != 2 {
		t.Fatalf("after split: %d hands, want 2", len(s.hands))
	}
	if s.chips != 900 {
		t.Errorf("chips = %d, want 900 (second bet)", s.chips)
	}
	for i, h := range s.hands {
		if len(h.cards) != 2 {
			t.Errorf("split hand %d has %d cards, want 2", i, len(h.cards))
		}
	}
}

func TestSplitAcesTakeOneCardEach(t *testing.T) {
	rm, tr, a, _ := turnsSetup(t, hand{{rankAce, suitSpade}, {rankAce, suitHeart}}, hand{{10, suitClub}, {6, suitDiamond}})
	rm.seats[a.AccountID].chips = 950
	rm.OnInput(tr, a, runeInput('p'))

	s := rm.seats[a.AccountID]
	if len(s.hands) != 2 {
		t.Fatalf("split aces: %d hands, want 2", len(s.hands))
	}
	for i, h := range s.hands {
		if !h.resolved || len(h.cards) != 2 {
			t.Errorf("split-ace hand %d: resolved=%v cards=%d, want resolved/2", i, h.resolved, len(h.cards))
		}
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
	s.chips = 0 // bet already deducted; this hand loses
	s.highScore = 2500
	s.hands = []*phand{{cards: hand{{10, suitSpade}, {10, suitHeart}, {5, suitClub}}, bet: 50}} // 25, bust
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}}

	rm.settle(tr)

	if s.chips != rebuyChips {
		t.Errorf("chips = %d, want re-buy to %d", s.chips, rebuyChips)
	}
	if s.highScore != 2500 {
		t.Errorf("highScore = %d, want 2500 (a bust must not lower it)", s.highScore)
	}
}

func TestBlackjackPays3to2(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.chips = 950 // 100 already staked at deal
	s.hands = []*phand{{cards: hand{{rankAce, suitSpade}, {rankKing, suitHeart}}, bet: 100}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // dealer 19, no blackjack

	rm.settle(tr)

	// 3:2 blackjack credits stake (100) + 150 = 250, on top of the 950 left.
	if s.chips != 1200 {
		t.Errorf("chips = %d, want 1200 (blackjack pays 3:2)", s.chips)
	}
}

func TestInsurancePaysOnDealerBlackjack(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)
	s := rm.seats[a.AccountID]
	s.placed = true
	s.bet = 50
	s.chips = 950
	rm.dealer = hand{{rankAce, suitSpade}, {rankKing, suitHeart}} // dealer blackjack

	rm.takeInsurance(s, true) // insurance = 25, chips 925
	if s.insurance != 25 || s.chips != 925 {
		t.Fatalf("after taking insurance: ins=%d chips=%d, want 25/925", s.insurance, s.chips)
	}

	rm.resolveInsurance(tr) // 2:1 -> +75, dealer BJ settles the round

	if s.chips != 1000 {
		t.Errorf("chips = %d, want 1000 (insurance paid 2:1)", s.chips)
	}
	if rm.dealerHole {
		t.Error("dealer hole card should be revealed after a dealer blackjack")
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

// TestWalletSeedsPersistsAndPostsPeak covers the durable wallet (balance sum /
// peak max) and the leaderboard Post on a new personal peak.
func TestWalletSeedsPersistsAndPostsPeak(t *testing.T) {
	a := mkPlayer("a")
	rm, tr := newGame(t, a)
	rm.OnJoin(tr, a)

	// First seat seeds the default stack into KV.
	if got := string(tr.KV[a.AccountID][keyBalance]); got != "1000" {
		t.Fatalf("seeded balance KV = %q, want 1000", got)
	}
	if got := tr.KVRules[a.AccountID][keyBalance]; got != kit.MergeSum {
		t.Errorf("balance merge rule = %v, want MergeSum", got)
	}
	if got := tr.KVRules[a.AccountID][keyPeak]; got != kit.MergeMax {
		t.Errorf("peak merge rule = %v, want MergeMax", got)
	}

	// A winning settle raises the peak, persists it, and posts it to the board.
	s := rm.seats[a.AccountID]
	s.placed = true
	s.chips = 1900 // 100 staked at deal, hand will win 200
	s.hands = []*phand{{cards: hand{{rankKing, suitSpade}, {rankQueen, suitHeart}}, bet: 100}}
	rm.dealer = hand{{10, suitClub}, {9, suitDiamond}} // dealer 19, player 20 wins
	rm.settle(tr)

	if s.chips != 2100 || s.highScore != 2100 {
		t.Fatalf("after win: chips=%d high=%d, want 2100/2100", s.chips, s.highScore)
	}
	if got := string(tr.KV[a.AccountID][keyPeak]); got != "2100" {
		t.Errorf("persisted peak KV = %q, want 2100", got)
	}
	if len(tr.Posted) == 0 {
		t.Fatal("a new peak should Post to the leaderboard")
	}
	last := tr.Posted[len(tr.Posted)-1]
	if len(last.Rankings) != 1 || last.Rankings[0].Metric != 2100 {
		t.Errorf("posted ranking = %+v, want metric 2100", last.Rankings)
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

// dealerReady puts the table at the moment the last player has resolved, with a
// live (non-bust) player so the dealer will reveal and draw. The dealer's hand
// and the shoe's next draws are caller-supplied so the reveal is deterministic.
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
	rm.dealerHole = true
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
	// Dealer 16 hits a ten -> 26, a deterministic bust on the first draw.
	rm, tr, a := dealerReady(t, hand{{10, suitClub}, {6, suitDiamond}}, card{10, suitClub})

	rm.enterDealer(tr) // schedules the slow reveal + the busting hit
	rm.render(tr)

	if !rm.dealingActive() {
		t.Fatal("dealer reveal should still be animating right after enterDealer")
	}
	if row := kittest.String(tr.LastFrame(a), dealerValRow); strings.Contains(row, "BUST") {
		t.Fatalf("dealer BUST shown before the hit landed: %q", row)
	}

	// Once the whole reveal has played out the bust shows and the round settles
	// (stay inside the results window, before the next betting round clears it).
	pump(rm, tr, 5*time.Second)
	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results once the reveal finished", rm.phase)
	}
	if row := kittest.String(tr.LastFrame(a), dealerValRow); !strings.Contains(row, "BUST") {
		t.Fatalf("dealer BUST not shown after the hit landed: %q", row)
	}
}

// TestDealerRevealIsPaced asserts the reveal is unhurried: settlement is deferred
// well past the bare hole-card flip even when the dealer stands pat, so the
// turned hole card and its total have time to read.
func TestDealerRevealIsPaced(t *testing.T) {
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}, {8, suitDiamond}}) // 18, no hit

	start := tr.Now()
	rm.enterDealer(tr)

	if rm.what != pendSettle {
		t.Fatalf("dealer reveal should defer settlement, what = %v", rm.what)
	}
	if delay := rm.pendAt.Sub(start); delay < holeRevealHold {
		t.Fatalf("settle deferred only %v, want at least the hole-reveal hold %v", delay, holeRevealHold)
	}
}

// TestDealerTotalTicksUpAsCardsLand checks the displayed total reflects only the
// face-up cards: just the up card while the hole is concealed, then the hole, in
// step with the animation rather than the full hand up front.
func TestDealerTotalTicksUpAsCardsLand(t *testing.T) {
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}, {7, suitDiamond}}) // 17, stands

	// Mid-turn the hole is still hidden: only the up card counts.
	if got := rm.dealerShownCount(); got != 1 {
		t.Fatalf("with the hole concealed, shown count = %d, want 1", got)
	}

	rm.enterDealer(tr) // turns the hole; the reveal flip is still in flight
	rm.render(tr)
	if got := rm.dealerShownCount(); got != 1 {
		t.Fatalf("mid hole-flip, shown count = %d, want 1 (hole not yet face up)", got)
	}

	// After the reveal settles (still within the results window) both cards
	// count and the total is complete.
	pump(rm, tr, 3*time.Second)
	if rm.phase != phResults {
		t.Fatalf("phase = %q, want results once the reveal finished", rm.phase)
	}
	if got := rm.dealerShownCount(); got != 2 {
		t.Fatalf("after the reveal, shown count = %d, want 2 (both cards face up)", got)
	}
	if got := rm.dealer.total(); got != 17 {
		t.Fatalf("dealer total = %d, want 17", got)
	}
}

// TestDealerHoleRevealIsDelayedAndAnimated asserts the second (hole) card turns
// over after a short lead-in beat rather than the instant the dealer's turn
// begins, and that it is an in-place flip animation (a flip scheduled, no slide)
// so it visibly turns rather than snapping face up.
func TestDealerHoleRevealIsDelayedAndAnimated(t *testing.T) {
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}, {8, suitDiamond}}) // 18, stands pat

	start := tr.Now()
	rm.enterDealer(tr)

	if len(rm.sched) == 0 {
		t.Fatal("no dealer reveal animation scheduled")
	}
	hole := rm.sched[0]
	if hole.kind != animDealer || hole.cardIdx != 1 {
		t.Fatalf("first scheduled anim is not the hole card: %+v", hole)
	}
	if lead := hole.flipStart.Sub(start); lead < holeRevealDelay {
		t.Fatalf("hole flip starts %v after the turn, want at least the lead-in %v", lead, holeRevealDelay)
	}
	if hole.flipStart.IsZero() {
		t.Fatal("hole reveal has no flip scheduled - it would snap, not animate")
	}
	if !hole.slideStart.IsZero() {
		t.Fatalf("hole reveal should turn in place, not slide: %+v", hole)
	}

	// Through the lead-in beat the hole is still concealed: only the up card counts.
	pump(rm, tr, holeRevealDelay/2)
	if got := rm.dealerShownCount(); got != 1 {
		t.Fatalf("during the lead-in, shown count = %d, want 1 (hole not yet turned)", got)
	}
}

// TestDealerHitSlotHiddenUntilHoleShown asserts the dealer's row stays two cards
// wide — no slot reserved for the upcoming hit — until the hole card has turned
// over and that hit actually begins to slide in.
func TestDealerHitSlotHiddenUntilHoleShown(t *testing.T) {
	// Dealer 16 will draw a ten; with a live player the reveal schedules the hit.
	rm, tr, _ := dealerReady(t, hand{{10, suitClub}, {6, suitDiamond}}, card{10, suitClub})
	rm.enterDealer(tr)

	// Before the hit begins arriving (through the lead-in, flip, and read beat),
	// only the two dealt cards occupy the row even though the full hand is known.
	if got := len(rm.dealer); got != 3 {
		t.Fatalf("dealer hand size = %d, want 3 (hit already computed)", got)
	}
	rm.render(tr)
	if got := rm.dealerLayoutCount(); got != 2 {
		t.Fatalf("layout count = %d right after the turn, want 2 (no slot for the pending hit)", got)
	}

	// Past the reveal + read beat the hit is sliding in, so the row now includes it.
	pump(rm, tr, holeRevealDelay+flipDur+holeRevealHold+slideDur/2)
	if got := rm.dealerLayoutCount(); got != 3 {
		t.Fatalf("layout count = %d once the hit is arriving, want 3", got)
	}
}

// TestInsuranceSkipsTimerOnceAllDecided covers the insurance early-resolve: once
// every placed seat has answered, the window resolves without waiting out the
// timer.
func TestInsuranceSkipsTimerOnceAllDecided(t *testing.T) {
	a, b := mkPlayer("a"), mkPlayer("b")
	rm, tr := newGame(t, a, b)
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	for _, p := range []kit.Player{a, b} {
		s := rm.seats[p.AccountID]
		s.placed = true
		s.bet = 50
		s.chips = 950
		s.hands = []*phand{{cards: hand{{10, suitSpade}, {7, suitHeart}}, bet: 50}}
	}
	rm.dealer = hand{{rankAce, suitSpade}, {6, suitHeart}} // shows an Ace, no blackjack
	rm.dealerHole = true
	rm.enterInsurance(tr)

	rm.OnInput(tr, a, runeInput('n')) // a answers
	if rm.phase != phInsurance {
		t.Fatalf("phase = %q, want still insurance (b has not answered)", rm.phase)
	}
	rm.OnInput(tr, b, runeInput('y')) // b answers -> every placed seat decided
	if rm.phase != phTurns {
		t.Fatalf("phase = %q, want turns (insurance resolved early without the timer)", rm.phase)
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
	s.chips = 1000 // "$1000" is wider than "PUSH"
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
	s.chips = 950
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
	s.chips = 1000
	s.pairsBet = 25
	s.pairsKind = "colored"
	s.pairsWin = 325
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

	dealerLabelRow := kittest.String(f, dealerRow-1)
	if !strings.Contains(dealerLabelRow, "blackjack pays 3:2") {
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
	s.chips = 800
	s.hands = []*phand{
		{cards: hand{{8, suitSpade}, {3, suitHeart}}, bet: 50, fromSplit: true},
		{cards: hand{{8, suitClub}, {10, suitDiamond}}, bet: 50, fromSplit: true},
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
		s.chips = 950
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
