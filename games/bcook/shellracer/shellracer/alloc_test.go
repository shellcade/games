package shellracer

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// composePassage runs on every render, per viewer. It used to word-wrap the
// (fixed) passage on every call, allocating a fresh [][2]int — permanent growth
// under -gc=leaking. The wrap is now computed once in OnStart, so composePassage
// must allocate nothing.
func TestComposePassageAllocFree(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	p := kittest.Player("alice")
	d.join(p)
	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		d.rm.composePassage(f, p)
	})
	if allocs != 0 {
		t.Fatalf("composePassage allocates %.0f/call — the passage wrap must be cached (computed once in OnStart), not re-wrapped per render", allocs)
	}
}

// The full per-render compose for a RACING-phase room with several racers must
// allocate nothing: under -gc=leaking every per-render allocation is a permanent
// leak. Guards the header, racer strip, WPM/accuracy and progress-bar paths.
func TestComposeRacingAllocFree(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a := player("a")
	d.join(a)
	for _, id := range []string{"b", "c", "d", "e"} {
		d.join(player(id)) // capacity reached at 5 -> racing
	}
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after 5 joins, want racing", d.rm.phase)
	}
	// Type a little so cursor/errors/WPM paths render non-trivial values.
	d.input(a, runeIn(d.rm.passage[0]))
	d.input(a, runeIn(d.rm.passage[0]+1)) // an error -> accuracy < 100%
	d.advance(2 * time.Second)            // non-zero elapsed for live WPM

	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		d.rm.compose(f, d.r, a)
	})
	if allocs != 0 {
		t.Fatalf("racing-phase compose allocates %.0f/call — steady-state race render must be alloc-free", allocs)
	}
}

// The results-phase compose (header + ranking rows) must also be alloc-free.
func TestComposeResultsAllocFree(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second) // -> racing
	d.advance(2 * time.Second)
	// A finishes; let the race resolve to results with rankings populated.
	for _, r := range d.rm.passage {
		d.input(a, runeIn(r))
	}
	d.advance(stragglerDur + time.Second)
	if d.rm.phase != phResults {
		t.Fatalf("phase=%q, want results", d.rm.phase)
	}
	if len(d.rm.result.Rankings) < 2 {
		t.Fatalf("rankings=%d, want >=2", len(d.rm.result.Rankings))
	}

	f := kit.NewFrame()
	allocs := testing.AllocsPerRun(100, func() {
		d.rm.compose(f, d.r, a)
	})
	if allocs != 0 {
		t.Fatalf("results-phase compose allocates %.0f/call — must be alloc-free", allocs)
	}
}

// putIntRight pads on the LEFT to the field width and writes the digits flush
// right; putIntLeft writes the digits then pads on the RIGHT. Both must report
// the column past the field and produce the exact glyphs.
func TestPutIntFields(t *testing.T) {
	cases := []struct {
		name  string
		fn    func(f *kit.Frame, row, col, n, width int, st kit.Style) int
		n     int
		width int
		want  string
	}{
		{"right-pad-left", putIntRight, 7, 3, "  7"},
		{"right-exact", putIntRight, 123, 3, "123"},
		{"right-neg", putIntRight, -5, 4, "  -5"},
		{"left-pad-right", putIntLeft, 7, 3, "7  "},
		{"left-exact", putIntLeft, 123, 3, "123"},
		{"left-neg", putIntLeft, -5, 4, "-5  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := kit.NewFrame()
			next := tc.fn(f, 0, 2, tc.n, tc.width, stPlain)
			if want := 2 + tc.width; next != want {
				t.Fatalf("next col = %d, want %d", next, want)
			}
			var got []rune
			for c := 2; c < 2+tc.width; c++ {
				r := f.Cells[0][c].Rune
				if r == 0 {
					r = ' '
				}
				got = append(got, r)
			}
			if string(got) != tc.want {
				t.Fatalf("field = %q, want %q", string(got), tc.want)
			}
		})
	}
}
