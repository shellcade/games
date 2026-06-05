package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"

	"alan/chess/engine"
)

// --- test scaffolding ------------------------------------------------------

// newGame builds a 2-player chess room driven by a kittest.Room, started.
func newGame(t *testing.T) (*room, *kittest.Room) {
	t.Helper()
	a, b := kittest.Player("alice"), kittest.Player("bob")
	tr := kittest.NewRoom(a, b)
	tr.Cfg.Mode = kit.ModeQuick
	tr.Cfg.MinPlayers = 2
	rm := newRoom(tr.Cfg, tr.Services())
	rm.OnStart(tr)
	return rm, tr
}

// pair joins both players (the kittest roster) and returns them. After this the
// game is playing.
func pair(t *testing.T, rm *room, tr *kittest.Room) (kit.Player, kit.Player) {
	t.Helper()
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)
	return a, b
}

// sq is a tiny algebraic-square helper for tests, e.g. sq("e4").
func sq(name string) engine.Square {
	return engine.SquareAt(int(name[0]-'a'), int(name[1]-'1'))
}

// play drives a move by coordinates via the real makeMove path (white-box).
func (rm *room) play(t *testing.T, tr *kittest.Room, from, to string, promo engine.PieceType) bool {
	t.Helper()
	return rm.makeMove(tr, engine.Move{From: sq(from), To: sq(to), Promo: promo})
}

// TestPublishesNavInputContext verifies chess declares CtxNav.
func TestPublishesNavInputContext(t *testing.T) {
	_, tr := newGame(t)
	if tr.InputCtx != kit.CtxNav {
		t.Fatalf("published input context = %v, want CtxNav", tr.InputCtx)
	}
}

// --- pairing ---------------------------------------------------------------

func TestSecondJoinStartsPlay(t *testing.T) {
	rm, tr := newGame(t)
	a, b := tr.Players[0], tr.Players[1]

	rm.OnJoin(tr, a)
	if rm.phase != phWaiting {
		t.Fatalf("after 1st join phase=%q, want %q", rm.phase, phWaiting)
	}
	rm.OnJoin(tr, b)
	if rm.phase != phPlaying {
		t.Fatalf("after 2nd join phase=%q, want %q", rm.phase, phPlaying)
	}

	// Both colours assigned, distinct.
	ca, oka := rm.color[a.AccountID]
	cb, okb := rm.color[b.AccountID]
	if !oka || !okb || ca == cb {
		t.Fatalf("colours a=%v(%v) b=%v(%v), want both set and distinct", ca, oka, cb, okb)
	}
}

func TestColorAssignmentReproducibleUnderSeed(t *testing.T) {
	// With a fixed seed, the same player must always be White.
	var firstWhite string
	for i := 0; i < 5; i++ {
		rm, tr := newGame(t)
		a, b := pair(t, rm, tr)
		var white string
		if rm.color[a.AccountID] == engine.White {
			white = a.AccountID
		} else {
			white = b.AccountID
		}
		if i == 0 {
			firstWhite = white
		} else if white != firstWhite {
			t.Fatalf("seed produced White=%v on run %d, want stable %v", white, i, firstWhite)
		}
	}
}

// --- mate / stalemate ------------------------------------------------------

func TestScriptedFoolsMate(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	// 1. f3 e5 2. g4 Qh4#  — Black mates White.
	mustPlay(t, rm, tr, "f2", "f3")
	mustPlay(t, rm, tr, "e7", "e5")
	mustPlay(t, rm, tr, "g2", "g4")
	mustPlay(t, rm, tr, "d8", "h4")

	if rm.phase != phOver {
		t.Fatalf("phase=%q after Qh4#, want over", rm.phase)
	}
	if !contains(rm.outcome, "Checkmate") || !contains(rm.outcome, "Black wins") {
		t.Fatalf("outcome=%q, want Checkmate - Black wins", rm.outcome)
	}

	// Results hold then settle on the wake past the deadline.
	settleAfterResults(tr, rm)
	if tr.Ended == nil {
		t.Fatal("game did not settle after results hold")
	}
	checkRank(t, *tr.Ended, black, 1, kit.StatusFinished)
	checkRank(t, *tr.Ended, white, 2, kit.StatusFinished)
}

func TestStalemateIsDraw(t *testing.T) {
	rm, tr := newGame(t)
	pair(t, rm, tr)
	// Stalemate-in-one: White Kg6, Qf1; Black Kh8. After Qf7 the Black king has
	// no legal move and is NOT in check.
	pos, err := engine.ParseFEN("7k/8/6K1/8/8/8/8/5Q2 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	rm.pos = pos
	rm.history = map[string]int{pos.RepetitionKey(): 1}
	if !rm.play(t, tr, "f1", "f7", engine.Empty) {
		t.Fatal("Qf7 was rejected as illegal")
	}
	if rm.phase != phOver {
		t.Fatalf("phase=%q after stalemating move, want over", rm.phase)
	}
	if !contains(rm.outcome, "Stalemate") {
		t.Fatalf("outcome=%q, want Stalemate - draw", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkDraw(t, *tr.Ended)
}

// --- resign / draw offer ---------------------------------------------------

func TestResignWithConfirm(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('r'))
	if rm.phase != phPlaying {
		t.Fatalf("a single 'r' ended the game (phase=%q); it must only arm", rm.phase)
	}
	if !rm.resignArm[white.AccountID] {
		t.Fatal("first 'r' did not arm resignation")
	}
	rm.OnInput(tr, white, runeInput('r'))
	if rm.phase != phOver {
		t.Fatalf("phase=%q after confirm, want over", rm.phase)
	}
	if !contains(rm.outcome, "resigns") {
		t.Fatalf("outcome=%q, want a resignation", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkRank(t, *tr.Ended, black, 1, kit.StatusFinished)
	checkRank(t, *tr.Ended, white, 2, kit.StatusFinished)
}

func TestDrawOfferAccepted(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('d')) // White offers
	if rm.drawOffer != engine.White {
		t.Fatalf("drawOffer=%v after White offers, want White", rm.drawOffer)
	}
	if rm.phase != phPlaying {
		t.Fatal("a draw offer must not end the game on its own")
	}
	rm.OnInput(tr, black, runeInput('y')) // Black accepts
	if rm.phase != phOver {
		t.Fatalf("phase=%q after accept, want over", rm.phase)
	}
	settleAfterResults(tr, rm)
	checkDraw(t, *tr.Ended)
}

func TestDrawOfferDeclined(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('d'))
	rm.OnInput(tr, black, runeInput('n'))
	if rm.drawOffer != noOffer {
		t.Fatalf("drawOffer=%v after decline, want none", rm.drawOffer)
	}
	if rm.phase != phPlaying {
		t.Fatalf("phase=%q after decline, want still playing", rm.phase)
	}
}

// --- flag-fall -------------------------------------------------------------

func TestFlagFallLoses(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	// White is to move at the start; advance past White's whole time, then wake
	// drives the flag-fall.
	tr.Advance(mainClock + time.Second)
	rm.OnWake(tr)

	if rm.phase != phOver {
		t.Fatalf("phase=%q after White flags, want over", rm.phase)
	}
	if !contains(rm.outcome, "Time") || !contains(rm.outcome, "Black wins") {
		t.Fatalf("outcome=%q, want Time - Black wins", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkRank(t, *tr.Ended, black, 1, kit.StatusFinished)
	checkRankStatus(t, *tr.Ended, white, kit.StatusFlagged)
}

func TestFlagFallVsLoneKingIsDraw(t *testing.T) {
	rm, tr := newGame(t)
	pair(t, rm, tr)
	// White to move and on the clock, but Black has only a king + pawn that can't
	// mate alone → White flagging is a draw.
	pos, err := engine.ParseFEN("7k/8/8/8/8/8/4P3/4K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	rm.pos = pos
	rm.turnStart = tr.Clock

	tr.Advance(mainClock + time.Second)
	rm.OnWake(tr)

	if rm.phase != phOver {
		t.Fatalf("phase=%q after flag, want over", rm.phase)
	}
	if !contains(rm.outcome, "draw") {
		t.Fatalf("outcome=%q, want a draw (insufficient mating material)", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkDraw(t, *tr.Ended)
}

// --- abandonment -----------------------------------------------------------

func TestAbandonmentOpponentWins(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnLeave(tr, white) // White walks away mid-game
	if rm.phase != phOver {
		t.Fatalf("phase=%q after abandonment, want over", rm.phase)
	}
	settleAfterResults(tr, rm)
	checkRank(t, *tr.Ended, black, 1, kit.StatusFinished)
	checkRankStatus(t, *tr.Ended, white, kit.StatusDNF)
}

// --- threefold / fifty-move ------------------------------------------------

func TestThreefoldRepetitionDraws(t *testing.T) {
	rm, tr := newGame(t)
	pair(t, rm, tr)
	moves := [][2]string{
		{"g1", "f3"}, {"g8", "f6"},
		{"f3", "g1"}, {"f6", "g8"},
		{"g1", "f3"}, {"g8", "f6"},
		{"f3", "g1"}, {"f6", "g8"},
	}
	for _, m := range moves {
		if rm.phase == phOver {
			break
		}
		mustPlay(t, rm, tr, m[0], m[1])
	}
	if rm.phase != phOver {
		t.Fatalf("phase=%q after threefold, want over", rm.phase)
	}
	if !contains(rm.outcome, "threefold") {
		t.Fatalf("outcome=%q, want threefold draw", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkDraw(t, *tr.Ended)
}

func TestFiftyMoveRuleDraws(t *testing.T) {
	rm, tr := newGame(t)
	pair(t, rm, tr)
	pos, err := engine.ParseFEN("r6k/8/8/8/8/8/8/R3K3 w - - 99 60")
	if err != nil {
		t.Fatal(err)
	}
	rm.pos = pos
	rm.history = map[string]int{pos.RepetitionKey(): 1}
	if !rm.play(t, tr, "a1", "b1", engine.Empty) {
		t.Fatal("Rb1 rejected")
	}
	if rm.phase != phOver {
		t.Fatalf("phase=%q after 50-move move, want over", rm.phase)
	}
	if !contains(rm.outcome, "fifty-move") {
		t.Fatalf("outcome=%q, want fifty-move draw", rm.outcome)
	}
	settleAfterResults(tr, rm)
	checkDraw(t, *tr.Ended)
}

// --- input path coverage ---------------------------------------------------

func TestMoveViaRealInputs(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, _ := whiteBlack(rm, a, b)

	sel := rm.sel[white.AccountID]
	sel.cursor = sq("e2")
	rm.OnInput(tr, white, keyInput(kit.KeyEnter)) // select e2
	if sel.from != sq("e2") {
		t.Fatalf("Enter on e2 did not select; from=%v", sel.from)
	}
	rm.OnInput(tr, white, keyInput(kit.KeyUp))
	rm.OnInput(tr, white, keyInput(kit.KeyUp))
	if sel.cursor != sq("e4") {
		t.Fatalf("cursor=%v after two Up, want e4", sel.cursor)
	}
	rm.OnInput(tr, white, keyInput(kit.KeyEnter)) // make e2e4

	if rm.pos.Board[sq("e4")].Type != engine.Pawn {
		t.Fatalf("e4 not a pawn after real-input move; board=%v", rm.pos.Board[sq("e4")])
	}
	if rm.pos.Side != engine.Black {
		t.Fatalf("side=%v after e4, want Black to move", rm.pos.Side)
	}
	if len(rm.moves) != 1 || rm.moves[0] != "e2e4" {
		t.Fatalf("move list=%v, want [e2e4]", rm.moves)
	}
}

func TestOffTurnPlayerCannotMove(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	_, black := whiteBlack(rm, a, b)

	sel := rm.sel[black.AccountID]
	sel.cursor = sq("e7")
	rm.OnInput(tr, black, keyInput(kit.KeyEnter))
	if sel.from != engine.NoSquare {
		t.Fatalf("off-turn Black selected a piece (from=%v); must be a no-op", sel.from)
	}
	if len(rm.moves) != 0 {
		t.Fatalf("off-turn input produced moves %v", rm.moves)
	}
}

func TestWaitTimeoutClosesRoom(t *testing.T) {
	rm, tr := newGame(t)
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	tr.Advance(waitTimeout + time.Second)
	rm.OnWake(tr)
	if rm.phase != phOver {
		t.Fatalf("phase=%q after wait timeout, want over", rm.phase)
	}
	if tr.Ended == nil {
		t.Fatal("wait timeout did not settle the room")
	}
}

func TestResignArmClearedByOtherInput(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, _ := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('r')) // arm
	if !rm.resignArm[white.AccountID] {
		t.Fatal("first 'r' did not arm the resignation")
	}
	rm.OnInput(tr, white, keyInput(kit.KeyUp)) // any other key disarms
	if rm.resignArm[white.AccountID] {
		t.Fatal("a cursor move did not disarm the pending resign confirm")
	}
	rm.OnInput(tr, white, runeInput('y')) // a later 'y' must NOT resign
	if rm.phase != phPlaying {
		t.Fatalf("phase=%q after a stray 'y'; the resign arm leaked across input", rm.phase)
	}
}

func TestCursorSnapsToLegalTargetsWhenSelected(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, _ := whiteBlack(rm, a, b)
	sel := rm.sel[white.AccountID]

	sel.cursor = sq("b1") // knight; legal targets a3, c3
	rm.OnInput(tr, white, keyInput(kit.KeyEnter))
	if sel.from != sq("b1") {
		t.Fatalf("knight not selected; from=%v", sel.from)
	}
	isTarget := func(s engine.Square) bool {
		for _, m := range sel.targets {
			if m.To == s {
				return true
			}
		}
		return false
	}
	for _, key := range []kit.Key{kit.KeyUp, kit.KeyLeft, kit.KeyRight, kit.KeyDown} {
		rm.OnInput(tr, white, keyInput(key))
		if sel.cursor != sq("b1") && !isTarget(sel.cursor) {
			t.Fatalf("after %v, cursor=%v is neither origin nor a legal target %v", key, sel.cursor, sel.targets)
		}
	}
}

func TestCannotSelectImmovablePiece(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, _ := whiteBlack(rm, a, b)
	sel := rm.sel[white.AccountID]

	sel.cursor = sq("a1") // boxed-in rook
	rm.OnInput(tr, white, keyInput(kit.KeyEnter))
	if sel.from != engine.NoSquare {
		t.Fatalf("selected an immovable rook; from=%v", sel.from)
	}
}

// --- promotion -------------------------------------------------------------

// TestPromotionPickerAndUnderpromote drives a pawn to the last rank, opens the
// promotion picker, cycles to Knight, and confirms an underpromotion.
func TestPromotionPickerAndUnderpromote(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, _ := whiteBlack(rm, a, b)

	// White pawn on g7, Black king tucked away — White to move and can push g8.
	pos, err := engine.ParseFEN("8/6P1/8/8/8/8/k7/7K w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	rm.pos = pos
	rm.history = map[string]int{pos.RepetitionKey(): 1}
	sel := rm.sel[white.AccountID]
	sel.cursor = sq("g7")

	rm.OnInput(tr, white, keyInput(kit.KeyEnter)) // select g7
	if sel.from != sq("g7") {
		t.Fatalf("pawn not selected; from=%v", sel.from)
	}
	// Move cursor up one rank to g8 (the promotion square) and confirm to open the picker.
	rm.OnInput(tr, white, keyInput(kit.KeyUp))
	if sel.cursor != sq("g8") {
		t.Fatalf("cursor=%v after Up, want g8", sel.cursor)
	}
	rm.OnInput(tr, white, keyInput(kit.KeyEnter)) // open picker
	if !sel.promoing {
		t.Fatal("promotion picker did not open on a promoting move")
	}
	// Cycle right thrice: Q -> R -> B -> N (index 3 = Knight).
	rm.OnInput(tr, white, keyInput(kit.KeyRight))
	rm.OnInput(tr, white, keyInput(kit.KeyRight))
	rm.OnInput(tr, white, keyInput(kit.KeyRight))
	if promoOrder[sel.promoSel] != engine.Knight {
		t.Fatalf("picker on %v, want Knight", promoOrder[sel.promoSel])
	}
	rm.OnInput(tr, white, keyInput(kit.KeyEnter)) // confirm underpromotion

	if got := rm.pos.Board[sq("g8")]; got.Type != engine.Knight || got.Color != engine.White {
		t.Fatalf("g8 = %+v, want a White knight", got)
	}
	if len(rm.moves) != 1 || rm.moves[0] != "g7g8=N" {
		t.Fatalf("move list=%v, want [g7g8=N]", rm.moves)
	}
}

// --- rendering -------------------------------------------------------------

var figurineGlyphs = map[rune]bool{
	'♔': true, '♕': true, '♖': true, '♗': true, '♘': true, '♙': true,
	'♚': true, '♛': true, '♜': true, '♝': true, '♞': true, '♟': true,
}

func TestComposedFrameRenderableAndFlipped(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	mustPlay(t, rm, tr, "e2", "e4")
	mustPlay(t, rm, tr, "e7", "e5")
	rm.render(tr)

	wf := tr.LastFrame(white)
	bf := tr.LastFrame(black)
	if wf == nil || bf == nil {
		t.Fatal("missing per-viewer frame")
	}

	// Every rune must be ASCII or a known chess figurine.
	for _, fr := range []*kit.Frame{wf, bf} {
		for r := 0; r < kit.Rows; r++ {
			for c := 0; c < kit.Cols; c++ {
				ru := fr.Cells[r][c].Rune
				if ru != 0 && ru > 127 && !figurineGlyphs[ru] {
					t.Fatalf("unrenderable glyph %q (U+%04X) at row %d col %d", ru, ru, r, c)
				}
			}
		}
	}

	// The board must be flipped for Black.
	wr, wc := screenSquare(sq("a1"), engine.White)
	br, bc := screenSquare(sq("a1"), engine.Black)
	if wr == br && wc == bc {
		t.Fatalf("a1 maps to same screen cell for both orientations — board not flipped")
	}
	if !(wr > br) {
		t.Fatalf("expected a1 lower on White's view (wr=%d) than Black's (br=%d)", wr, br)
	}

	// a1 is the White rook: '♖' at col+1, 'R' at col+2 in both views.
	for _, v := range []struct {
		name string
		fr   *kit.Frame
		r, c int
	}{{"White", wf, wr, wc}, {"Black", bf, br, bc}} {
		glyph := v.fr.Cells[v.r+sqH/2][v.c+1].Rune
		letter := v.fr.Cells[v.r+sqH/2][v.c+2].Rune
		if glyph != '♖' || letter != 'R' {
			t.Fatalf("%s view a1: glyph=%q letter=%q, want '♖' then 'R'", v.name, glyph, letter)
		}
	}
}

// --- hibernation determinism (state-only reconstruction) -------------------

// TestStateOnlyReconstruction is a light guard for the hibernation contract: a
// fresh room replaying the same callbacks (same seed, same clock advances) must
// reach byte-identical frames. The wasm conformance harness is the real gate;
// this catches accidental dependence on map order, wall time, or ambient state
// at the Go level.
func TestStateOnlyReconstruction(t *testing.T) {
	run := func() *kit.Frame {
		a, b := kittest.Player("alice"), kittest.Player("bob")
		tr := kittest.NewRoom(a, b)
		tr.Cfg.Mode = kit.ModeQuick
		rm := newRoom(tr.Cfg, tr.Services())
		rm.OnStart(tr)
		rm.OnJoin(tr, a)
		rm.OnJoin(tr, b)
		white, _ := whiteBlack(rm, a, b)
		_ = white
		// Play the same opening; advance the clock the same way; wake.
		rm.makeMove(tr, engine.Move{From: sq("e2"), To: sq("e4")})
		tr.Advance(time.Second)
		rm.makeMove(tr, engine.Move{From: sq("e7"), To: sq("e5")})
		tr.Advance(2 * time.Second)
		rm.OnWake(tr)
		return tr.LastFrame(a)
	}
	f1, f2 := run(), run()
	if f1 == nil || f2 == nil {
		t.Fatal("no frame produced")
	}
	if *f1 != *f2 {
		t.Fatal("two identical runs produced different frames — non-deterministic state")
	}
}

// --- helpers ---------------------------------------------------------------

func runeInput(r rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: r} }
func keyInput(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}

// settleAfterResults advances past the results-hold deadline and wakes, settling
// the room (the wake-driven replacement for the native After timer).
func settleAfterResults(tr *kittest.Room, rm *room) {
	tr.Advance(resultsDur + time.Second)
	rm.OnWake(tr)
}

func whiteBlack(rm *room, a, b kit.Player) (white, black kit.Player) {
	if rm.color[a.AccountID] == engine.White {
		return a, b
	}
	return b, a
}

func mustPlay(t *testing.T, rm *room, tr *kittest.Room, from, to string) {
	t.Helper()
	if !rm.play(t, tr, from, to, engine.Empty) {
		t.Fatalf("move %s%s was rejected as illegal (side=%v)", from, to, rm.pos.Side)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func checkRank(t *testing.T, res kit.Result, p kit.Player, rank int, status kit.Status) {
	t.Helper()
	for _, pr := range res.Rankings {
		if pr.Player.AccountID == p.AccountID {
			if pr.Rank != rank {
				t.Errorf("player %v rank=%d, want %d", p.Handle, pr.Rank, rank)
			}
			if pr.Status != status {
				t.Errorf("player %v status=%v, want %v", p.Handle, pr.Status, status)
			}
			return
		}
	}
	t.Errorf("player %v not in rankings %+v", p.Handle, res.Rankings)
}

func checkRankStatus(t *testing.T, res kit.Result, p kit.Player, status kit.Status) {
	t.Helper()
	for _, pr := range res.Rankings {
		if pr.Player.AccountID == p.AccountID {
			if pr.Status != status {
				t.Errorf("player %v status=%v, want %v", p.Handle, pr.Status, status)
			}
			return
		}
	}
	t.Errorf("player %v not in rankings %+v", p.Handle, res.Rankings)
}

func checkDraw(t *testing.T, res kit.Result) {
	t.Helper()
	if len(res.Rankings) == 0 {
		t.Fatal("draw result has no rankings")
	}
	for _, pr := range res.Rankings {
		if pr.Rank != 1 {
			t.Errorf("draw: player %v rank=%d, want all rank 1", pr.Player.Handle, pr.Rank)
		}
		if pr.Status != kit.StatusFinished {
			t.Errorf("draw: player %v status=%v, want finished", pr.Player.Handle, pr.Status)
		}
	}
}
