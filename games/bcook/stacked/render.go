package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// Layout: the big well sits top-left under the scoreboard; the next-piece
// preview and run stats sit to its right; the rival miniature panel runs down
// the far right.
const (
	bigTop  = 2 // top border row of the big well
	bigLeft = 2 // left border col of the big well

	infoLeft = bigLeft + wellW*2 + 4 // info column (preview, stats)
	miniLeft = 40                    // rival miniature panel left edge
)

var (
	borderColor  = kit.RGB(0x55, 0x60, 0x70)
	garbageColor = kit.RGB(0x88, 0x88, 0x88)
	ghostColor   = kit.RGB(0x40, 0x46, 0x52)
	dimText      = kit.DimGray
)

// render composes and sends a tailored frame to every connected player, reusing
// one long-lived frame buffer (Send copies immediately).
func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		rm.frame.Clear()
		rm.composeFor(rm.frame, p)
		r.Send(p, rm.frame)
	}
}

func (rm *room) composeFor(f *kit.Frame, viewer kit.Player) {
	rm.drawScoreboard(f, viewer)

	w := rm.wells[viewer.AccountID]
	if w != nil {
		rm.drawBigWell(f, viewer, w)
		rm.drawInfo(f, viewer, w)
	}
	rm.drawRivalPanel(f, viewer)
	rm.drawControls(f)
	rm.drawBanners(f, viewer, w)
}

// --- scoreboard --------------------------------------------------------------

func (rm *room) drawScoreboard(f *kit.Frame, viewer kit.Player) {
	col := 1
	for _, id := range rm.rankedOrder() {
		w := rm.wells[id]
		p := rm.names[id]
		if w == nil {
			continue
		}
		name := p.Handle
		if len([]rune(name)) > 7 {
			name = string([]rune(name)[:7])
		}
		marker := "●"
		mc := w.color
		if !w.alive {
			marker = "✗"
			mc = dimText
		}
		col = f.Text(0, col, marker+" ", kit.Style{FG: mc, Attr: kit.AttrBold})
		f.Set(0, col, kit.CharacterCell(p.Character)) // character tile + a space
		col += 2
		seg := fmt.Sprintf("%s %d", name, w.score)
		if id == viewer.AccountID {
			seg += "*"
		}
		col = f.Text(0, col, seg+"  ", kit.Style{FG: w.color})
		if col > 70 {
			break
		}
	}
}

// --- the big well ------------------------------------------------------------

func (rm *room) drawBigWell(f *kit.Frame, viewer kit.Player, w *well) {
	// Header: the owner's tag + label above the well. The tag is the player's
	// character tile when they have one, or the '◆' accent glyph otherwise.
	drawOwnerTag(f, bigTop-1, bigLeft, viewer, w, w.color)
	f.Text(bigTop-1, bigLeft+2, "YOUR WELL", kit.Style{FG: w.color, Attr: kit.AttrBold})

	// Border box around a wellW*2-wide interior (cells are drawn double-wide so
	// the well looks square-ish on a terminal).
	innerW := wellW * 2
	drawBox(f, bigTop, bigLeft, wellH, innerW, borderColor)

	// Ghost piece: where the active piece would land on a hard drop.
	if w.alive && w.hasPiece && len(w.clearing) == 0 {
		ghost := w.cur
		for {
			n := ghost
			n.row++
			if rm.collides(w, n) {
				break
			}
			ghost = n
		}
		for _, c := range ghost.cells(pieces) {
			if c[0] < 0 {
				continue
			}
			rm.paintBig(f, c[0], c[1], '·', ghostColor)
		}
	}

	// Settled stack.
	flash := rm.now.UnixNano()/int64(70*1e6)%2 == 0
	clearing := map[int]bool{}
	for _, r0 := range w.clearing {
		clearing[r0] = true
	}
	for r0 := 0; r0 < wellH; r0++ {
		for c := 0; c < wellW; c++ {
			v := w.grid[r0][c]
			if v == cellEmpty {
				continue
			}
			ch, fg := cellGlyph(v)
			if clearing[r0] {
				if flash {
					ch, fg = '█', kit.White
				} else {
					ch, fg = '▓', w.color
				}
			}
			rm.paintBig(f, r0, c, ch, fg)
		}
	}

	// Active piece on top.
	if w.alive && w.hasPiece && len(w.clearing) == 0 {
		p := pieces[w.cur.kind]
		for _, c := range w.cur.cells(pieces) {
			if c[0] < 0 {
				continue
			}
			rm.paintBig(f, c[0], c[1], p.glyph, p.color)
		}
	}
}

// paintBig fills a well cell as a double-wide block in the big well.
func (rm *room) paintBig(f *kit.Frame, row, col int, ch rune, fg kit.Color) {
	r := bigTop + 1 + row
	c := bigLeft + 1 + col*2
	st := kit.Style{FG: fg, Attr: kit.AttrBold}
	f.SetRune(r, c, ch, st)
	f.SetRune(r, c+1, ch, st)
}

// --- info column (preview + stats) -------------------------------------------

func (rm *room) drawInfo(f *kit.Frame, viewer kit.Player, w *well) {
	x := infoLeft
	f.Text(bigTop-1, x, "NEXT", kit.Style{FG: dimText, Attr: kit.AttrBold})
	// Preview box, 4 wide x 3 tall interior (8 cols double-wide).
	drawBox(f, bigTop, x, 4, 8, borderColor)
	if w.next >= 0 && w.next < len(pieces) {
		p := pieces[w.next]
		for _, c := range p.states[0] {
			pr := bigTop + 1 + c[0]
			pc := x + 1 + c[1]*2
			st := kit.Style{FG: p.color, Attr: kit.AttrBold}
			f.SetRune(pr, pc, p.glyph, st)
			f.SetRune(pr, pc+1, p.glyph, st)
		}
		f.Text(bigTop+5, x, p.name, kit.Style{FG: p.color})
	}

	// Stats.
	sy := bigTop + 7
	f.Text(sy, x, fmt.Sprintf("SCORE %d", w.score), kit.Style{FG: kit.White, Attr: kit.AttrBold})
	f.Text(sy+1, x, fmt.Sprintf("LINES %d", w.lines), kit.Style{FG: kit.White})
	f.Text(sy+2, x, fmt.Sprintf("LEVEL %d", w.level), kit.Style{FG: kit.White})
	f.Text(sy+3, x, fmt.Sprintf("BEST  %d", w.best), kit.Style{FG: dimText})
	if rm.solo() {
		f.Text(sy+5, x, "SCORE ATTACK", kit.Style{FG: kit.Yellow, Attr: kit.AttrBold})
	}
}

// --- rival miniatures --------------------------------------------------------

// drawRivalPanel renders each other player's well as a live miniature down the
// right side panel — one cell per grid cell so you can watch rivals tumble.
func (rm *room) drawRivalPanel(f *kit.Frame, viewer kit.Player) {
	rivals := make([]string, 0, len(rm.order))
	for _, id := range rm.order {
		if id != viewer.AccountID && rm.wells[id] != nil {
			rivals = append(rivals, id)
		}
	}
	if len(rivals) == 0 {
		return
	}
	f.Text(1, miniLeft, "RIVALS", kit.Style{FG: dimText, Attr: kit.AttrBold})

	// Up to 5 miniatures laid out in up to three columns (2 per column) so they
	// always fit the frame. Each mini is a tag row + a compact bordered well.
	const (
		perCol   = 2  // miniatures stacked per column
		colStep  = 13 // horizontal step between mini columns
		rowStep  = 10 // vertical step between stacked minis
		rowStart = 2
	)
	for i, id := range rivals {
		if i >= 5 {
			break
		}
		col := miniLeft + (i/perCol)*colStep
		rowTop := rowStart + (i%perCol)*rowStep
		rm.drawMini(f, rowTop, col, rm.wells[id], rm.names[id])
	}
}

// drawMini renders a compact miniature of a rival well: a small bordered box
// where each interior cell shows two stacked grid rows merged, plus the owner's
// tag and a warning marker when garbage is inbound.
func (rm *room) drawMini(f *kit.Frame, top, left int, w *well, p kit.Player) {
	name := p.Handle
	if len([]rune(name)) > 5 {
		name = string([]rune(name)[:5])
	}
	tag := w.color
	if !w.alive {
		tag = dimText
	}
	drawOwnerTag(f, top, left, p, w, tag)
	f.Text(top, left+2, name, kit.Style{FG: tag})

	// Inbound-garbage warning marker beside the name.
	if w.alive && !w.garbageAt.IsZero() {
		blink := rm.now.UnixNano()/int64(200*1e6)%2 == 0
		if blink {
			f.Text(top, left+9, "!", kit.Style{FG: kit.Red, Attr: kit.AttrBold})
		}
	}

	// Compress three grid rows into one display row (any filled => a block),
	// keeping the miniature short enough to stack two per column.
	const dispH = wellH / 3 // 6 display rows for an 18-row well
	drawBox(f, top+1, left, dispH, wellW, borderColor)
	for dr := 0; dr < dispH; dr++ {
		for c := 0; c < wellW; c++ {
			filled, junk := false, false
			for k := 0; k < 3; k++ {
				v := w.grid[dr*3+k][c]
				if v != cellEmpty {
					filled = true
					if v == garbageCell {
						junk = true
					}
				}
			}
			if !filled {
				continue
			}
			fg := w.color
			if junk {
				fg = garbageColor
			}
			f.SetRune(top+2+dr, left+1+c, '█', kit.Style{FG: fg, Attr: kit.AttrBold})
		}
	}
	if !w.alive {
		f.Text(top+2, left+1, "OUT", kit.Style{FG: kit.Red, Attr: kit.AttrBold})
	}
}

// drawOwnerTag draws a player's one-cell well tag at (row, col): their
// character tile when they have one, or the well's '◆'-style accent glyph in
// the given color when they don't.
func drawOwnerTag(f *kit.Frame, row, col int, p kit.Player, w *well, fg kit.Color) {
	if p.Character.Glyph != "" {
		f.Set(row, col, kit.CharacterCell(p.Character))
		return
	}
	f.SetRune(row, col, w.glyph, kit.Style{FG: fg, Attr: kit.AttrBold})
}

// --- chrome ------------------------------------------------------------------

func (rm *room) drawControls(f *kit.Frame) {
	f.Text(rows-1, 1, "←/→ move  ↓ soft  ↑/x rotate  z ccw  SPACE drop  Q quit",
		kit.Style{FG: dimText})
}

// drawBanners overlays incoming-garbage warnings, top-out, winner, etc.
func (rm *room) drawBanners(f *kit.Frame, viewer kit.Player, w *well) {
	mid := wellH/2 + bigTop

	// Incoming-garbage warning for the viewer's own well.
	if w != nil && w.alive && !w.garbageAt.IsZero() {
		blink := rm.now.UnixNano()/int64(180*1e6)%2 == 0
		if blink {
			banner := fmt.Sprintf("  ⚠ INCOMING +%d ⚠  ", w.pendingGarbage)
			f.Text(mid, bigLeft+1, banner, kit.Style{FG: kit.Red, Attr: kit.AttrBold | kit.AttrReverse})
		}
	}

	// Match / run end overlays.
	if rm.matchOver && !rm.solo() {
		if rm.winner == viewer.AccountID {
			rm.centerBanner(f, mid, "✦  YOU WIN THE MATCH  ✦", kit.Green)
		} else if rm.winner != "" {
			win := rm.names[rm.winner].Handle
			rm.centerBanner(f, mid, "WINNER: "+win, kit.Yellow)
		}
	} else if w != nil && !w.alive {
		if rm.solo() {
			rm.centerBanner(f, mid, "GAME OVER — "+fmt.Sprintf("score %d", w.score), kit.Red)
		} else {
			rm.centerBanner(f, mid, "✗  TOPPED OUT — spectating  ✗", kit.Red)
		}
	}
}

func (rm *room) centerBanner(f *kit.Frame, row int, s string, fg kit.Color) {
	// Centered over the big well region.
	field := bigLeft + wellW*2 + 2
	x := bigLeft + (field-len([]rune(s)))/2
	if x < 1 {
		x = 1
	}
	f.Text(row, x, s, kit.Style{FG: fg, Attr: kit.AttrBold | kit.AttrReverse})
}

// --- helpers -----------------------------------------------------------------

// cellGlyph maps a stored grid cell value to its glyph + color.
func cellGlyph(v int) (rune, kit.Color) {
	if v == garbageCell {
		return '▒', garbageColor
	}
	idx := v - 1
	if idx >= 0 && idx < len(pieces) {
		return pieces[idx].glyph, pieces[idx].color
	}
	return '#', kit.White
}

// drawBox draws a single-line border box; interior is innerH rows x innerW cols,
// border drawn just outside it. Top-left of the border is (top, left).
func drawBox(f *kit.Frame, top, left, innerH, innerW int, fg kit.Color) {
	st := kit.Style{FG: fg}
	right := left + innerW + 1
	bottom := top + innerH + 1
	f.SetRune(top, left, '┌', st)
	f.SetRune(top, right, '┐', st)
	f.SetRune(bottom, left, '└', st)
	f.SetRune(bottom, right, '┘', st)
	for c := left + 1; c < right; c++ {
		f.SetRune(top, c, '─', st)
		f.SetRune(bottom, c, '─', st)
	}
	for r := top + 1; r < bottom; r++ {
		f.SetRune(r, left, '│', st)
		f.SetRune(r, right, '│', st)
	}
}
