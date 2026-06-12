package main

import (
	"time"
	"unicode/utf8"

	kit "github.com/shellcade/kit/v2"
)

// The sector screen's fixed chrome (see ARTBOARDS.md):
const (
	rowStatus = 0
	rowOrder  = 2 // order box rows 2..5
	rowBanner = 6
	rowHdr    = 7
	rowPanel  = 8 // widget rows 8..13 and 14..19
	rowComms1 = 21
	rowComms2 = 22
	rowHints  = 23

	widgetW = 20
	widgetH = 6
)

var (
	stTitle  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim    = kit.Style{FG: kit.Gray(0x88)}
	stFaint  = kit.Style{FG: kit.Gray(0x55)}
	stHull   = kit.Style{FG: kit.RGB(0xff, 0x49, 0x6b), Attr: kit.AttrBold}
	stWarp   = kit.Style{FG: kit.RGB(0x4f, 0xd0, 0xe8)}
	stKey    = kit.Style{FG: kit.RGB(0xff, 0xd7, 0x4a), Attr: kit.AttrBold}
	stOrder  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stGood   = kit.Style{FG: kit.RGB(0x49, 0xe2, 0x6a), Attr: kit.AttrBold}
	stBad    = kit.Style{FG: kit.RGB(0xff, 0x49, 0x6b), Attr: kit.AttrBold}
	stAmber  = kit.Style{FG: kit.RGB(0xff, 0xe0, 0x3a), Attr: kit.AttrBold}
	stComms  = kit.Style{FG: kit.RGB(0x6f, 0xc7, 0xd0)}
	stBorder = kit.Style{FG: kit.Gray(0x60)}
	stState  = kit.Style{FG: kit.White}

	barGood = kit.Style{FG: kit.RGB(0x49, 0xe2, 0x6a)}
	barWarn = kit.Style{FG: kit.RGB(0xff, 0xe0, 0x3a)}
	barCrit = kit.Style{FG: kit.RGB(0xff, 0x49, 0x6b)}
)

func (rm *room) render(r kit.Room) {
	for _, v := range r.Members() {
		rm.frame.Clear()
		rm.compose(rm.frame, v)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(f *kit.Frame, v kit.Player) {
	switch rm.phase {
	case phLobby:
		rm.drawLobby(f, v)
	case phWarp:
		rm.drawWarp(f)
	case phOver:
		rm.drawOver(f)
	case phSector:
		c := rm.crewByID(v.AccountID)
		rm.drawStatus(f)
		if c == nil || !c.boarded {
			center(f, 11, "BEAMING ABOARD AT NEXT WARP...", stTitle)
			center(f, 13, "watch the comms — the crew is mid-sector", stDim)
		} else {
			rm.drawOrderBox(f, c)
			rm.drawBanner(f)
			rm.drawPanelHdr(f, c)
			rm.drawPanel(f, c)
			if rm.meteorActive() {
				rm.drawMeteor(f, c)
			}
		}
		rm.drawComms(f, v)
		rm.drawHints(f, c)
	}
}

// --- shared chrome -----------------------------------------------------------

func (rm *room) drawStatus(f *kit.Frame) {
	col := f.Text(rowStatus, 1, "SECTOR ", stDim)
	col = drawInt(f, rowStatus, col, rm.sector, stTitle)
	col = f.Text(rowStatus, col, " · ", stDim)
	f.Text(rowStatus, col, sectorName(rm.sector), stTitle)

	// Shared hull, flashing on damage.
	hullSt, emptySt := stHull, stFaint
	if rm.now.Before(rm.hullFlashUntil) {
		hullSt = kit.Style{FG: kit.White, BG: kit.RGB(0xb0, 0x20, 0x30), Attr: kit.AttrBold}
		emptySt = hullSt
	}
	col = f.Text(rowStatus, 32, "HULL ", stDim)
	for i := 0; i < maxHull; i++ {
		g, st := '▮', hullSt
		if i >= rm.hull {
			g, st = '▯', emptySt
		}
		f.SetRune(rowStatus, col+i, g, st)
	}

	// Shared warp progress, scaled to a fixed 17-cell bar.
	col = f.Text(rowStatus, 52, "WARP ", stDim)
	filled := 0
	if rm.need > 0 {
		filled = rm.charges * 17 / rm.need
	}
	for i := 0; i < 17; i++ {
		g, st := '▰', stWarp
		if i >= filled {
			g, st = '▱', stFaint
		}
		f.SetRune(rowStatus, col+i, g, st)
	}
	col += 18
	col = drawInt(f, rowStatus, col, rm.charges, stWarp)
	col = f.Text(rowStatus, col, "/", stDim)
	drawInt(f, rowStatus, col, rm.need, stDim)
}

func (rm *room) drawOrderBox(f *kit.Frame, c *crew) {
	border := stBorder
	if rm.now.Before(c.goodUntil) {
		border = stGood
	}
	title := " YOUR ORDER "
	if rm.meteorActive() {
		title = " METEOR STORM "
		border = stBad
	}
	drawBox(f, rowOrder, 0, rowOrder+3, kit.Cols-1, border)
	f.Text(rowOrder, 2, title, stTitle)

	if rm.meteorActive() {
		f.Text(rowOrder+1, 2, "ORDERS SUSPENDED — BRACE FOR IMPACT", stBad)
		return
	}
	if !c.ord.active {
		f.Text(rowOrder+1, 2, "...", stDim)
		return
	}
	f.Text(rowOrder+1, 2, c.ord.text, stOrder)

	// The 40-cell countdown: green > amber (<50%) > red (<25%), numerals only
	// in the final 5s (turn-clock restraint).
	total := rm.orderDur()
	rem := c.ord.expires.Sub(rm.now)
	if rem < 0 {
		rem = 0
	}
	frac := float64(rem) / float64(total)
	st := barGood
	if frac < 0.25 {
		st = barCrit
	} else if frac < 0.5 {
		st = barWarn
	}
	filled := int(frac*40 + 0.999)
	if filled > 40 {
		filled = 40
	}
	for i := 0; i < 40; i++ {
		g, s := '▮', st
		if i >= filled {
			g, s = '▯', stFaint
		}
		f.SetRune(rowOrder+2, 2+i, g, s)
	}
	if rem <= 5*time.Second {
		col := drawInt(f, rowOrder+2, 44, int(rem.Seconds()), st)
		f.SetRune(rowOrder+2, col, 's', st)
	}
}

func (rm *room) drawBanner(f *kit.Frame) {
	switch {
	case rm.anStage == asWarn:
		if blink(rm.now) {
			col := f.Text(rowBanner, 1, "!! INBOUND: ", stAmber)
			col = f.Text(rowBanner, col, anomalyNames[rm.anKind], stAmber)
			f.Text(rowBanner, col, " !!", stAmber)
		}
	case rm.flareActive():
		f.Text(rowBanner, 1, "SOLAR FLARE — control labels scrambled", stAmber)
	case rm.wormActive():
		f.Text(rowBanner, 1, "WORMHOLE TRANSIT — panel mirrored, keys unchanged", stAmber)
	case rm.anKind == anLeak && rm.anStage == asLive:
		f.Text(rowBanner, 1, "COOLANT LEAK — wipe fogged controls (mash their key)", stAmber)
	case rm.now.Before(rm.fumbleUntil):
		col := f.Text(rowBanner, 1, "FUMBLED: ", stBad)
		f.Text(rowBanner, col, rm.fumbleText, kit.Style{FG: kit.Gray(0x99), Attr: kit.AttrDim})
	}
}

func (rm *room) drawPanelHdr(f *kit.Frame, c *crew) {
	col := f.Text(rowHdr, 1, "── YOUR PANEL ── ", stFaint)
	col = f.Text(rowHdr, col, handleOf(c.player), stTitle)
	col++
	for x := col; x < kit.Cols-1; x++ {
		f.SetRune(rowHdr, x, '─', stFaint)
	}
}

func (rm *room) drawPanel(f *kit.Frame, c *crew) {
	per := len(c.panel) / 2
	pad := (kit.Cols - per*widgetW) / 2
	mirror := rm.wormActive()
	for i := range c.panel {
		row, col := i/per, i%per
		if mirror {
			col = per - 1 - col
		}
		rm.drawWidget(f, rowPanel+row*widgetH, pad+col*widgetW, &c.panel[i], i == c.sel)
	}
}

func (rm *room) drawWidget(f *kit.Frame, top, left int, c *control, sel bool) {
	border := stBorder
	if rm.now.Before(c.litUntil) {
		border = stGood
	}
	if sel {
		border = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	}
	drawBox(f, top, left, top+widgetH-1, left+widgetW-1, border)

	// Hotkey badge on the top border: ┌[W]────
	f.SetRune(top, left+1, '[', stKey)
	f.SetRune(top, left+2, upper(c.key), stKey)
	f.SetRune(top, left+3, ']', stKey)

	in := left + 2 // interior text column
	if c.fog > 0 {
		for row := 1; row <= 4; row++ {
			for x := left + 1; x < left+widgetW-1; x++ {
				f.SetRune(top+row, x, '░', stComms)
			}
		}
		col := f.Text(top+3, in, " WIPE x", stTitle)
		drawInt(f, top+3, col, c.fog, stTitle)
		return
	}

	if rm.flareActive() {
		drawStatic(f, top+1, in, len(c.adj))
		drawStatic(f, top+2, in, len(c.jot))
	} else {
		f.Text(top+1, in, c.adj, stState)
		f.Text(top+2, in, c.jot, stState)
	}

	switch c.kind {
	case ckDial:
		f.Text(top+3, in, "DIAL", stDim)
		col := f.Text(top+3, in+9, "( ", stDim)
		col = drawInt(f, top+3, col, c.state, stTitle)
		f.Text(top+3, col, " )", stDim)
		for i := 0; i <= dialMax; i++ {
			g, st := '○', stDim
			if i == c.state {
				g, st = '●', stGood
			}
			f.SetRune(top+4, in+i*2, g, st)
			if i < dialMax {
				f.SetRune(top+4, in+i*2+1, '─', stFaint)
			}
		}
	case ckSwitch:
		f.Text(top+3, in, "SWITCH", stDim)
		if c.state == 1 {
			col := f.Text(top+4, in, "●───╴   ", stGood)
			f.Text(top+4, col, "ON", stGood)
		} else {
			col := f.Text(top+4, in, "╶───○   ", stDim)
			f.Text(top+4, col, "OFF", stDim)
		}
	case ckSlider:
		col := f.Text(top+3, in, "SLIDER  LVL ", stDim)
		col = drawInt(f, top+3, col, c.state, stTitle)
		f.Text(top+3, col, "/4", stDim)
		for i := 0; i < sliderMax; i++ {
			g, st := '▰', stGood
			if i >= c.state {
				g, st = '▱', stFaint
			}
			f.SetRune(top+4, in+i, g, st)
		}
	case ckButton:
		f.Text(top+3, in, "BUTTON", stDim)
		st := stState
		if rm.now.Before(c.litUntil) {
			st = kit.Style{FG: kit.White, Attr: kit.AttrBold | kit.AttrReverse}
		}
		f.Text(top+4, in+2, "( PRESS )", st)
	}
}

func drawStatic(f *kit.Frame, row, col, n int) {
	for i := 0; i < n; i++ {
		f.SetRune(row, col+i, '▒', stComms)
	}
}

// drawMeteor overlays the storm modal: the assigned mash key, the press bar,
// and the closing window.
func (rm *room) drawMeteor(f *kit.Frame, c *crew) {
	const top, left, w, h = 9, 22, 36, 7
	blank := kit.Cell{Rune: ' '}
	f.Fill(top, left, top+h-1, left+w-1, blank)
	drawDoubleBox(f, top, left, top+h-1, left+w-1, stBad)
	f.Text(top, left+9, " METEOR STORM ", stTitle)

	col := f.Text(top+2, left+6, "MASH  [", stTitle)
	col = drawRune(f, top+2, col, upper(c.mashKey), stKey)
	col = f.Text(top+2, col, "]  x", stTitle)
	drawInt(f, top+2, col, mashNeed, stTitle)

	n := c.mashN
	if n > mashNeed {
		n = mashNeed
	}
	for i := 0; i < mashNeed; i++ {
		g, st := '▮', stGood
		if i >= n {
			g, st = '▯', stFaint
		}
		f.SetRune(top+3, left+6+i, g, st)
	}
	col = drawInt(f, top+3, left+6+mashNeed+3, n, stTitle)
	col = f.Text(top+3, col, "/", stDim)
	drawInt(f, top+3, col, mashNeed, stDim)

	rem := rm.anEndAt.Sub(rm.now)
	if rem < 0 {
		rem = 0
	}
	col = drawInt(f, top+4, left+6, int(rem.Seconds()), stAmber)
	f.SetRune(top+4, col, 's', stAmber)
}

func (rm *room) drawComms(f *kit.Frame, v kit.Player) {
	f.Text(rowComms1, 1, "COMMS", stDim)
	// Newest two entries, oldest first, text column aligned.
	n := len(rm.hails)
	first := 0
	if n > 2 {
		first = n - 2
	}
	row := rowComms1
	for i := first; i < n; i++ {
		h := &rm.hails[i]
		f.SetRune(row, 8, '▸', stComms)
		if h.id == v.AccountID {
			f.Text(row, 10, "you", stTitle)
		} else {
			f.Text(row, 10, h.who, stComms)
		}
		f.Text(row, 19, h.text, stComms)
		// age, right-aligned-ish
		secs := int(rm.now.Sub(h.at).Seconds())
		if secs < 2 {
			f.Text(row, 70, "· just now", stFaint)
		} else {
			col := f.Text(row, 70, "· ", stFaint)
			col = drawInt(f, row, col, secs, stFaint)
			f.SetRune(row, col, 's', stFaint)
		}
		row++
	}
}

func (rm *room) drawHints(f *kit.Frame, c *crew) {
	if c == nil || !c.boarded {
		f.Text(rowHints, 1, "[esc] leave", stFaint)
		return
	}
	keys := "[w e r t s d f g] actuate"
	if len(c.panel) == 6 {
		keys = "[w e r s d f] actuate"
	}
	col := f.Text(rowHints, 1, keys, stFaint)
	if rm.boardedCount() > 1 {
		st := stDim
		if rm.now.Before(c.hailReadyAt) {
			st = stFaint
		}
		col = f.Text(rowHints, col+4, "[h] hail your order", st)
	}
	f.Text(rowHints, col+4, "[esc] leave", stFaint)
}

// --- the muster lobby ----------------------------------------------------------

func (rm *room) drawLobby(f *kit.Frame, v kit.Player) {
	drawDoubleBox(f, 1, 6, 4, 53, stBorder)
	f.Text(2, 9, "S P A C E T E R M", stTitle)
	f.Text(3, 9, "panic responsibly — a co-op bridge crew", stDim)

	col := f.Text(6, 6, "CREW MUSTER ── ", stDim)
	col = drawInt(f, 6, col, len(rm.crews), stTitle)
	f.Text(6, col, " ABOARD", stDim)

	row := 8
	for _, c := range rm.crews {
		if row > 13 {
			break
		}
		if c.player.Character.Glyph != "" {
			f.Set(row, 8, kit.CharacterCell(c.player.Character))
		} else {
			f.SetRune(row, 8, '◉', stComms)
		}
		f.Text(row, 10, handleOf(c.player), stTitle)
		f.Text(row, 20, "ENGINEER", stDim)
		f.Text(row, 32, "READY", stGood)
		if c.best > 0 {
			col = f.Text(row, 40, "best ", stFaint)
			drawInt(f, row, col, c.best, stFaint)
		}
		row++
	}

	col = f.Text(15, 6, "DIFFICULTY    ", stDim)
	col = f.Text(15, col, "< ", stFaint)
	col = f.Text(15, col, difficultyNames[rm.difficulty], stAmber)
	col = f.Text(15, col, " >", stFaint)
	f.Text(15, col+4, "(left/right to change — shared)", stFaint)

	f.Text(17, 6, "MISSION       ENDLESS — clear sectors until the hull gives out", stDim)

	col = f.Text(19, 6, "▸ [SPACE] LAUNCH", stGood)
	if !rm.lobbyUntil.IsZero() {
		rem := int(rm.lobbyUntil.Sub(rm.now).Seconds())
		if rem < 0 {
			rem = 0
		}
		col = f.Text(19, col+5, "auto-launch in ", stFaint)
		col = drawInt(f, 19, col, rem, stFaint)
		f.SetRune(19, col, 's', stFaint)
	}

	f.Text(rowHints, 1, "[< >] difficulty     [SPACE] launch     crew 1-6 — share the room code", stFaint)
}

// --- warp jump -------------------------------------------------------------------

// streak rows and phase offsets for the star-streak animation.
var streakRows = [...]int{1, 2, 3, 19, 20, 21}
var streakOff = [...]int{12, 47, 28, 5, 60, 36}

func (rm *room) drawWarp(f *kit.Frame) {
	elapsed := warpDur - rm.warpUntil.Sub(rm.now)
	shift := int(elapsed.Seconds() * 30)
	for i, row := range streakRows {
		x := (streakOff[i] + shift) % (kit.Cols + 20)
		for j := 0; j < 8; j++ {
			cx := x - 8 + j
			if cx < 0 || cx >= kit.Cols {
				continue
			}
			g := '─'
			if j >= 5 {
				g = '━'
			}
			f.SetRune(row, cx, g, stWarp)
		}
	}

	const top, left, w = 5, 20, 40
	drawDoubleBox(f, top, left, top+12, left+w-1, stBorder)
	center(f, top+2, "★  W A R P   J U M P  ★", stTitle)
	col := f.Text(top+4, left+4, "SECTOR ", stGood)
	col = drawInt(f, top+4, col, rm.sector, stGood)
	f.Text(top+4, col, " CLEAR", stGood)

	col = f.Text(top+6, left+4, "orders completed ............ ", stDim)
	drawInt(f, top+6, col, rm.warpOrders, stTitle)
	col = f.Text(top+7, left+4, "hull patched ................ +", stDim)
	drawInt(f, top+7, col, hullPatch, stGood)
	f.Text(top+8, left+4, "new panels issued to all crew", stDim)

	col = f.Text(top+10, left+4, "NEXT: SECTOR ", stDim)
	col = drawInt(f, top+10, col, rm.sector+1, stTitle)
	col = f.Text(top+10, col, " · ", stDim)
	f.Text(top+10, col, sectorName(rm.sector+1), stTitle)
}

// --- debrief ----------------------------------------------------------------------

func (rm *room) drawOver(f *kit.Frame) {
	center(f, 2, "✸ ✸ ✸  H U L L   B R E A C H  ✸ ✸ ✸", stBad)
	col := f.Text(4, 6, "THE SHIP COMES APART IN SECTOR ", stDim)
	col = drawInt(f, 4, col, rm.sector, stTitle)
	col = f.Text(4, col, " · ", stDim)
	f.Text(4, col, sectorName(rm.sector), stTitle)

	col = f.Text(6, 6, "SECTORS CLEARED: ", stDim)
	col = drawInt(f, 6, col, rm.score, stGood)
	best := 0
	for _, c := range rm.crews {
		if c.best > best {
			best = c.best
		}
	}
	col = f.Text(6, col+8, "room best: ", stFaint)
	drawInt(f, 6, col, best, stFaint)

	drawBox(f, 8, 6, 15, 59, stBorder)
	f.Text(8, 8, " CREW LOG ", stDim)
	f.Text(9, 9, "crew          orders   hails   fumbles", stFaint)
	row := 10
	for _, c := range rm.crews {
		if row > 14 {
			break
		}
		if c.player.Character.Glyph != "" {
			f.Set(row, 9, kit.CharacterCell(c.player.Character))
		} else {
			f.SetRune(row, 9, '◉', stComms)
		}
		f.Text(row, 11, handleOf(c.player), stState)
		drawIntRight(f, row, 27, c.done, stState)
		drawIntRight(f, row, 35, c.hailsSent, stState)
		drawIntRight(f, row, 44, c.fumbles, stState)
		row++
	}

	f.Text(18, 6, "▸ [SPACE] NEW SHIFT — same crew, fresh ship", stGood)
	f.Text(rowHints, 1, "[SPACE] back to the lobby     score posts to the Sectors leaderboard", stFaint)
}

// --- drawing helpers -----------------------------------------------------------------

func drawBox(f *kit.Frame, r0, c0, r1, c1 int, st kit.Style) {
	for c := c0 + 1; c < c1; c++ {
		f.SetRune(r0, c, '─', st)
		f.SetRune(r1, c, '─', st)
	}
	for r := r0 + 1; r < r1; r++ {
		f.SetRune(r, c0, '│', st)
		f.SetRune(r, c1, '│', st)
	}
	f.SetRune(r0, c0, '┌', st)
	f.SetRune(r0, c1, '┐', st)
	f.SetRune(r1, c0, '└', st)
	f.SetRune(r1, c1, '┘', st)
}

func drawDoubleBox(f *kit.Frame, r0, c0, r1, c1 int, st kit.Style) {
	for c := c0 + 1; c < c1; c++ {
		f.SetRune(r0, c, '═', st)
		f.SetRune(r1, c, '═', st)
	}
	for r := r0 + 1; r < r1; r++ {
		f.SetRune(r, c0, '║', st)
		f.SetRune(r, c1, '║', st)
	}
	f.SetRune(r0, c0, '╔', st)
	f.SetRune(r0, c1, '╗', st)
	f.SetRune(r1, c0, '╚', st)
	f.SetRune(r1, c1, '╝', st)
}

func drawRune(f *kit.Frame, row, col int, g rune, st kit.Style) int {
	f.SetRune(row, col, g, st)
	return col + 1
}

func drawInt(f *kit.Frame, row, col, n int, st kit.Style) int {
	if n < 0 {
		f.SetRune(row, col, '-', st)
		col++
		n = -n
	}
	w := intWidth(n)
	for i := w - 1; i >= 0; i-- {
		f.SetRune(row, col+i, rune('0'+n%10), st)
		n /= 10
	}
	return col + w
}

// drawIntRight draws n so its last digit lands on col `end`.
func drawIntRight(f *kit.Frame, row, end, n int, st kit.Style) {
	drawInt(f, row, end-intWidth(n)+1, n, st)
}

func intWidth(n int) int {
	if n < 0 {
		n = -n
	}
	w := 1
	for n >= 10 {
		w++
		n /= 10
	}
	return w
}

func center(f *kit.Frame, row int, s string, st kit.Style) int {
	return f.Text(row, (kit.Cols-utf8.RuneCountInString(s))/2, s, st)
}

func upper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

func blink(now time.Time) bool {
	return (now.UnixMilli()/300)%2 == 0
}
