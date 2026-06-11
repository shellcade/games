package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// newTestRoom builds a started room driven by a kittest.Room (deterministic
// clock + rng), with no players joined yet.
func newTestRoom(t *testing.T, handles ...string) (*room, *kittest.Room) {
	t.Helper()
	players := make([]kit.Player, len(handles))
	for i, h := range handles {
		players[i] = kittest.Player(h)
	}
	tr := kittest.NewRoom(players...)
	rm := newRoom(tr.Cfg, tr.Services())
	rm.OnStart(tr)
	return rm, tr
}

func keyRune(ru rune) kit.Input { return kit.Input{Kind: kit.InputRune, Rune: ru} }
func keyNamed(k kit.Key) kit.Input {
	return kit.Input{Kind: kit.InputKey, Key: k}
}

// fillRow fills an entire grid row with garbage so a piece dropped into the gap
// completes it. Returns nothing; mutates the well.
func fillRowExcept(w *well, row, gap int) {
	for c := 0; c < wellW; c++ {
		if c == gap {
			continue
		}
		w.grid[row][c] = garbageCell
	}
}

func TestStartSpawnsPieceAndNext(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	if !w.alive {
		t.Fatal("well should be alive after join")
	}
	if !w.hasPiece {
		t.Fatal("a piece should be falling after start")
	}
	if w.next < 0 || w.next >= len(pieces) {
		t.Fatalf("next piece index out of range: %d", w.next)
	}
}

func TestDriveNoPanic(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	inputs := []kit.Input{
		keyNamed(kit.KeyLeft), keyNamed(kit.KeyRight), keyNamed(kit.KeyDown),
		keyNamed(kit.KeyUp), keyRune('z'), keyRune('x'), keyRune(' '),
	}
	for i := 0; i < 600; i++ {
		p := tr.Players[i%2]
		rm.OnInput(tr, p, inputs[i%len(inputs)])
		tr.Advance(50 * time.Millisecond)
		rm.OnWake(tr)
		// Active piece, when present, must stay within the well bounds.
		for _, id := range rm.order {
			w := rm.wells[id]
			if !w.hasPiece {
				continue
			}
			for _, c := range w.cur.cells(pieces) {
				if c[1] < 0 || c[1] >= wellW || c[0] >= wellH {
					t.Fatalf("piece cell out of bounds: %v", c)
				}
			}
		}
	}
}

func TestRotationCyclesAndIsLegal(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	// Force a Tee in open space mid-well.
	w.cur = active{kind: pieceIndex("Tee"), rot: 0, row: 4, col: 3}
	start := w.cur.rot
	for i := 0; i < 4; i++ {
		if !rm.tryRotate(w, +1) {
			t.Fatalf("rotation %d should be legal in open space", i)
		}
	}
	if w.cur.rot != start {
		t.Fatalf("4 cw rotations should return to start, got rot=%d", w.cur.rot)
	}
}

func TestWallKickFromLeftWall(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	// A vertical Bar flush against the left wall; rotating to horizontal would
	// poke past col 0 from its anchor — the kick must shove it right so it fits.
	w.cur = active{kind: pieceIndex("Bar"), rot: 1, row: 2, col: -1}
	if rm.collides(w, w.cur) {
		t.Fatal("setup: vertical bar at col -1 (anchor col +1 cells) should be legal")
	}
	if !rm.tryRotate(w, +1) {
		t.Fatal("rotation against the left wall should succeed via a wall kick")
	}
	for _, c := range w.cur.cells(pieces) {
		if c[1] < 0 {
			t.Fatalf("kicked piece still pokes past the left wall: %v", c)
		}
	}
}

func TestLineClearDetectionAndScore(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	w.level = 0

	// Pre-fill the bottom row except one column, then drop a single cell into it.
	fillRowExcept(w, wellH-1, 0)
	full := rm.fullRows(w)
	if len(full) != 0 {
		t.Fatalf("row should not be full before filling the gap: %v", full)
	}
	w.grid[wellH-1][0] = garbageCell
	full = rm.fullRows(w)
	if len(full) != 1 || full[0] != wellH-1 {
		t.Fatalf("expected bottom row full, got %v", full)
	}

	before := w.score
	w.clearing = full
	rm.collapseRows(tr, w)
	if w.lines != 1 {
		t.Fatalf("expected 1 line cleared, got %d", w.lines)
	}
	if got := w.score - before; got < lineScore[1] {
		t.Fatalf("score gain %d, want >= %d for a single clear", got, lineScore[1])
	}
	// The cleared row must be gone (bottom row empty again).
	for c := 0; c < wellW; c++ {
		if w.grid[wellH-1][c] != cellEmpty {
			t.Fatalf("bottom row not cleared at col %d", c)
		}
	}
}

func TestMultiClearScoresMore(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	w.level = 0
	w.score = 0
	w.clearing = []int{wellH - 1, wellH - 2}
	// Fill those rows so removal is well-defined.
	for c := 0; c < wellW; c++ {
		w.grid[wellH-1][c] = garbageCell
		w.grid[wellH-2][c] = garbageCell
	}
	rm.collapseRows(tr, w)
	if w.score < lineScore[2] {
		t.Fatalf("double clear scored %d, want >= %d", w.score, lineScore[2])
	}
	if lineScore[2] <= 2*lineScore[1] {
		t.Fatal("design invariant: a double should out-score two singles")
	}
}

func TestGarbageTargetsTallestRival(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	a := rm.wells[tr.Players[0].AccountID]
	b := rm.wells[tr.Players[1].AccountID]
	c := rm.wells[tr.Players[2].AccountID]
	// Clear stacks, then make cleo the clear leader.
	for _, w := range []*well{a, b, c} {
		for r := 0; r < wellH; r++ {
			for cc := 0; cc < wellW; cc++ {
				w.grid[r][cc] = cellEmpty
			}
		}
	}
	// bob: short stack; cleo: tall stack.
	b.grid[wellH-1][0] = garbageCell
	for r := wellH - 6; r < wellH; r++ {
		c.grid[r][0] = garbageCell
	}
	if c.height() <= b.height() {
		t.Fatal("setup: cleo should be taller than bob")
	}
	rm.sendGarbage(tr, a, 3)
	if c.pendingGarbage != 3 {
		t.Fatalf("tallest rival (cleo) should be targeted with 3 rows, got %d", c.pendingGarbage)
	}
	if b.pendingGarbage != 0 {
		t.Fatalf("shorter rival (bob) should get no garbage, got %d", b.pendingGarbage)
	}
}

func TestGarbageApplicationAddsRowsWithGap(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	w := rm.wells[tr.Players[0].AccountID]
	// Clear the grid and remove the active piece so geometry is clean.
	for r := 0; r < wellH; r++ {
		for c := 0; c < wellW; c++ {
			w.grid[r][c] = cellEmpty
		}
	}
	w.hasPiece = false
	rm.applyGarbage(tr, w, 2, 3)
	// The bottom two rows should be garbage with col 3 open.
	for r := wellH - 2; r < wellH; r++ {
		for c := 0; c < wellW; c++ {
			want := garbageCell
			if c == 3 {
				want = cellEmpty
			}
			if w.grid[r][c] != want {
				t.Fatalf("garbage row %d col %d = %d, want %d", r, c, w.grid[r][c], want)
			}
		}
	}
}

func TestTopOutOnSpawnCollision(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	// Fill the top rows so the next spawn cannot fit.
	for r := 0; r < 4; r++ {
		for c := 0; c < wellW; c++ {
			w.grid[r][c] = garbageCell
		}
	}
	w.hasPiece = false
	rm.spawnPiece(tr, w)
	if w.alive {
		t.Fatal("spawning into a filled ceiling should top the well out")
	}
}

func TestPvpEliminationDeclaresWinner(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	a, b := tr.Players[0], tr.Players[1]
	rm.topOut(tr, rm.wells[b.AccountID])
	rm.checkWinner(tr)
	if !rm.matchOver {
		t.Fatal("match should be over once one of two wells tops out")
	}
	if rm.winner != a.AccountID {
		t.Fatalf("winner should be alice, got %q", rm.winner)
	}
}

func TestSoloGarbageTimerAccelerates(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	first := rm.soloGarbageInterval(w)
	w.soloWave += 5
	later := rm.soloGarbageInterval(w)
	if later >= first {
		t.Fatalf("solo garbage should accelerate: wave0=%v wave5=%v", first, later)
	}
	if later < soloGarbageMin {
		t.Fatalf("interval %v fell below the floor %v", later, soloGarbageMin)
	}
	// And it must clamp at the floor for very long runs.
	w.soloWave += 1000
	if got := rm.soloGarbageInterval(w); got != soloGarbageMin {
		t.Fatalf("long run interval = %v, want clamp to %v", got, soloGarbageMin)
	}
}

func TestGravityAcceleratesWithLevel(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	g0 := rm.gravity(w)
	w.level = 5
	g5 := rm.gravity(w)
	if g5 >= g0 {
		t.Fatalf("gravity should speed up with level: L0=%v L5=%v", g0, g5)
	}
	w.level = 1000
	if got := rm.gravity(w); got != minGravity {
		t.Fatalf("high level gravity = %v, want clamp to %v", got, minGravity)
	}
}

// TestPieceSequenceDeterministic asserts two rooms started from the same seed
// produce the identical piece sequence (the bag draws from the room rng).
func TestPieceSequenceDeterministic(t *testing.T) {
	seq := func() []int {
		tr := kittest.NewRoom(kittest.Player("alice"))
		rm := newRoom(tr.Cfg, tr.Services())
		rm.OnStart(tr)
		rm.OnJoin(tr, tr.Players[0])
		w := rm.wells[tr.Players[0].AccountID]
		var out []int
		out = append(out, w.cur.kind, w.next)
		for i := 0; i < 30; i++ {
			out = append(out, w.drawPiece(tr.Rand()))
		}
		return out
	}
	a, b := seq(), seq()
	if len(a) != len(b) {
		t.Fatalf("sequence lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("piece sequences diverge at %d: %d vs %d", i, a[i], b[i])
		}
	}
}

// TestBagIsFair asserts each refilled bag contains exactly one of every piece.
func TestBagIsFair(t *testing.T) {
	_, tr := newTestRoom(t, "alice")
	bag := refillBag(tr.Rand())
	if len(bag) != len(pieces) {
		t.Fatalf("bag size %d, want %d", len(bag), len(pieces))
	}
	seen := map[int]bool{}
	for _, idx := range bag {
		if idx < 0 || idx >= len(pieces) {
			t.Fatalf("bag has out-of-range index %d", idx)
		}
		if seen[idx] {
			t.Fatalf("bag has a duplicate piece %d", idx)
		}
		seen[idx] = true
	}
}

func TestHardDropLocksAndScores(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	rm.OnJoin(tr, tr.Players[0])
	w := rm.wells[tr.Players[0].AccountID]
	// Place a known piece up top in open space.
	w.cur = active{kind: pieceIndex("Box"), rot: 0, row: 0, col: 4}
	w.hasPiece = true
	before := w.score
	rm.hardDrop(tr, w)
	if w.hasPiece {
		// A new piece spawned (unless it cleared into the next), so hasPiece is
		// true again — assert instead that the dropped Box landed on the floor.
	}
	if w.score <= before {
		t.Fatalf("hard drop should award drop points, score %d -> %d", before, w.score)
	}
	// The bottom rows should now contain the locked Box cells.
	filled := 0
	for r := 0; r < wellH; r++ {
		for c := 0; c < wellW; c++ {
			if w.grid[r][c] != cellEmpty {
				filled++
			}
		}
	}
	if filled < 4 {
		t.Fatalf("expected the 4-cell Box welded into the grid, found %d filled", filled)
	}
}

func TestComposeRendersFullFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	f := kit.NewFrame()
	rm.composeFor(f, tr.Players[0])
	if len(f.Cells) != kit.Rows || len(f.Cells[0]) != kit.Cols {
		t.Fatal("frame is not 24x80")
	}
}

func TestRenderReusesFrame(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	before := rm.frame
	rm.render(tr)
	rm.render(tr)
	if rm.frame != before {
		t.Fatal("render replaced rm.frame — it must reuse the single long-lived buffer")
	}
}

// pieceIndex looks up a piece by name for tests.
func pieceIndex(name string) int {
	for i, p := range pieces {
		if p.name == name {
			return i
		}
	}
	panic("no piece named " + name)
}
