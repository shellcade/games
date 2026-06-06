package main

import (
	"strconv"
	"time"

	kit "github.com/shellcade/kit/v2"

	"alan/chess/engine"
)

// Board geometry. Each square is sqW x sqH cells; the 8x8 board sits with a rank
// label column to its left and a file label row below it.
const (
	sqW      = 4
	sqH      = 2
	boardCol = 3 // left edge of the board; rank labels at col 1 with a blank col 2 gutter
	boardRow = 2 // top edge of the board
	boardH   = 8 * sqH
	fileRow  = boardRow + boardH // row holding the a..h file labels
	panelCol = 38                // left edge of the side panel
)

// black is the truecolor black the kit palette omits (kit exports White but not
// Black); used for cursor brackets and the active promotion chip.
var black = kit.RGB(0x00, 0x00, 0x00)

// Colours. Highlights are designed to survive a colourless terminal: each also
// carries an ASCII marker or a reverse attribute, so colour is never the only
// signal.
var (
	stTitle  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stTag    = kit.Style{FG: kit.DimGray}
	stFooter = kit.Style{FG: kit.DimGray}
	stLabel  = kit.Style{FG: kit.DimGray}

	bgLight = kit.RGB(0xb5, 0x88, 0x63) // light square
	bgDark  = kit.RGB(0x6f, 0x4e, 0x37) // dark square

	fgWhitePiece = kit.RGB(0xf5, 0xf5, 0xf0)
	fgBlackPiece = kit.RGB(0x20, 0x20, 0x20)

	bgCursor = kit.RGB(0x55, 0xff, 0xff) // cursor square
	bgSelect = kit.RGB(0xff, 0xdd, 0x55) // selected origin
	bgTarget = kit.RGB(0x66, 0xaa, 0x66) // legal capture target tint
	bgLast   = kit.RGB(0x88, 0x99, 0x55) // last-move from/to tint
	bgCheck  = kit.RGB(0xcc, 0x44, 0x44) // king in check

	stPanelName  = kit.Style{FG: kit.White}
	stPanelOwn   = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stPanelTurn  = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stStatus     = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stStatusWarn = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stStatusDraw = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stMoveList   = kit.Style{FG: kit.Gray(0xbb)}
	stMoveHdr    = kit.Style{FG: kit.DimGray}

	stPromoActive = kit.Style{FG: black, BG: kit.Yellow, Attr: kit.AttrBold}
	stPromoIdle   = kit.Style{FG: kit.DimGray}
	stPromoLabel  = kit.Style{FG: kit.White}

	stClock    = kit.Style{FG: kit.White}
	stClockLow = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
)

// render composes an independent, viewer-oriented frame per member and sends it.
// Per-viewer composition replaces the native BroadcastFunc; one reused scratch
// frame keeps the steady state allocation-light (Send copies immediately).
func (rm *room) render(r kit.Room) {
	for _, v := range r.Members() {
		rm.frame.Clear()
		rm.compose(v, rm.frame)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(v kit.Player, f *kit.Frame) {
	// Row 0: title + tagline + mode.
	f.Text(0, 1, "CHESS", stTitle)
	f.Text(0, 7, "a two-player duel", stTag)
	f.TextRight(0, kit.Cols-1, modeLabel(rm.cfg.Mode), stTag)

	if rm.phase == phWaiting {
		rm.composeWaiting(f, v)
		f.Text(23, 1, "Waiting for an opponent...  -  Esc leave", stFooter)
		return
	}

	orient := rm.orientationColor(v)
	rm.drawBoard(f, v, orient)
	rm.drawPanel(f, v)
	rm.drawFooter(f, v)
}

func modeLabel(m kit.Mode) string {
	switch m {
	case kit.ModeQuick:
		return "quick"
	case kit.ModePrivate:
		return "private"
	case kit.ModeSolo:
		return "solo"
	}
	return ""
}

func (rm *room) composeWaiting(f *kit.Frame, v kit.Player) {
	msg := "Waiting for an opponent..."
	f.Text(10, (kit.Cols-len(msg))/2, msg, stStatus)
	sub := "you'll be seated as White or Black at random"
	f.Text(12, (kit.Cols-len(sub))/2, sub, stTag)
}

// --- board -----------------------------------------------------------------

// screenSquare maps a board square to its on-screen (row, col) top-left cell,
// given the viewer's orientation. White viewers see rank 8 at top, rank 1 at
// bottom; Black viewers see the board flipped.
func screenSquare(sq engine.Square, orient engine.Color) (row, col int) {
	file, rank := sq.File(), sq.Rank()
	var dispFile, dispRank int
	if orient == engine.Black {
		dispFile = 7 - file
		dispRank = rank // rank 8 (7) ends up at the bottom for Black
	} else {
		dispFile = file
		dispRank = 7 - rank // rank 8 (7) at the top, rank 1 (0) at the bottom
	}
	row = boardRow + dispRank*sqH
	col = boardCol + dispFile*sqW
	return row, col
}

func (rm *room) drawBoard(f *kit.Frame, v kit.Player, orient engine.Color) {
	sel := rm.sel[v.AccountID]
	side := rm.pos.Side
	kingInCheck := engine.NoSquare
	if engine.InCheck(rm.pos, side) {
		kingInCheck = kingSquareOf(rm.pos, side)
	}

	// Precompute legal-target squares for the viewer's current selection.
	type tgt struct{ cap bool }
	targets := map[engine.Square]tgt{}
	if sel != nil && sel.from != engine.NoSquare {
		for _, m := range sel.targets {
			isCap := rm.pos.Board[m.To].Type != engine.Empty ||
				(rm.pos.Board[m.From].Type == engine.Pawn && m.From.File() != m.To.File())
			targets[m.To] = tgt{cap: isCap}
		}
	}

	for sq := engine.Square(0); sq < 64; sq++ {
		row, col := screenSquare(sq, orient)
		light := (sq.File()+sq.Rank())%2 == 1
		bg := bgDark
		if light {
			bg = bgLight
		}

		// Highlight precedence: check > cursor > selected > last-move > target.
		_, isTarget := targets[sq]
		isCap := isTarget && targets[sq].cap
		switch {
		case sq == kingInCheck:
			bg = bgCheck
		case sel != nil && sq == sel.cursor:
			bg = bgCursor
		case sel != nil && sq == sel.from:
			bg = bgSelect
		case rm.lastMv != nil && (sq == rm.lastMv.From || sq == rm.lastMv.To):
			bg = bgLast
		case isCap:
			bg = bgTarget
		}

		// Fill the square's cells with the background.
		for rr := 0; rr < sqH; rr++ {
			for cc := 0; cc < sqW; cc++ {
				f.Set(row+rr, col+cc, kit.Cell{Rune: ' ', BG: bg})
			}
		}

		pc := rm.pos.Board[sq]
		pr, pc2 := row+sqH/2, col+sqW/2 // centre cell (piece row, file centre)
		markSt := kit.Style{FG: kit.White, BG: bg, Attr: kit.AttrBold}
		switch {
		case pc.Type != engine.Empty:
			st := kit.Style{FG: fgWhitePiece, BG: bg, Attr: kit.AttrBold}
			if pc.Color == engine.Black {
				st = kit.Style{FG: fgBlackPiece, BG: bg, Attr: kit.AttrBold}
			}
			// Figurine plus its piece-letter label, centred as a pair across the two
			// middle columns, so a piece stays identifiable even where the glyph
			// renders faintly.
			f.SetRune(pr, col+1, pieceGlyph(pc), st)
			f.SetRune(pr, col+2, pieceLetter(pc), st)
			if isTarget {
				// An occupied legal target is a capture: mark it with an 'x' above
				// the piece so the highlight survives a colourless terminal.
				f.SetRune(row, pc2, 'x', markSt)
			}
		case isTarget:
			// An empty legal target (including en passant): a centred dot.
			f.SetRune(pr, pc2, '.', markSt)
		}

		// ASCII markers so highlights survive a colourless terminal.
		if sq == kingInCheck {
			f.SetRune(row, pc2, '+', markSt)
		}
		if sel != nil && sq == sel.cursor {
			cst := kit.Style{FG: black, BG: bg, Attr: kit.AttrBold}
			f.SetRune(row, col, '[', cst)
			f.SetRune(row, col+sqW-1, ']', cst)
		}
	}

	// Rank labels (left of the board) and file labels (below it).
	for i := 0; i < 8; i++ {
		// i is the on-screen row index from the top (0=top).
		var rankVal int
		if orient == engine.Black {
			rankVal = i + 1 // Black: top row is rank 1
		} else {
			rankVal = 8 - i // White: top row is rank 8
		}
		f.SetRune(boardRow+i*sqH+sqH/2, boardCol-2, rune('0'+rankVal), stLabel)

		// File labels.
		var fileCh rune
		if orient == engine.Black {
			fileCh = rune('h' - i)
		} else {
			fileCh = rune('a' + i)
		}
		f.SetRune(fileRow, boardCol+i*sqW+sqW/2, fileCh, stLabel)
	}
}

// pieceGlyph maps a piece to a single-cell Unicode chess figurine: White uses the
// outline set (♔♕♖♗♘♙) and Black the filled set (♚♛♜♝♞♟).
func pieceGlyph(pc engine.Piece) rune {
	white := pc.Color == engine.White
	switch pc.Type {
	case engine.Pawn:
		if white {
			return '♙'
		}
		return '♟'
	case engine.Knight:
		if white {
			return '♘'
		}
		return '♞'
	case engine.Bishop:
		if white {
			return '♗'
		}
		return '♝'
	case engine.Rook:
		if white {
			return '♖'
		}
		return '♜'
	case engine.Queen:
		if white {
			return '♕'
		}
		return '♛'
	case engine.King:
		if white {
			return '♔'
		}
		return '♚'
	}
	return ' '
}

// pieceLetter is the piece's algebraic letter, upper-case for White and
// lower-case for Black.
func pieceLetter(pc engine.Piece) rune {
	var u rune
	switch pc.Type {
	case engine.Pawn:
		u = 'P'
	case engine.Knight:
		u = 'N'
	case engine.Bishop:
		u = 'B'
	case engine.Rook:
		u = 'R'
	case engine.Queen:
		u = 'Q'
	case engine.King:
		u = 'K'
	default:
		return ' '
	}
	if pc.Color == engine.Black {
		return u + ('a' - 'A')
	}
	return u
}

// kingSquareOf returns colour c's king square (NoSquare if absent).
func kingSquareOf(p engine.Position, c engine.Color) engine.Square {
	for s := engine.Square(0); s < 64; s++ {
		pc := p.Board[s]
		if pc.Type == engine.King && pc.Color == c {
			return s
		}
	}
	return engine.NoSquare
}

// --- side panel ------------------------------------------------------------

func (rm *room) drawPanel(f *kit.Frame, v kit.Player) {
	// Player rows with clocks; side-to-move highlighted; the viewer's own name
	// marked. Ordered to mirror the board: the far side sits at the top and the
	// viewer's own side at the bottom.
	orient := rm.orientationColor(v)
	rm.drawPlayerLine(f, 2, v, orient^1) // far side
	rm.drawPlayerLine(f, 4, v, orient)   // the viewer's own side

	// Status line.
	rm.drawStatus(f, 6, v)

	// Move list header + recent rows.
	f.Text(8, panelCol, "Moves", stMoveHdr)
	rm.drawMoveList(f, 9)

	// Promotion picker (if the viewer is mid-promotion).
	if sel := rm.sel[v.AccountID]; sel != nil && sel.promoing {
		rm.drawPromoPicker(f, 21, sel)
	}
}

func (rm *room) drawPlayerLine(f *kit.Frame, row int, v kit.Player, c engine.Color) {
	name := "-"
	var who kit.Player
	for _, p := range rm.seats {
		if rm.color[p.AccountID] == c {
			who, name = p, p.Handle
			break
		}
	}
	if len(name) > 16 {
		name = name[:16]
	}

	st := stPanelName
	if rm.phase == phPlaying && rm.pos.Side == c {
		st = stPanelTurn // side to move
	}
	label := colorName(c)
	prefix := "  "
	if who.AccountID == v.AccountID && who.AccountID != "" {
		prefix = "> " // your own side
		if eqColor(st.FG, stPanelName.FG) {
			st = stPanelOwn
		}
	}
	f.Text(row, panelCol, prefix+label+" "+name, st)

	// Clock on the right of the panel.
	var rem time.Duration
	if rm.phase == phPlaying && rm.pos.Side == c {
		rem = rm.liveRemaining(c, rm.lastNow)
	} else {
		rem = rm.clock[c]
	}
	cst := stClock
	if rem < 30*time.Second {
		cst = stClockLow
	}
	f.TextRight(row, kit.Cols-1, fmtClock(rem), cst)
}

func (rm *room) drawStatus(f *kit.Frame, row int, v kit.Player) {
	text := rm.outcome
	st := stStatus
	switch rm.phase {
	case phOver:
		if isDrawText(text) {
			st = stStatusDraw
		}
	case phPlaying:
		switch {
		case rm.drawOffer != noOffer:
			offerer := offerName(rm)
			text = offerer + " offers a draw (y/n)"
			st = stStatusDraw
		case rm.resignArm[v.AccountID]:
			text = "Resign? press r again"
			st = stStatusWarn
		case engine.InCheck(rm.pos, rm.pos.Side):
			text = "Check! " + colorName(rm.pos.Side) + " to move"
			st = stStatusWarn
		default:
			text = colorName(rm.pos.Side) + " to move"
		}
	}
	if len(text) > kit.Cols-1-panelCol {
		text = text[:kit.Cols-1-panelCol]
	}
	f.Text(row, panelCol, text, st)
}

func isDrawText(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i:i+5] == "draw " || s[i:i+5] == "Draw " {
			return true
		}
	}
	// also catch "- draw" endings
	return len(s) >= 4 && (s[len(s)-4:] == "draw")
}

func offerName(rm *room) string {
	for _, p := range rm.seats {
		if rm.color[p.AccountID] == rm.drawOffer {
			return p.Handle
		}
	}
	return colorName(rm.drawOffer)
}

// drawMoveList shows the most recent moves as "N. white  black" rows.
func (rm *room) drawMoveList(f *kit.Frame, top int) {
	const rows = 11 // rows top..top+rows-1, leaving room for the promo picker
	type line struct {
		num          int
		white, black string
	}
	var lines []line
	for i := 0; i < len(rm.moves); i += 2 {
		ln := line{num: i/2 + 1, white: rm.moves[i]}
		if i+1 < len(rm.moves) {
			ln.black = rm.moves[i+1]
		}
		lines = append(lines, ln)
	}
	// Show the last `rows` lines.
	start := len(lines) - rows
	if start < 0 {
		start = 0
	}
	for r := 0; r < rows && start+r < len(lines); r++ {
		ln := lines[start+r]
		f.Text(top+r, panelCol, moveListLine(ln.num, ln.white, ln.black), stMoveList)
	}
}

// moveListLine formats a numbered move-pair row ("  1. e2e4   e7e5") without
// fmt — TinyGo-friendly and allocation-light.
func moveListLine(num int, white, black string) string {
	return padLeft(strconv.Itoa(num), 3) + ". " + padRight(white, 7) + " " + padRight(black, 7)
}

func padLeft(s string, n int) string {
	for len(s) < n {
		s = " " + s
	}
	return s
}

func padRight(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

func (rm *room) drawPromoPicker(f *kit.Frame, row int, sel *selection) {
	f.Text(row, panelCol, "Promote:", stPromoLabel)
	col := panelCol + 9
	for i, pt := range promoOrder {
		st := stPromoIdle
		if i == sel.promoSel {
			st = stPromoActive
		}
		f.Text(row, col, " "+string(promoLetter(pt))+" ", st)
		col += 4
	}
}

func promoLetter(pt engine.PieceType) rune {
	switch pt {
	case engine.Queen:
		return 'Q'
	case engine.Rook:
		return 'R'
	case engine.Bishop:
		return 'B'
	case engine.Knight:
		return 'N'
	}
	return '?'
}

// --- footer ----------------------------------------------------------------

func (rm *room) drawFooter(f *kit.Frame, v kit.Player) {
	var hint string
	switch rm.phase {
	case phOver:
		hint = "Enter: continue"
	case phPlaying:
		sel := rm.sel[v.AccountID]
		if sel != nil && sel.promoing {
			hint = "<-/-> choose  -  Enter confirm  -  Backspace cancel"
		} else {
			hint = "Arrows move - Enter select/move - Bksp cancel - r resign - d draw - Esc leave"
		}
	}
	f.Text(23, 1, hint, stFooter)
}

// fmtClock formats a duration as m:ss, clamped at 0.
func fmtClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second).Seconds())
	m := total / 60
	s := total % 60
	sec := strconv.Itoa(s)
	if len(sec) < 2 {
		sec = "0" + sec
	}
	return strconv.Itoa(m) + ":" + sec
}

// eqColor compares two colours by set-ness and components (kit.Color has no
// exported equality helper).
func eqColor(a, b kit.Color) bool {
	if a.IsSet() != b.IsSet() {
		return false
	}
	ar, ag, ab := a.RGBVals()
	br, bg, bb := b.RGBVals()
	return ar == br && ag == bg && ab == bb
}
