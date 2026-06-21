package main

import (
	"fmt"
	"math"
	"sort"

	kit "github.com/shellcade/kit/v2"
)

// composeFor builds the whole 80x24 frame for one viewer: course, rivals'
// ghost balls, the viewer's bright ball with aim/power overlay, the HUD, and —
// between holes — the scorecard panel on top.
func (rm *room) composeFor(f *kit.Frame, viewer kit.Player) {
	rm.drawCourse(f)
	rm.drawWindmill(f)
	rm.drawCup(f)
	rm.drawGhostBalls(f, viewer)
	rm.drawOwnBall(f, viewer)
	rm.drawHUD(f, viewer)

	switch rm.phase {
	case phaseScorecard:
		rm.drawScorecard(f, viewer, false)
	case phaseFinal:
		rm.drawScorecard(f, viewer, true)
	}
}

// --- course ------------------------------------------------------------------

func (rm *room) drawCourse(f *kit.Frame) {
	h := &holes[rm.holeIdx]
	for ry := 0; ry < courseH; ry++ {
		row := ry + top
		for col := 0; col < cols; col++ {
			switch h.tiles[ry][col] {
			case tileWall:
				f.SetRune(row, col, '#', kit.Style{FG: wallColor})
			case tileSand:
				f.SetRune(row, col, ':', kit.Style{FG: sandColor})
			case tileWater:
				f.SetRune(row, col, '~', kit.Style{FG: waterColor})
			case tileFairway:
				f.SetRune(row, col, '.', kit.Style{FG: fairwayColor, Attr: kit.AttrDim})
			}
		}
	}
}

func (rm *room) drawWindmill(f *kit.Frame) {
	h := &holes[rm.holeIdx]
	wm := h.windmill
	if wm == nil {
		return
	}
	st := kit.Style{FG: wallColor, Attr: kit.AttrBold}
	f.SetRune(wm.hubY, wm.hubX, '+', st)
	for a := 0; a < wm.arms; a++ {
		ang := rm.hub + float64(a)*2*math.Pi/float64(wm.arms)
		dx, dy := math.Cos(ang), math.Sin(ang)*aspect
		glyph := armGlyph(ang)
		for l := 1; l <= wm.length; l++ {
			cx := roundCell(float64(wm.hubX) + dx*float64(l))
			cy := roundCell(float64(wm.hubY) + dy*float64(l))
			f.SetRune(cy, cx, glyph, st)
		}
	}
}

// armGlyph picks a line glyph that matches the arm's slope.
func armGlyph(ang float64) rune {
	s := headingSector(ang)
	switch s {
	case 0, 4:
		return '-'
	case 2, 6:
		return '|'
	case 1, 5:
		return '\\'
	default:
		return '/'
	}
}

func headingSector(heading float64) int {
	twoPi := 2 * math.Pi
	hh := math.Mod(heading, twoPi)
	if hh < 0 {
		hh += twoPi
	}
	return int(math.Round(hh/(twoPi/8))) % 8
}

func (rm *room) drawCup(f *kit.Frame) {
	h := &holes[rm.holeIdx]
	// Flag pole + pennant just above the cup if there's room, then the cup.
	if h.cupY-1 >= top {
		f.SetRune(h.cupY-1, h.cupX, 'P', kit.Style{FG: kit.Red, Attr: kit.AttrBold})
	}
	f.SetRune(h.cupY, h.cupX, 'H', kit.Style{FG: cupColor, Attr: kit.AttrBold})
}

// --- balls -------------------------------------------------------------------

func (rm *room) drawGhostBalls(f *kit.Frame, viewer kit.Player) {
	for id, g := range rm.golfers {
		if id == viewer.AccountID || g.state == stateSunk {
			continue
		}
		f.SetRune(roundCell(g.y), roundCell(g.x), '○', kit.Style{FG: ghostColor, Attr: kit.AttrDim})
	}
}

func (rm *room) drawOwnBall(f *kit.Frame, viewer kit.Player) {
	g := rm.golfers[viewer.AccountID]
	if g == nil {
		return
	}
	// A motion trail when moving fast, so a struck ball reads as alive.
	if g.state == stateRoll && math.Hypot(g.vx, g.vy/aspect) > trailSpeed {
		tx, ty := roundCell((g.x+g.prevX)/2), roundCell((g.y+g.prevY)/2)
		f.SetRune(ty, tx, '·', kit.Style{FG: g.color, Attr: kit.AttrDim})
	}

	st := kit.Style{FG: g.color, Attr: kit.AttrBold | kit.AttrReverse}
	if g.state == stateSunk {
		st = kit.Style{FG: cupColor, Attr: kit.AttrBold}
	}
	f.SetRune(roundCell(g.y), roundCell(g.x), g.glyph, st)

	// Aim overlay only while the viewer is setting up a shot.
	if g.state == stateAim {
		rm.drawAim(f, g)
	}
}

// drawAim renders dotted aim pips marching out from the ball along the heading.
// The line lengthens with the dialed power notch so you feel the dial as range.
func (rm *room) drawAim(f *kit.Frame, g *golfer) {
	dx, dy := math.Cos(g.aim), math.Sin(g.aim)*aspect
	st := kit.Style{FG: g.color}
	pips := 2 + g.notch
	for i := 1; i <= pips; i++ {
		px := roundCell(g.x + dx*float64(i))
		py := roundCell(g.y + dy*float64(i))
		if py < top || py > bottom || px < 0 || px >= cols {
			break
		}
		// Don't paint over walls — the pips show the open line.
		if holes[rm.holeIdx].at(py, px) == tileWall {
			break
		}
		ch := '·'
		if i == pips {
			ch = arrowGlyph(g.aim)
		}
		f.SetRune(py, px, ch, st)
	}
}

func arrowGlyph(ang float64) rune {
	return [8]rune{'→', '↘', '↓', '↙', '←', '↖', '↑', '↗'}[headingSector(ang)]
}

// drawPowerDial renders the always-visible notched power meter on the controls
// bar: one cell per notch, filled up to the dial setting, shifting cool→hot.
func (rm *room) drawPowerDial(f *kit.Frame, g *golfer, row, col int) {
	col = f.Text(row, col, "PWR ", kit.Style{FG: kit.White, Attr: kit.AttrBold})
	for i := 1; i <= powerNotches; i++ {
		ch := '▯'
		fg := kit.DimGray
		if i <= g.notch {
			ch = '▮'
			frac := float64(i) / powerNotches
			switch {
			case frac > 0.75:
				fg = kit.Red
			case frac > 0.45:
				fg = kit.RGB(0xff, 0xa5, 0x33)
			default:
				fg = kit.Green
			}
		}
		f.SetRune(row, col, ch, kit.Style{FG: fg, Attr: kit.AttrBold})
		col++
	}
}

// --- HUD ---------------------------------------------------------------------

func (rm *room) drawHUD(f *kit.Frame, viewer kit.Player) {
	h := &holes[rm.holeIdx]

	// Top-left: hole number, name, par, your strokes for this hole.
	left := fmt.Sprintf("HOLE %d/%d  %s  PAR %d", rm.holeIdx+1, len(holes), h.name, h.par)
	f.Text(0, 1, left, kit.Style{FG: kit.White, Attr: kit.AttrBold})

	if vg := rm.golfers[viewer.AccountID]; vg != nil {
		f.TextRight(0, cols-1, fmt.Sprintf("STROKES %d   TOTAL %d", vg.strokes, vg.total()),
			kit.Style{FG: vg.color, Attr: kit.AttrBold})
	}

	// Bottom controls bar + the always-visible power dial.
	f.Text(bottom+1, 1, "←/→ aim  ↑/↓ power  SPACE putt  Q quit",
		kit.Style{FG: kit.DimGray})
	if vg := rm.golfers[viewer.AccountID]; vg != nil {
		rm.drawPowerDial(f, vg, bottom+1, 42)
	}

	if vg := rm.golfers[viewer.AccountID]; vg != nil && rm.phase == phasePlay {
		switch vg.state {
		case stateRoll:
			f.TextRight(bottom+1, cols-1, "rolling…", kit.Style{FG: kit.White})
		case stateSunk:
			f.TextRight(bottom+1, cols-1, holeResultLabel(vg.strokes, h.par)+" - waiting",
				kit.Style{FG: cupColor, Attr: kit.AttrBold})
		}
	}
}

// holeResultLabel names a hole score relative to par (golf vocabulary).
func holeResultLabel(strokes, par int) string {
	d := strokes - par
	switch {
	case strokes == 1:
		return "HOLE IN ONE!"
	case d <= -3:
		return "ALBATROSS"
	case d == -2:
		return "EAGLE"
	case d == -1:
		return "BIRDIE"
	case d == 0:
		return "PAR"
	case d == 1:
		return "BOGEY"
	case d == 2:
		return "DOUBLE BOGEY"
	default:
		return fmt.Sprintf("+%d", d)
	}
}

// --- scorecard ---------------------------------------------------------------

// drawScorecard overlays a centered panel: per-golfer totals (and the just-
// finished hole's result), sorted low-to-high. final adds the winner banner.
func (rm *room) drawScorecard(f *kit.Frame, viewer kit.Player, final bool) {
	type row struct {
		id    string
		g     *golfer
		p     kit.Player
		total int
	}
	var list []row
	for _, id := range rm.order {
		g := rm.golfers[id]
		p := rm.names[id]
		if g == nil {
			continue
		}
		list = append(list, row{id: id, g: g, p: p, total: g.total()})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].total < list[j].total })

	w := 50
	x0 := (cols - w) / 2
	y0 := 3
	hgt := 5 + len(list)
	if hgt > courseH-2 {
		hgt = courseH - 2
	}
	// Panel background box.
	f.Fill(y0, x0, y0+hgt, x0+w, kit.Cell{Rune: ' ', BG: kit.RGB(0x10, 0x18, 0x12)})
	for c := x0; c <= x0+w; c++ {
		f.SetRune(y0, c, '=', kit.Style{FG: cupColor, Attr: kit.AttrBold})
		f.SetRune(y0+hgt, c, '=', kit.Style{FG: cupColor, Attr: kit.AttrBold})
	}

	title := "SCORECARD"
	if final {
		title = "FINAL - ROUND OF 9"
	} else {
		title = fmt.Sprintf("HOLE %d COMPLETE", rm.holeIdx+1)
	}
	f.Text(y0+1, x0+(w-len([]rune(title)))/2, title, kit.Style{FG: cupColor, Attr: kit.AttrBold})

	justHole := rm.holeIdx // the hole whose result we just appended
	for i, rw := range list {
		ry := y0 + 3 + i
		if ry >= y0+hgt {
			break
		}
		marker := fmt.Sprintf("%d.", i+1)
		col := x0 + 2
		col = f.Text(ry, col, marker+" ", kit.Style{FG: kit.White})
		f.Set(ry, col, kit.CharacterCell(rw.p.Character))
		col += 2
		name := rw.p.Handle
		if len([]rune(name)) > 10 {
			name = string([]rune(name)[:10])
		}
		seg := name
		if rw.id == viewer.AccountID {
			seg += "*"
		}
		col = f.Text(ry, col, seg, kit.Style{FG: rw.g.color, Attr: kit.AttrBold})

		// This hole's result + running total, right-aligned in the panel.
		result := ""
		if justHole < len(rw.g.scores) {
			result = holeResultLabel(rw.g.scores[justHole], holes[justHole].par)
		}
		tail := fmt.Sprintf("%s   TOT %d", result, rw.total)
		f.TextRight(ry, x0+w-2, tail, kit.Style{FG: rw.g.color})
	}

	if final && len(list) > 0 {
		win := list[0]
		banner := fmt.Sprintf("WINNER: %s  (%d strokes)", win.p.Handle, win.total)
		if len(list) == 1 {
			banner = fmt.Sprintf("FINAL SCORE: %d strokes", win.total)
		}
		f.Text(y0+hgt-1, x0+(w-len([]rune(banner)))/2, banner,
			kit.Style{FG: kit.White, Attr: kit.AttrBold})
	}
}
