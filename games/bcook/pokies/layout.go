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

func (rm *room) compose(v kit.Player) *kit.Frame {
	f := composeFrame
	f.Clear()

	f.Text(0, 2, "*** POKIES ***", stTitle)
	f.TextRight(0, kit.Cols-2, "pull the lever - chase your high score", stDim)

	if rm.tickerActive(rm.lastNow) {
		// The winner's character tile (kit v2.9.0) rides immediately before
		// their name (ticker text starts with the name); a zero character
		// degrades to the plain centered banner.
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

	n := len(rm.order)
	if n > maxMachines {
		n = maxMachines
	}
	if n > 0 {
		group := n*cardW + (n-1)*gutter
		start := (kit.Cols - group) / 2
		for i := 0; i < n; i++ {
			id := rm.order[i]
			rm.drawCard(f, start+i*(cardW+gutter), cardTop, id, id == v.AccountID)
		}
	}

	rm.drawPaytable(f, payRowY)

	controls := "Up/Down bet   SPACE spin   Esc leave"
	if m := rm.machines[v.AccountID]; m != nil {
		switch {
		case m.gamble != nil:
			controls = "Arrows pick   SPACE lock/take   Esc leave"
		case m.freeSpins > 0:
			controls = "FREE SPINS auto-playing...   Esc leave"
		}
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
	f.Text(kit.Rows-1, 2, controls, stDim)
	return f
}

// drawCard renders one rounded cabinet at (col,top).
//
//	row layout (relative to top):
//	 0 top border + name           5 screen bottom frame
//	 1 screen top frame + lever    6 HI
//	 2 reel row (top)              7 BAL
//	 3 reel row (CENTER/payline)   8 BET
//	 4 reel row (bottom)           9 status
//	                              10 bottom border + coin slot
func (rm *room) drawCard(f *kit.Frame, col, top int, id string, own bool) {
	m := rm.machines[id]
	if m == nil {
		return
	}
	bord, nameSt := stBordDim, stName
	switch {
	case m.freeSpins > 0:
		bord = stBordFree // gold cabinet: the floor can see who's in a feature
	case own:
		bord, nameSt = stBordOwn, stNameOwn
	}
	left, right := col, col+cardW-1

	// Top border with the player's character tile (kit v2.9.0) immediately
	// before the (truncated) handle; the tile + its space come out of the
	// name budget so the marquee never outgrows the cabinet.
	rm.border(f, top, col, '╭', '╮', bord)
	name := id
	var ch kit.Character
	if p, ok := rm.names[id]; ok {
		name, ch = p.Handle, p.Character
	}
	maxName, nameCol := cardW-4, col+2
	if ch.Glyph != "" {
		maxName = cardW - 6
		f.SetRune(top, nameCol, ' ', nameSt)
		f.Set(top, nameCol+1, kit.CharacterCell(ch))
		nameCol += 2
	}
	if len(name) > maxName {
		name = name[:maxName]
	}
	f.Text(top, nameCol, " "+name+" ", nameSt)

	// Sides for every interior row.
	for r := 1; r <= 9; r++ {
		f.SetRune(top+r, left, '│', bord)
		f.SetRune(top+r, right, '│', bord)
	}

	// Gamble takes over the cabinet interior: the owner gets the interactive
	// selector, other viewers a compact at-risk indicator (frames are per-viewer).
	if m.gamble != nil {
		rm.drawGamble(f, col, top, m, own)
		rm.border(f, top+10, col, '╰', '╯', bord)
		f.Text(top+10, col+5, "[__]", bord)
		return
	}

	// Reel screen box (cols col+2..col+9): ╭──────╮ / │ 🍒🍒🍒 │ / ╰──────╯ —
	// an 8-wide box whose 6-col interior packs three width-2 emoji faces.
	sx := col + 2
	f.SetRune(top+1, sx, '╭', bord)
	f.SetRune(top+5, sx, '╰', bord)
	f.SetRune(top+1, sx+7, '╮', bord)
	f.SetRune(top+5, sx+7, '╯', bord)
	for c := sx + 1; c < sx+7; c++ {
		f.SetRune(top+1, c, '─', bord)
		f.SetRune(top+5, c, '─', bord)
	}
	for r := 2; r <= 4; r++ {
		f.SetRune(top+r, sx, '│', bord)
		f.SetRune(top+r, sx+7, '│', bord)
	}

	// The 3x3 faces: width-2 emoji art packed at sx+1, sx+3, sx+5; the center
	// row (top+3) is the payline. The blank pre-spin face is a plain dash (a
	// width-1 rune leaves its second slot cell empty — still one slot).
	g := rm.grid(m)
	for row := 0; row < 3; row++ {
		st := stReelDim
		if row == 1 {
			st = stPayline
		}
		for reel := 0; reel < 3; reel++ {
			c := sx + 1 + reel*2
			if s := g[row][reel]; s == symBlank {
				f.SetRune(top+2+row, c, '-', st)
			} else {
				f.SetGraphemeWide(top+2+row, c, faceArt[s], st)
			}
		}
	}
	// Payline markers pointing at the center row.
	f.SetRune(top+3, sx-1, '>', stMarker)
	f.SetRune(top+3, sx+8, '<', stMarker)

	// Lever to the right of the screen: knob rides up when idle, drops mid-spin.
	rm.lever(f, col, top, m)

	// Readouts. The bet line shows FREE + remaining spins during a feature.
	rm.body(f, top+6, col, "HI", m.highScore)
	rm.body(f, top+7, col, "BAL", m.balance)
	if m.freeSpins > 0 {
		f.Text(top+8, col+2, fmt.Sprintf("FREE %d", m.freeSpins), stWin)
	} else {
		rm.body(f, top+8, col, "BET", m.bet)
	}
	rm.status(f, top+9, col, m)

	// Bottom border with a coin slot.
	rm.border(f, top+10, col, '╰', '╯', bord)
	f.Text(top+10, col+5, "[__]", bord)
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
		rm.body(f, top+6, col, "HI", m.highScore)
		rm.body(f, top+7, col, "BAL", m.balance)
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

// drawPaytable centers the active variant's paying triples on one row under
// the cabinets — "7️⃣7️⃣7️⃣ x500   💎💎💎 x150  …" — naming each symbol with its
// reel art. Width is computed up front (each glyph is declared width-2) so the
// strip centers; an absurd admin variant that overflows simply clamps at the
// canvas edges (SetGraphemeWide/SetRune refuse out-of-bounds writes).
func (rm *room) drawPaytable(f *kit.Frame, row int) {
	v := rm.variant
	if v == nil {
		return
	}
	rows, labels := v.payRowsCache, v.payLabels // precomputed at variant compile
	if len(rows) == 0 {
		return
	}
	const glyphsW, gap = 3 * 2, 3 // three width-2 faces; gap between entries
	width := (len(rows) - 1) * gap
	for i := range rows {
		width += glyphsW + len(labels[i])
	}
	col := (kit.Cols - width) / 2
	if col < 0 {
		col = 0
	}
	for i, pr := range rows {
		if i > 0 {
			col += gap
		}
		for n := 0; n < 3; n++ {
			col = f.SetGraphemeWide(row, col, faceArt[pr.sym], stReelDim)
		}
		col = f.Text(row, col, labels[i], stLabel)
	}
}

// border draws a rounded horizontal edge with the given left/right corners.
func (rm *room) border(f *kit.Frame, row, col int, lc, rc rune, st kit.Style) {
	f.SetRune(row, col, lc, st)
	f.SetRune(row, col+cardW-1, rc, st)
	for c := col + 1; c < col+cardW-1; c++ {
		f.SetRune(row, c, '─', st)
	}
}

// body draws a "LABEL    value" interior line (label left, number right).
func (rm *room) body(f *kit.Frame, row, col int, label string, val int) {
	f.Text(row, col+2, label, stLabel)
	f.TextRight(row, col+cardW-2, fmt.Sprintf("%d", val), stTitle)
}

func (rm *room) status(f *kit.Frame, row, col int, m *machine) {
	text, st := "ready", stReady
	switch {
	case m.flash == "RE-BUY":
		text, st = m.flash, stRebuy
	case m.flash != "":
		text, st = m.flash, stWin
	case m.spin != nil:
		text, st = "spinning", stReady
	}
	if len(text) > cardW-2 {
		text = text[:cardW-2]
	}
	f.Text(row, col+(cardW-len(text))/2, text, st)
}

// lever draws the side lever; the knob sits high when idle and drops while the
// reels spin, so a pull reads as motion.
func (rm *room) lever(f *kit.Frame, col, top int, m *machine) {
	lx := col + 11
	knob := top + 1 // idle: up
	if m.spin != nil {
		knob = top + 3 // pulled: down
	}
	for r := 1; r <= 4; r++ {
		ch := '│' // arm
		if top+r == knob {
			ch = 'O' // knob
		}
		f.SetRune(top+r, lx, ch, stLever)
	}
	f.SetRune(top+5, lx, '┴', stLever) // pivot
}

// grid returns the 3x3 visible faces as symbols, indexed [row][reel] with row 0
// the top, row 1 the center payline, row 2 the bottom. Reels scroll while
// spinning (cycle derived from elapsed time), freeze to their landing window as
// they settle, show the last result when idle, and are blank before the first
// spin.
func (rm *room) grid(m *machine) [3][3]symbol {
	var out [3][3]symbol
	for reel := 0; reel < 3; reel++ {
		var w [3]symbol
		switch {
		case m.spin != nil && reel >= m.spin.landed:
			w = rollWindow(spinStrip(m.spin), m.spin.cycle(rm.lastNow)*scrollSpeed+reel*7)
		case m.spin != nil:
			w = windowAt(spinStrip(m.spin), m.spin.stopIdx[reel])
		case m.spun:
			w = windowAt(idleStrip(m), m.lastIdx[reel])
		default:
			w = [3]symbol{symBlank, symBlank, symBlank}
		}
		for row := 0; row < 3; row++ {
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
