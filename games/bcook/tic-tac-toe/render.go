package main

import kit "github.com/shellcade/kit"

// Rendering: a centered 3x3 board, a title, both players' names with their
// marks, and a status line. The view is identical for everyone (nothing is
// personalized), so one composed frame is broadcast with Identical.

var (
	stTitle  = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim    = kit.Style{FG: kit.DimGray}
	stGrid   = kit.Style{FG: kit.DimGray}
	stX      = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stO      = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stEmpty  = kit.Style{FG: kit.DimGray}
	stTurn   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stWin    = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stDraw   = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stWaitFG = kit.Style{FG: kit.DimGray}
)

// Board geometry: each cell is 5 wide x 1 tall of content, separated by grid
// lines, centered on the 80x24 canvas.
const (
	cellW    = 5
	boardW   = cellW*3 + 2 // two vertical separators
	boardCol = (kit.Cols - boardW) / 2
	boardRow = 8
)

func (rm *room) render(r kit.Room) {
	f := rm.frame
	f.Clear()

	// Title.
	title := "TIC - TAC - TOE"
	f.Text(1, (kit.Cols-len(title))/2, title, stTitle)

	rm.drawPlayers(f)
	rm.drawBoard(f)
	rm.drawStatus(f)

	hint := "Press 1-9 to place your mark"
	f.Text(kit.Rows-2, (kit.Cols-len(hint))/2, hint, stDim)

	r.Identical(f)
}

// drawPlayers shows X on the left, O on the right, with names and a marker on
// whoever is to move.
func (rm *room) drawPlayers(f *kit.Frame) {
	xName, oName := "(waiting)", "(waiting)"
	if rm.xID != "" {
		xName = rm.players[rm.xID].DisplayName()
	}
	if rm.oID != "" {
		oName = rm.players[rm.oID].DisplayName()
	}

	left := "X  " + xName
	right := oName + "  O"
	f.Text(4, 4, left, styleFor(markX))
	f.TextRight(4, kit.Cols-5, right, styleFor(markO))

	if !rm.over && rm.bothSeated() {
		if rm.turn == markX {
			f.SetRune(4, 2, '>', stTurn)
		} else {
			f.SetRune(4, kit.Cols-3, '<', stTurn)
		}
	}
}

func styleFor(mark byte) kit.Style {
	if mark == markX {
		return stX
	}
	return stO
}

// drawBoard renders the 3x3 grid. Empty cells show their 1-9 address dimly so
// players know what to press; filled cells show the mark.
func (rm *room) drawBoard(f *kit.Frame) {
	for cell := 0; cell < 9; cell++ {
		rowq, colq := cell/3, cell%3
		// Cell content's top-left within the board box.
		cr := boardRow + rowq*2
		cc := boardCol + colq*(cellW+1)
		ch := rm.board[cell]
		mid := cc + cellW/2
		switch ch {
		case markX:
			f.SetRune(cr, mid, 'X', stX)
		case markO:
			f.SetRune(cr, mid, 'O', stO)
		default:
			f.SetRune(cr, mid, rune('1'+cell), stEmpty)
		}
	}
	// Vertical separators between columns (after col 0 and col 1).
	for q := 1; q <= 2; q++ {
		sep := boardCol + q*(cellW+1) - 1
		for rowq := 0; rowq < 3; rowq++ {
			f.SetRune(boardRow+rowq*2, sep, '|', stGrid)
		}
	}
	// Horizontal separators between rows.
	for q := 1; q <= 2; q++ {
		sr := boardRow + q*2 - 1
		for c := 0; c < boardW; c++ {
			f.SetRune(sr, boardCol+c, '-', stGrid)
		}
		// Crosses where the lines meet.
		for j := 1; j <= 2; j++ {
			f.SetRune(sr, boardCol+j*(cellW+1)-1, '+', stGrid)
		}
	}
}

func (rm *room) drawStatus(f *kit.Frame) {
	row := boardRow + 3*2 + 1
	var msg string
	var st kit.Style
	switch {
	case !rm.bothSeated():
		msg = "Waiting for both players..."
		st = stWaitFG
	case rm.over && rm.winnerID == "":
		msg = "Draw!"
		st = stDraw
	case rm.over:
		msg = rm.players[rm.winnerID].DisplayName() + " wins!"
		st = stWin
	case rm.turn == markX:
		msg = "X to move"
		st = stTurn
	default:
		msg = "O to move"
		st = stTurn
	}
	f.Text(row, (kit.Cols-len([]rune(msg)))/2, msg, st)
}
