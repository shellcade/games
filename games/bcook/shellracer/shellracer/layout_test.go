package shellracer

import (
	"strings"
	"testing"
	"time"

	kit "github.com/shellcade/kit"
)

// Spec row constants (0-based frame rows; the spec table is 1-based).
const (
	rowHeader     = 0  // spec 1
	rowPanelTop   = 2  // spec 3  (passage panel 3–17)
	rowPanelBot   = 16 // spec 17
	rowSpacerBot  = 17 // spec 18 (blank)
	rowStripTop   = 18 // spec 19 (opponent strip 19–23)
	rowStripBot   = 22 // spec 23
	rowStatus     = 23 // spec 24
	passageColTop = 2  // passage glyphs start at column 2
)

func styleOf(f *kit.Frame, row, col int) kit.Style {
	c := f.Cells[row][col]
	return kit.Style{FG: c.FG, BG: c.BG, Attr: c.Attr}
}

func frameText(f *kit.Frame, row int) string {
	b := make([]rune, kit.Cols)
	for c := 0; c < kit.Cols; c++ {
		r := f.Cells[row][c].Rune
		if r == 0 {
			r = ' '
		}
		b[c] = r
	}
	return string(b)
}

func frameContains(f *kit.Frame, sub string) bool {
	for row := 0; row < kit.Rows; row++ {
		if strings.Contains(frameText(f, row), sub) {
			return true
		}
	}
	return false
}

// passageCell returns the (row, col) where passage index idx is drawn, assuming
// the panel is at its un-scrolled top (first == 0). Valid for small idx.
func passageCell(rm *room, idx int) (int, int) {
	lines := wrap(rm.passage, 76)
	for li, ln := range lines {
		if idx >= ln[0] && idx < ln[1] {
			return rowPanelTop + li, passageColTop + (idx - ln[0])
		}
	}
	return -1, -1
}

// frameFor renders and returns the latest frame for viewer v.
func (d *driver) frameFor(v kit.Player) *kit.Frame {
	d.rm.render(d.r)
	return d.r.LastFrame(v)
}

// blankRows asserts every cell in the inclusive row range is a blank space.
func blankRows(t *testing.T, f *kit.Frame, lo, hi int) {
	t.Helper()
	for row := lo; row <= hi; row++ {
		for col := 0; col < kit.Cols; col++ {
			c := f.Cells[row][col]
			if c.Rune != ' ' && c.Rune != 0 {
				t.Fatalf("row %d col %d = %q (style %+v), want blank", row, col, c.Rune, styleOf(f, row, col))
			}
		}
	}
}

// A mistyped position renders the PASSAGE character in the error style inline,
// with surrounding correct positions in the done style and no separate echo.
func TestInlineErrorFrame(t *testing.T) {
	d, a := soloDriver(t)
	ps := d.rm.st[a.AccountID]

	// position 0 correct, then a typo at position 1
	d.input(a, runeIn(d.rm.passage[0]))
	wrong := d.rm.passage[1] + 1
	d.input(a, runeIn(wrong))
	if ps.cursor != 1 || ps.outstanding != 1 {
		t.Fatalf("cursor=%d outstanding=%d, want 1/1", ps.cursor, ps.outstanding)
	}

	f := d.frameFor(a)

	// position 0: done (green), drawn with the passage rune
	r0, c0 := passageCell(d.rm, 0)
	if got := f.Cells[r0][c0].Rune; got != d.rm.passage[0] {
		t.Fatalf("pos0 rune=%q, want %q", got, d.rm.passage[0])
	}
	if got := styleOf(f, r0, c0); got != stDone {
		t.Fatalf("pos0 style=%+v, want done %+v", got, stDone)
	}

	// position 1 (the typo): the PASSAGE char in error style (red), not the typed rune
	r1, c1 := passageCell(d.rm, 1)
	if got := f.Cells[r1][c1].Rune; got != d.rm.passage[1] {
		t.Fatalf("pos1 rune=%q, want passage char %q (not the typed %q)", got, d.rm.passage[1], wrong)
	}
	if got := styleOf(f, r1, c1); got != stErr {
		t.Fatalf("pos1 style=%+v, want error %+v", got, stErr)
	}

	// no separate echo: spacer row 18 and the opponent strip (solo: empty) are blank.
	blankRows(t, f, rowSpacerBot, rowSpacerBot)
	blankRows(t, f, rowStripTop, rowStripBot)
}

// Backspacing a typo and retyping it correctly drops the error style; the
// position re-renders in the done style.
func TestCorrectedErrorFrame(t *testing.T) {
	d, a := soloDriver(t)
	ps := d.rm.st[a.AccountID]

	d.input(a, runeIn(d.rm.passage[0]))
	d.input(a, runeIn(d.rm.passage[1]+1)) // typo at position 1
	if ps.outstanding != 1 {
		t.Fatalf("outstanding=%d, want 1", ps.outstanding)
	}

	r1, c1 := passageCell(d.rm, 1)
	if got := styleOf(d.frameFor(a), r1, c1); got != stErr {
		t.Fatalf("pre-correction pos1 style=%+v, want error", got)
	}

	d.input(a, keyIn(kit.KeyBackspace))
	d.input(a, runeIn(d.rm.passage[1]))
	if ps.cursor != 2 || ps.outstanding != 0 {
		t.Fatalf("cursor=%d outstanding=%d, want 2/0", ps.cursor, ps.outstanding)
	}

	f := d.frameFor(a)
	if got := styleOf(f, r1, c1); got != stDone {
		t.Fatalf("post-correction pos1 style=%+v, want done %+v", got, stDone)
	}
	if got := f.Cells[r1][c1].Rune; got != d.rm.passage[1] {
		t.Fatalf("post-correction pos1 rune=%q, want %q", got, d.rm.passage[1])
	}
}

// The cursor position renders the cursor highlight when there are no outstanding
// errors there; correct positions behind it are done.
func TestCursorAndDoneStyles(t *testing.T) {
	d, a := soloDriver(t)

	d.input(a, runeIn(d.rm.passage[0]))
	d.input(a, runeIn(d.rm.passage[1])) // cursor now at 2, no errors

	f := d.frameFor(a)
	r2, c2 := passageCell(d.rm, 2)
	if got := styleOf(f, r2, c2); got != stCursor {
		t.Fatalf("cursor pos2 style=%+v, want cursor %+v", got, stCursor)
	}
	r0, c0 := passageCell(d.rm, 0)
	if got := styleOf(f, r0, c0); got != stDone {
		t.Fatalf("pos0 style=%+v, want done", got)
	}
}

// The race frame writes nothing below the passage panel except the opponent
// strip and the status row; header and status text are present.
func TestNoEchoRegionLayout(t *testing.T) {
	d, a := soloDriver(t)
	d.input(a, runeIn(d.rm.passage[0]))

	f := d.frameFor(a)
	if frameText(f, rowHeader)[1:12] != "Shell Racer" {
		t.Fatalf("header row=%q", frameText(f, rowHeader))
	}
	blankRows(t, f, rowSpacerBot, rowSpacerBot)
	blankRows(t, f, rowStripTop, rowStripBot)
	if got := frameText(f, rowStatus); got[1:11] != "Esc: leave" {
		t.Fatalf("status row=%q, want Esc: leave hint", got)
	}
}

// cursorRow returns the panel row that holds the cursor highlight, or -1.
func cursorRow(f *kit.Frame) int {
	row := -1
	for r := rowPanelTop; r <= rowPanelBot; r++ {
		for c := 0; c < kit.Cols; c++ {
			if styleOf(f, r, c) == stCursor {
				row = r
			}
		}
	}
	return row
}

// Auto-scroll keeps the viewing player's cursor on the 3rd-from-bottom visible
// row of the 15-row window, and never writes outside rows 3–17. Uses a
// synthetic long passage so the behaviour is exercised deterministically.
func TestPassageAutoScroll(t *testing.T) {
	d, a := soloDriver(t)
	ps := d.rm.st[a.AccountID]

	// 40 single-token "words" of 76 chars each => 40 wrapped lines.
	var sb []rune
	for i := 0; i < 40; i++ {
		if i > 0 {
			sb = append(sb, ' ')
		}
		for j := 0; j < 76; j++ {
			sb = append(sb, 'a'+rune(i%26))
		}
	}
	d.rm.passage = sb
	lines := wrap(d.rm.passage, 76)
	const panelRows = 15
	if len(lines) <= panelRows {
		t.Fatalf("synthetic passage wraps to %d lines, want > %d", len(lines), panelRows)
	}

	// Before any typing the cursor is at index 0 -> top of the panel, no scroll.
	if got := cursorRow(d.frameFor(a)); got != rowPanelTop {
		t.Fatalf("initial cursor row=%d, want %d (no scroll)", got, rowPanelTop)
	}

	// Type onto a wrapped line deep in the passage to force a scroll.
	targetLine := panelRows + 5
	target := lines[targetLine][0]
	for ps.cursor < target {
		d.input(a, runeIn(d.rm.passage[ps.cursor]))
	}

	f := d.frameFor(a)
	blankRows(t, f, rowSpacerBot, rowSpacerBot)

	want := rowPanelTop + (panelRows - 3)
	if got := cursorRow(f); got != want {
		t.Fatalf("scrolled cursor row=%d, want 3rd-from-bottom row %d", got, want)
	}
}

// A 5-player race shows four opponents to each viewer in rows 19–22, leaving row
// 23 blank, and the viewer never appears in their own strip.
func TestFivePlayerOpponentStrip(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a := player("a")
	d.join(a)
	for _, id := range []string{"b", "c", "d", "e"} {
		d.join(player(id)) // capacity reached at 5 -> racing
	}
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after 5 joins, want racing", d.rm.phase)
	}

	f := d.frameFor(a)

	seen := map[string]bool{}
	for row := rowStripTop; row <= rowStripTop+3; row++ {
		line := frameText(f, row)
		if line[1] == ' ' {
			t.Fatalf("opponent row %d is blank: %q", row, line)
		}
		for _, id := range []string{"b", "c", "d", "e"} {
			if line[1:1+len(id)] == id && line[1+len(id)] == ' ' {
				seen[id] = true
			}
		}
		if line[1] == 'a' && line[2] == ' ' {
			t.Fatalf("viewer 'a' appears in its own strip on row %d: %q", row, line)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("opponent strip showed %v, want b,c,d,e", seen)
	}
	blankRows(t, f, rowStripBot, rowStripBot)
}

// Errors and typing state are per-viewer: B sees no error styling when A mistypes.
func TestPerViewerNoLeakage(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second)
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q, want racing", d.rm.phase)
	}

	d.input(a, runeIn(d.rm.passage[0]+1)) // A mistypes at position 0
	if d.rm.st[a.AccountID].outstanding != 1 {
		t.Fatalf("A outstanding=%d, want 1", d.rm.st[a.AccountID].outstanding)
	}

	d.rm.render(d.r)
	fa := d.r.LastFrame(a)
	fb := d.r.LastFrame(b)

	r0, c0 := passageCell(d.rm, 0)
	if got := styleOf(fa, r0, c0); got != stErr {
		t.Fatalf("A pos0 style=%+v, want error", got)
	}
	for row := rowPanelTop; row <= rowPanelBot; row++ {
		for col := 0; col < kit.Cols; col++ {
			if styleOf(fb, row, col) == stErr {
				t.Fatalf("B's frame has an error cell at %d,%d but B made no typo", row, col)
			}
		}
	}
}

// the countdown renders a live remaining count.
func TestCountdownRendered(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)                  // -> countdown
	d.advance(2 * time.Second) // ~8s remaining, still counting down
	if d.rm.phase != phCountdown {
		t.Fatalf("phase=%q, want countdown", d.rm.phase)
	}
	f := d.frameFor(a)
	if !frameContains(f, "Starting in 8") {
		t.Fatalf("countdown not rendered with remaining count; row9=%q", frameText(f, 9))
	}
}
