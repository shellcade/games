package main

import kit "github.com/shellcade/kit/v2"

// Alloc-free numeric formatting for render paths. Under -gc=leaking every heap
// allocation during a render is permanent, so the steady-state HUD and the
// week-resident sub-screens write digits straight into the frame rather than
// building strings via itoa(...)+concat (which escapes and allocates).

func intWidth(n int) int {
	w := 1
	if n < 0 {
		w++
		n = -n
	}
	for n >= 10 {
		n /= 10
		w++
	}
	return w
}

// putInt writes n in base-10 at (row,col), returns next column. Alloc-free.
// (This is the frame-writing analogue of the existing string-returning itoa.)
func putInt(f *kit.Frame, row, col, n int, st kit.Style) int {
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		b[i] = '-'
	}
	for ; i < len(b); i++ {
		f.SetRune(row, col, rune(b[i]), st)
		col++
	}
	return col
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// centerStart returns the starting column to center `width` columns of content
// across the grid (the alloc-free analogue of the center() helper's
// (kit.Cols - len([]rune(s)))/2), clamped to 0.
func centerStart(width int) int {
	col := (kit.Cols - width) / 2
	if col < 0 {
		col = 0
	}
	return col
}

// clampWidth returns the column width clampCols(s, n) would occupy: runeLen(s)
// when it fits, else exactly n (n-1 runes + the ellipsis).
func clampWidth(s string, n int) int {
	if w := runeLen(s); w <= n {
		return w
	}
	return n
}

// clampWriter writes a sequence of string/int parts into one frame row,
// clamped to a rune budget with a trailing ellipsis — the alloc-free analogue
// of clampCols(a+b+itoa(n)+..., budget). It never builds an intermediate
// string. Call str/num in left-to-right order, then close().
type clampWriter struct {
	f    *kit.Frame
	row  int
	col  int
	left int  // remaining rune budget
	cut  bool // budget exhausted; trailing content dropped
	st   kit.Style
}

func newClampWriter(f *kit.Frame, row, col, budget int, st kit.Style) *clampWriter {
	return &clampWriter{f: f, row: row, col: col, left: budget, st: st}
}

// str appends a string part.
func (w *clampWriter) str(s string) *clampWriter {
	for _, r := range s {
		if w.left <= 0 {
			w.cut = true
			break
		}
		w.f.SetRune(w.row, w.col, r, w.st)
		w.col++
		w.left--
	}
	return w
}

// strClamp appends s pre-clamped to at most n runes with its own trailing
// ellipsis (the alloc-free analogue of clampCols(s, n)), still bounded by the
// writer's outer budget.
func (w *clampWriter) strClamp(s string, n int) *clampWriter {
	if runeLen(s) <= n {
		return w.str(s)
	}
	written := 0
	for _, r := range s {
		if written >= n-1 || w.left <= 0 {
			break
		}
		if w.left <= 0 {
			w.cut = true
			break
		}
		w.f.SetRune(w.row, w.col, r, w.st)
		w.col++
		w.left--
		written++
	}
	if w.left > 0 {
		w.f.SetRune(w.row, w.col, '…', w.st)
		w.col++
		w.left--
	} else {
		w.cut = true
	}
	return w
}

// num appends a base-10 int part.
func (w *clampWriter) num(n int) *clampWriter {
	if w.left <= 0 {
		w.cut = true
		return w
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		b[i] = '-'
	}
	for ; i < len(b); i++ {
		if w.left <= 0 {
			w.cut = true
			break
		}
		w.f.SetRune(w.row, w.col, rune(b[i]), w.st)
		w.col++
		w.left--
	}
	return w
}

// done finalizes the line, replacing the final written column with '…' when
// content was truncated (matching clampCols' ellipsis on overflow).
func (w *clampWriter) done() {
	if w.cut {
		w.f.SetRune(w.row, w.col-1, '…', w.st)
	}
}
