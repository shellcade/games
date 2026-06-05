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
	scrollSpeed = 2 // reel-strip rows advanced per animation cycle while spinning
)

var (
	stTitle   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim     = kit.Style{FG: kit.DimGray}
	stTicker  = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stBordOwn = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stBordDim = kit.Style{FG: kit.DimGray}
	stNameOwn = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stName    = kit.Style{FG: kit.White}
	stPayline = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold} // center row
	stReelDim = kit.Style{FG: kit.DimGray}                     // top/bottom rows
	stMarker  = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stLabel   = kit.Style{FG: kit.DimGray}
	stWin     = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stRebuy   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stReady   = kit.Style{FG: kit.DimGray}
	stLever   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
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
		msg := "* " + rm.ticker.text + " *"
		f.Text(1, (kit.Cols-len(msg))/2, msg, stTicker)
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

	f.Text(kit.Rows-1, 2, "Up/Down bet   SPACE spin   Esc leave", stDim)
	if m := rm.machines[v.AccountID]; m != nil {
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
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
	if own {
		bord, nameSt = stBordOwn, stNameOwn
	}
	left, right := col, col+cardW-1

	// Top border with the (truncated) handle.
	rm.border(f, top, col, '╭', '╮', bord)
	name := id
	if p, ok := rm.names[id]; ok {
		name = p.Handle
	}
	if maxName := cardW - 4; len(name) > maxName {
		name = name[:maxName]
	}
	f.Text(top, col+2, " "+name+" ", nameSt)

	// Sides for every interior row.
	for r := 1; r <= 9; r++ {
		f.SetRune(top+r, left, '│', bord)
		f.SetRune(top+r, right, '│', bord)
	}

	// Reel screen box (cols col+2..col+8): ╭─────╮ / │ . . . │ / ╰─────╯
	sx := col + 2
	f.SetRune(top+1, sx, '╭', bord)
	f.SetRune(top+5, sx, '╰', bord)
	f.SetRune(top+1, sx+6, '╮', bord)
	f.SetRune(top+5, sx+6, '╯', bord)
	for c := sx + 1; c < sx+6; c++ {
		f.SetRune(top+1, c, '─', bord)
		f.SetRune(top+5, c, '─', bord)
	}
	for r := 2; r <= 4; r++ {
		f.SetRune(top+r, sx, '│', bord)
		f.SetRune(top+r, sx+6, '│', bord)
	}

	// The 3x3 faces; the center row (top+3) is the payline.
	g := rm.grid(m)
	for row := 0; row < 3; row++ {
		st := stReelDim
		if row == 1 {
			st = stPayline
		}
		for reel := 0; reel < 3; reel++ {
			f.SetRune(top+2+row, sx+1+reel*2, g[row][reel], st)
		}
	}
	// Payline markers pointing at the center row.
	f.SetRune(top+3, sx-1, '>', stMarker)
	f.SetRune(top+3, sx+7, '<', stMarker)

	// Lever to the right of the screen: knob rides up when idle, drops mid-spin.
	rm.lever(f, col, top, m)

	// Readouts.
	rm.body(f, top+6, col, "HI", m.highScore)
	rm.body(f, top+7, col, "BAL", m.balance)
	rm.body(f, top+8, col, "BET", m.bet)
	rm.status(f, top+9, col, m)

	// Bottom border with a coin slot.
	rm.border(f, top+10, col, '╰', '╯', bord)
	f.Text(top+10, col+5, "[__]", bord)
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

// grid returns the 3x3 visible faces as runes, indexed [row][reel] with row 0
// the top, row 1 the center payline, row 2 the bottom. Reels scroll while
// spinning (cycle derived from elapsed time), freeze to their landing window as
// they settle, show the last result when idle, and are blank before the first
// spin.
func (rm *room) grid(m *machine) [3][3]rune {
	var out [3][3]rune
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
			out[row][reel] = rune(w[row])
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
