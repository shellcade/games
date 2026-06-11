package main

import (
	"math"

	kit "github.com/shellcade/kit/v2"
)

// All field pieces (bricks, paddle, bit, danger floor) are background-coloured
// spaces — always one cell wide, so nothing can desync the layout — over a
// box-drawn rail. Numbers are drawn digit-by-digit so a render never allocates.

var (
	stTitle   = kit.Style{FG: kit.RGB(0xff, 0x5b, 0xd0), Attr: kit.AttrBold}
	stScore   = kit.Style{FG: kit.RGB(0xff, 0xe0, 0x3a), Attr: kit.AttrBold}
	stLevel   = kit.Style{FG: kit.RGB(0x3a, 0xd6, 0xff), Attr: kit.AttrBold}
	stDim     = kit.Style{FG: kit.Gray(0x88)}
	stLife    = kit.Style{FG: kit.RGB(0xff, 0x49, 0x6b), Attr: kit.AttrBold}
	stWall    = kit.Style{FG: kit.RGB(0x46, 0x6c, 0xc8), Attr: kit.AttrBold}
	stFloor   = kit.Style{BG: kit.RGB(0x4a, 0x12, 0x16)} // dim red kill strip
	stBall    = kit.Style{BG: kit.RGB(0xff, 0xff, 0xff)} // bright bit
	stPaddle  = kit.Style{BG: kit.RGB(0x3a, 0xd6, 0xff)}
	stPaddleW = kit.Style{BG: kit.RGB(0x49, 0xe2, 0x6a)}
	stMsg     = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stFlash   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stToast   = kit.Style{FG: kit.RGB(0x6f, 0xff, 0xe0), Attr: kit.AttrBold}
	stBadgeW  = kit.Style{FG: kit.Gray(0x10), BG: kit.RGB(0x49, 0xe2, 0x6a), Attr: kit.AttrBold}
	stBadgeS  = kit.Style{FG: kit.Gray(0x10), BG: kit.RGB(0x3a, 0xd6, 0xff), Attr: kit.AttrBold}
)

// puGlyph / puStyle render a falling powerup as a small lettered tile.
var puGlyph = [puKindCount]rune{puWide: 'W', puMulti: 'M', puSlow: 'S', puLife: '+'}
var puStyle = [puKindCount]kit.Style{
	puWide:  {FG: kit.Gray(0x10), BG: kit.RGB(0x49, 0xe2, 0x6a), Attr: kit.AttrBold},
	puMulti: {FG: kit.Gray(0x10), BG: kit.RGB(0xff, 0xe0, 0x3a), Attr: kit.AttrBold},
	puSlow:  {FG: kit.Gray(0x10), BG: kit.RGB(0x3a, 0xd6, 0xff), Attr: kit.AttrBold},
	puLife:  {FG: kit.White, BG: kit.RGB(0xff, 0x49, 0x6b), Attr: kit.AttrBold},
}

func (rm *room) render(r kit.Room) {
	for _, v := range r.Members() {
		rm.frame.Clear()
		rm.compose(rm.frame, v)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(f *kit.Frame, v kit.Player) {
	b := rm.boards[v.AccountID]
	if b == nil {
		return
	}
	drawArena(f, b)
	drawBricks(f, b)
	drawParticles(f, b)
	drawPowerups(f, b)
	drawPaddle(f, b, v.Character)
	drawBalls(f, b)
	rm.drawHUD(f, b)
	rm.drawStatus(f, b, v)
	drawOverlay(f, b)
}

// --- arena -------------------------------------------------------------------

func drawArena(f *kit.Frame, b *board) {
	wall := stWall
	if b.clock.Before(b.flashUntil) {
		wall = stFlash // a quick celebratory flash on a fresh wall
	}
	f.SetRune(wallTop, 0, '╔', wall)
	f.SetRune(wallTop, kit.Cols-1, '╗', wall)
	for c := 1; c < kit.Cols-1; c++ {
		f.SetRune(wallTop, c, '═', wall)
	}
	for row := fieldTop; row <= floorRow; row++ {
		f.SetRune(row, 0, '║', wall)
		f.SetRune(row, kit.Cols-1, '║', wall)
	}
	for c := colMin; c <= colMax; c++ {
		f.SetRune(floorRow, c, ' ', stFloor)
	}
}

// --- wall of bytes -----------------------------------------------------------

func drawBricks(f *kit.Frame, b *board) {
	for r := range b.bricks {
		row := brickTop + r
		for c := range b.bricks[r] {
			br := b.bricks[r][c]
			if !br.alive {
				continue
			}
			start := colMin + c*brickW
			st := kit.Style{BG: br.color}
			for k := 0; k < brickW-1; k++ { // last column left as a dark mortar gap
				f.SetRune(row, start+k, ' ', st)
			}
			if br.hits >= 2 { // an armoured byte wears a stud
				f.SetRune(row, start+brickW/2-1, '=', kit.Style{FG: kit.Gray(0x10), BG: br.color, Attr: kit.AttrBold})
			}
		}
	}
}

func drawParticles(f *kit.Frame, b *board) {
	for i := range b.parts {
		p := &b.parts[i]
		row, col := int(math.Round(p.y)), int(math.Round(p.x))
		if row < fieldTop || row > floorRow || col < colMin || col > colMax {
			continue
		}
		f.SetRune(row, col, p.glyph, kit.Style{FG: p.color})
	}
}

func drawPowerups(f *kit.Frame, b *board) {
	for _, p := range b.powerups {
		row, col := int(math.Round(p.y)), int(math.Round(p.x))
		if row < fieldTop || row > floorRow {
			continue
		}
		st := puStyle[p.kind]
		for dc := -1; dc <= 1; dc++ {
			if cc := col + dc; cc >= colMin && cc <= colMax {
				f.SetRune(row, cc, ' ', st)
			}
		}
		if col >= colMin && col <= colMax {
			f.SetRune(row, col, puGlyph[p.kind], st)
		}
	}
}

func drawPaddle(f *kit.Frame, b *board, ch kit.Character) {
	st := stPaddle
	if b.wide {
		st = stPaddleW
	}
	// The bar wears the character's BACKGROUND colour; the wide power-up
	// still reads from the longer run. Players without a character keep the
	// stock cyan (green when wide).
	if ch.Glyph != "" {
		st = kit.Style{BG: kit.RGB(ch.BgR, ch.BgG, ch.BgB)}
	}
	half := b.paddleHalf()
	center := int(math.Round(b.paddleX))
	for c := center - half; c <= center+half; c++ {
		if c >= colMin && c <= colMax {
			f.SetRune(paddleRow, c, ' ', st)
		}
	}
	// The player's character rides the centre cell of the board. The run is
	// 2*half+1 cells so the centre is exact (were the paddle ever even-width,
	// this rounded centre would land left-of-centre). Skip the tile on a
	// sliver of a paddle (<3 cells) so it still reads as a paddle, and for
	// players without a character.
	if ch.Glyph != "" && half >= 1 && center >= colMin && center <= colMax {
		f.Set(paddleRow, center, kit.CharacterCell(ch))
	}
}

func drawBalls(f *kit.Frame, b *board) {
	for _, bl := range b.balls {
		row, col := int(math.Round(bl.y)), int(math.Round(bl.x))
		if row >= fieldTop && row <= floorRow && col >= colMin && col <= colMax {
			f.SetRune(row, col, ' ', stBall)
		}
	}
}

// --- HUD + status ------------------------------------------------------------

func (rm *room) drawHUD(f *kit.Frame, b *board) {
	f.Text(hudRow, 2, "BYTEBREAKER", stTitle)
	col := f.Text(hudRow, 16, "SCORE ", stDim)
	col = drawInt(f, hudRow, col, b.score, stScore)
	col = f.Text(hudRow, col+2, "LVL ", stDim)
	col = drawInt(f, hudRow, col, b.level, stLevel)
	col = f.Text(hudRow, col+2, "HI ", stDim)
	drawInt(f, hudRow, col, b.best, stDim)

	// Lives as little bits on the right.
	start := kit.Cols - 2 - (6 + b.lives)
	c := f.Text(hudRow, start, "BALLS ", stDim)
	for i := 0; i < b.lives; i++ {
		f.SetRune(hudRow, c+i, 'O', stLife)
	}
}

func (rm *room) drawStatus(f *kit.Frame, b *board, v kit.Player) {
	// Left: controls / context.
	switch b.phase {
	case phReady:
		f.Text(statusRow, 2, "left/right  move        SPACE  launch", stDim)
	case phOver:
		f.Text(statusRow, 2, "SPACE  play again", stDim)
	default:
		f.Text(statusRow, 2, "left/right  move the paddle", stDim)
	}
	// Centre: active-powerup badges.
	bx := 40
	if b.wide {
		bx = f.Text(statusRow, bx, " WIDE ", stBadgeW) + 1
	}
	if b.slow {
		f.Text(statusRow, bx, " SLOW ", stBadgeS)
	}
	// Right: rivals' live scores (only with company).
	rm.drawRivals(f, v)
}

func (rm *room) drawRivals(f *kit.Frame, v kit.Player) {
	if len(rm.order) < 2 {
		return
	}
	// Build right-to-left so the strip hugs the right edge without allocating.
	col := kit.Cols - 2
	for i := len(rm.order) - 1; i >= 0; i-- {
		id := rm.order[i]
		if id == v.AccountID {
			continue
		}
		ob := rm.boards[id]
		p, ok := rm.names[id]
		if ob == nil || !ok {
			continue
		}
		name := p.Handle
		if len(name) > 6 {
			name = name[:6]
		}
		w := 2 + len(name) + 1 + intWidth(ob.score) // character tile + space + name + score
		col -= w
		if col < 44 {
			break
		}
		f.Set(statusRow, col, kit.CharacterCell(p.Character))
		cc := f.Text(statusRow, col+2, name, stDim)
		drawInt(f, statusRow, cc+1, ob.score, stScore)
		col -= 2
	}
}

// --- centred overlays --------------------------------------------------------

func drawOverlay(f *kit.Frame, b *board) {
	// A transient pickup / bit-lost banner, while a bit is in play.
	if (b.phase == phPlaying || b.phase == phReady) && b.toast != "" && b.clock.Before(b.toastUntil) {
		center(f, 15, b.toast, stToast)
	}
	switch b.phase {
	case phReady:
		x := (kit.Cols - (6 + intWidth(b.level))) / 2
		cx := f.Text(10, x, "LEVEL ", stMsg)
		drawInt(f, 10, cx, b.level, stLevel)
		center(f, 12, "press SPACE to launch the bit", stDim)
	case phClear:
		center(f, 11, "* WALL CLEARED *", stScore)
	case phOver:
		center(f, 10, "GAME OVER", stLife)
		center(f, 12, "press SPACE to play again", stDim)
	}
}

// --- little drawing helpers (alloc-free) -------------------------------------

// drawInt prints n at (row,col) digit-by-digit and returns the next column.
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
	return f.Text(row, (kit.Cols-len(s))/2, s, st)
}
