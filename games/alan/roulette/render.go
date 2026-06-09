package main

import (
	"strconv"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Rendering. One shared felt, composed per viewer so each player sees their own
// cursor, chip stake, and bet list over the common table. The number grid is a
// real bordered 3x12 table drawn with box lines, so the cursor can sit on a
// number, on the line between two numbers (a split), or on an intersection (a
// corner) — and a chip lands exactly there. While the wheel is spinning the
// whole screen switches to a dedicated wheel view (drawSpinScreen).
//
// Grid geometry: the zero box occupies the left margin; each number cell is
// IW-wide with single-column lines between, and the dozen / even-money / column
// boxes ring the grid. A fine lattice point (fr, fc) maps to the screen as
// row = gridTop+fr, and a column derived from the cell pitch.
const (
	gridTop   = 2 // screen row of the grid's top border (fine row fr=0)
	gridLeft  = 4 // screen col of the grid's left border (line k=0)
	pitch     = 4 // screen columns per number cell (IW interior + 1 line)
	iw        = 3 // number-cell interior width
	zeroCol   = 1 // where the "0" sits in the left margin
	colBoxCol = 54
	colBoxW   = 4
	panelLeft = 59
	helpRow   = 23
)

// rowOfRR / colInterior / lineCol map grid coordinates to the screen.
func rowOfRR(rr int) int  { return gridTop + 1 + 2*rr } // number rows 3,5,7
func lineCol(k int) int   { return gridLeft + k*pitch } // boundary line columns
func colInterior(c int) int {
	return lineCol(c) + 1 // a cell's interior left col (number drawn at +1..+2)
}

var (
	stTitle = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stDim   = kit.Style{FG: kit.DimGray}
	stHead  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stFrame = kit.Style{FG: kit.Gray(0x55)}

	stRedFelt   = kit.Style{FG: kit.White, BG: kit.RGB(0xb0, 0x20, 0x20), Attr: kit.AttrBold}
	stBlackFelt = kit.Style{FG: kit.White, BG: kit.Gray(0x30), Attr: kit.AttrBold}
	stGreenFelt = kit.Style{FG: kit.White, BG: kit.RGB(0x10, 0x80, 0x30), Attr: kit.AttrBold}

	stOutside = kit.Style{FG: kit.White, BG: kit.Gray(0x28)}
	stChip    = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stWin     = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stLose    = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stReady   = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stArmed   = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stCursor  = kit.Style{FG: kit.Yellow, BG: kit.Gray(0x44), Attr: kit.AttrBold | kit.AttrReverse}
	stCover   = kit.Style{Attr: kit.AttrBold | kit.AttrUnderline}
	stShared  = kit.Style{FG: kit.White, Attr: kit.AttrBold} // a spot held by 2+ players
)

// chipColors is the per-player chip palette: each seated player gets a distinct
// colour (assigned in room.freeColorIdx), drawn on the felt so everyone sees
// everyone's chips. The roster doubles as the legend.
var chipColors = []kit.Color{
	kit.RGB(0x4f, 0xc3, 0xf7), // sky blue
	kit.RGB(0xff, 0xa7, 0x26), // amber
	kit.RGB(0xba, 0x68, 0xc8), // violet
	kit.RGB(0x4d, 0xd0, 0xb1), // teal
	kit.RGB(0xf0, 0x6e, 0x8e), // pink
	kit.RGB(0xc5, 0xe1, 0x7a), // lime
}

const numChipColors = 6

func chipStyle(idx int) kit.Style {
	if idx < 0 || idx >= len(chipColors) {
		idx = 0
	}
	return kit.Style{FG: chipColors[idx], Attr: kit.AttrBold}
}

// render composes and sends a per-viewer frame to every member.
func (rm *room) render(r kit.Room) {
	rm.lastNow = r.Now()
	for _, v := range r.Members() {
		r.Send(v, rm.compose(v))
	}
}

func (rm *room) compose(v kit.Player) *kit.Frame {
	f := rm.frame
	f.Clear()
	now := rm.lastNow
	pl := rm.players[v.AccountID]
	rm.viewer = pl

	// The wheel takes over the whole screen while it spins.
	if rm.phase == phSpinning {
		rm.drawSpinScreen(f, now)
		return f
	}

	f.Text(0, 2, "* ROULETTE *", stTitle)
	rm.drawStatusLine(f, now)
	rm.drawMarquee(f, 1)
	rm.drawFelt(f, pl)
	rm.drawRoster(f, v)
	rm.drawSidebar(f, pl)
	rm.drawHelp(f, pl)
	return f
}

func (rm *room) drawStatusLine(f *kit.Frame, now time.Time) {
	var msg string
	st := stDim
	switch rm.phase {
	case phBetting:
		if rm.closing {
			msg = "ALL READY - spinning..."
			st = stReady
		} else {
			msg = "PLACE BETS  " + strconv.Itoa(rm.remaining(now)) + "s"
		}
	case phResults:
		msg = "WINNER: " + numLabel(rm.result) + "  -  next " + strconv.Itoa(rm.remaining(now)) + "s"
		st = colorStyle(rm.result)
	}
	f.TextRight(0, kit.Cols-2, msg, st)
}

func (rm *room) drawMarquee(f *kit.Frame, row int) {
	if len(rm.history) == 0 {
		f.Text(row, 2, "no spins yet - place your chips and ready up", stDim)
		return
	}
	col := f.Text(row, 2, "recent ", stDim)
	for _, n := range rm.history {
		col = f.Text(row, col, strconv.Itoa(n), colorStyle(n))
		col = f.Text(row, col, " ", stDim)
	}
}

// --- the felt ---------------------------------------------------------------

func (rm *room) drawFelt(f *kit.Frame, pl *player) {
	rm.drawZeroBox(f)
	rm.drawGridFrame(f)
	for n := 1; n <= 36; n++ {
		rm.drawNumber(f, n, "")
	}
	rm.drawOutsideBoxes(f)

	// Overlays: every player's chips (each in their colour), then this viewer's
	// cursor, then (at results) the winning number.
	rm.drawAllChips(f)
	if pl != nil && rm.phase == phBetting {
		rm.drawCursor(f, pl)
	}
	if rm.phase == phResults {
		rm.highlightWinner(f)
	}
}

// drawZeroBox paints the green zero spanning the three number rows.
func (rm *room) drawZeroBox(f *kit.Frame) {
	for r := rowOfRR(0); r <= rowOfRR(2); r++ {
		for c := 0; c < gridLeft; c++ {
			f.SetRune(r, c, ' ', stGreenFelt)
		}
	}
	f.Text(rowOfRR(1), zeroCol, "0", stGreenFelt)
}

// drawGridFrame draws the box lines of the 3x12 number grid.
func (rm *room) drawGridFrame(f *kit.Frame) {
	const cols = 12
	// Horizontal rules: top (fr0), the two inner separators (fr2, fr4), bottom (fr6).
	for _, fr := range []int{0, 2, 4, 6} {
		row := gridTop + fr
		for c := lineCol(0); c <= lineCol(cols); c++ {
			f.SetRune(row, c, '-', stFrame)
		}
		for k := 0; k <= cols; k++ {
			f.SetRune(row, lineCol(k), junction(fr, k, cols), stFrame)
		}
	}
	// Verticals on the three number rows.
	for rr := 0; rr <= 2; rr++ {
		row := rowOfRR(rr)
		for k := 0; k <= cols; k++ {
			f.SetRune(row, lineCol(k), '|', stFrame)
		}
	}
}

// junction returns the box-corner glyph at horizontal rule fr and line column k.
func junction(fr, k, cols int) rune {
	left := k == 0
	right := k == cols
	switch fr {
	case 0: // top
		if left {
			return '+'
		}
		if right {
			return '+'
		}
		return '+'
	default:
		return '+'
	}
}

// drawNumber paints number n in its felt colour; marker (if non-empty) occupies
// the cell's left slot (used for a chip dot).
func (rm *room) drawNumber(f *kit.Frame, n int, marker string) {
	rr, c := gridRC(n)
	row := rowOfRR(rr)
	col := colInterior(c)
	st := feltStyle(n)
	f.SetRune(row, col, ' ', st)
	f.Text(row, col+1, pad2(n), st)
	if marker != "" {
		f.Text(row, col, marker, stChip)
	}
}

// drawOutsideBoxes draws the dozen, even-money, and column "2:1" boxes.
func (rm *room) drawOutsideBoxes(f *kit.Frame) {
	// Dozens (row below the grid): three 16-wide boxes.
	rm.drawBox(f, dozenRowY, lineCol(0), 16, "1st 12", stOutside)
	rm.drawBox(f, dozenRowY, lineCol(4), 16, "2nd 12", stOutside)
	rm.drawBox(f, dozenRowY, lineCol(8), 17, "3rd 12", stOutside)
	// Even-money strip: six 8-wide boxes.
	rm.drawBox(f, evenRowY, lineCol(0), 8, "1-18", stOutside)
	rm.drawBox(f, evenRowY, lineCol(2), 8, "EVEN", stOutside)
	rm.drawBox(f, evenRowY, lineCol(4), 8, "RED", redBox())
	rm.drawBox(f, evenRowY, lineCol(6), 8, "BLACK", blackBox())
	rm.drawBox(f, evenRowY, lineCol(8), 8, "ODD", stOutside)
	rm.drawBox(f, evenRowY, lineCol(10), 9, "19-36", stOutside)
	// Column "2:1" boxes at the right of each number row.
	for rr := 0; rr <= 2; rr++ {
		rm.drawBox(f, rowOfRR(rr), colBoxCol, colBoxW, "2:1", stOutside)
	}
}

const (
	dozenRowY = gridTop + 8  // row 10
	evenRowY  = gridTop + 10 // row 12
)

func redBox() kit.Style   { return kit.Style{FG: kit.White, BG: kit.RGB(0xb0, 0x20, 0x20)} }
func blackBox() kit.Style { return kit.Style{FG: kit.White, BG: kit.Gray(0x30)} }

func (rm *room) drawBox(f *kit.Frame, row, col, w int, label string, st kit.Style) {
	for i := 0; i < w; i++ {
		f.SetRune(row, col+i, ' ', st)
	}
	f.Text(row, col+centerPad(w, len(label)), label, st)
}

// --- cursor + overlays ------------------------------------------------------

// drawCursor highlights the covered numbers and marks the exact lattice point
// (number / split line / corner / outside box) the cursor sits on.
func (rm *room) drawCursor(f *kit.Frame, pl *player) {
	mi := pl.sel.betIndex()
	b := masterBets[mi]

	// Light up every covered number.
	for _, n := range b.nums {
		rm.shadeNumber(f, n, stCover)
	}

	if b.outside {
		rm.shadeOutside(f, b, stCursor)
		return
	}

	sp := spots[mi]
	row := gridTop + sp.fr
	switch {
	case sp.fc < 0: // straight on 0
		f.SetRune(rowOfRR(1), zeroCol, '0', stCursor)
	case sp.fr%2 == 1 && sp.fc%2 == 1: // a number centre
		rm.shadeNumber(f, b.nums[0], stCursor)
	case sp.fr%2 == 1 && sp.fc%2 == 0: // vertical line: a split
		f.SetRune(row, lineCol(sp.fc/2), '#', stCursor)
	case sp.fr%2 == 0 && sp.fc%2 == 1: // horizontal line: a split / street
		c := sp.fc / 2
		for i := 0; i < iw; i++ {
			f.SetRune(row, colInterior(c)+i, '=', stCursor)
		}
	default: // an intersection: corner / six-line
		f.SetRune(row, lineCol(sp.fc/2), '#', stCursor)
	}
}

// shadeNumber re-styles number n's cell, preserving its felt background.
func (rm *room) shadeNumber(f *kit.Frame, n int, overlay kit.Style) {
	if n == 0 {
		st := merge(stGreenFelt, overlay)
		f.SetRune(rowOfRR(1), zeroCol, '0', st)
		return
	}
	rr, c := gridRC(n)
	row := rowOfRR(rr)
	st := merge(feltStyle(n), overlay)
	col := colInterior(c)
	f.SetRune(row, col, ' ', st)
	f.Text(row, col+1, pad2(n), st)
}

// shadeOutside re-styles an outside box.
func (rm *room) shadeOutside(f *kit.Frame, b bet, overlay kit.Style) {
	row, col, w := outsideRect(b)
	base := stOutside
	switch b.kind {
	case kRed:
		base = redBox()
	case kBlack:
		base = blackBox()
	}
	st := merge(base, overlay)
	for i := 0; i < w; i++ {
		f.SetRune(row, col+i, ' ', st)
	}
	f.Text(row, col+centerPad(w, len(outsideLabel(b))), outsideLabel(b), st)
}

// chipPos returns the screen cell where a bet's chip marker sits — a number's
// left slot, the centre of a street/split line, the line/cross of a split or
// corner, or the corner of an outside box.
func chipPos(mi int) (row, col int) {
	b := masterBets[mi]
	if b.outside {
		row, col, _ := outsideRect(b)
		return row, col
	}
	sp := spots[mi]
	switch {
	case sp.fc < 0:
		return rowOfRR(1), zeroCol
	case sp.fr%2 == 1 && sp.fc%2 == 1:
		rr, c := gridRC(b.nums[0])
		return rowOfRR(rr), colInterior(c)
	case sp.fr%2 == 0 && sp.fc%2 == 1:
		// A horizontal line (a vertical split, or a street on the bottom edge):
		// centre the chip in the cell's line segment, not on the left grid line.
		return gridTop + sp.fr, colInterior(sp.fc/2) + iw/2
	default:
		// A vertical line (a horizontal split) or an intersection (corner /
		// six-line): the chip sits on the line/cross itself.
		return gridTop + sp.fr, lineCol(sp.fc / 2)
	}
}

// drawAllChips marks every player's chips on the felt: a spot held by one
// player shows a '*' in that player's colour; a spot shared by two or more
// shows a white '+'. The chipBits scratch (a per-bet bitmask of player colours)
// is reused and reset here, so no allocation leaks per render.
func (rm *room) drawAllChips(f *kit.Frame) {
	for i := range rm.chipBits {
		rm.chipBits[i] = 0
	}
	for _, id := range rm.order {
		p := rm.players[id]
		if p == nil {
			continue
		}
		for _, b := range p.bets {
			rm.chipBits[b.master] |= 1 << uint(p.colorIdx)
		}
	}
	for mi, bits := range rm.chipBits {
		if bits == 0 {
			continue
		}
		row, col := chipPos(mi)
		if onlyOneBit(bits) {
			f.SetRune(row, col, '*', chipStyle(lowestBit(bits)))
		} else {
			f.SetRune(row, col, '+', stShared)
		}
	}
}

func onlyOneBit(b uint8) bool { return b != 0 && b&(b-1) == 0 }

func lowestBit(b uint8) int {
	for i := 0; i < 8; i++ {
		if b&(1<<uint(i)) != 0 {
			return i
		}
	}
	return 0
}

func (rm *room) highlightWinner(f *kit.Frame) {
	rm.shadeNumber(f, rm.result, kit.Style{Attr: kit.AttrReverse | kit.AttrBold})
}

// --- the spinning screen ----------------------------------------------------

func (rm *room) drawSpinScreen(f *kit.Frame, now time.Time) {
	title := "* * *   S P I N N I N G   * * *"
	f.Text(4, (kit.Cols-len(title))/2, title, stTitle)

	idx := rm.wheelDisplayIndex(now)

	// The pocket track, scrolling then resting, with a pointer over the centre.
	const window, slotW = 9, 6
	center := window / 2
	trackW := window * slotW
	left := (kit.Cols - trackW) / 2
	top := 10
	px := left + center*slotW + slotW/2
	f.SetRune(top-1, px, 'v', stTitle)
	f.SetRune(top+3, px, '^', stTitle)
	for i := 0; i <= window; i++ {
		x := left + i*slotW
		f.SetRune(top, x, '+', stFrame)
		f.SetRune(top+2, x, '+', stFrame)
		f.SetRune(top+1, x, '|', stFrame)
		if i < window {
			for j := 1; j < slotW; j++ {
				f.SetRune(top, x+j, '-', stFrame)
				f.SetRune(top+2, x+j, '-', stFrame)
			}
		}
	}
	for i := 0; i < window; i++ {
		pi := ((idx-center+i)%pockets + pockets) % pockets
		n := wheelSeq[pi]
		st := feltStyle(n)
		x := left + i*slotW + 1
		for j := 0; j < slotW-1; j++ {
			f.SetRune(top+1, x+j, ' ', st)
		}
		s := strconv.Itoa(n)
		f.Text(top+1, x+(slotW-1-len(s))/2, s, st)
	}

	// Once the wheel has come to rest, hold on the result for a beat.
	if now.Sub(rm.spinStart) >= spinAnimDur {
		msg := "rests on " + numLabel(rm.result)
		f.Text(top+6, (kit.Cols-len(msg))/2, msg, colorStyle(rm.result))
	} else {
		msg := "the ball is rolling..."
		f.Text(top+6, (kit.Cols-len(msg))/2, msg, stDim)
	}
}

// wheelDisplayIndex returns the strip index under the pointer: a decelerating
// sweep that lands exactly on the result (integer-eased, so native and wasm
// agree byte-for-byte), or the last result at rest.
func (rm *room) wheelDisplayIndex(now time.Time) int {
	switch rm.phase {
	case phSpinning:
		ms := now.Sub(rm.spinStart).Milliseconds()
		total := spinAnimDur.Milliseconds() // decelerate over the anim window, then hold
		p := ms * 1000 / total
		if p < 0 {
			p = 0
		}
		if p > 1000 {
			p = 1000
		}
		u := 1000 - p
		eased := 1000 - u*u*u/1_000_000 // ease-out cubic, in permille
		target := int64(wheelIndex(rm.result))
		steps := 4*int64(pockets) + target
		step := steps * eased / 1000
		return int((step%int64(pockets) + int64(pockets)) % int64(pockets))
	case phResults:
		return wheelIndex(rm.result)
	default:
		if rm.spunOnce {
			return wheelIndex(rm.result)
		}
		return 0
	}
}

// --- roster panel -----------------------------------------------------------

func (rm *room) drawRoster(f *kit.Frame, v kit.Player) {
	f.Text(gridTop, panelLeft, "TABLE", stHead)
	row := gridTop + 1
	for _, id := range rm.order {
		pl := rm.players[id]
		if pl == nil || row > gridTop+8 {
			continue
		}
		name := pl.p.Handle
		if len(name) > 7 {
			name = name[:7]
		}
		// Chip-colour swatch + name (the roster is the legend): the viewer's own
		// row is bold so they can spot their colour.
		f.SetRune(row, panelLeft, '*', chipStyle(pl.colorIdx))
		nameSt := kit.Style{FG: chipColors[pl.colorIdx]}
		if id == v.AccountID {
			nameSt.Attr |= kit.AttrBold
		}
		f.Text(row, panelLeft+2, name, nameSt)
		f.TextRight(row, kit.Cols-1, pad2r(pl.balance, 5), stHead)
		switch rm.phase {
		case phResults:
			if pl.lastPlayed {
				f.Text(row, panelLeft+10, signed(pl.lastNet), netStyle(pl.lastNet))
			} else {
				f.Text(row, panelLeft+10, ".", stDim)
			}
		default:
			if pl.ready {
				f.Text(row, panelLeft+10, "rdy", stReady)
			} else if s := pl.staked(); s > 0 {
				f.Text(row, panelLeft+10, "@"+strconv.Itoa(s), stChip)
			}
		}
		row++
	}
}

// --- sidebar: armed bet + your chips ----------------------------------------

func (rm *room) drawSidebar(f *kit.Frame, pl *player) {
	if pl == nil {
		return
	}
	if rm.phase == phBetting {
		b := masterBets[pl.sel.betIndex()]
		name := b.kind.name() + " " + b.label
		if b.outside {
			name = b.label
		}
		f.Text(14, 2, "> "+name+"   pays "+strconv.Itoa(b.kind.payout())+":1", stArmed)
		f.TextRight(14, kit.Cols-2, "chip "+strconv.Itoa(stakeTiers[pl.stakeIdx]), stHead)
	}

	header := "your chips (" + strconv.Itoa(pl.staked()) + " down):"
	if rm.phase == phResults {
		header = "your chips this round:"
	}
	f.Text(15, 2, header, stHead)
	groups := rm.groupBets(pl.bets)
	if len(groups) == 0 {
		f.Text(16, 4, "none yet", stDim)
		return
	}
	yourSt := chipStyle(pl.colorIdx)
	col, row := 4, 16
	for i, g := range groups {
		seg := masterBets[g.master].label + " x" + strconv.Itoa(g.stake)
		if col+len(seg) > kit.Cols-2 {
			row++
			col = 4
		}
		if row > 21 { // out of room
			f.Text(21, col, "+"+strconv.Itoa(len(groups)-i)+" more", stDim)
			break
		}
		col = f.Text(row, col, seg, yourSt)
		col = f.Text(row, col, "   ", stDim)
	}
}

func (rm *room) drawHelp(f *kit.Frame, pl *player) {
	var help string
	switch rm.phase {
	case phBetting:
		help = "arrows move (numbers/lines/corners)  enter place  +/-chip  bksp undo  c clear  r ready"
		if len(help) > kit.Cols-12 {
			help = "move  enter place  +/-chip  bksp undo  c clear  r ready"
		}
	case phResults:
		help = "paying out - next round opens shortly"
	}
	f.Text(helpRow, 2, help, stDim)
	if pl != nil {
		f.TextRight(helpRow, kit.Cols-2, "BAL "+strconv.Itoa(pl.balance), stTitle)
	}
}

// --- lookups / styling ------------------------------------------------------

type betGroup struct {
	master int
	stake  int
}

func (rm *room) groupBets(bets []placedBet) []betGroup {
	gs := rm.groupBuf[:0]
	for _, b := range bets {
		found := false
		for i := range gs {
			if gs[i].master == b.master {
				gs[i].stake += b.stake
				found = true
				break
			}
		}
		if !found {
			gs = append(gs, betGroup{master: b.master, stake: b.stake})
		}
	}
	rm.groupBuf = gs
	return gs
}

// outsideRect returns the screen rect of an outside box (matches drawOutsideBoxes).
func outsideRect(b bet) (row, col, w int) {
	switch b.kind {
	case kColumn:
		switch b.nums[0] {
		case 1:
			return rowOfRR(2), colBoxCol, colBoxW
		case 2:
			return rowOfRR(1), colBoxCol, colBoxW
		default:
			return rowOfRR(0), colBoxCol, colBoxW
		}
	case kDozen:
		switch b.nums[0] {
		case 1:
			return dozenRowY, lineCol(0), 16
		case 13:
			return dozenRowY, lineCol(4), 16
		default:
			return dozenRowY, lineCol(8), 17
		}
	case kLow:
		return evenRowY, lineCol(0), 8
	case kEven:
		return evenRowY, lineCol(2), 8
	case kRed:
		return evenRowY, lineCol(4), 8
	case kBlack:
		return evenRowY, lineCol(6), 8
	case kOdd:
		return evenRowY, lineCol(8), 8
	case kHigh:
		return evenRowY, lineCol(10), 9
	}
	return evenRowY, lineCol(0), 8
}

func outsideLabel(b bet) string {
	if b.kind == kColumn {
		return "2:1"
	}
	return b.label
}

func feltStyle(n int) kit.Style {
	switch {
	case n == 0:
		return stGreenFelt
	case colorOf(n) == red:
		return stRedFelt
	default:
		return stBlackFelt
	}
}

// merge overlays attrs (and a non-default FG) onto a base felt style, keeping
// the base background so a highlighted number keeps its red/black/green felt.
func merge(base, overlay kit.Style) kit.Style {
	base.Attr |= overlay.Attr
	if overlay.FG.IsSet() {
		base.FG = overlay.FG
	}
	if overlay.BG.IsSet() {
		base.BG = overlay.BG
	}
	return base
}

func colorStyle(n int) kit.Style {
	switch colorOf(n) {
	case green:
		return kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	case red:
		return kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	default:
		return kit.Style{FG: kit.White, Attr: kit.AttrBold}
	}
}

func netStyle(net int) kit.Style {
	if net > 0 {
		return stWin
	}
	if net < 0 {
		return stLose
	}
	return stDim
}

func numLabel(n int) string {
	name := "GREEN"
	switch colorOf(n) {
	case red:
		name = "RED"
	case black:
		name = "BLACK"
	}
	return strconv.Itoa(n) + " " + name
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return " " + s
	}
	return s
}

func pad(s string, w int) string {
	for len(s) < w {
		s += " "
	}
	return s
}

func pad2r(n, w int) string {
	s := strconv.Itoa(n)
	for len(s) < w {
		s = " " + s
	}
	return s
}

func signed(n int) string {
	if n > 0 {
		return "+" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func centerPad(w, l int) int {
	if l >= w {
		return 0
	}
	return (w - l) / 2
}
