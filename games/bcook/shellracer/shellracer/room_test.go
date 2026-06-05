package shellracer

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit"
	"github.com/shellcade/kit/kittest"
)

// --- test harness -----------------------------------------------------------

// driver wraps a kittest.Room and the game Handler, managing the roster the way
// the native TestRoom did (kittest's roster is a plain slice the test owns).
type driver struct {
	r  *kittest.Room
	rm *room
}

func newDriver(mode kit.Mode, capacity int) *driver {
	r := kittest.NewRoom()
	r.Cfg = kit.RoomConfig{Mode: mode, Capacity: capacity, MinPlayers: 1, Seed: 1, SeedSet: true}
	rm := newRoom(r.Cfg, r.Services())
	rm.OnStart(r)
	return &driver{r: r, rm: rm}
}

func player(id string) kit.Player {
	return kit.Player{AccountID: id, Handle: id, Kind: kit.KindMember, Conn: "conn-" + id}
}

func (d *driver) join(p kit.Player) {
	d.r.Players = append(d.r.Players, p)
	d.rm.OnJoin(d.r, p)
}

// leave mirrors the ABI's leave contract (ABI.md §2): during the OnLeave
// callback the departed player is still carried in the roster (as the final
// entry); the host removes them only after the callback returns.
func (d *driver) leave(p kit.Player) {
	// Move the leaver to the end of the roster, as the host does.
	for i, m := range d.r.Players {
		if m.AccountID == p.AccountID {
			d.r.Players = append(d.r.Players[:i], d.r.Players[i+1:]...)
			break
		}
	}
	d.r.Players = append(d.r.Players, p)
	d.rm.OnLeave(d.r, p)
	// After the callback the leaver is no longer a member.
	d.r.Players = d.r.Players[:len(d.r.Players)-1]
}

func (d *driver) input(p kit.Player, in kit.Input) { d.rm.OnInput(d.r, p, in) }

// advance moves the virtual clock then fires a wake, mirroring the native
// TestRoom.Advance (which drained timers). A single wake suffices because the
// game compares against absolute deadlines, not per-wake accumulation.
func (d *driver) advance(dd time.Duration) {
	d.r.Advance(dd)
	d.rm.OnWake(d.r)
}

func runeIn(r rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: r} }
func keyIn(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}

// soloDriver boots a solo race already in the racing phase.
func soloDriver(t *testing.T) (*driver, kit.Player) {
	t.Helper()
	d := newDriver(kit.ModeSolo, 1)
	a := player("a")
	d.join(a)
	if d.rm.phase != phRacing {
		t.Fatalf("solo room phase=%q, want racing", d.rm.phase)
	}
	return d, a
}

// --- behavioral tests (ported from the native room_test.go) ------------------

// During the race the game publishes CtxText (so letters incl. q/j/k are typed,
// not navigation); after results it returns to CtxNav.
func TestPublishesTextContextWhileRacing(t *testing.T) {
	d, _ := soloDriver(t)
	if d.r.InputCtx != kit.CtxText {
		t.Errorf("racing InputCtx = %v, want CtxText", d.r.InputCtx)
	}
	d.rm.enterResults(d.r)
	if d.r.InputCtx != kit.CtxNav {
		t.Errorf("results InputCtx = %v, want CtxNav", d.r.InputCtx)
	}
}

// solo enters racing immediately; typing the passage correctly finishes the
// race and settles with a finished result after the results hold.
func TestSoloRaceFinishes(t *testing.T) {
	d, a := soloDriver(t)
	// give the WPM clock a non-zero elapsed so the finish snapshot is sane
	d.advance(2 * time.Second)
	for _, r := range d.rm.passage {
		d.input(a, runeIn(r))
	}
	if d.rm.st[a.AccountID].status != kit.StatusFinished {
		t.Fatalf("status=%v after typing passage, want finished", d.rm.st[a.AccountID].status)
	}
	// after the results hold, the room settles
	d.advance(resultsDur + time.Second)
	if d.r.Ended == nil {
		t.Fatal("room did not settle after results hold")
	}
	res := *d.r.Ended
	if len(res.Rankings) != 1 || res.Rankings[0].Status != kit.StatusFinished {
		t.Fatalf("result=%+v, want one finished player", res.Rankings)
	}
}

// server-authoritative validation: a wrong key records an error and blocks the
// cursor; N errors need N backspaces.
func TestValidationErrorsAndBackspace(t *testing.T) {
	d, a := soloDriver(t)
	ps := d.rm.st[a.AccountID]

	first := d.rm.passage[0]
	wrong := first + 1 // a rune that differs from passage[0]

	d.input(a, runeIn(wrong))
	d.input(a, runeIn(wrong))
	if ps.cursor != 0 {
		t.Fatalf("cursor=%d after 2 wrong keys, want 0", ps.cursor)
	}
	if ps.errorsTotal != 2 || ps.outstanding != 2 {
		t.Fatalf("errorsTotal=%d outstanding=%d, want 2/2", ps.errorsTotal, ps.outstanding)
	}
	// a correct-looking key while errors outstanding is still an error
	d.input(a, runeIn(first))
	if ps.cursor != 0 || ps.outstanding != 3 {
		t.Fatalf("cursor=%d outstanding=%d, want 0/3 (advance blocked by errors)", ps.cursor, ps.outstanding)
	}
	// backspaces clear outstanding errors before the cursor can advance
	d.input(a, keyIn(kit.KeyBackspace))
	d.input(a, keyIn(kit.KeyBackspace))
	d.input(a, keyIn(kit.KeyBackspace))
	if ps.outstanding != 0 {
		t.Fatalf("outstanding=%d after 3 backspaces, want 0", ps.outstanding)
	}
	d.input(a, runeIn(first))
	if ps.cursor != 1 {
		t.Fatalf("cursor=%d after correct key, want 1", ps.cursor)
	}
}

// pre-race keystrokes are dropped (quick room in lobby phase).
func TestPreRaceKeystrokesDropped(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a := player("a")
	d.join(a) // single quick player -> still lobby, not racing
	if d.rm.phase != phLobby {
		t.Fatalf("phase=%q, want lobby (lone quick player)", d.rm.phase)
	}
	d.input(a, runeIn(d.rm.passage[0]))
	if d.rm.st[a.AccountID].cursor != 0 {
		t.Fatal("pre-race keystroke advanced the cursor")
	}
}

// two players in one quick room can BOTH type and advance independently after
// the countdown — regression for "only one player could type".
func TestTwoPlayersBothType(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a) // lone quick player -> lobby
	d.join(b) // 2nd join -> countdown
	if d.rm.phase != phCountdown {
		t.Fatalf("phase=%q after 2nd join, want countdown", d.rm.phase)
	}
	d.advance(countdownDur + time.Second) // -> racing
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after countdown, want racing", d.rm.phase)
	}

	// both type the first character correctly; both cursors must advance
	d.input(a, runeIn(d.rm.passage[0]))
	d.input(b, runeIn(d.rm.passage[0]))
	if d.rm.st[a.AccountID].cursor != 1 {
		t.Fatalf("player A cursor=%d after typing, want 1", d.rm.st[a.AccountID].cursor)
	}
	if d.rm.st[b.AccountID].cursor != 1 {
		t.Fatalf("player B cursor=%d after typing, want 1 (could B not type?)", d.rm.st[b.AccountID].cursor)
	}

	// they progress independently
	d.input(a, runeIn(d.rm.passage[1]))
	if d.rm.st[a.AccountID].cursor != 2 || d.rm.st[b.AccountID].cursor != 1 {
		t.Fatalf("independent typing broke: A=%d B=%d, want A=2 B=1", d.rm.st[a.AccountID].cursor, d.rm.st[b.AccountID].cursor)
	}

	// each player's frame was composed for both viewers
	if d.r.LastFrame(a) == nil {
		t.Fatal("no frame for A")
	}
	if d.r.LastFrame(b) == nil {
		t.Fatal("no frame for B")
	}
}

// second join starts a 10s countdown; capacity start is immediate.
func TestQuickCountdownOnSecondJoin(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	d.join(player("a"))
	d.join(player("b"))
	if d.rm.phase != phCountdown {
		t.Fatalf("phase=%q after 2nd join, want countdown", d.rm.phase)
	}
	d.advance(countdownDur + time.Second)
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after countdown, want racing", d.rm.phase)
	}
}

// reaching capacity starts the race immediately (no countdown).
func TestCapacityStartsImmediately(t *testing.T) {
	d := newDriver(kit.ModeQuick, 2)
	d.join(player("a"))
	if d.rm.phase != phLobby {
		t.Fatalf("phase=%q after 1 join, want lobby", d.rm.phase)
	}
	d.join(player("b")) // capacity 2 reached -> racing now
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q at capacity, want racing", d.rm.phase)
	}
}

// the quick solo fallback: a lone quick player races after the grace window.
func TestQuickSoloFallback(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	d.join(player("a"))
	if d.rm.phase != phLobby {
		t.Fatalf("phase=%q, want lobby", d.rm.phase)
	}
	d.advance(graceWindow + time.Second)
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after grace window, want racing", d.rm.phase)
	}
}

// AFK timeout: an idle racer is dropped DNF on the wake past the timeout.
func TestAFKTimeoutDropsRacer(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second) // -> racing
	// A keeps typing; B goes idle.
	d.input(a, runeIn(d.rm.passage[0]))
	// keep A alive while B's idle clock runs out
	for elapsed := time.Duration(0); elapsed < afkTimeout+5*time.Second; elapsed += 5 * time.Second {
		d.r.Advance(5 * time.Second)
		d.input(a, runeIn(d.rm.passage[d.rm.st[a.AccountID].cursor]))
		d.rm.OnWake(d.r)
	}
	if d.rm.st[b.AccountID].status != kit.StatusDNF {
		t.Fatalf("B status=%v after AFK timeout, want DNF", d.rm.st[b.AccountID].status)
	}
	if !d.rm.st[a.AccountID].playing() {
		t.Fatalf("A should still be playing (kept typing)")
	}
}

// race cap: an unfinished race ends in results after the max duration.
func TestRaceCapEndsRace(t *testing.T) {
	d, _ := soloDriver(t)
	d.advance(maxRaceDur + time.Second)
	if d.rm.phase != phResults {
		t.Fatalf("phase=%q after race cap, want results", d.rm.phase)
	}
}

// finish ranking: finishers rank above DNF, ordered by net WPM descending.
func TestRankingFinishersAboveDNF(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second) // racing
	d.advance(2 * time.Second)            // elapsed for a non-zero WPM

	// A finishes the passage; B types nothing (will be DNF).
	for _, r := range d.rm.passage {
		d.input(a, runeIn(r))
	}
	if d.rm.st[a.AccountID].status != kit.StatusFinished {
		t.Fatalf("A status=%v, want finished", d.rm.st[a.AccountID].status)
	}
	// B times out / race resolves to results.
	d.advance(stragglerDur + time.Second)
	if d.rm.phase != phResults {
		t.Fatalf("phase=%q, want results", d.rm.phase)
	}
	res := d.rm.result
	if len(res.Rankings) != 2 {
		t.Fatalf("rankings=%d, want 2", len(res.Rankings))
	}
	if res.Rankings[0].Player.AccountID != "a" || res.Rankings[0].Status != kit.StatusFinished {
		t.Fatalf("rank 1 = %+v, want finished A", res.Rankings[0])
	}
	if res.Rankings[1].Player.AccountID != "b" || res.Rankings[1].Status != kit.StatusDNF {
		t.Fatalf("rank 2 = %+v, want DNF B", res.Rankings[1])
	}
}

// the anti-cheat flag: a finished result above the threshold is flagged.
func TestAntiCheatFlag(t *testing.T) {
	d, a := soloDriver(t)
	// force a tiny threshold via config and refresh
	d.r.ConfigVals["flag-wpm"] = []byte("1")
	d.rm.loadConfig(d.r)
	if d.rm.flagThreshold != 1 {
		t.Fatalf("flagThreshold=%d, want 1 from config", d.rm.flagThreshold)
	}
	// finish quickly so net WPM is well above 1
	d.advance(time.Second + 500*time.Millisecond)
	for _, r := range d.rm.passage {
		d.input(a, runeIn(r))
	}
	if d.rm.phase != phResults {
		// allDone after finish should have moved us to results
		t.Fatalf("phase=%q after finishing, want results", d.rm.phase)
	}
	if got := d.rm.result.Rankings[0].Status; got != kit.StatusFlagged {
		t.Fatalf("status=%v with threshold 1 and a real finish, want flagged", got)
	}
}

// leave during a race snapshots the player DNF and resolves results when the
// room empties of live players.
func TestLeaveResolvesRace(t *testing.T) {
	d, a := soloDriver(t)
	d.advance(2 * time.Second)
	d.leave(a)
	if got := d.rm.st[a.AccountID].status; got != kit.StatusDNF {
		t.Fatalf("left player status=%v, want DNF", got)
	}
	if d.rm.phase != phResults {
		t.Fatalf("phase=%q after sole racer left, want results", d.rm.phase)
	}
}

// pressing Enter on the results screen ends the room early.
func TestResultsEnterEnds(t *testing.T) {
	d, a := soloDriver(t)
	d.advance(2 * time.Second)
	for _, r := range d.rm.passage {
		d.input(a, runeIn(r))
	}
	if d.rm.phase != phResults {
		t.Fatalf("phase=%q, want results", d.rm.phase)
	}
	d.input(a, keyIn(kit.KeyEnter))
	if d.r.Ended == nil {
		t.Fatal("Enter on results did not end the room")
	}
}

// passage selection is deterministic for a given seed (room-seeded RNG).
func TestPassageDeterministic(t *testing.T) {
	d1 := newDriver(kit.ModeSolo, 1)
	d2 := newDriver(kit.ModeSolo, 1)
	if d1.rm.ptext != d2.rm.ptext {
		t.Fatalf("same seed picked different passages:\n  %q\n  %q", d1.rm.ptext, d2.rm.ptext)
	}
}
