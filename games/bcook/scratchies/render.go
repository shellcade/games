package main

import (
	"fmt"
	"strconv"

	kit "github.com/shellcade/kit/v2"
)

// Frame and Style alias the kit types so mechanic files read cleanly and the
// kit import stays concentrated in render.go / room.go.
type Frame = kit.Frame
type Style = kit.Style

// Theme is a ticket's presentation: an accent colour and (for find-symbol) the
// target symbol's ink. Mechanics may use it; the shared renderer falls back to
// the fixed styles below when a theme is zero.
type Theme struct {
	Accent kit.Color
	Symbol kit.Color
}

// Shared styles, keyed to the ARTBOARDS colour key.
var (
	stTitle   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stWallet  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim     = kit.Style{FG: kit.DimGray}
	stHint    = kit.Style{FG: kit.DimGray}
	stTicker  = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stWin     = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stBig     = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stRail    = kit.Style{FG: kit.DimGray}
	stRailHot = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stPrice   = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stMatch   = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stBust    = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stCoin    = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stLatex   = kit.Style{FG: kit.DimGray}
	stReveal  = kit.Style{FG: kit.White}
	stSel     = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
)

// Grid cell geometry: a 6-wide × 3-tall box ("┌────┐ / │ rr │ / └────┘") with a
// 4-column interior, stepped 8 columns apart so up to 6 columns fit in 80.
const (
	cellW     = 6
	cellH     = 3
	cellStepX = 8
)

// drawChrome paints the shared frame furniture: title + wallet (rows 0–1), the
// big-win ticker (rows 21–22) and the hint line (row 23). The body is rows 2–20.
func drawChrome(f *Frame, title string, balance int, ticker, hint string) {
	f.Text(0, 1, title, stTitle)
	f.TextRight(0, kit.Cols-1, fmt.Sprintf("◉ you · %s cr", commaInt(balance)), stWallet)
	ruleRow(f, 1)
	f.Text(21, 1, "─ BIG WINS ", stDim)
	for c := 12; c < kit.Cols; c++ {
		f.SetRune(21, c, '─', stDim)
	}
	if ticker != "" {
		f.Text(22, 3, ticker, stTicker)
	}
	f.Text(23, 1, hint, stHint)
}

// ruleRow draws a horizontal rule across the whole row.
func ruleRow(f *Frame, row int) {
	for c := 0; c < kit.Cols; c++ {
		f.SetRune(row, c, '─', stDim)
	}
}

// box draws a single-line border rectangle (inclusive corners).
func box(f *Frame, r0, c0, r1, c1 int, st Style) {
	f.SetRune(r0, c0, '┌', st)
	f.SetRune(r0, c1, '┐', st)
	f.SetRune(r1, c0, '└', st)
	f.SetRune(r1, c1, '┘', st)
	for c := c0 + 1; c < c1; c++ {
		f.SetRune(r0, c, '─', st)
		f.SetRune(r1, c, '─', st)
	}
	for r := r0 + 1; r < r1; r++ {
		f.SetRune(r, c0, '│', st)
		f.SetRune(r, c1, '│', st)
	}
}

// latexCells renders the 4-column latex interior. The latex always shows fully
// opaque (100%) regardless of how many rubs remain — a panel looks identically
// covered until the final rub pops it open, so you can't tell a 1-rub panel
// from a stubborn 3-rub one by looking. The `layers` count (1–3) still governs
// how many SPACE presses it takes to reveal; it's just not telegraphed.
func latexCells(int) string {
	return "▓▓▓▓"
}

// centre4 pads s to a 4-column field, centred (e.g. "$5" -> " $5 ").
func centre4(s string) string {
	if len([]rune(s)) >= 4 {
		return string([]rune(s)[:4])
	}
	pad := 4 - len([]rune(s))
	left := pad / 2
	return spaces(left) + s + spaces(pad-left)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// commaInt formats n with thousands separators ("1234567" -> "1,234,567").
func commaInt(n int) string {
	s := strconv.Itoa(n)
	neg := ""
	if n < 0 {
		neg = "-"
		s = s[1:]
	}
	var out []byte
	for i, d := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, d)
	}
	return neg + string(out)
}

// drawGrid renders a scratch grid into the frame at (top,left), showing at most
// `viewport` cell-rows and scrolling so the cursor panel stays visible. A scroll
// rail is drawn to the right when the grid is taller than the viewport.
func drawGrid(f *Frame, g *Grid, top, left, viewport int) {
	curRow := 0
	if g.Cols > 0 {
		curRow = g.Cur / g.Cols
	}
	first := 0
	if curRow >= viewport {
		first = curRow - viewport + 1
	}
	if maxFirst := g.Rows - viewport; first > maxFirst && maxFirst > 0 {
		first = maxFirst
	}
	last := first + viewport
	if last > g.Rows {
		last = g.Rows
	}
	for gr := first; gr < last; gr++ {
		ry := top + (gr-first)*cellH
		for gc := 0; gc < g.Cols; gc++ {
			idx := gr*g.Cols + gc
			drawCell(f, ry, left+gc*cellStepX, g.Panels[idx], idx == g.Cur)
		}
	}
	if g.Rows > viewport {
		drawRail(f, top, left+g.Cols*cellStepX+1, viewport*cellH, first, viewport, g.Rows)
	}
}

// drawCell draws one panel box with its latex or revealed content.
func drawCell(f *Frame, ry, cx int, p Panel, focused bool) {
	bst := stDim
	if focused {
		bst = stCoin
	}
	box(f, ry, cx, ry+2, cx+cellW-1, bst)
	if p.Hidden {
		f.Text(ry+1, cx+1, latexCells(p.Layers), stLatex)
		return
	}
	if isWideGlyph(p.Reveal) {
		// A width-2 emoji: clear the 4-col interior, then draw it centred
		// (occupying the middle two columns) via the kit grapheme cell.
		f.Text(ry+1, cx+1, "    ", p.Ink)
		f.SetGraphemeWide(ry+1, cx+2, p.Reveal, p.Ink)
		return
	}
	f.Text(ry+1, cx+1, centre4(p.Reveal), p.Ink)
}

// isWideGlyph reports whether s is a single width-2 glyph (an emoji), as opposed
// to text like "$5" / "07" / "CROC" or the block-element latex "▓▓▓▓". Emoji and
// the misc-symbol pictographs sit at/above U+2600; block elements (U+2580–U+259F)
// and box drawing fall below it, so a single rune ≥ U+2600 is the wide case.
func isWideGlyph(s string) bool {
	r := []rune(s)
	return len(r) == 1 && r[0] >= 0x2600
}

// drawRail draws the vertical scroll rail with a proportional thumb.
func drawRail(f *Frame, top, x, height, first, viewport, total int) {
	if height < 2 {
		return
	}
	thumbStart := top
	if total > 0 {
		thumbStart = top + first*height/total
	}
	thumbEnd := thumbStart + viewport*height/total
	for r := top; r < top+height; r++ {
		st := stRail
		g := '░'
		if r >= thumbStart && r <= thumbEnd {
			st = stRailHot
			g = '▓'
		}
		f.SetRune(r, x, g, st)
	}
	f.SetRune(top, x, '▲', stRail)
	f.SetRune(top+height-1, x, '▼', stRail)
}
