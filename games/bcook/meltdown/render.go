package main

import (
	"fmt"
	"math"

	kit "github.com/shellcade/kit/v2"
)

// render composes and sends a tailored frame to every connected crew member,
// reusing one long-lived frame buffer (Send copies, so steady-state rendering
// is allocation-free regardless of crew size).
func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		rm.frame.Clear()
		rm.composeFor(rm.frame, p)
		r.Send(p, rm.frame)
	}
}

func (rm *room) composeFor(f *kit.Frame, viewer kit.Player) {
	rm.drawShip(f)
	rm.drawCore(f)
	rm.drawFaults(f)
	rm.drawCrew(f, viewer)
	rm.drawHUD(f, viewer)
	if rm.phase == phaseOver {
		rm.drawEndScreen(f, viewer)
	} else {
		rm.drawFixPanels(f, viewer)
	}
}

// drawShip renders the hull and bulkheads. Walls use light box characters; a
// station with no active fault shows a dim marker so crew can read the layout.
func (rm *room) drawShip(f *kit.Frame) {
	for r := 0; r < interiorRows; r++ {
		for c := 0; c < cols; c++ {
			row := top + r
			switch rm.grid[r][c] {
			case cellWall:
				f.SetRune(row, c, rm.wallGlyph(r, c), kit.Style{FG: wallColor})
			case cellStation:
				if rm.faultAt(r, c) == nil {
					f.SetRune(row, c, '·', kit.Style{FG: kit.Gray(0x55)})
				}
			}
		}
	}
}

// wallGlyph picks a box-drawing character for a wall cell from its walled
// neighbours, so the hull and bulkheads read as connected piping.
func (rm *room) wallGlyph(r, c int) rune {
	isWall := func(rr, cc int) bool {
		if rr < 0 || rr >= interiorRows || cc < 0 || cc >= cols {
			return false
		}
		return rm.grid[rr][cc] == cellWall
	}
	up := isWall(r-1, c)
	down := isWall(r+1, c)
	left := isWall(r, c-1)
	right := isWall(r, c+1)
	switch {
	case up && down && left && right:
		return '┼'
	case up && down && right:
		return '├'
	case up && down && left:
		return '┤'
	case left && right && down:
		return '┬'
	case left && right && up:
		return '┴'
	case down && right:
		return '┌'
	case down && left:
		return '┐'
	case up && right:
		return '└'
	case up && left:
		return '┘'
	case up || down:
		return '│'
	default:
		return '─'
	}
}

// drawCore renders the central reactor core. It pulses, and its colour shifts
// from hot yellow toward a dim cold red as integrity falls — the glow IS the
// shared health bar.
func (rm *room) drawCore(f *kit.Frame) {
	frac := rm.core / coreMax
	// Colour ramp by integrity.
	var col kit.Color
	switch {
	case frac > 0.6:
		col = coreHot
	case frac > 0.3:
		col = coreWarm
	default:
		col = coreCold
	}
	// Pulse: bright cells alternate with the glyph each ~half second; a dying
	// core pulses faster (more frantic).
	period := 0.5 - 0.3*(1-frac)
	if period < 0.12 {
		period = 0.12
	}
	bright := math.Mod(float64(rm.now.UnixNano())/1e9, period*2) < period
	glyph := '▓'
	if !bright {
		glyph = '▒'
	}
	if frac <= 0.3 && !bright {
		glyph = '░'
	}
	attr := kit.Attr(0)
	if bright && frac > 0.3 {
		attr = kit.AttrBold
	}
	for r := 0; r < interiorRows; r++ {
		for c := 0; c < cols; c++ {
			if rm.grid[r][c] == cellCore {
				f.SetRune(top+r, c, glyph, kit.Style{FG: col, Attr: attr})
			}
		}
	}
}

// drawFaults renders each active hazard at its station, tinted by type and
// pulsing while it festers.
func (rm *room) drawFaults(f *kit.Frame) {
	blink := rm.now.UnixNano()/int64(250*1e6)%2 == 0
	for _, fl := range rm.faults {
		st := kit.Style{FG: faultColor[fl.kind], Attr: kit.AttrBold}
		if blink {
			st.Attr |= kit.AttrReverse
		}
		f.SetRune(top+fl.row, fl.col, faultGlyph[fl.kind], st)
	}
}

// drawCrew renders each crew member's body; the viewer's own body is
// reverse-video so they can find themselves at a glance.
func (rm *room) drawCrew(f *kit.Frame, viewer kit.Player) {
	for id, m := range rm.crew {
		if !m.joined {
			continue
		}
		st := kit.Style{FG: m.color, Attr: kit.AttrBold}
		if id == viewer.AccountID {
			st.Attr |= kit.AttrReverse
		}
		f.SetRune(top+m.row, m.col, m.glyph, st)
	}
}

// --- HUD ---------------------------------------------------------------------

func (rm *room) drawHUD(f *kit.Frame, viewer kit.Player) {
	frac := rm.core / coreMax

	// Top-left: crew roster in join order, each in their colour with fix count.
	col := 1
	for _, id := range rm.order {
		m := rm.crew[id]
		p := rm.names[id]
		if m == nil {
			continue
		}
		name := p.Handle
		if len([]rune(name)) > 7 {
			name = string([]rune(name)[:7])
		}
		fg := m.color
		if !m.joined {
			fg = kit.DimGray
		}
		f.Set(0, col, kit.CharacterCell(p.Character)) // character tile
		col += 2
		seg := fmt.Sprintf("%s %d", name, m.fixes)
		if id == viewer.AccountID {
			seg += "*"
		}
		col = f.Text(0, col, seg+"  ", kit.Style{FG: fg})
		if col > 44 {
			break
		}
	}

	// Top-right: the core integrity readout. At low integrity a klaxon banner
	// flashes over it.
	if frac <= 0.25 && rm.phase == phaseRunning && rm.now.UnixNano()/int64(300*1e6)%2 == 0 {
		f.TextRight(0, cols-1, "!! ALARM !!  CORE CRITICAL",
			kit.Style{FG: kit.Red, Attr: kit.AttrBold | kit.AttrReverse})
	} else {
		f.TextRight(0, cols-1, fmt.Sprintf("CORE %3d%%  %ds", int(frac*100+0.5),
			int(rm.survivedSeconds())), rm.coreReadoutStyle(frac))
	}

	// Bottom: controls + status.
	f.Text(bottom+1, 1, "↑↓←→/hjkl move  SPACE patch/hold  type valve keys  Q leave",
		kit.Style{FG: kit.DimGray})
	f.TextRight(bottom+1, cols-1, fmt.Sprintf("FAULTS %d", len(rm.faults)),
		kit.Style{FG: kit.Gray(0x99)})
}

func (rm *room) coreReadoutStyle(frac float64) kit.Style {
	switch {
	case frac > 0.6:
		return kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	case frac > 0.3:
		return kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	default:
		return kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	}
}

// drawFixPanels shows a progress bar for the fault the viewer is currently
// working, plus a hint of what to do. Only the viewer's own active fault is
// shown so the panel doesn't clutter for spectators.
func (rm *room) drawFixPanels(f *kit.Frame, viewer kit.Player) {
	m := rm.crew[viewer.AccountID]
	if m == nil || !m.joined {
		return
	}
	// Find the fault this crew member can work right now.
	var working *fault
	for _, fl := range rm.faults {
		if rm.adjacentToFault(m, fl) {
			working = fl
			break
		}
	}
	if working == nil {
		return
	}
	rm.drawFixBar(f, working, m)
}

// drawFixBar paints a one-line progress panel near the bottom for the fault the
// crew member is working.
func (rm *room) drawFixBar(f *kit.Frame, fl *fault, m *crewMember) {
	row := bottom // just above the controls line
	color := faultColor[fl.kind]
	label := faultName[fl.kind]

	var hint string
	switch fl.kind {
	case faultLeak:
		hint = "MASH SPACE"
	case faultFire:
		hint = "HOLD SPACE"
	case faultValve:
		hint = "TYPE: " + rm.valveHint(fl)
	case faultBreach:
		if fl.holders >= 2 {
			hint = "HOLD — both crew on it"
		} else {
			hint = "NEEDS 2 CREW"
		}
	}

	const barW = 16
	filled := int(fl.progress*float64(barW) + 0.5)
	if filled > barW {
		filled = barW
	}
	bar := make([]rune, barW)
	for i := range bar {
		if i < filled {
			bar[i] = '█'
		} else {
			bar[i] = '░'
		}
	}
	text := fmt.Sprintf(" %s [%s] %s ", label, string(bar), hint)
	startCol := (cols - len([]rune(text))) / 2
	if startCol < 1 {
		startCol = 1
	}
	f.Text(row, startCol, text, kit.Style{FG: color, Attr: kit.AttrBold})
}

// valveHint renders the valve sequence with the keys already matched dimmed
// out, so the crew member sees what to press next.
func (rm *room) valveHint(fl *fault) string {
	var b []rune
	for i, ru := range fl.seq {
		if i < fl.seqAt {
			b = append(b, '·')
		} else {
			b = append(b, ru)
		}
	}
	return string(b)
}

// drawEndScreen overlays the meltdown summary: total survival time and each
// crew member's fix count.
func (rm *room) drawEndScreen(f *kit.Frame, viewer kit.Player) {
	box := []string{}
	title := "*** CORE MELTDOWN ***"
	box = append(box, title)
	box = append(box, "")
	secs := int(rm.survivedSeconds())
	box = append(box, fmt.Sprintf("The crew held the reactor for %d seconds.", secs))
	if vm := rm.crew[viewer.AccountID]; vm != nil {
		best := vm.best
		if secs > best {
			best = secs
		}
		box = append(box, fmt.Sprintf("Your best shift: %d seconds.", best))
	}
	box = append(box, "")
	// Per-member fix tally in join order.
	if len(rm.order) > 0 {
		box = append(box, "— crew —")
		for _, id := range rm.order {
			m := rm.crew[id]
			p := rm.names[id]
			if m == nil {
				continue
			}
			box = append(box, fmt.Sprintf("%-10s  %d fixes", trimName(p.Handle, 10), m.fixes))
		}
		box = append(box, "")
	}
	box = append(box, "The shift is over — your time rides the leaderboard.")

	// Center the panel, clearing a backdrop so it reads as a clean modal over
	// the dead reactor.
	h := len(box)
	startRow := (kit.Rows-h)/2 - 1
	if startRow < 1 {
		startRow = 1
	}
	widest := 0
	for _, line := range box {
		if n := len([]rune(line)); n > widest {
			widest = n
		}
	}
	pad := 2
	bc0 := (cols-widest)/2 - pad
	bc1 := bc0 + widest + 2*pad - 1
	blank := kit.Cell{Rune: ' ', BG: kit.RGB(0x10, 0x06, 0x06)}
	f.Fill(startRow-1, bc0, startRow+h, bc1, blank)
	for i, line := range box {
		rw := []rune(line)
		startCol := (cols - len(rw)) / 2
		if startCol < 0 {
			startCol = 0
		}
		bg := kit.RGB(0x10, 0x06, 0x06)
		st := kit.Style{FG: kit.White, BG: bg}
		switch {
		case i == 0:
			st = kit.Style{FG: kit.Red, BG: bg, Attr: kit.AttrBold}
		case i == 2:
			st = kit.Style{FG: kit.Yellow, BG: bg, Attr: kit.AttrBold}
		}
		f.Text(startRow+i, startCol, line, st)
	}
}

func trimName(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}
