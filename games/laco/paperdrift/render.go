package main

import (
	"fmt"
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// Screen layout: row 0 is the HUD, rows 1..21 the sky viewport, row 22 the
// pilot strip, row 23 the key help. Each viewer gets their own camera, so
// frames are composed per member into one reused buffer.
const (
	hudRow   = 0
	viewTop  = 1
	stripRow = 22
	helpRow  = 23

	gliderCol = 18 // the focused glider sits at this screen column

	// vBarMax is the airspeed that fills the HUD speed bar. It is a display
	// scale only — independent of the physics clamp vMax — so a steep dive can
	// peg the bar while launch/cruise still read at a useful resolution.
	vBarMax = 30.0
)

var (
	hudSt    = kit.Style{FG: kit.RGB(0xe0, 0xe0, 0xe0)}
	hudKeySt = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	dimSt    = kit.Style{FG: kit.DimGray}
	warnSt   = kit.Style{FG: kit.Red, Attr: kit.AttrBold}

	ceilSt  = kit.Style{FG: kit.Gray(0x4a)}
	dirtSt  = kit.Style{FG: kit.Gray(0x3a)}
	grassSt = kit.Style{FG: kit.RGB(0x6a, 0x8f, 0x4f)}
	gateSt  = kit.Style{FG: kit.RGB(0xff, 0x8c, 0x00)}
	thermSt = kit.Style{FG: kit.RGB(0x55, 0xd7, 0xff), Attr: kit.AttrDim}
	crashSt = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stormSt = kit.Style{FG: kit.RGB(0x9a, 0x6c, 0xd6)}
	edgeSt  = kit.Style{FG: kit.RGB(0xc4, 0x8a, 0xff), Attr: kit.AttrBold}
)

var pilotColors = []kit.Color{
	kit.RGB(0x00, 0xe5, 0xff), // cyan
	kit.RGB(0xff, 0x5f, 0x87), // pink
	kit.RGB(0x39, 0xff, 0x14), // lime
	kit.RGB(0xff, 0xd7, 0x00), // gold
	kit.RGB(0xda, 0x70, 0xd6), // orchid
	kit.RGB(0xff, 0x8c, 0x00), // orange
}

func pilotColor(ps *pilot) kit.Color { return pilotColors[ps.joinOrder%len(pilotColors)] }

func gliderGlyph(pitch float64) rune {
	switch {
	case pitch > 0.9:
		return '↑'
	case pitch > 0.25:
		return '↗'
	case pitch < -0.9:
		return '↓'
	case pitch < -0.25:
		return '↘'
	default:
		return '→'
	}
}

func (rm *room) render(r kit.Room) {
	now := r.Now()
	rm.lastSecShown = rm.shownSecond(now)
	for _, p := range r.Members() {
		rm.frame.Clear()
		switch rm.phase {
		case phLobby:
			rm.composeLobby(rm.frame, r, p, now)
		case phCountdown, phFlying:
			rm.composeFlight(rm.frame, r, p, now)
		case phResults:
			rm.composeResults(rm.frame, r, p, now)
		}
		r.Send(p, rm.frame)
	}
}

func textCenter(f *kit.Frame, row int, s string, st kit.Style) {
	col := (kit.Cols - len([]rune(s))) / 2
	if col < 0 {
		col = 0
	}
	f.Text(row, col, s, st)
}

// ---- lobby ------------------------------------------------------------------

func (rm *room) composeLobby(f *kit.Frame, r kit.Room, viewer kit.Player, now time.Time) {
	accent := kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	textCenter(f, 2, "P A P E R D R I F T", accent)
	textCenter(f, 3, "one sheet of paper vs. the sky", dimSt)

	how := []string{
		"↑/↓ trim your nose — momentum is life:",
		"dive to build speed, climb to bank height,",
		"fly too slow and you stall.",
		"ride ↑↑ thermals, thread the gaps in the walls,",
		"and outrun the storm closing in from behind.",
		"everyone launches together;",
		"the furthest flight without crashing wins the round.",
	}
	for i, line := range how {
		textCenter(f, 6+i, line, hudSt)
	}

	textCenter(f, 14, "pilots on the fold", dimSt)
	row := 15
	for _, id := range rm.order {
		ps := rm.pilots[id]
		p := rm.playerFor(id)
		name := p.DisplayName()
		if ps.best > 0 {
			name += fmt.Sprintf("  (best %dm)", ps.best)
		}
		st := kit.Style{FG: pilotColor(ps)}
		if id == viewer.AccountID {
			st.Attr = kit.AttrBold
		}
		textCenter(f, row, "● "+name, st)
		row++
	}

	footer := "Enter — launch"
	if !rm.graceDeadline.IsZero() {
		secs := int(rm.graceDeadline.Sub(now).Seconds())
		if secs < 0 {
			secs = 0
		}
		footer += fmt.Sprintf("   ·   auto-launch in %ds", secs)
	} else if r.Count() < 2 && rm.cfg.Mode != kit.ModeSolo {
		footer += "   ·   or wait for company"
	}
	textCenter(f, helpRow-1, footer, hudKeySt)
}

// ---- flight (countdown + flying share the world view) ------------------------

func (rm *room) composeFlight(f *kit.Frame, r kit.Room, viewer kit.Player, now time.Time) {
	camX := rm.cameraX(viewer)
	rm.drawWorld(f, camX, now)
	if rm.phase == phFlying {
		rm.drawStorm(f, camX, now)
	}
	rm.drawPilots(f, viewer, camX, now)
	rm.drawHUD(f, viewer, now)
	rm.drawStrip(f, viewer)
	f.Text(helpRow, 1, "↑/↓ trim · dive for speed · ride ↑ thermals · outrun the storm", dimSt)

	if rm.phase == phCountdown {
		secs := int(math.Ceil(rm.countdownDeadline.Sub(now).Seconds()))
		if secs < 1 {
			secs = 1
		}
		textCenter(f, 10, fmt.Sprintf("  LAUNCH IN %d  ", secs),
			kit.Style{FG: kit.Yellow, Attr: kit.AttrBold | kit.AttrReverse})
		textCenter(f, 12, "trim with ↑/↓ — the wind is picking up", dimSt)
	}
}

// cameraX picks the viewer's focus: their own glider while it flies (or sits
// on the fold), the furthest live glider once they're down.
func (rm *room) cameraX(viewer kit.Player) int {
	ps := rm.pilots[viewer.AccountID]
	if ps != nil && (ps.alive || rm.phase == phCountdown) {
		return int(ps.x) - gliderCol
	}
	if lid := rm.leaderID(); lid != "" {
		return int(rm.pilots[lid].x) - gliderCol
	}
	if ps != nil {
		return int(ps.x) - gliderCol
	}
	return int(spawnX) - gliderCol
}

func (rm *room) drawWorld(f *kit.Frame, camX int, now time.Time) {
	// Thermal arrows drift upward on a derived clock (framerate-independent).
	tphase := int(now.UnixNano() / int64(90*time.Millisecond))
	for sc := 0; sc < kit.Cols; sc++ {
		wc := camX + sc
		if wc < 0 || wc >= len(rm.terr.floor) {
			continue
		}
		ce, fl := int(rm.terr.ceil[wc]), int(rm.terr.floor[wc])
		for row := 0; row < ce; row++ {
			f.SetRune(viewTop+row, sc, '▓', ceilSt)
		}
		f.SetRune(viewTop+fl, sc, '█', grassSt)
		for row := fl + 1; row < worldRows; row++ {
			f.SetRune(viewTop+row, sc, '▓', dirtSt)
		}
		if gT := int(rm.terr.gTop[wc]); gT >= 0 {
			gB := int(rm.terr.gBot[wc])
			for row := ce; row < fl; row++ {
				if row < gT || row > gB {
					f.SetRune(viewTop+row, sc, '█', gateSt)
				}
			}
		} else if rm.terr.therm[wc] {
			for row := ce; row < fl; row++ {
				if (row+tphase+wc%3)%3 == 0 {
					f.SetRune(viewTop+row, sc, '↑', thermSt)
				}
			}
		}
	}
}

// drawStorm overlays the advancing weather wall over everything it has
// swallowed, with a bright leading edge.
func (rm *room) drawStorm(f *kit.Frame, camX int, now time.Time) {
	edge := int(rm.stormX(now)) - camX
	if edge < 0 {
		return
	}
	for sc := 0; sc <= edge && sc < kit.Cols; sc++ {
		st, ch := stormSt, '░'
		if sc == edge {
			st, ch = edgeSt, '▒'
		}
		for row := 0; row < worldRows; row++ {
			f.SetRune(viewTop+row, sc, ch, st)
		}
	}
}

func (rm *room) drawPilots(f *kit.Frame, viewer kit.Player, camX int, now time.Time) {
	blink := (now.UnixNano()/int64(250*time.Millisecond))%2 == 0
	// Others first, the viewer's own glider last so it overdraws.
	for pass := 0; pass < 2; pass++ {
		for _, id := range rm.order {
			if (id == viewer.AccountID) != (pass == 1) {
				continue
			}
			ps := rm.pilots[id]
			if rm.phase == phFlying && !ps.flew {
				continue
			}
			rm.drawPilot(f, ps, id == viewer.AccountID, camX, blink)
		}
	}
}

func (rm *room) drawPilot(f *kit.Frame, ps *pilot, own bool, camX int, blink bool) {
	color := pilotColor(ps)
	if ps.alive {
		trailSt := kit.Style{FG: color, Attr: kit.AttrDim}
		n := ps.trailN
		if n > len(ps.trail) {
			n = len(ps.trail)
		}
		for i := 0; i < n; i++ {
			d := ps.trail[i]
			sc := int(d.x) - camX
			if sc >= 0 && sc < kit.Cols {
				f.SetRune(viewTop+clampInt(int(d.y/rowUnits), 0, worldRows-1), sc, '·', trailSt)
			}
		}
	}

	sc := int(ps.x) - camX
	row := viewTop + clampInt(int(ps.y/rowUnits), 0, worldRows-1)
	st := kit.Style{FG: color}
	if own {
		st.Attr = kit.AttrBold
	}
	switch {
	case sc < 0:
		if ps.alive {
			f.SetRune(row, 0, '«', st)
		}
	case sc >= kit.Cols:
		if ps.alive {
			f.SetRune(row, kit.Cols-1, '»', st)
		}
	case !ps.alive && ps.flew:
		f.SetRune(row, sc, '✗', crashSt)
	default:
		if ps.stalled && blink {
			st = kit.Style{FG: kit.Red, Attr: st.Attr}
		}
		f.SetRune(row, sc, gliderGlyph(ps.gamma), st)
	}
}

func (rm *room) drawHUD(f *kit.Frame, viewer kit.Player, now time.Time) {
	col := f.Text(hudRow, 1, fmt.Sprintf("PAPERDRIFT R%d", rm.roundNum), dimSt)
	ps := rm.pilots[viewer.AccountID]
	switch {
	case ps == nil || (rm.phase == phFlying && !ps.flew):
		f.Text(hudRow, col+3, "spectating — you launch next round", hudSt)
	case ps.alive || rm.phase == phCountdown:
		col = f.Text(hudRow, col+3, fmt.Sprintf("DIST %4dm", maxInt(ps.liveDist(), 0)), hudSt)
		bar := int(ps.v / vBarMax * 8)
		if bar > 8 {
			bar = 8 // a steep dive can exceed the bar scale; peg it full
		}
		col = f.Text(hudRow, col+3, "SPD ", hudSt)
		for i := 0; i < 8; i++ {
			ch := '▱'
			if i < bar {
				ch = '▰'
			}
			col = f.Text(hudRow, col, string(ch), kit.Style{FG: speedColor(ps.v)})
		}
		deg := int(ps.pitch * 180 / 3.14159265)
		col = f.Text(hudRow, col+3, fmt.Sprintf("TRIM %+3d°", deg), hudSt)
		if rm.phase == phFlying {
			gap := int(ps.x - rm.stormX(now))
			st := dimSt
			if gap < 25 {
				st = warnSt
			}
			col = f.Text(hudRow, col+3, fmt.Sprintf("STORM %dm back", maxInt(gap, 0)), st)
		}
		if ps.stalled {
			f.Text(hudRow, col+3, "STALL", warnSt)
		}
	default:
		msg := fmt.Sprintf("DOWN at %dm", ps.dist)
		if lid := rm.leaderID(); lid != "" {
			msg += " — watching " + rm.playerFor(lid).Handle
		}
		f.Text(hudRow, col+3, msg, hudSt)
	}
}

func speedColor(v float64) kit.Color {
	switch {
	case v < vStall:
		return kit.RGB(0xff, 0x55, 0x55)
	case v > 34:
		return kit.RGB(0xff, 0xd7, 0x00)
	default:
		return kit.RGB(0x55, 0xff, 0x55)
	}
}

func (rm *room) drawStrip(f *kit.Frame, viewer kit.Player) {
	col := 1
	for _, id := range rm.order {
		ps := rm.pilots[id]
		name := rm.playerFor(id).Handle
		if rs := []rune(name); len(rs) > 10 {
			name = string(rs[:10])
		}
		status := '◌' // waiting for the next launch
		dist := ps.dist
		switch {
		case ps.alive:
			status = '✈'
			dist = ps.liveDist()
		case ps.flew && ps.left:
			status = '–'
		case ps.flew:
			status = '✗'
		}
		st := kit.Style{FG: pilotColor(ps)}
		if id == viewer.AccountID {
			st.Attr = kit.AttrBold
		}
		entry := fmt.Sprintf("● %s %dm %c", name, maxInt(dist, 0), status)
		if col+len([]rune(entry))+3 > kit.Cols {
			break
		}
		col = f.Text(stripRow, col, entry, st)
		col = f.Text(stripRow, col, "   ", dimSt)
	}
}

// ---- results ------------------------------------------------------------------

func (rm *room) composeResults(f *kit.Frame, r kit.Room, viewer kit.Player, now time.Time) {
	accent := kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	textCenter(f, 2, fmt.Sprintf("ROUND %d — RESULTS", rm.roundNum), accent)

	res := rm.buildResult()
	row := 5
	for _, pr := range res.Rankings {
		ps := rm.pilots[pr.Player.AccountID]
		line := fmt.Sprintf("%d. %-14s %5dm", pr.Rank, pr.Player.Handle, pr.Metric)
		if pr.Status == kit.StatusDNF {
			line += "  (left mid-flight)"
		} else if ps != nil && ps.newPB {
			line += "  ★ NEW BEST"
		}
		st := hudSt
		if ps != nil {
			st = kit.Style{FG: pilotColor(ps)}
		}
		if pr.Player.AccountID == viewer.AccountID {
			st.Attr = kit.AttrBold
		}
		f.Text(row, 22, line, st) // fixed left edge: the table reads as columns
		row++
	}
	if vps := rm.pilots[viewer.AccountID]; vps != nil && vps.best > 0 {
		textCenter(f, row+1, fmt.Sprintf("your personal best: %dm", vps.best), dimSt)
	}

	secs := int(rm.resultsDeadline.Sub(now).Seconds())
	if secs < 0 {
		secs = 0
	}
	textCenter(f, helpRow-1, fmt.Sprintf("Enter — relaunch now   ·   next launch in %ds", secs), hudKeySt)
}
