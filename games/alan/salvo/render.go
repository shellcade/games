package main

import (
	"math"

	kit "github.com/shellcade/kit/v2"
)

var (
	stTitle = kit.Style{FG: kit.RGB(0xff, 0x9f, 0x1c), Attr: kit.AttrBold}
	stDim   = kit.Style{FG: kit.Gray(0x88)}
	stWind  = kit.Style{FG: kit.RGB(0x6f, 0xff, 0xe0), Attr: kit.AttrBold}
	stHP    = kit.Style{FG: kit.RGB(0x7d, 0xff, 0x6b), Attr: kit.AttrBold}
	stWarn  = kit.Style{FG: kit.RGB(0xff, 0x49, 0x6b), Attr: kit.AttrBold}
	stMsg   = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stTraj  = kit.Style{FG: kit.Gray(0x66)}

	grass = kit.RGB(0x4f, 0xa0, 0x3c)
	dirt  = kit.RGB(0x6e, 0x4b, 0x2c)
	dirtD = kit.RGB(0x4c, 0x33, 0x1d)
)

func (rm *room) render(r kit.Room) {
	for _, v := range r.Members() {
		rm.frame.Clear()
		rm.compose(rm.frame, v)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(f *kit.Frame, v kit.Player) {
	drawSky(f)
	if rm.phase == phLobby {
		rm.drawLobby(f)
		return
	}
	rm.drawTerrain(f)
	rm.drawTanks(f)
	if rm.phase == phAim {
		rm.drawTrajectory(f)
	}
	rm.drawShell(f)
	rm.drawBooms(f)
	rm.drawParticles(f)
	rm.drawHUD(f, v)
	rm.drawPanel(f, v)
	rm.drawOverlay(f)
}

// --- the muster lobby --------------------------------------------------------

func (rm *room) drawLobby(f *kit.Frame) {
	center(f, 3, "S A L V O", stTitle)
	center(f, 5, "turn-based tank artillery", stDim)
	center(f, 7, "- BATTLE LOBBY -", stMsg)

	row := 10
	if len(rm.order) == 0 {
		center(f, row, "waiting for commanders to roll in...", stDim)
	}
	for i, id := range rm.order {
		if i >= len(tankPalette) {
			break
		}
		p := rm.players[id]
		name := handleOf(p)
		wins := rm.wins[id]
		bodyW := 2 + len(name) + 2 + intWidth(wins) + 1
		x := (scrW - bodyW) / 2
		if p.Character.Glyph != "" {
			f.Set(row, x, kit.CharacterCell(p.Character))
		} else {
			f.SetRune(row, x, '#', kit.Style{FG: kit.Gray(0x10), BG: tankPalette[i], Attr: kit.AttrBold})
		}
		c := f.Text(row, x+2, name, kit.Style{FG: tankPalette[i], Attr: kit.AttrBold})
		c = drawInt(f, row, c+2, wins, stDim)
		f.Text(row, c, "w", stDim)
		row++
	}

	center(f, 18, "press SPACE to start the battle", stMsg)
	if !rm.lobbyUntil.IsZero() {
		rem := int(math.Ceil(rm.lobbyUntil.Sub(rm.now).Seconds()))
		if rem < 0 {
			rem = 0
		}
		s := "auto-starts in "
		x := (scrW - (len(s) + intWidth(rem) + 1)) / 2
		c := f.Text(20, x, s, stDim)
		c = drawInt(f, 20, c, rem, stDim)
		f.Text(20, c, "s", stDim)
	}
	if len(rm.order) <= 1 {
		center(f, 22, "(solo: you'll battle two CPU tanks)", stDim)
	}
}

// --- scenery -----------------------------------------------------------------

func skyColor(row int) kit.Color {
	// A vertical gradient: deep indigo up top easing to a warmer horizon.
	t := float64(row-skyTop) / float64(groundBottom-skyTop)
	r := int(0x0c + t*0x20)
	g := int(0x0e + t*0x10)
	b := int(0x2c + t*0x14)
	return kit.RGB(uint8(r), uint8(g), uint8(b))
}

func drawSky(f *kit.Frame) {
	for row := skyTop; row <= groundBottom; row++ {
		st := kit.Style{BG: skyColor(row)}
		for c := 0; c < scrW; c++ {
			f.SetRune(row, c, ' ', st)
		}
	}
}

func (rm *room) drawTerrain(f *kit.Frame) {
	for c := 0; c < scrW; c++ {
		surf := rm.terrain[c]
		if surf > groundBottom {
			continue // a pit — blown clean through
		}
		f.SetRune(surf, c, ' ', kit.Style{BG: grass})
		for row := surf + 1; row <= groundBottom; row++ {
			col := dirt
			if row-surf > 3 {
				col = dirtD
			}
			f.SetRune(row, c, ' ', kit.Style{BG: col})
		}
	}
}

// --- tanks -------------------------------------------------------------------

func (rm *room) drawTanks(f *kit.Frame) {
	cur := rm.currentTank()
	for _, t := range rm.tanks {
		row, col := int(math.Round(t.y)), t.col
		if !inField(row, col) {
			continue
		}
		if !t.alive {
			f.SetRune(row, col, 'x', kit.Style{FG: kit.Gray(0x55)}) // a wreck
			continue
		}
		if t.player.Character.Glyph != "" {
			f.Set(row, col, kit.CharacterCell(t.player.Character))
		} else {
			f.SetRune(row, col, '#', kit.Style{FG: kit.Gray(0x10), BG: t.color, Attr: kit.AttrBold})
		}
		if t == cur && (rm.phase == phAim || rm.phase == phFlight) {
			rm.drawBarrel(f, t)
		}
	}
}

func (rm *room) drawBarrel(f *kit.Frame, t *tank) {
	dx, dy := t.aimVec()
	g := barrelGlyph(t.angle)
	for i := 1.0; i <= barrelLen+0.4; i += 0.8 {
		row := int(math.Round(t.y - 0.6 + dy*i))
		col := int(math.Round(float64(t.col) + dx*i))
		if inField(row, col) {
			f.SetRune(row, col, g, kit.Style{FG: t.color, Attr: kit.AttrBold})
		}
	}
}

func barrelGlyph(angle float64) rune {
	switch {
	case angle <= 28 || angle >= 152:
		return '-'
	case angle < 68:
		return '/'
	case angle <= 112:
		return '|'
	default:
		return '\\'
	}
}

// --- trajectory preview (a short aim stub, not the full solution) ------------

func (rm *room) drawTrajectory(f *kit.Frame) {
	t := rm.currentTank()
	if t == nil {
		return
	}
	dx0, dy0 := t.aimVec()
	x := float64(t.col) + dx0*barrelLen
	y := t.y - 0.6 + dy0*barrelLen
	spd := minSpeed + t.power/100*(maxSpeed-minSpeed)
	rr := t.angle * math.Pi / 180
	vx, vy := spd*math.Cos(rr), -spd*math.Sin(rr)/aspect

	const dt = 0.04
	traveled, lastCol, lastRow := 0.0, -1, -1
	for i := 0; i < 80 && traveled < 16; i++ {
		px, py := x, y
		x, y, vx, vy = integrate(x, y, vx, vy, rm.wind, dt)
		traveled += math.Hypot(x-px, y-py)
		col, row := int(math.Round(x)), int(math.Round(y))
		if !inField(row, col) {
			break
		}
		if (col != lastCol || row != lastRow) && traveled > 1.5 {
			f.SetRune(row, col, '.', stTraj)
			lastCol, lastRow = col, row
		}
	}
}

// --- shell + fx --------------------------------------------------------------

func (rm *room) drawShell(f *kit.Frame) {
	s := rm.shell
	if s == nil {
		return
	}
	for i, p := range s.trail {
		row, col := int(math.Round(p.y)), int(math.Round(p.x))
		if !inField(row, col) {
			continue
		}
		g := '.'
		if i > len(s.trail)*2/3 {
			g = ':'
		}
		f.SetRune(row, col, g, kit.Style{FG: kit.Gray(uint8(0x55 + i*4))})
	}
	row, col := int(math.Round(s.y)), int(math.Round(s.x))
	if inField(row, col) {
		f.SetRune(row, col, s.w.glyph, kit.Style{FG: s.w.color, Attr: kit.AttrBold})
	}
}

func (rm *room) drawBooms(f *kit.Frame) {
	for _, b := range rm.booms {
		age := rm.now.Sub(b.bornAt).Seconds() / impactDur.Seconds()
		if age > 1 {
			continue
		}
		rr := b.radius * (0.35 + age)
		cx := int(math.Round(b.x))
		ir := int(math.Ceil(rr))
		for dr := -ir; dr <= ir; dr++ {
			for dc := -ir; dc <= ir; dc++ {
				row, col := int(math.Round(b.y))+dr, cx+dc
				if !inField(row, col) {
					continue
				}
				d := math.Hypot(float64(dc), float64(dr)*aspect)
				if d > rr {
					continue
				}
				st := kit.Style{BG: blastColor(d/rr, age)}
				f.SetRune(row, col, ' ', st)
			}
		}
	}
}

// blastColor goes white-hot at the core to orange at the edge, dimming with age.
func blastColor(t, age float64) kit.Color {
	fade := 1 - age
	if t < 0.4 {
		return kit.RGB(uint8(255*fade), uint8(250*fade), uint8(210*fade))
	}
	if t < 0.75 {
		return kit.RGB(uint8(255*fade), uint8(190*fade), uint8(60*fade))
	}
	return kit.RGB(uint8(220*fade), uint8(90*fade), uint8(30*fade))
}

func (rm *room) drawParticles(f *kit.Frame) {
	for i := range rm.parts {
		p := &rm.parts[i]
		row, col := int(math.Round(p.y)), int(math.Round(p.x))
		if inField(row, col) {
			f.SetRune(row, col, p.glyph, kit.Style{FG: p.color})
		}
	}
}

// --- HUD ---------------------------------------------------------------------

func (rm *room) drawHUD(f *kit.Frame, v kit.Player) {
	f.Text(hudRow, 2, "SALVO", stTitle)

	// Wind gauge, centred.
	n := clampI(int(math.Round(math.Abs(rm.wind)/3.0)), 0, 6)
	col := f.Text(hudRow, 32, "WIND ", stDim)
	arrow := '>'
	if rm.wind < 0 {
		arrow = '<'
	}
	if n == 0 {
		f.Text(hudRow, col, "calm", stDim)
	} else {
		for i := 0; i < n; i++ {
			f.SetRune(hudRow, col+i, arrow, stWind)
		}
	}

	// Viewer's career wins, right.
	w := rm.wins[v.AccountID]
	start := kit.Cols - 2 - (6 + intWidth(w))
	c := f.Text(hudRow, start, "WINS ", stDim)
	drawInt(f, hudRow, c, w, stTitle)
}

// --- the aiming panel --------------------------------------------------------

func (rm *room) drawPanel(f *kit.Frame, v kit.Player) {
	if rm.phase == phOver {
		rm.drawGameOverBar(f)
		return
	}
	t := rm.currentTank()
	if t == nil {
		return
	}
	tankSt := kit.Style{FG: t.color, Attr: kit.AttrBold}
	f.SetRune(panelRow, 2, '>', tankSt)
	col := f.Text(panelRow, 4, t.name, tankSt) + 2

	col = f.Text(panelRow, col, "HP ", stDim)
	col = drawInt(f, panelRow, col, t.health, hpStyle(t.health)) + 2
	col = f.Text(panelRow, col, "ANG ", stDim)
	col = drawInt(f, panelRow, col, int(math.Round(t.angle)), stMsg) + 2
	col = f.Text(panelRow, col, "PWR ", stDim)
	col = drawInt(f, panelRow, col, int(math.Round(t.power)), stMsg) + 3

	drawWeaponBar(f, panelRow, col, t)

	// Right side: controls for the viewer on their turn, else who's up.
	if !t.cpu && t.id == v.AccountID {
		f.TextRight(panelRow, kit.Cols-2, "W weapon  SPACE fire", stDim)
	} else if t.cpu {
		f.TextRight(panelRow, kit.Cols-2, "CPU is taking aim...", stDim)
	} else {
		f.TextRight(panelRow, kit.Cols-2, "their turn...", stDim)
	}
}

// drawWeaponBar shows all three weapons with the selected one as a bright chip,
// so it's always obvious what's loaded (and that W cycles it). Spent weapons dim.
func drawWeaponBar(f *kit.Frame, row, col int, t *tank) int {
	for i := range weapons {
		w := weapons[i]
		sel := i == t.weapon
		if sel {
			st := kit.Style{FG: kit.Gray(0x10), BG: w.color, Attr: kit.AttrBold}
			f.SetRune(row, col, ' ', st)
			c := f.Text(row, col+1, w.name, st)
			if t.ammo[i] >= 0 {
				c = f.Text(row, c, " x", st)
				c = drawInt(f, row, c, t.ammo[i], st)
			}
			f.SetRune(row, c, ' ', st)
			col = c + 1
		} else {
			st := stDim
			if t.ammo[i] == 0 {
				st = kit.Style{FG: kit.Gray(0x40)} // spent
			}
			c := f.Text(row, col, w.name, st)
			if t.ammo[i] >= 0 {
				c = f.Text(row, c, " x", st)
				c = drawInt(f, row, c, t.ammo[i], st)
			}
			col = c + 1
		}
	}
	return col
}

func (rm *room) drawGameOverBar(f *kit.Frame) {
	if rm.winner == nil {
		f.Text(panelRow, 2, "everyone's scrap metal — a draw.   SPACE for a rematch", stDim)
		return
	}
	st := kit.Style{FG: rm.winner.color, Attr: kit.AttrBold}
	col := f.Text(panelRow, 2, rm.winner.name, st)
	f.Text(panelRow, col, " wins the battle!   SPACE for a rematch", stDim)
}

// --- centred overlays --------------------------------------------------------

func (rm *room) drawOverlay(f *kit.Frame) {
	if rm.phase == phOver && rm.winner != nil {
		st := kit.Style{FG: rm.winner.color, Attr: kit.AttrBold}
		x := (kit.Cols - (len(rm.winner.name) + 6)) / 2
		cx := f.Text(9, x, rm.winner.name, st)
		f.Text(9, cx, " WINS", stMsg)
		return
	}
	if rm.msg != "" && rm.now.Before(rm.msgUntil) {
		center(f, 9, rm.msg, stMsg)
	}
}

// --- helpers -----------------------------------------------------------------

func inField(row, col int) bool {
	return row >= skyTop && row <= groundBottom && col >= 0 && col < scrW
}

func hpStyle(hp int) kit.Style {
	if hp <= 30 {
		return stWarn
	}
	return stHP
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
