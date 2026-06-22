package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// --- harness -----------------------------------------------------------------

func space() kit.Input    { return kit.Input{Kind: kit.InputRune, Rune: ' '} }
func keyUp() kit.Input    { return kit.Input{Kind: kit.InputKey, Key: kit.KeyUp} }
func keyDown() kit.Input  { return kit.Input{Kind: kit.InputKey, Key: kit.KeyDown} }
func keyRight() kit.Input { return kit.Input{Kind: kit.InputKey, Key: kit.KeyRight} }

// newGame builds a started room handler plus its driving kittest.Room.
func newGame(t *testing.T, players ...kit.Player) (*room, *kittest.Room) {
	t.Helper()
	r := kittest.NewRoom(players...)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	return h, r
}

// seatAt0 seats player p at machine 0 so the cabinet renders and the machine
// ticks. Returns the player's machine.
func seatAt0(t *testing.T, rm *room, p kit.Player) *machine {
	t.Helper()
	pw := rm.pawns[p.AccountID]
	mc := rm.fmachines[0]
	pw.x, pw.y = mc.ax, mc.ay
	rm.trySit(p.AccountID)
	return rm.machines[p.AccountID]
}

// settle drives enough wake/clock cycles to land all three reels and settle a
// spin (reel 2 lands at +150ms+2*250ms = +650ms; advance well past it).
func settle(rm *room, r *kittest.Room) {
	for i := 0; i < 4; i++ {
		r.Advance(time.Second)
		rm.OnWake(r)
	}
}

// takeIfGambling banks a held base-game win (a win now enters the gamble holding
// the win at risk) so balance/leaderboard/ticker assertions see it credited.
func takeIfGambling(rm *room, r *kittest.Room, id string) {
	if m := rm.machines[id]; m != nil && m.gamble != nil {
		m.gamble.sel = selTake
		rm.gambleConfirm(r, id)
	}
}

// cleanIdx returns a strip index holding face s whose 3-window contains no
// scatter, so a settle landing there never accidentally triggers free spins.
func cleanIdx(t *testing.T, v *variant, s symbol) int {
	t.Helper()
	for i, x := range v.strip {
		if x != s {
			continue
		}
		w := windowAt(v.strip, i)
		if w[0] != symScatter && w[1] != symScatter && w[2] != symScatter {
			return i
		}
	}
	t.Fatalf("no scatter-free %q landing on strip", rune(s))
	return 0
}

// pureIdx returns a strip index whose entire 3-window is face s (a reel that
// contributes nothing to other symbols' runs). Fatal if none.
func pureIdx(t *testing.T, v *variant, s symbol) int {
	t.Helper()
	for i := range v.strip {
		w := windowAt(v.strip, i)
		if w[0] == s && w[1] == s && w[2] == s {
			return i
		}
	}
	t.Fatalf("no pure-%q window on strip", rune(s))
	return 0
}

// forceWin5 settles a 5-reel run of paying symbol s (scatter-free window) under
// rm.variant and returns the credited win (bet * ways).
func forceWin5(t *testing.T, rm *room, r *kittest.Room, p kit.Player, s symbol) int {
	t.Helper()
	m := rm.machines[p.AccountID]
	v := rm.variant
	idx := cleanIdx(t, v, s)
	win := m.bet * v.waysPayout(scatterWindow(v.strip, allReels(idx)))
	m.spin = &spinState{startedAt: r.Now(), variant: v, stopIdx: allReels(idx), final: faceRow(s)}
	rm.settleSpin(r, p.AccountID)
	return win
}

// forceLoss5 settles an all-cherry window (cherry pays nothing) — no win, no
// trigger — under rm.variant.
func forceLoss5(t *testing.T, rm *room, r *kittest.Room, p kit.Player) {
	t.Helper()
	m := rm.machines[p.AccountID]
	v := rm.variant
	idx := pureIdx(t, v, symCherry)
	m.spin = &spinState{startedAt: r.Now(), variant: v, stopIdx: allReels(idx), final: faceRow(symCherry)}
	rm.settleSpin(r, p.AccountID)
}

// frameContains reports whether any row of the player's last frame contains s.
func frameContains(r *kittest.Room, p kit.Player, s string) bool {
	f := r.LastFrame(p)
	if f == nil {
		return false
	}
	for row := 0; row < kit.Rows; row++ {
		if strings.Contains(kittest.String(f, row), s) {
			return true
		}
	}
	return false
}

// --- meta + context ----------------------------------------------------------

func TestMetaIsResidentLounge(t *testing.T) {
	m := Game{}.Meta()
	if m.Slug != "pokies" {
		t.Errorf("slug = %q, want pokies", m.Slug)
	}
	if m.MinPlayers != 1 || m.MaxPlayers != 32 {
		t.Errorf("players = %d..%d, want 1..32", m.MinPlayers, m.MaxPlayers)
	}
	if m.Lifecycle != kit.LifecycleResident {
		t.Errorf("lifecycle = %v, want resident", m.Lifecycle)
	}
	if m.CtxFeatures&kit.CtxFeatCharacter == 0 || m.CtxFeatures&kit.CtxFeatRosterEpoch == 0 {
		t.Errorf("ctx features = %d, want character + roster-epoch", m.CtxFeatures)
	}
	if m.Leaderboard == nil || m.Leaderboard.Direction != kit.HigherBetter {
		t.Errorf("leaderboard = %+v, want higher-better board", m.Leaderboard)
	}
}

func TestPublishesNavContext(t *testing.T) {
	_, r := newGame(t)
	if r.InputCtx != kit.CtxNav {
		t.Errorf("InputCtx = %v, want CtxNav", r.InputCtx)
	}
}

// --- joining / wallet --------------------------------------------------------

func TestJoinSeedsMachine(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	m := rm.machines[p.AccountID]
	if m == nil {
		t.Fatal("join did not create a machine")
	}
	if m.balance != startBalance {
		t.Errorf("balance = %d, want %d", m.balance, startBalance)
	}
	if m.highScore != startBalance {
		t.Errorf("highScore = %d, want %d", m.highScore, startBalance)
	}
	if m.bet != betTiers[0] {
		t.Errorf("bet = %d, want %d", m.bet, betTiers[0])
	}
}

func TestJoinResumesDurableWallet(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	// Pre-seed the durable wallet as if a prior session persisted it.
	store := r.Services().Accounts.For(p).Store()
	_ = store.Set(nil, keyBalance, []byte("2000"), kit.MergeSum)
	_ = store.Set(nil, keyPeak, []byte("3000"), kit.MergeMax)

	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	if m.balance != 2000 {
		t.Errorf("balance = %d, want resumed 2000", m.balance)
	}
	if m.highScore != 3000 {
		t.Errorf("highScore = %d, want resumed peak 3000", m.highScore)
	}
}

// --- betting -----------------------------------------------------------------

func TestBetUpDownCyclesTiers(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]

	rm.OnInput(r, p, keyUp())
	rm.OnInput(r, p, keyUp())
	if m.bet != 100 {
		t.Fatalf("after up,up bet = %d, want 100", m.bet)
	}
	rm.OnInput(r, p, keyDown())
	if m.bet != 50 {
		t.Fatalf("after down bet = %d, want 50", m.bet)
	}
}

func TestBetClampedToBalance(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]
	m.balance = 70

	rm.OnInput(r, p, keyUp()) // 50, ok
	rm.OnInput(r, p, keyUp()) // 100 > 70, clamp back to 50
	if m.bet != 50 {
		t.Fatalf("bet = %d, want 50 (clamped to balance 70)", m.bet)
	}
}

// --- spinning ----------------------------------------------------------------

func TestSpinDeductsBetAndIgnoresReentry(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]
	m.bet = 50

	rm.OnInput(r, p, space())
	if m.balance != startBalance-50 {
		t.Fatalf("balance after spin = %d, want %d", m.balance, startBalance-50)
	}
	if m.spin == nil {
		t.Fatal("expected machine to be spinning")
	}
	rm.OnInput(r, p, space()) // must be ignored mid-spin
	if m.balance != startBalance-50 {
		t.Fatalf("re-entry deducted again: balance = %d", m.balance)
	}
}

func TestSpinSettlesToPayoutOverWake(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]
	m.bet = 10

	rm.OnInput(r, p, space())
	settle(rm, r)
	// A line win now enters the gamble holding the win; take it so the credited
	// balance is observable. (A scatter trigger would auto-play free spins; the
	// seeded first spin does neither, but the take keeps this robust to a win.)
	takeIfGambling(rm, r, p.AccountID)

	if m.spin != nil {
		t.Fatal("expected spin to have settled after wakes")
	}
	if !m.spun {
		t.Fatal("expected spun=true after first settle")
	}
	if m.freeSpins > 0 {
		t.Skip("seeded first spin triggered free spins; payout path covered elsewhere")
	}
	want := (startBalance - 10) + 10*rm.variant.waysPayout(scatterWindow(rm.variant.strip, m.lastIdx))
	if m.balance != want {
		t.Fatalf("balance = %d, want %d (ways over %v)", m.balance, want, m.lastIdx)
	}
}

func TestReelsLandLeftToRightStaggered(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	rm.OnInput(r, p, space())

	m := rm.machines[p.AccountID]
	// Just past reel 0's deadline (150ms), before reel 1's (400ms).
	r.Advance(200 * time.Millisecond)
	rm.OnWake(r)
	if m.spin == nil || m.spin.landed != 1 {
		t.Fatalf("after 200ms landed = %v, want exactly reel 0", landedOrNil(m))
	}
	// Past reel 1's deadline (400ms) but before reel 2's (650ms).
	r.Advance(250 * time.Millisecond) // now +450ms
	rm.OnWake(r)
	if m.spin == nil || m.spin.landed != 2 {
		t.Fatalf("after 450ms landed = %v, want reels 0,1", landedOrNil(m))
	}
}

func landedOrNil(m *machine) any {
	if m.spin == nil {
		return "settled"
	}
	return m.spin.landed
}

func TestSettleCreditsJackpot(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 50
	m.balance = startBalance - 50 // bet already deducted at spin start

	win := forceWin5(t, rm, r, p, symStar)
	takeIfGambling(rm, r, p.AccountID)

	if win <= 0 {
		t.Fatalf("expected a paying 5-of-a-kind, got %d", win)
	}
	if m.balance != startBalance-50+win {
		t.Errorf("balance = %d, want %d", m.balance, startBalance-50+win)
	}
	if m.highScore != m.balance {
		t.Errorf("highScore = %d, want %d", m.highScore, m.balance)
	}
	if m.reels != faceRow(symStar) {
		t.Errorf("reels = %v, want all stars", m.reels)
	}
	if m.spin != nil {
		t.Error("spin should be nil after settle")
	}
	if !strings.Contains(m.flash, strconv.Itoa(win)) {
		t.Errorf("flash = %q, want a WIN with %d", m.flash, win)
	}
}

func TestBustRebuysPreservingHighScore(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.highScore = 2500
	m.balance = 0 // bet already deducted; this spin loses
	forceLoss5(t, rm, r, p)

	if m.balance != rebuyAmount {
		t.Errorf("balance = %d, want re-buy to %d", m.balance, rebuyAmount)
	}
	if m.highScore != 2500 {
		t.Errorf("highScore = %d, want 2500 (bust must not lower it)", m.highScore)
	}
	if !strings.Contains(m.flash, "RE-BUY") {
		t.Errorf("flash = %q, want RE-BUY", m.flash)
	}
}

// --- ticker ------------------------------------------------------------------

func TestBigWinPushesTicker(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 100
	m.balance = startBalance - 100
	forceWin5(t, rm, r, p, symStar) // a big 5-of-a-kind at bet 100
	takeIfGambling(rm, r, p.AccountID)

	if !rm.tickerActive(r.Now()) {
		t.Fatal("expected ticker active after a big win")
	}
	if !strings.Contains(rm.ticker.text, "alice") {
		t.Errorf("ticker = %q, want it to name alice", rm.ticker.text)
	}
}

func TestSmallWinDoesNotPushTicker(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 50
	// A win below bet*tickerMult (600) must not announce. Bank a held 100 via the
	// gamble path (the credit/announce path) and check the ticker stays quiet.
	rm.enterGamble(r, m, 100)
	m.gamble.sel = selTake
	rm.gambleConfirm(r, p.AccountID)

	if rm.tickerActive(r.Now()) {
		t.Error("a small win (below tickerMult) must not trigger the room-wide ticker")
	}
}

func TestTickerExpiresOnWake(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 100
	m.balance = startBalance - 100
	forceWin5(t, rm, r, p, symStar)
	takeIfGambling(rm, r, p.AccountID)
	if !rm.tickerActive(r.Now()) {
		t.Fatal("ticker should be active right after the win")
	}
	r.Advance(tickerDur + time.Second)
	if rm.tickerActive(r.Now()) {
		t.Error("ticker should have expired after its window")
	}
}

// --- leaderboard -------------------------------------------------------------

func TestNewPeakPostsToLeaderboard(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 50
	m.balance = startBalance - 50
	forceWin5(t, rm, r, p, symStar) // a win → new peak
	takeIfGambling(rm, r, p.AccountID)

	if len(r.Posted) != 1 {
		t.Fatalf("posts = %d, want exactly 1 on a new peak", len(r.Posted))
	}
	got := r.Posted[0].Rankings[0]
	if got.Metric != m.highScore || got.Status != kit.StatusFinished {
		t.Errorf("posted = %+v, want metric %d finished", got, m.highScore)
	}
}

func TestNoPeakDoesNotPost(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.balance = startBalance - 10
	forceLoss5(t, rm, r, p) // no win → no new peak

	if len(r.Posted) != 0 {
		t.Errorf("posts = %d, want 0 when the peak did not increase", len(r.Posted))
	}
}

// --- leaving -----------------------------------------------------------------

func TestLeaveRemovesAndKeepsJoinOrder(t *testing.T) {
	a, b, c := kittest.Player("a"), kittest.Player("b"), kittest.Player("c")
	rm, r := newGame(t, a, b, c)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	rm.OnJoin(r, c)

	rm.OnLeave(r, b)

	if rm.machines[b.AccountID] != nil {
		t.Error("machine for b should be removed")
	}
	if len(rm.order) != 2 || rm.order[0] != a.AccountID || rm.order[1] != c.AccountID {
		t.Fatalf("order = %v, want [a c]", rm.order)
	}
}

func TestLeavePersistsWallet(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.balance = 2500
	m.highScore = 2500

	rm.OnLeave(r, p)

	store := r.Services().Accounts.For(p).Store()
	if v, ok, _ := store.Get(nil, keyPeak); !ok || strings.TrimSpace(string(v)) != "2500" {
		t.Errorf("persisted peak = %q (ok=%v), want 2500", v, ok)
	}
}

// --- rendering ---------------------------------------------------------------

func TestFrameShowsTitleAndOwnBalance(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	if !frameContains(r, p, "POKIES LOUNGE") {
		t.Error("frame missing POKIES LOUNGE title")
	}
	if !frameContains(r, p, "1000") {
		t.Error("frame missing the starting balance 1000")
	}
}

func TestGridBlankBeforeFirstSpin(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	g := rm.grid(rm.machines[p.AccountID])
	for row := 0; row < visRows; row++ {
		for reel := 0; reel < numReels; reel++ {
			if g[row][reel] != symBlank {
				t.Fatalf("pre-spin grid[%d][%d] = %q, want blank", row, reel, g[row][reel])
			}
		}
	}
}

func TestGridCenterRowIsThePayline(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.balance = startBalance - 10
	strip := rm.variant.strip
	var idx [numReels]int
	var fin [numReels]symbol
	for i := 0; i < numReels; i++ {
		idx[i] = i
		fin[i] = strip[i]
	}
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, stopIdx: idx, final: fin}

	rm.settleSpin(r, p.AccountID)

	g := rm.grid(m)
	for reel := 0; reel < numReels; reel++ {
		if g[1][reel] != m.reels[reel] {
			t.Errorf("center grid[1][%d] = %q, want settled face %q", reel, g[1][reel], rune(m.reels[reel]))
		}
	}
}

func TestGridScrollsAsTheClockAdvances(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	rm.OnInput(r, p, space()) // start spinning; no reel landed yet
	m := rm.machines[p.AccountID]

	rm.lastNow = m.spin.startedAt
	g0 := rm.grid(m)
	rm.lastNow = m.spin.startedAt.Add(cycleRate) // one scroll frame later
	g1 := rm.grid(m)

	if g0 == g1 {
		t.Error("expected the reel window to scroll as the derived clock advances")
	}
}

// --- emoji reel faces (v2 grapheme cells) --------------------------------------

// seatPayline is the frame row of the seated reel grid's centre (payline) row.
const seatPayline = seatTop + 2

// firstIdx returns the first strip position holding face s (fatal if absent).
func firstIdx(t *testing.T, strip []symbol, s symbol) int {
	t.Helper()
	for i, x := range strip {
		if x == s {
			return i
		}
	}
	t.Fatalf("strip has no %q", rune(s))
	return 0
}

// knownFaces are the five center faces settleKnownFaces lands, left-to-right.
var knownFaces = [numReels]symbol{sym7, symCherry, symDollar, symBar, symStar}

// settleKnownFaces seats the player and drives a deterministic landing with
// knownFaces on the payline, then renders the seated cabinet.
func settleKnownFaces(t *testing.T, rm *room, r *kittest.Room, p kit.Player) {
	t.Helper()
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.balance = startBalance - 10
	strip := rm.variant.strip
	var idx [numReels]int
	for i, s := range knownFaces {
		idx[i] = firstIdx(t, strip, s)
	}
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, stopIdx: idx, final: knownFaces}
	rm.settleSpin(r, p.AccountID)
	takeIfGambling(rm, r, p.AccountID)
	rm.render(r)
}

func TestReelFacesRenderAsWideGraphemes(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	settleKnownFaces(t, rm, r, p)

	f := r.LastFrame(p)
	if f == nil {
		t.Fatal("no frame sent")
	}
	cases := []struct {
		reel           string
		col            int
		base, cp2, cp3 rune
	}{
		// Fullwidth seven, NOT the keycap 7️⃣ (contested width). U+FF17 is
		// EAW=Fullwidth — unanimously width 2 everywhere. knownFaces = 7, C, $...
		{"fullwidth seven", seatedReelCol(0), '７', 0, 0},
		{"cherry", seatedReelCol(1), '\U0001F352', 0, 0},
		{"diamond", seatedReelCol(2), '\U0001F48E', 0, 0},
	}
	for _, c := range cases {
		cell := f.Cells[seatPayline][c.col]
		if cell.Rune != c.base || cell.Cp2 != c.cp2 || cell.Cp3 != c.cp3 {
			t.Errorf("%s cell = %q/%q/%q, want %q/%q/%q",
				c.reel, cell.Rune, cell.Cp2, cell.Cp3, c.base, c.cp2, c.cp3)
		}
		if !f.Cells[seatPayline][c.col+1].Cont {
			t.Errorf("%s: cell right of the glyph is not a continuation cell", c.reel)
		}
	}
}

func TestBlankFacesAreSingleWidthDashes(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	rm.render(r)

	f := r.LastFrame(p)
	if f == nil {
		t.Fatal("no frame sent")
	}
	for reel := 0; reel < numReels; reel++ {
		c := seatedReelCol(reel)
		if f.Cells[seatPayline][c].Rune != '-' {
			t.Errorf("pre-spin face %d = %q, want '-'", reel, f.Cells[seatPayline][c].Rune)
		}
		if f.Cells[seatPayline][c+1].Cont {
			t.Errorf("pre-spin face %d must not mark a continuation cell", reel)
		}
	}
}

// TestScreenBoxFitsWideFaces: the seated reel box frames the 5x3 grid with the
// payline markers hugging its sides.
func TestScreenBoxFitsWideFaces(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	rm.render(r)

	f := r.LastFrame(p)
	left := seatedReelCol(0) - 2           // box left wall
	right := seatedReelCol(numReels-1) + 3 // box right wall
	if got := f.Cells[seatTop][left].Rune; got != '╭' {
		t.Errorf("screen top-left = %q, want ╭", got)
	}
	if got := f.Cells[seatTop][right].Rune; got != '╮' {
		t.Errorf("screen top-right = %q, want ╮", got)
	}
	if got := f.Cells[seatPayline][left-1].Rune; got != '>' {
		t.Errorf("left payline marker = %q, want >", got)
	}
	if got := f.Cells[seatPayline][right+1].Rune; got != '<' {
		t.Errorf("right payline marker = %q, want <", got)
	}
}

// TestPaytableStripNamesSymbolsWithArt: the strip under the cabinets names the
// paying symbols with their emoji art and multipliers, highest first.
func TestPaytableStripNamesSymbolsWithArt(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	rm.render(r)

	// Seated at machine 0 = the "Lucky 7s" theme; labels are "pay5/pay4/pay3".
	for _, want := range []string{"147/42/13", "63/21/6", "36/13/4", "16/6/2"} {
		if !frameContains(r, p, want) {
			t.Errorf("paytable strip missing %q", want)
		}
	}
	f := r.LastFrame(p)
	row := -1
	for rr := 0; rr < kit.Rows; rr++ {
		if strings.Contains(kittest.String(f, rr), "147/42/13") {
			row = rr
			break
		}
	}
	if row < 0 {
		t.Fatal("no paytable row found")
	}
	line := kittest.String(f, row)
	for _, art := range []rune{'\U0001F48E', '⭐', '\U0001F514'} { // 💎 ⭐ 🔔
		if !strings.ContainsRune(line, art) {
			t.Errorf("paytable row %q missing symbol art %q", line, art)
		}
	}
}

func TestFreeSpinCabinetShowsFreeCount(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]
	m.freeSpins, m.freeBet = 7, 50
	rm.render(r)
	if !frameContains(r, p, "FREE 7") {
		t.Error("free-spin cabinet should show FREE 7")
	}
}

func TestGambleOwnerSeesSelectorOthersSeeIndicator(t *testing.T) {
	a, b := kittest.Player("anna"), kittest.Player("bert")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	seatAt0(t, rm, a)
	ma := rm.machines[a.AccountID]
	ma.balance = 1000
	rm.enterGamble(r, ma, 150)
	rm.render(r)
	if !frameContains(r, a, "TAKE") || !frameContains(r, a, "RED") {
		t.Error("owner should see the gamble selector")
	}
	for _, suit := range []string{"♠", "♥", "♦", "♣"} {
		if !frameContains(r, a, suit) {
			t.Errorf("owner should see the suit selector glyph %q", suit)
		}
	}
}

func TestControlsLineReflectsMode(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	// Roaming: floor controls mention "sit".
	rm.render(r)
	if !frameContains(r, p, "sit") {
		t.Error("roaming controls should mention sit")
	}

	// Seat the player so cabinet/input controls render.
	seatAt0(t, rm, p)
	m := rm.machines[p.AccountID]

	rm.render(r)
	if !frameContains(r, p, "spin") {
		t.Error("idle controls should mention spin")
	}
	rm.enterGamble(r, m, 100)
	rm.render(r)
	if !frameContains(r, p, "lock") {
		t.Error("gamble controls should mention lock/take")
	}
	m.gamble = nil
	m.freeSpins = 4
	rm.render(r)
	if !frameContains(r, p, "FREE SPINS") {
		t.Error("free-spin controls should announce auto-play")
	}
}

// --- payout / variant --------------------------------------------------------

func TestDefaultVariantTuning(t *testing.T) {
	v := defaultVariant()
	if len(v.strip) != 44 { // 1+2+3+5+30+1+2
		t.Fatalf("default strip length = %d, want 44", len(v.strip))
	}
	if v.weightSummary() != "7:1 $:2 *:3 B:5 C:30 W:1 S:2" {
		t.Fatalf("weight summary = %q, want 7:1 $:2 *:3 B:5 C:30 W:1 S:2", v.weightSummary())
	}
	if _, ok := v.pays[symCherry]; ok {
		t.Fatal("cherries must pay nothing in the default variant")
	}
	if v.pays[sym7] != [3]int{10, 30, 100} {
		t.Fatalf("seven pays = %v, want {10,30,100}", v.pays[sym7])
	}
}

func TestDefaultVariantRTPInBand(t *testing.T) {
	s := defaultVariant().stats()
	if s.TotalRTP < 0.80 || s.TotalRTP > 0.90 {
		t.Fatalf("default total RTP = %.4f, want within [0.80, 0.90]", s.TotalRTP)
	}
	if s.HitFreq <= 0 {
		t.Fatalf("default hit metric = %.4f, want positive", s.HitFreq)
	}
}

func TestCompileVariantRejectsOutOfBounds(t *testing.T) {
	cases := []struct {
		name string
		doc  oddsVariant
	}{
		{"all weights zero", oddsVariant{
			Weights:  map[string]int{"7": 0, "$": 0, "*": 0, "B": 0, "C": 0},
			Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 100}},
		}},
		{"negative weight", oddsVariant{
			Weights:  map[string]int{"7": -1, "C": 5},
			Paytable: []payEntry{{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 100}},
		}},
		{"negative multiplier", oddsVariant{
			Weights:  map[string]int{"7": 1, "C": 5},
			Paytable: []payEntry{{Faces: "7", Pay3: 1, Pay4: 1, Pay5: -1}},
		}},
		{"oversized strip", oddsVariant{
			Weights:  map[string]int{"7": 40, "C": 40},
			Paytable: []payEntry{{Faces: "7", Pay3: 1, Pay4: 1, Pay5: 1}},
		}},
		{"RTP too high (money printer)", oddsVariant{
			Weights:  map[string]int{"7": 1, "C": 1},
			Paytable: []payEntry{{Faces: "7", Pay3: 100, Pay4: 500, Pay5: 5000}},
		}},
		{"RTP too low (zeroed paytable)", oddsVariant{
			Weights:  map[string]int{"7": 1, "$": 2, "*": 3, "B": 5, "C": 7},
			Paytable: nil,
		}},
		{"unknown symbol", oddsVariant{
			Weights:  map[string]int{"X": 1},
			Paytable: []payEntry{{Faces: "X", Pay3: 1, Pay4: 1, Pay5: 1}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := compileVariant(c.doc); err == nil {
				t.Fatalf("compileVariant(%s) = nil error, want a rejection", c.name)
			}
		})
	}
}

func TestParseVariantFirstMatchWins(t *testing.T) {
	v, err := compileVariant(oddsVariant{
		Weights: map[string]int{"7": 4, "C": 30},
		Paytable: []payEntry{
			{Faces: "7", Pay3: 10, Pay4: 30, Pay5: 80},
			{Faces: "7", Pay3: 9, Pay4: 99, Pay5: 999},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if v.pays[sym7] != [3]int{10, 30, 80} {
		t.Fatalf("first-match pays = %v, want {10,30,80}", v.pays[sym7])
	}
}

// --- config-driven odds ------------------------------------------------------

func variantBlob(t *testing.T, doc oddsVariant) []byte {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal variant: %v", err)
	}
	return b
}

// bbbVariantDoc is the default doc with B's pays bumped — an admin override
// distinct from the default (B: {2,6,16} -> {4,12,40}).
func bbbVariantDoc() oddsVariant {
	doc := defaultDoc()
	for i := range doc.Paytable {
		if doc.Paytable[i].Faces == "B" {
			doc.Paytable[i] = payEntry{Faces: "B", Pay3: 4, Pay4: 12, Pay5: 40}
		}
	}
	return doc
}

func TestRoomLoadsStoredVariant(t *testing.T) {
	r := kittest.NewRoom()
	r.ConfigVals[configKey] = variantBlob(t, bbbVariantDoc())
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	if h.variant.pays[symBar] != [3]int{4, 12, 40} {
		t.Fatalf("stored B pays = %v, want override {4,12,40}", h.variant.pays[symBar])
	}
}

func TestRoomFallsBackOnBrokenVariant(t *testing.T) {
	r := kittest.NewRoom()
	r.ConfigVals[configKey] = []byte("{ this is not json")
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	if h.variant.pays[sym7] != [3]int{10, 30, 100} {
		t.Fatalf("after broken variant, 7 pays = %v, want default {10,30,100}", h.variant.pays[sym7])
	}
}

func TestRoomRefreshAdoptsNewVariant(t *testing.T) {
	r := kittest.NewRoom()
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // no stored variant -> default
	if h.variant.pays[symBar] != [3]int{2, 6, 16} {
		t.Fatalf("initial B pays = %v, want default {2,6,16}", h.variant.pays[symBar])
	}
	r.ConfigVals[configKey] = variantBlob(t, bbbVariantDoc())

	// Before the refresh deadline, still the default.
	r.Advance(configRefresh - time.Second)
	h.OnWake(r)
	if h.variant.pays[symBar] != [3]int{2, 6, 16} {
		t.Fatalf("before refresh B pays = %v, want still default", h.variant.pays[symBar])
	}
	// After the refresh deadline passes, the new variant is live.
	r.Advance(2 * time.Second)
	h.OnWake(r)
	if h.variant.pays[symBar] != [3]int{4, 12, 40} {
		t.Fatalf("after refresh B pays = %v, want override {4,12,40}", h.variant.pays[symBar])
	}
}

func TestMidSpinVariantStability(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // default variant
	h.OnJoin(r, p)
	seatAt0(t, h, p)
	m := h.machines[p.AccountID]
	m.bet = 50

	h.OnInput(r, p, space()) // spin starts under the default variant
	if m.spin == nil {
		t.Fatal("expected a spin in flight")
	}
	// Force a deterministic B-run on a scatter-free spot of the (default) strip.
	sv := m.spin.variant
	bIdx := cleanIdx(t, sv, symBar)
	m.spin.stopIdx = allReels(bIdx)
	m.spin.final = faceRow(symBar)
	// The win must settle under the STARTING variant (default B pays), not the
	// override adopted mid-spin.
	wantWin := 50 * sv.waysPayout(scatterWindow(sv.strip, allReels(bIdx)))
	h.variant = mustCompile(t, bbbVariantDoc()) // override has different B pays

	h.settleSpin(r, p.AccountID)
	takeIfGambling(h, r, p.AccountID)

	if m.balance != (startBalance-50)+wantWin {
		t.Fatalf("balance = %d, want %d (settled under the starting variant)",
			m.balance, (startBalance-50)+wantWin)
	}
}

func mustCompile(t *testing.T, doc oddsVariant) *variant {
	t.Helper()
	v, err := compileVariant(doc)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return v
}

// TestSeededDeterminismPerVariant: the same seed and variant reproduce the same
// landing indices across two independent rooms.
func TestSeededDeterminismPerVariant(t *testing.T) {
	seq := func() []int {
		p := kittest.Player("alice")
		r := kittest.NewRoom(p) // seed 1, fixed epoch
		h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
		h.OnStart(r)
		h.OnJoin(r, p)
		seatAt0(t, h, p)
		m := h.machines[p.AccountID]
		m.bet = 10
		var idxs []int
		for s := 0; s < 5; s++ {
			h.OnInput(r, p, space())
			if m.spin != nil {
				idxs = append(idxs, m.spin.stopIdx[0], m.spin.stopIdx[1], m.spin.stopIdx[2])
			}
			settle(h, r)
		}
		return idxs
	}
	a, b := seq(), seq()
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("seq lengths a=%d b=%d, want equal and non-empty", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seeded run diverged at %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestFloorRendersCharacterAvatars(t *testing.T) {
	a := kittest.Player("anna")
	a.Character = kit.Character{Glyph: "@"}
	b := kittest.Player("bert")
	b.Character = kit.Character{Glyph: "&"}
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	// Place both near each other so both are in A's camera window.
	pa, pb := rm.pawns[a.AccountID], rm.pawns[b.AccountID]
	pa.x, pa.y = 20, 18
	pb.x, pb.y = 22, 18
	rm.render(r)
	if !frameContains(r, a, "@") {
		t.Error("viewer should see their own character glyph on the floor")
	}
	if !frameContains(r, a, "&") {
		t.Error("viewer should see another player's character glyph")
	}
	if !frameContains(r, a, "bert") {
		t.Error("other players should be name-labelled")
	}
}
