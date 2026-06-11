package main

import (
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
