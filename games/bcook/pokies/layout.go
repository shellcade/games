package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// Rendering: a near-verbatim port of the native pokies layout to the kit grid.
// Five 15-wide cabinets plus 1-col gutters fit inside 80 cols (5*15 + 4 = 79);
// each cabinet frames a 3x3 reel window whose center row is the payline.
const (
	cardW       = 15
	cardH       = 11
	gutter      = 1
	cardTop     = 3
	maxMachines = 5
	scrollSpeed = 2  // reel-strip rows advanced per animation cycle while spinning
	payRowY     = 15 // paytable strip row, one blank line below the cabinets
)

// faceArt maps a symbol to its reel art, drawn width-2 via SetGraphemeWide
// (the kit v2 grapheme cells). The symbol byte stays the logical/config ID;
// this is presentation only.
//
// Width-2 is the author's contract, so every face must be a glyph whose
// rendered width is UNCONTESTED: all five are single code points with East
// Asian Width W/F, which runewidth, uniseg, x/ansi, and real terminals all
// agree render as two columns. The keycap 7️⃣ ('7'+VS16+U+20E3) was tried
// first and corrupted layouts in production: its width is contested
// (runewidth/uniseg say 1, x/ansi says 2, terminals split on font
// composition), and a narrow-rendering viewer desyncs every column to the
// glyph's right. U+FF17 FULLWIDTH DIGIT SEVEN is the defensive choice — and
// the wide ７７７ is what a slot reel wants anyway. Non-UTF-8 sessions
// degrade host-side via asciiFallback (７→7, 🍒→C, etc.).
var faceArt = map[symbol]string{
	sym7:       "\uFF17",     // ７ fullwidth seven
	symDollar:  "\U0001F48E", // 💎
	symStar:    "\u2B50",     // ⭐
	symBar:     "\U0001F514", // 🔔
	symCherry:  "\U0001F352", // 🍒
	symWild:    "\U0001F451", // 👑 crown — wild (single codepoint, EAW=Wide)
	symScatter: "\U0001F381", // 🎁 gift — scatter (single codepoint, EAW=Wide)
}

// suitArt renders the four card suits as width-2 emoji (base + VS16 emoji
// presentation), used in the gamble card reveal and suit selector. The emoji
// forms carry their own colour (red hearts/diamonds). Unlike the reel faces these
// are NOT unanimously-wide — VS16 width is contested — but the authentic pips are
// the chosen tradeoff for the gamble screen (a narrow-rendering viewer may shift
// that one row); non-UTF-8 sessions degrade host-side.
var suitArt = [4]string{
	suitSpades:   "♠️", // ♠️
	suitHearts:   "♥️", // ♥️
	suitDiamonds: "♦️", // ♦️
	suitClubs:    "♣️", // ♣️
}

var (
	stTitle   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim     = kit.Style{FG: kit.DimGray}
	stTicker  = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stBordOwn = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stBordDim = kit.Style{FG: kit.DimGray}
	stNameOwn = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stName    = kit.Style{FG: kit.White}
	stPayline = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold} // center row
	stReelDim = kit.Style{FG: kit.DimGray}                    // top/bottom rows
	stMarker  = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stLabel   = kit.Style{FG: kit.DimGray}
	stWin     = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stRebuy   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stReady   = kit.Style{FG: kit.DimGray}
	stLever   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}

	stBordFree = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}   // gold cabinet during free spins
	stGamble   = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}   // gamble banner / at-risk
	stGamHi    = kit.Style{FG: kit.White, Attr: kit.AttrReverse} // highlighted gamble option
	stGamOpt   = kit.Style{FG: kit.DimGray}                      // un-highlighted gamble option
)

// fallbackStrip is the compiled default strip, computed once. spinStrip/idleStrip
// use it only when a machine's pinned/last strip is somehow unset, so the steady
// state never allocates a fresh variant per render.
var fallbackStrip = defaultVariant().strip

// composeFrame is the single reused render buffer. The guest runs as a serial
// actor and each composed frame is fully consumed by r.Send before the next
// compose, so a package-global frame cleared per call is alloc-free in steady
// state (no kit.NewFrame() allocation per viewer per render).
var composeFrame = kit.NewFrame()

// render composes and sends a per-viewer frame to every member.
func (rm *room) render(r kit.Room) {
	rm.lastNow = r.Now()
	for _, v := range r.Members() {
		r.Send(v, rm.compose(v))
	}
}

// drawTicker renders the room-wide big-win banner onto row 1 when active.
// The winner's character tile (kit v2.9.0) rides immediately before their name
// (ticker text starts with the name); a zero character degrades to the plain
// centred banner.
func (rm *room) drawTicker(f *kit.Frame) {
	if ch := rm.ticker.ch; ch.Glyph != "" {
		w := 2 + 2 + len([]rune(rm.ticker.text)) + 2
		c := f.Text(1, (kit.Cols-w)/2, "* ", stTicker)
		f.Set(1, c, kit.CharacterCell(ch))
		f.Text(1, c+2, rm.ticker.text+" *", stTicker)
	} else {
		msg := "* " + rm.ticker.text + " *"
		f.Text(1, (kit.Cols-len(msg))/2, msg, stTicker)
	}
}

func (rm *room) compose(v kit.Player) *kit.Frame {
	f := composeFrame
	f.Clear()

	pw := rm.pawns[v.AccountID]
	if pw != nil && pw.seated {
		rm.composeSeated(f, v)
		return f
	}
	rm.composeFloor(f, v)
	return f
}

func (rm *room) composeFloor(f *kit.Frame, v kit.Player) {
	f.Text(0, 2, "*** POKIES LOUNGE ***", stTitle)
	if rm.tickerActive(rm.lastNow) {
		rm.drawTicker(f)
	}
	rm.drawFloor(f, v)
	f.Text(kit.Rows-1, 2, "Arrows move   SPACE sit   Esc leave", stDim)
	if m := rm.machines[v.AccountID]; m != nil {
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
}

func (rm *room) composeSeated(f *kit.Frame, v kit.Player) {
	id := v.AccountID
	pw := rm.pawns[id]
	var mc *floorMachine
	if pw != nil {
		mc = rm.machineByID(pw.seat)
	}
	title := "*** POKIES ***"
	if mc != nil {
		title = "*** " + mc.name + " ***"
	}
	f.Text(0, 2, title, stTitle)
	if rm.tickerActive(rm.lastNow) {
		rm.drawTicker(f)
	}
	rm.drawSeated(f, rm.machines[id])
	rm.drawPaytableFor(f, payRowY, rm.seatVariant(id))
	controls := "Up/Down bet   SPACE spin   Esc stand"
	if m := rm.machines[id]; m != nil {
		switch {
		case m.gamble != nil:
			controls = "Arrows pick   SPACE lock/take   Esc stand"
		case m.freeSpins > 0:
			controls = "FREE SPINS auto-playing...   Esc stand"
		}
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
	f.Text(kit.Rows-1, 2, controls, stDim)
}

func (rm *room) seatVariant(id string) *variant {
	if pw := rm.pawns[id]; pw != nil && pw.seated && pw.seat >= 0 && pw.seat < len(rm.themes) {
		return rm.themes[pw.seat]
	}
	return rm.variant
}

// seatTop is the frame row of the seated reel box top border.
const seatTop = 3

// seatedReelCol returns the frame column of reel r's (width-2) face in the seated
// cabinet. Faces are packed width-2 with 1-col gaps, the grid centered on 80 cols.
func seatedReelCol(r int) int {
	inner := numReels*2 + (numReels - 1) // 14
	left := (kit.Cols - (inner + 4)) / 2
	return left + 2 + r*3
}

// drawSeated renders the full-screen 5x3 seated machine: a framed reel grid, the
// payline markers, the bet/balance readout, the status flash, and — when active —
// the gamble selector (which takes over the screen). The cabinet recolors gold
// during free spins.
func (rm *room) drawSeated(f *kit.Frame, m *machine) {
	if m == nil {
		return
	}
	if m.gamble != nil {
		// The gamble selector reuses the cabinet-relative drawer, centered.
		rm.drawGamble(f, (kit.Cols-cardW)/2, seatTop+1, m, true)
		return
	}
	bord := stBordOwn
	if m.freeSpins > 0 {
		bord = stBordFree
	}
	inner := numReels*2 + (numReels - 1) // 14
	left := (kit.Cols - (inner + 4)) / 2
	right := left + inner + 3
	sx := left + 2

	// Box frame: top border (seatTop), visRows reel rows, bottom border.
	f.SetRune(seatTop, left, '╭', bord)
	f.SetRune(seatTop, right, '╮', bord)
	f.SetRune(seatTop+visRows+1, left, '╰', bord)
	f.SetRune(seatTop+visRows+1, right, '╯', bord)
	for c := left + 1; c < right; c++ {
		f.SetRune(seatTop, c, '─', bord)
		f.SetRune(seatTop+visRows+1, c, '─', bord)
	}
	for r := 1; r <= visRows; r++ {
		f.SetRune(seatTop+r, left, '│', bord)
		f.SetRune(seatTop+r, right, '│', bord)
	}

	// Faces: row 1 (center) is the payline.
	g := rm.grid(m)
	for row := 0; row < visRows; row++ {
		st := stReelDim
		if row == 1 {
			st = stPayline
		}
		for reel := 0; reel < numReels; reel++ {
			c := sx + reel*3
			if s := g[row][reel]; s == symBlank {
				f.SetRune(seatTop+1+row, c, '-', st)
			} else {
				f.SetGraphemeWide(seatTop+1+row, c, faceArt[s], st)
			}
		}
	}
	// Payline markers either side of the center reel row.
	f.SetRune(seatTop+2, left-1, '>', stMarker)
	f.SetRune(seatTop+2, right+1, '<', stMarker)

	// Status flash / spinning, centered above the readout.
	statusRow := seatTop + visRows + 2
	switch {
	case m.flash == "RE-BUY":
		f.Text(statusRow, (kit.Cols-len(m.flash))/2, m.flash, stRebuy)
	case m.flash != "":
		f.Text(statusRow, (kit.Cols-len(m.flash))/2, m.flash, stWin)
	case m.spin != nil:
		f.Text(statusRow, (kit.Cols-8)/2, "spinning", stReady)
	}

	// Readout line: BET (or FREE during a feature), BAL, HI.
	info := fmt.Sprintf("BET %d     BAL %d     HI %d", m.bet, m.balance, m.highScore)
	st := stTitle
	if m.freeSpins > 0 {
		info = fmt.Sprintf("FREE %d     BAL %d     HI %d", m.freeSpins, m.balance, m.highScore)
		st = stWin
	}
	f.Text(seatTop+visRows+3, (kit.Cols-len(info))/2, info, st)
}

// drawOpt draws one gamble selector option, highlighted (reverse video) when
// selected, and returns the next column (one space gap).
func drawOpt(f *kit.Frame, row, c int, label string, sel bool) int {
	st := stGamOpt
	if sel {
		st = stGamHi
	}
	return f.Text(row, c, label, st) + 1
}

// drawGamble renders the double-up ladder inside the cabinet: the interactive
// selector for the owner, a compact at-risk indicator for other viewers.
func (rm *room) drawGamble(f *kit.Frame, col, top int, m *machine, own bool) {
	g := m.gamble
	risk := fmt.Sprintf("+%d", g.atRisk)
	if !own {
		f.Text(top+2, col+2, "GAMBLE", stGamOpt)
		f.SetGraphemeWide(top+4, col+3, "\U0001F3B2", stGamble) // 🎲
		f.Text(top+4, col+6, risk, stGamble)
		f.Text(top+6, col+2, fmt.Sprintf("HI %d", m.highScore), stLabel)
		f.Text(top+7, col+2, fmt.Sprintf("BAL %d", m.balance), stLabel)
		return
	}
	f.Text(top+1, col+2, "GAMBLE", stGamble)
	win := "WIN " + risk
	if len(win) > cardW-3 {
		win = risk
	}
	f.Text(top+2, col+2, win, stWin)

	f.Text(top+3, col+2, "CARD", stGamOpt)
	if g.card >= 0 {
		f.SetGraphemeWide(top+3, col+8, suitArt[g.card], stName)
	} else {
		f.Text(top+3, col+8, "?", stGamOpt)
	}

	// Row 1: TAKE / RED / BLACK (×2). Row 2: the four suits as wide emoji (×4).
	c := drawOpt(f, top+6, col+1, "TAKE", g.sel == selTake)
	c = drawOpt(f, top+6, c, "RED", g.sel == selRed)
	drawOpt(f, top+6, c, "BLK", g.sel == selBlack)
	cx := col + 1
	for s := 0; s < 4; s++ {
		st := stGamOpt
		if g.sel == selSpades+s {
			st = stGamHi // reverse-video highlight on the selected suit
		}
		f.SetGraphemeWide(top+7, cx, suitArt[s], st)
		cx += 3 // 2-wide glyph + 1-col gap
	}
	f.Text(top+9, col+1, "SPACE pick", stGamOpt)
}

// drawPaytableFor centers the variant's ways paytable on one row: each paying
// symbol's face plus its "pay5/pay4/pay3" label (highest 5-of-a-kind first). An
// absurd admin variant that overflows simply clamps at the canvas edges
// (SetGraphemeWide/Text refuse out-of-bounds writes).
func (rm *room) drawPaytableFor(f *kit.Frame, row int, v *variant) {
	if v == nil {
		return
	}
	rows, labels := v.payRowsCache, v.payLabels // precomputed at variant compile
	if len(rows) == 0 {
		return
	}
	const faceW, gap = 2, 3
	width := (len(rows) - 1) * gap
	for i := range rows {
		width += faceW + len(labels[i])
	}
	col := (kit.Cols - width) / 2
	if col < 0 {
		col = 0
	}
	for i, pr := range rows {
		if i > 0 {
			col += gap
		}
		col = f.SetGraphemeWide(row, col, faceArt[pr.sym], stReelDim)
		col = f.Text(row, col, labels[i], stLabel)
	}
}

// drawPaytable is a thin wrapper around drawPaytableFor using the room's
// active variant; kept so existing callers (including alloc tests) compile.
func (rm *room) drawPaytable(f *kit.Frame, row int) {
	rm.drawPaytableFor(f, row, rm.variant)
}

// grid returns the visRows × numReels visible faces, indexed [row][reel] with row
// 0 the top, row 1 the center payline, row 2 the bottom. Reels scroll while
// spinning (cycle derived from elapsed time), freeze to their landing window as
// they settle, show the last result when idle, and are blank before the first
// spin.
func (rm *room) grid(m *machine) [visRows][numReels]symbol {
	var out [visRows][numReels]symbol
	for reel := 0; reel < numReels; reel++ {
		var w [visRows]symbol
		switch {
		case m.spin != nil && reel >= m.spin.landed:
			w = rollWindow(spinStrip(m.spin), m.spin.cycle(rm.lastNow)*scrollSpeed+reel*7)
		case m.spin != nil:
			w = windowAt(spinStrip(m.spin), m.spin.stopIdx[reel])
		case m.spun:
			w = windowAt(idleStrip(m), m.lastIdx[reel])
		default:
			w = [visRows]symbol{symBlank, symBlank, symBlank}
		}
		for row := 0; row < visRows; row++ {
			out[row][reel] = w[row]
		}
	}
	return out
}

// spinStrip is the strip an in-flight spin's indices index into (its pinned
// variant), falling back to the default strip if unset.
func spinStrip(s *spinState) []symbol {
	if s.variant != nil && len(s.variant.strip) > 0 {
		return s.variant.strip
	}
	return fallbackStrip
}

// idleStrip is the strip a settled machine's lastIdx values index into (the
// variant of its last spin), falling back to the default strip.
func idleStrip(m *machine) []symbol {
	if len(m.lastStrip) > 0 {
		return m.lastStrip
	}
	return fallbackStrip
}
