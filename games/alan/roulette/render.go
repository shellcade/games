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
// board stays up (chips locked in) and the wheel runs in the panel below it
// (drawSpinnerPanel), where the betting sidebar sits the rest of the time.
//
// Grid geometry: the zero box occupies the left margin; each number cell is
// IW-wide with single-column lines between, and the dozen / even-money / column
// boxes ring the grid. A fine lattice point (fr, fc) maps to the screen as
// row = gridTop+fr, and a column derived from the cell pitch.
const (
	gridTop  = 2 // screen row of the grid's top border (fine row fr=0)
	zeroLeft = 4 // left border of the zero boxes — the board's left margin
	gridLeft = 9 // grid's left border (line k=0); also the zero boxes' right edge
	pitch    = 5 // screen columns per number cell (iw interior + 1 line)
	iw       = 4 // number-cell interior width

	colBoxCol = 71 // the "2:1" column boxes, just right of the grid
	colBoxW   = 5
	dozenW    = 4 * pitch // a dozen box spans four number columns
	evenW     = 2 * pitch // an even-money box spans two columns

	// The board fills rows 2..12; the players sit beneath it and the betting /
	// wheel / results panel below them.
	seatsRow = 14
	panelRow = 17
	helpRow  = 23
)

// zeroTextCol is the start column of a green pocket's label, right-aligned
// against the grid border inside its box; zeroChipCol is the chip slot to its
// left. The box interior is columns zeroLeft+1..gridLeft-1.
func zeroTextCol(n int) int { return gridLeft - len(pocketLabel(n)) }
func zeroChipCol(n int) int { return zeroTextCol(n) - 1 }

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

	f.Text(0, 2, "* ROULETTE *", stTitle)
	rm.drawMarquee(f) // recent winners, inline on the title row
	rm.drawStatusLine(f, now)
	// row 1 left blank for breathing room
	rm.drawFelt(f, pl)
	rm.drawSeats(f, v)
	// While the wheel spins, the board and seats stay up (chips locked in) and
	// the wheel runs in the panel below them, in place of the betting sidebar.
	if rm.phase == phSpinning {
		rm.drawSpinnerPanel(f, now, pl)
	} else {
		rm.drawSidebar(f, pl)
	}
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
	case phSpinning:
		msg = "no more bets - spinning..."
		st = stArmed
	case phResults:
		msg = "WINNER: " + numLabel(rm.result) + "  -  next " + strconv.Itoa(rm.remaining(now)) + "s"
		st = colorStyle(rm.result)
	}
	f.TextRight(0, kit.Cols-2, msg, st)
}

// drawMarquee shows the recent winners inline on the title row, newest first,
// trimmed before the status readout on the right.
func (rm *room) drawMarquee(f *kit.Frame) {
	const startCol, maxCol = 15, 50
	if len(rm.history) == 0 {
		return
	}
	col := f.Text(0, startCol, "recent ", stDim)
	for i := len(rm.history) - 1; i >= 0; i-- {
		s := pocketLabel(rm.history[i])
		if col+len(s)+1 > maxCol {
			break
		}
		col = f.Text(0, col, s, colorStyle(rm.history[i]))
		col = f.Text(0, col, " ", stDim)
	}
}

// --- the felt ---------------------------------------------------------------

func (rm *room) drawFelt(f *kit.Frame, pl *player) {
	rm.drawZeroBox(f)
	rm.drawGridFrame(f)
	for n := 1; n <= 36; n++ {
		rm.drawNumber(f, n)
	}
	rm.drawOutsideBoxes(f)

	// Overlays: every player's chips (each in their colour), then this viewer's
	// cursor, then (at results) the winning number.
	rm.drawAllChips(f)
	if pl != nil && rm.phase == phBetting {
		rm.drawCursor(f, pl)
	}
	if rm.winnerHighlightOn() {
		rm.highlightWinner(f)
		rm.drawAllChips(f) // redraw on top: a winning square must never hide its chip
	}
}

// winnerHighlightOn reports whether the winning squares are lit this frame: solid
// through the results board, and — once the ball has rested for a beat — flashing
// over the spinning board until the wheel disappears at settlement.
func (rm *room) winnerHighlightOn() bool {
	switch rm.phase {
	case phResults:
		return true
	case phSpinning:
		since := rm.lastNow.Sub(rm.spinStart) - spinAnimDur // since the ball landed
		if since < flashDelay {
			return false
		}
		return ((since-flashDelay)/flashPeriod)%2 == 0
	}
	return false
}

// zeroRow returns the screen row of a green pocket's box: 0 up top, 00 below.
func zeroRow(n int) int {
	if n == doubleZero {
		return rowOfRR(2)
	}
	return rowOfRR(0)
}

// drawZeroBox draws the left margin as two tall green cells — 0 on top, 00 on
// the bottom — split by a single dividing line that is itself the 0-00 split
// bet. The grid's own left border (lineCol 0) doubles as the cells' right edge.
func (rm *room) drawZeroBox(f *kit.Frame) {
	right := lineCol(0)
	rule := func(row int) {
		for c := zeroLeft; c <= right; c++ {
			f.SetRune(row, c, '-', stFrame)
		}
		f.SetRune(row, zeroLeft, '+', stFrame)
		f.SetRune(row, right, '+', stFrame)
	}
	rule(gridTop)     // top of the 0 cell
	rule(gridTop + 3) // the dividing line (the 0-00 split)
	rule(gridTop + 6) // bottom of the 00 cell
	for _, r := range []int{gridTop + 1, gridTop + 2, gridTop + 4, gridTop + 5} {
		f.SetRune(r, zeroLeft, '|', stFrame)
		for c := zeroLeft + 1; c < right; c++ {
			f.SetRune(r, c, ' ', stGreenFelt)
		}
	}
	f.Text(zeroRow(0), zeroTextCol(0), "0", stGreenFelt)
	f.Text(zeroRow(doubleZero), zeroTextCol(doubleZero), "00", stGreenFelt)
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
			f.SetRune(row, lineCol(k), '+', stFrame)
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

// drawNumber paints number n centred in its felt cell (the left slot doubles as
// the chip slot, overdrawn by drawAllChips).
func (rm *room) drawNumber(f *kit.Frame, n int) {
	rr, c := gridRC(n)
	row := rowOfRR(rr)
	col := colInterior(c)
	st := feltStyle(n)
	for i := 0; i < iw; i++ {
		f.SetRune(row, col+i, ' ', st)
	}
	f.Text(row, col+numOff, pad2(n), st)
}

// numOff centres the two-digit number in the iw-wide interior (the cell's left
// slot, col+0, stays the chip slot).
const numOff = (iw - 2 + 1) / 2

// drawOutsideBoxes draws the dozen, even-money, and column "2:1" boxes. Each
// dozen spans four number columns, each even-money box two; the rightmost of a
// row is one wider to meet the grid's right border.
func (rm *room) drawOutsideBoxes(f *kit.Frame) {
	rm.drawBox(f, dozenRowY, lineCol(0), dozenW, "1st 12", stOutside)
	rm.drawBox(f, dozenRowY, lineCol(4), dozenW, "2nd 12", stOutside)
	rm.drawBox(f, dozenRowY, lineCol(8), dozenW+1, "3rd 12", stOutside)
	rm.drawBox(f, evenRowY, lineCol(0), evenW, "1-18", stOutside)
	rm.drawBox(f, evenRowY, lineCol(2), evenW, "EVEN", stOutside)
	rm.drawBox(f, evenRowY, lineCol(4), evenW, "RED", redBox())
	rm.drawBox(f, evenRowY, lineCol(6), evenW, "BLACK", blackBox())
	rm.drawBox(f, evenRowY, lineCol(8), evenW, "ODD", stOutside)
	rm.drawBox(f, evenRowY, lineCol(10), evenW+1, "19-36", stOutside)
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
		rm.shadeOutside(f, mi, stCursor)
		return
	}
	if involvesZero(b) {
		rm.drawZeroCursor(f, b, spots[mi])
		return
	}

	sp := spots[mi]
	row := gridTop + sp.fr
	switch {
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
	if n == 0 || n == doubleZero {
		// The zero boxes follow the same rule as the grid cells: highlight the
		// whole interior row, unless a chip sits in the box (a straight bet on
		// this pocket is master bet n) — then shade just the digits. (Master i ==
		// straight on pocket i, including 00 at index doubleZero.)
		st := merge(stGreenFelt, overlay)
		row := zeroRow(n)
		if rm.chipBits[n] == 0 {
			for c := zeroLeft + 1; c < gridLeft; c++ {
				f.SetRune(row, c, ' ', st)
			}
		}
		f.Text(row, zeroTextCol(n), pocketLabel(n), st)
		return
	}
	rr, c := gridRC(n)
	row := rowOfRR(rr)
	st := merge(feltStyle(n), overlay)
	col := colInterior(c)
	// Highlight the whole cell for a bold, obvious mark — but when a chip sits in
	// the cell's left slot (a straight bet on this number) leave the slot alone,
	// shading just the rest so the marker stays visible.
	start := 0
	if rm.chipBits[n] != 0 {
		start = 1
	}
	for i := start; i < iw; i++ {
		f.SetRune(row, col+i, ' ', st)
	}
	f.Text(row, col+numOff, pad2(n), st)
}

// shadeOutside re-styles outside box mi. Like shadeNumber it leaves the box's
// left slot alone when a chip sits there, so the marker stays visible.
func (rm *room) shadeOutside(f *kit.Frame, mi int, overlay kit.Style) {
	b := masterBets[mi]
	row, col, w := outsideRect(b)
	base := stOutside
	switch b.kind {
	case kRed:
		base = redBox()
	case kBlack:
		base = blackBox()
	}
	st := merge(base, overlay)
	start := 0
	if rm.chipBits[mi] != 0 {
		start = 1
	}
	for i := start; i < w; i++ {
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
	case involvesZero(b):
		return zeroChipPos(b, sp)
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

// zeroChipPos places a chip for a zero-area bet: beside the "0"/"00" for a
// straight, between the boxes for the 0-00 split, and on the grid's left
// boundary for the trios and the top line.
func zeroChipPos(b bet, sp spot) (row, col int) {
	switch b.kind {
	case kStraight:
		n := b.nums[0]
		return zeroRow(n), zeroChipCol(n) // beside the digit
	case kSplit: // 0-00, in the middle strip
		return gridTop + sp.fr, gridLeft - 2
	default: // trios + top line, on the grid's left boundary
		return gridTop + sp.fr, lineCol(0)
	}
}

// drawZeroCursor marks the cursor over a zero-area bet: the box itself for a
// straight (chip-aware, via shadeNumber), otherwise a marker at the bet's chip
// position.
func (rm *room) drawZeroCursor(f *kit.Frame, b bet, sp spot) {
	if b.kind == kStraight {
		rm.shadeNumber(f, b.nums[0], stCursor)
		return
	}
	row, col := zeroChipPos(b, sp)
	f.SetRune(row, col, '#', stCursor)
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
	win := kit.Style{Attr: kit.AttrReverse | kit.AttrBold}
	rm.shadeNumber(f, rm.result, win)
	// Also light up every outside bet the winner pays — its dozen, column, half,
	// colour, and parity — but not the inside split/street/corner/line bets or
	// the grid lines. (A green 0/00 pays no outside bet, so only the box lights.)
	for i, b := range masterBets {
		if b.outside && b.covers(rm.result) {
			rm.shadeOutside(f, i, win)
		}
	}
}

// --- the spinning panel (below the board) -----------------------------------

// drawSpinnerPanel runs the wheel in the lower panel, where the betting sidebar
// sits the rest of the time, so the board (with everyone's locked-in chips)
// stays visible through the spin.
func (rm *room) drawSpinnerPanel(f *kit.Frame, now time.Time, pl *player) {
	idx := rm.wheelDisplayIndex(now)

	// The pocket track, scrolling then resting, with a pointer over the centre.
	const window, slotW = 9, 6
	center := window / 2
	trackW := window * slotW
	left := (kit.Cols - trackW) / 2
	top := panelRow // pointer rides at top-1 (the gap below the seats); net at top+5
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
		s := pocketLabel(n)
		f.Text(top+1, x+(slotW-1-len(s))/2, s, st)
	}

	// Once the wheel has come to rest, hold on the result for a beat and tell the
	// viewer how they did this round.
	if now.Sub(rm.spinStart) >= spinAnimDur {
		msg := "rests on " + numLabel(rm.result)
		f.Text(top+4, (kit.Cols-len(msg))/2, msg, colorStyle(rm.result))
		if pl != nil {
			won, staked := rm.roundNet(pl)
			if s := roundSummary(won, staked); s != "" {
				f.Text(top+5, (kit.Cols-len(s))/2, s, netStyle(won-staked))
			}
		}
	} else {
		msg := "the ball is rolling..."
		f.Text(top+4, (kit.Cols-len(msg))/2, msg, stDim)
	}
}

// roundNet sums a player's gross returns and total stake against the spun
// result (computed from the chips on the felt, which persist through the spin
// and results screens, so it reads the same before and after settlement).
func (rm *room) roundNet(pl *player) (won, staked int) {
	for _, b := range pl.bets {
		won += settleReturn(masterBets[b.master], b.stake, rm.result)
		staked += b.stake
	}
	return won, staked
}

// roundSummary phrases a round's outcome. It leads with the bottom line — how
// many chips up or down you are this round — then the breakdown (what you bet
// and what came back), so a net loss is obvious even when some bets paid out.
// Empty when the player sat the round out.
func roundSummary(won, staked int) string {
	if staked == 0 {
		return ""
	}
	breakdown := "  (bet " + strconv.Itoa(staked) + ", back " + strconv.Itoa(won) + ")"
	net := won - staked
	switch {
	case net > 0:
		return "up " + strconv.Itoa(net) + breakdown
	case net < 0:
		return "down " + strconv.Itoa(-net) + breakdown
	default:
		return "even" + breakdown
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

// --- the seats (players, under the table) -----------------------------------

const (
	seatW     = 12        // width of one seat's swatch + name + balance
	seatLeft  = zeroLeft  // the strip spans the board…
	seatRight = colBoxCol + colBoxW - 1
)

// drawSeats lays the players out beneath the board, like chairs evenly spaced
// around the table: each is a colour swatch + name + balance with the round
// status underneath. Fewer players spread across the full width rather than
// bunching at the left. The strip doubles as the chip-colour legend.
func (rm *room) drawSeats(f *kit.Frame, v kit.Player) {
	n := len(rm.order)
	if n == 0 {
		return
	}
	if n > len(chipColors) {
		n = len(chipColors)
	}
	slot := (seatRight - seatLeft + 1) / n
	for i, id := range rm.order {
		if i >= n {
			break
		}
		pl := rm.players[id]
		if pl == nil {
			continue
		}
		x := seatLeft + i*slot + (slot-seatW)/2 // centre the seat in its slot
		if x < seatLeft {
			x = seatLeft
		}
		f.SetRune(seatsRow, x, '*', chipStyle(pl.colorIdx))
		name := pl.p.Handle
		if len(name) > 5 {
			name = name[:5]
		}
		nameSt := kit.Style{FG: chipColors[pl.colorIdx]}
		if id == v.AccountID {
			nameSt.Attr |= kit.AttrBold
		}
		f.Text(seatsRow, x+1, name, nameSt)
		f.TextRight(seatsRow, x+seatW-2, strconv.Itoa(pl.balance), stHead) // leave a gap to the next seat
		rm.drawSeatStatus(f, x+1, pl)
	}
}

// drawSeatStatus writes a seat's one-word status under its name.
func (rm *room) drawSeatStatus(f *kit.Frame, x int, pl *player) {
	row := seatsRow + 1
	switch rm.phase {
	case phResults:
		if pl.lastPlayed {
			f.Text(row, x, signed(pl.lastNet), netStyle(pl.lastNet))
		}
	case phSpinning:
		if s := pl.staked(); s > 0 {
			f.Text(row, x, "@"+strconv.Itoa(s), stChip)
		}
	default:
		if pl.ready {
			f.Text(row, x, "ready", stReady)
		} else if s := pl.staked(); s > 0 {
			f.Text(row, x, "@"+strconv.Itoa(s), stChip)
		}
	}
}

// --- sidebar: armed bet + your chips ----------------------------------------

func (rm *room) drawSidebar(f *kit.Frame, pl *player) {
	if pl == nil {
		return
	}
	if rm.phase == phBetting {
		b := masterBets[pl.sel.betIndex()]
		// Straight/split labels are bare numbers, so name the family; every other
		// label already reads as its bet ("Str 1-3", "Cnr 1-5", "Top line", "RED").
		desc := b.label
		switch b.kind {
		case kStraight:
			desc = "STRAIGHT " + b.label
		case kSplit:
			desc = "SPLIT " + b.label
		}
		f.Text(panelRow, 2, "> "+desc+"   pays "+strconv.Itoa(b.kind.payout())+":1", stArmed)
		f.TextRight(panelRow, kit.Cols-2, "chip "+strconv.Itoa(stakeTiers[pl.stakeIdx]), stHead)
	} else if rm.phase == phResults {
		won, staked := rm.roundNet(pl)
		if s := roundSummary(won, staked); s != "" {
			f.Text(panelRow, 2, s, netStyle(won-staked))
		}
	}

	// The chip detail is secondary (the markers are on the board) — a single
	// line, with a label and the bets, trimmed to fit.
	rm.drawYourChips(f, pl)
}

// drawYourChips lists the viewer's bets under the panel, one per line down the
// rows it has and flowing into extra columns when a player spreads a lot of
// chips, summarising the tail with "+N more" only when even that runs out.
func (rm *room) drawYourChips(f *kit.Frame, pl *player) {
	groups := rm.groupBets(pl.bets)
	label := "your chips (" + strconv.Itoa(pl.staked()) + " down):"
	if rm.phase == phResults {
		label = "your chips this round:"
	}
	f.Text(panelRow+1, 2, label, stHead)
	if len(groups) == 0 {
		f.Text(chipsTop, 4, "none yet", stDim)
		return
	}
	maxCells := chipsRows * chipsCols
	yourSt := chipStyle(pl.colorIdx)
	for i, g := range groups {
		c, r := i/chipsRows, i%chipsRows // column-major: fill a column top-down first
		x := 4 + c*chipsColW
		if len(groups) > maxCells && i == maxCells-1 {
			f.Text(chipsTop+r, x, "+"+strconv.Itoa(len(groups)-i)+" more", stDim)
			return
		}
		f.Text(chipsTop+r, x, masterBets[g.master].label+" x"+strconv.Itoa(g.stake), yourSt)
	}
}

const (
	chipsTop  = panelRow + 2        // first chip row (the label sits on panelRow+1)
	chipsRows = helpRow - chipsTop  // rows available down to the help line
	chipsColW = 18                  // width of one chip column
	chipsCols = (kit.Cols - 4) / chipsColW
)

func (rm *room) drawHelp(f *kit.Frame, pl *player) {
	var help string
	switch rm.phase {
	case phBetting:
		help = "arrows move  enter place  +/- chip  bksp undo  c clear  r ready"
	case phSpinning:
		help = "no more bets - the ball is rolling"
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
			return dozenRowY, lineCol(0), dozenW
		case 13:
			return dozenRowY, lineCol(4), dozenW
		default:
			return dozenRowY, lineCol(8), dozenW + 1
		}
	case kLow:
		return evenRowY, lineCol(0), evenW
	case kEven:
		return evenRowY, lineCol(2), evenW
	case kRed:
		return evenRowY, lineCol(4), evenW
	case kBlack:
		return evenRowY, lineCol(6), evenW
	case kOdd:
		return evenRowY, lineCol(8), evenW
	case kHigh:
		return evenRowY, lineCol(10), evenW + 1
	}
	return evenRowY, lineCol(0), evenW
}

func outsideLabel(b bet) string {
	if b.kind == kColumn {
		return "2:1"
	}
	return b.label
}

func feltStyle(n int) kit.Style {
	switch colorOf(n) {
	case green:
		return stGreenFelt
	case red:
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
	return pocketLabel(n) + " " + name
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return " " + s
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
