package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// --- harness -----------------------------------------------------------------

func space() kit.Input   { return kit.Input{Kind: kit.InputRune, Rune: ' '} }
func keyUp() kit.Input   { return kit.Input{Kind: kit.InputKey, Key: kit.KeyUp} }
func keyDown() kit.Input { return kit.Input{Kind: kit.InputKey, Key: kit.KeyDown} }

// newGame builds a started room handler plus its driving kittest.Room.
func newGame(t *testing.T, players ...kit.Player) (*room, *kittest.Room) {
	t.Helper()
	r := kittest.NewRoom(players...)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	return h, r
}

// settle drives enough wake/clock cycles to land all three reels and settle a
// spin (reel 2 lands at +150ms+2*250ms = +650ms; advance well past it).
func settle(rm *room, r *kittest.Room) {
	for i := 0; i < 4; i++ {
		r.Advance(time.Second)
		rm.OnWake(r)
	}
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

func TestMetaIsBareName(t *testing.T) {
	m := Game{}.Meta()
	if m.Slug != "pokies" {
		t.Errorf("slug = %q, want bare name pokies (the platform adds the namespace)", m.Slug)
	}
	if m.MinPlayers != 1 || m.MaxPlayers != 5 {
		t.Errorf("players = %d..%d, want 1..5", m.MinPlayers, m.MaxPlayers)
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
	m := rm.machines[p.AccountID]
	m.bet = 10

	rm.OnInput(r, p, space())
	settle(rm, r)

	if m.spin != nil {
		t.Fatal("expected spin to have settled after wakes")
	}
	if !m.spun {
		t.Fatal("expected spun=true after first settle")
	}
	dv := defaultVariant()
	want := (startBalance - 10) + 10*dv.payout(m.reels)
	if m.balance != want {
		t.Fatalf("balance = %d, want %d (reels %v pay %dx)", m.balance, want, m.reels, dv.payout(m.reels))
	}
}

func TestReelsLandLeftToRightStaggered(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{sym7, sym7, sym7}}

	rm.settleSpin(r, p.AccountID)

	if m.balance != startBalance-50+25000 {
		t.Errorf("balance = %d, want %d", m.balance, startBalance-50+25000)
	}
	if m.highScore != m.balance {
		t.Errorf("highScore = %d, want %d", m.highScore, m.balance)
	}
	if m.reels != [3]symbol{sym7, sym7, sym7} {
		t.Errorf("reels = %v, want triple seven", m.reels)
	}
	if m.spin != nil {
		t.Error("spin should be nil after settle")
	}
	if !strings.Contains(m.flash, "25000") {
		t.Errorf("flash = %q, want a WIN with 25000", m.flash)
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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{sym7, symDollar, symStar}}

	rm.settleSpin(r, p.AccountID)

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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{symStar, symStar, symStar}} // 55x

	rm.settleSpin(r, p.AccountID)

	if !rm.tickerActive(r.Now()) {
		t.Fatal("expected ticker active after a 55x win")
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
	m.balance = startBalance - 50
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{symBar, symBar, symBar}} // 10x, below 12x

	rm.settleSpin(r, p.AccountID)

	if rm.tickerActive(r.Now()) {
		t.Error("a 10x win must not trigger the room-wide ticker")
	}
}

func TestTickerExpiresOnWake(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	m.bet = 100
	m.balance = startBalance - 100
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{symStar, symStar, symStar}}
	rm.settleSpin(r, p.AccountID)
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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{sym7, sym7, sym7}} // jackpot, new peak

	rm.settleSpin(r, p.AccountID)

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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, final: [3]symbol{sym7, symDollar, symStar}} // loss

	rm.settleSpin(r, p.AccountID)

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

	if !frameContains(r, p, "POKIES") {
		t.Error("frame missing POKIES title")
	}
	if !frameContains(r, p, "1000") {
		t.Error("frame missing the starting balance 1000")
	}
	if !frameContains(r, p, "alice") {
		t.Error("frame missing the player's machine label")
	}
}

func TestFrameShowsAllMachines(t *testing.T) {
	a, b := kittest.Player("anna"), kittest.Player("bert")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)

	if !frameContains(r, a, "anna") || !frameContains(r, a, "bert") {
		t.Error("expected both machines to render for a viewer")
	}
}

func TestGridBlankBeforeFirstSpin(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	g := rm.grid(rm.machines[p.AccountID])
	for row := 0; row < 3; row++ {
		for reel := 0; reel < 3; reel++ {
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
	m.spin = &spinState{startedAt: r.Now(), variant: rm.variant, stopIdx: [3]int{4, 8, 1}}
	m.spin.final = [3]symbol{strip[4], strip[8], strip[1]}

	rm.settleSpin(r, p.AccountID)

	g := rm.grid(m)
	for reel := 0; reel < 3; reel++ {
		if g[1][reel] != m.reels[reel] {
			t.Errorf("center grid[1][%d] = %q, want settled face %q", reel, g[1][reel], rune(m.reels[reel]))
		}
	}
}

func TestGridScrollsAsTheClockAdvances(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
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

// soloCardCol is the left column of the (single) machine cabinet when exactly
// one player has joined: one card centered on the canvas.
func soloCardCol() int { return (kit.Cols - cardW) / 2 }

// soloFaceCol is the frame column of reel face `reel` (0..2) on the solo card:
// faces are width-2 glyphs packed at screen cols sx+1, sx+3, sx+5.
func soloFaceCol(reel int) int { return soloCardCol() + 2 + 1 + reel*2 }

// settleKnownFaces drives a deterministic landing: 7 on reel 0, cherry on reel
// 1, dollar on reel 2 (indices on the default strip), then renders.
func settleKnownFaces(t *testing.T, rm *room, r *kittest.Room, p kit.Player) {
	t.Helper()
	m := rm.machines[p.AccountID]
	m.bet = 10
	m.balance = startBalance - 10
	m.spin = &spinState{
		startedAt: r.Now(),
		variant:   rm.variant,
		stopIdx:   [3]int{0, 11, 1}, // default strip: [0]=7, [11]=C, [1]=$
		final:     [3]symbol{sym7, symCherry, symDollar},
	}
	rm.settleSpin(r, p.AccountID)
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
	payline := cardTop + 3
	cases := []struct {
		reel           string
		col            int
		base, cp2, cp3 rune
	}{
		// Fullwidth seven, NOT the keycap 7️⃣: keycap width is contested
		// (runewidth/uniseg say 1, x/ansi says 2, terminals split) and a
		// narrow-rendering viewer desyncs every column to its right. U+FF17
		// is EAW=Fullwidth — unanimously width 2 everywhere.
		{"fullwidth seven", soloFaceCol(0), '７', 0, 0},
		{"cherry", soloFaceCol(1), '\U0001F352', 0, 0},
		{"diamond", soloFaceCol(2), '\U0001F48E', 0, 0},
	}
	for _, c := range cases {
		cell := f.Cells[payline][c.col]
		if cell.Rune != c.base || cell.Cp2 != c.cp2 || cell.Cp3 != c.cp3 {
			t.Errorf("%s cell = %q/%q/%q, want %q/%q/%q",
				c.reel, cell.Rune, cell.Cp2, cell.Cp3, c.base, c.cp2, c.cp3)
		}
		if !f.Cells[payline][c.col+1].Cont {
			t.Errorf("%s: cell right of the glyph is not a continuation cell", c.reel)
		}
	}
}

func TestBlankFacesAreSingleWidthDashes(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	f := r.LastFrame(p)
	if f == nil {
		t.Fatal("no frame sent")
	}
	payline := cardTop + 3
	for reel := 0; reel < 3; reel++ {
		c := soloFaceCol(reel)
		if f.Cells[payline][c].Rune != '-' {
			t.Errorf("pre-spin face %d = %q, want '-'", reel, f.Cells[payline][c].Rune)
		}
		if f.Cells[payline][c+1].Cont {
			t.Errorf("pre-spin face %d must not mark a continuation cell", reel)
		}
	}
}

// TestScreenBoxFitsWideFaces: the reel screen box is 8 wide (three packed
// width-2 faces) with the payline markers hugging its sides.
func TestScreenBoxFitsWideFaces(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	f := r.LastFrame(p)
	sx := soloCardCol() + 2
	if got := f.Cells[cardTop+1][sx].Rune; got != '╭' {
		t.Errorf("screen top-left = %q, want ╭", got)
	}
	if got := f.Cells[cardTop+1][sx+7].Rune; got != '╮' {
		t.Errorf("screen top-right = %q, want ╮ at sx+7 (8-wide box)", got)
	}
	if got := f.Cells[cardTop+3][sx-1].Rune; got != '>' {
		t.Errorf("left payline marker = %q, want >", got)
	}
	if got := f.Cells[cardTop+3][sx+8].Rune; got != '<' {
		t.Errorf("right payline marker = %q, want < at sx+8", got)
	}
}

// TestPaytableStripNamesSymbolsWithArt: the strip under the cabinets names the
// paying symbols with their emoji art and multipliers, highest first.
func TestPaytableStripNamesSymbolsWithArt(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)

	for _, want := range []string{"x500", "x150", "x55", "x10"} {
		if !frameContains(r, p, want) {
			t.Errorf("paytable strip missing %q", want)
		}
	}
	f := r.LastFrame(p)
	row := -1
	for rr := 0; rr < kit.Rows; rr++ {
		if strings.Contains(kittest.String(f, rr), "x500") {
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

// --- payout / variant --------------------------------------------------------

func TestPayout(t *testing.T) {
	v := defaultVariant()
	cases := []struct {
		name  string
		reels [3]symbol
		want  int
	}{
		{"triple seven jackpot", [3]symbol{sym7, sym7, sym7}, 500},
		{"triple dollar", [3]symbol{symDollar, symDollar, symDollar}, 150},
		{"triple star", [3]symbol{symStar, symStar, symStar}, 55},
		{"triple bar", [3]symbol{symBar, symBar, symBar}, 10},
		{"triple cherry pays nothing", [3]symbol{symCherry, symCherry, symCherry}, 0},
		{"two cherries pay nothing", [3]symbol{symCherry, symCherry, sym7}, 0},
		{"no match", [3]symbol{sym7, symDollar, symStar}, 0},
		{"pair is nothing", [3]symbol{symBar, symBar, sym7}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.payout(c.reels); got != c.want {
				t.Fatalf("payout(%v) = %d, want %d", c.reels, got, c.want)
			}
		})
	}
}

func TestDefaultVariantTuning(t *testing.T) {
	v := defaultVariant()
	if len(v.strip) != 18 {
		t.Fatalf("default strip length = %d, want 18 (1+2+3+5+7)", len(v.strip))
	}
	if v.weightSummary() != "7:1 $:2 *:3 B:5 C:7" {
		t.Fatalf("weight summary = %q, want 7:1 $:2 *:3 B:5 C:7", v.weightSummary())
	}
	if _, ok := v.triples[symCherry]; ok {
		t.Fatal("cherries must pay nothing in the default variant")
	}
}

func TestDefaultVariantRTPIsAroundSeventyFivePercent(t *testing.T) {
	rtp, hitFreq := defaultVariant().stats()
	if rtp < 0.70 || rtp > 0.80 {
		t.Fatalf("default RTP = %.4f, want within [0.70, 0.80] (house edge)", rtp)
	}
	if hitFreq <= 0 || hitFreq > 0.10 {
		t.Fatalf("default hit frequency = %.4f, want a small positive share (high variance)", hitFreq)
	}
}

func TestCompileVariantRejectsOutOfBounds(t *testing.T) {
	cases := []struct {
		name string
		doc  oddsVariant
	}{
		{"all weights zero", oddsVariant{
			Weights:  map[string]int{"7": 0, "$": 0, "*": 0, "B": 0, "C": 0},
			Paytable: []payEntry{{Faces: "7", Multiplier: 10}},
		}},
		{"negative weight", oddsVariant{
			Weights:  map[string]int{"7": -1, "C": 5},
			Paytable: []payEntry{{Faces: "7", Multiplier: 10}},
		}},
		{"negative multiplier", oddsVariant{
			Weights:  map[string]int{"7": 1, "C": 5},
			Paytable: []payEntry{{Faces: "7", Multiplier: -1}},
		}},
		{"oversized strip", oddsVariant{
			Weights:  map[string]int{"7": 40, "C": 40},
			Paytable: []payEntry{{Faces: "7", Multiplier: 1}},
		}},
		{"RTP too high (money printer)", oddsVariant{
			Weights:  map[string]int{"7": 1},
			Paytable: []payEntry{{Faces: "7", Multiplier: 10}},
		}},
		{"RTP too low (zeroed paytable)", oddsVariant{
			Weights:  map[string]int{"7": 1, "$": 2, "*": 3, "B": 5, "C": 7},
			Paytable: nil,
		}},
		{"unknown symbol", oddsVariant{
			Weights:  map[string]int{"X": 1},
			Paytable: []payEntry{{Faces: "X", Multiplier: 1}},
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
		Weights: map[string]int{"7": 2, "C": 8},
		Paytable: []payEntry{
			{Faces: "7", Multiplier: 50},
			{Faces: "7", Multiplier: 999},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := v.payout([3]symbol{sym7, sym7, sym7}); got != 50 {
		t.Fatalf("first-match multiplier = %d, want 50", got)
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

func bbbVariantDoc() oddsVariant {
	doc := defaultDoc()
	for i := range doc.Paytable {
		if doc.Paytable[i].Faces == "B" {
			doc.Paytable[i].Multiplier = 20
		}
	}
	return doc
}

func TestRoomLoadsStoredVariant(t *testing.T) {
	r := kittest.NewRoom()
	r.ConfigVals[configKey] = variantBlob(t, bbbVariantDoc())
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	if got := h.variant.payout([3]symbol{symBar, symBar, symBar}); got != 20 {
		t.Fatalf("stored B B B = %d, want 20 (override)", got)
	}
}

func TestRoomFallsBackOnBrokenVariant(t *testing.T) {
	r := kittest.NewRoom()
	r.ConfigVals[configKey] = []byte("{ this is not json")
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r)
	if got := h.variant.payout([3]symbol{sym7, sym7, sym7}); got != 500 {
		t.Fatalf("after broken variant, 7 7 7 = %d, want default 500", got)
	}
}

func TestRoomRefreshAdoptsNewVariant(t *testing.T) {
	r := kittest.NewRoom()
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // no stored variant -> default
	if got := h.variant.payout([3]symbol{symBar, symBar, symBar}); got != 10 {
		t.Fatalf("initial B B B = %d, want default 10", got)
	}
	r.ConfigVals[configKey] = variantBlob(t, bbbVariantDoc())

	// Before the refresh deadline, still the default.
	r.Advance(configRefresh - time.Second)
	h.OnWake(r)
	if got := h.variant.payout([3]symbol{symBar, symBar, symBar}); got != 10 {
		t.Fatalf("before refresh B B B = %d, want still 10", got)
	}
	// After the refresh deadline passes, the new variant is live.
	r.Advance(2 * time.Second)
	h.OnWake(r)
	if got := h.variant.payout([3]symbol{symBar, symBar, symBar}); got != 20 {
		t.Fatalf("after refresh B B B = %d, want 20", got)
	}
}

func TestMidSpinVariantStability(t *testing.T) {
	p := kittest.Player("alice")
	r := kittest.NewRoom(p)
	h := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	h.OnStart(r) // default variant
	h.OnJoin(r, p)
	m := h.machines[p.AccountID]
	m.bet = 50

	h.OnInput(r, p, space()) // spin starts under the default variant
	if m.spin == nil {
		t.Fatal("expected a spin in flight")
	}
	// Force a deterministic B B B landing on the (default) strip.
	bIdx := -1
	for i, s := range m.spin.variant.strip {
		if s == symBar {
			bIdx = i
			break
		}
	}
	if bIdx < 0 {
		t.Fatal("default strip has no bar symbol")
	}
	m.spin.stopIdx = [3]int{bIdx, bIdx, bIdx}
	m.spin.final = [3]symbol{symBar, symBar, symBar}

	// Simulate an admin save having been adopted mid-spin (B B B = 20).
	h.variant = mustCompile(t, bbbVariantDoc())

	h.settleSpin(r, p.AccountID)

	// The spin must pay 50 × 10 (the variant it STARTED under), not × 20.
	if m.flash == "" || m.balance != (startBalance-50)+50*10 {
		t.Fatalf("balance = %d, want %d (settled under the starting variant's 10x)",
			m.balance, (startBalance-50)+50*10)
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
