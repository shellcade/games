package main

import (
	kit "github.com/shellcade/kit/v2"
)

// Rendering: a centered, clamped viewport over the delver's floor with
// torch-radius sight and explored-memory fog; HUD and message log below.
// One reused frame; cells written directly (allocation-free steady state).

// hintLegend is the row-23 key bar. LAYOUT INVARIANT: msgWidth + its rune
// count must fit 80 cols — Gate 5 measures THIS string, so growing it fails
// the build until msgWidth is re-budgeted.
const hintLegend = "[wasd > b l f e q]"

var (
	stWall     = kit.Style{FG: kit.DimGray}
	stFloorDim = kit.Style{FG: kit.Gray(0x38)} // explored, out of sight
	stFloorLit = kit.Style{FG: kit.Gray(0x58)}
	stWater    = kit.Style{FG: kit.RGB(0x3a, 0x5a, 0x8a)}
	stDoor     = kit.Style{FG: kit.RGB(0xb0, 0x8a, 0x3a)}
	stStairs   = kit.Style{FG: kit.Green, Attr: kit.AttrBold}
	stShrine   = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stSelf     = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stOther    = kit.Style{FG: kit.Cyan}
	stHUD      = kit.Style{FG: kit.White}
	stHUDDim   = kit.Style{FG: kit.DimGray}
	stTorch    = kit.Style{FG: kit.Yellow}
	stTorchOut = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
	stMsg      = kit.Style{FG: kit.Gray(0x9a)}
	stTitle    = kit.Style{FG: kit.Red, Attr: kit.AttrBold}
)

func tileStyle(t byte, lit bool) (rune, kit.Style) {
	switch t {
	case tWall:
		return '#', stWall
	case tDoor:
		return '+', stDoor
	case tDown:
		return '>', stStairs
	case tUp:
		return '<', stStairs
	case tShrine:
		return '_', stShrine
	case tWater:
		return '~', stWater
	case tCrypt:
		return '▒', kit.Style{FG: kit.RGB(0xa0, 0x90, 0x60), Attr: kit.AttrBold}
	default:
		if lit {
			return '.', stFloorLit
		}
		return '.', stFloorDim
	}
}

// compose renders d's view into rm.frame (every cell of every row is
// overwritten — no Clear needed).
func (rm *room) compose(d *delver) {
	fr := rm.frame
	f := rm.world.at(d.floor)
	mem := d.explored[d.floor]
	ox, oy := d.camera()
	sight := d.sightRadius()

	for vy := 0; vy < mapRows; vy++ {
		wy := oy + vy
		for vx := 0; vx < kit.Cols; vx++ {
			wx := ox + vx
			if mem == nil || !mem[wy][wx] {
				fr.Cells[vy][vx] = kit.Cell{} // unexplored void
				continue
			}
			dx, dy := wx-d.x, wy-d.y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			lit := dx <= sight && dy <= sight
			g, st := tileStyle(f.tiles[wy][wx], lit)
			fr.Cells[vy][vx] = kit.Cell{Rune: g, FG: st.FG, Attr: st.Attr}
		}
	}

	// Bones first (terrain-level: remembered like the map — and the unsprung
	// tomb mimic hides among them, byte-identical), then LIVE entities, which
	// render only inside current torch sight: the fog remembers floors, never
	// what walks them.
	for _, dr := range rm.drops {
		if dr.taken || dr.floor != d.floor {
			continue
		}
		if dr.def == nil {
			rm.plot(d, dr.x, dr.y, '$', kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}, false)
		} else {
			rm.plot(d, dr.x, dr.y, dr.def.glyph, dr.def.style, false)
		}
	}
	for _, c := range rm.bones {
		if c.floor == d.floor && !c.dust() {
			rm.plot(d, c.x, c.y, '%', kit.Style{FG: kit.Gray(0xb8)}, false)
		}
	}
	for _, m := range rm.monsters {
		if m.hp > 0 && m.floor == d.floor {
			switch {
			case m.hidden:
				rm.plot(d, m.x, m.y, '%', kit.Style{FG: kit.Gray(0xb8)}, false)
			case m.ally:
				rm.plot(d, m.x, m.y, m.sp.glyph, kit.Style{FG: kit.Green, Attr: kit.AttrBold}, true)
			case m.sp.stealthy():
				if cheb(m.x-d.x, m.y-d.y) <= 3 {
					rm.plot(d, m.x, m.y, m.sp.glyph, m.sp.style, true)
				} else if cheb(m.x-d.x, m.y-d.y) <= 6 {
					rm.plot(d, m.x, m.y, '?', kit.Style{FG: kit.DimGray}, true) // a faint wrongness
				}
			default:
				rm.plot(d, m.x, m.y, m.sp.glyph, m.sp.style, true)
			}
		}
	}

	// Other delvers on this floor (live co-presence), then self on top.
	for _, o := range rm.delvers {
		if o == d || o.floor != d.floor || !o.online {
			continue
		}
		rm.plot(d, o.x, o.y, '@', stOther, true)
	}
	rm.plot(d, d.x, d.y, '@', stSelf, true)

	rm.hud(d)
}

// plot writes a world-coordinate glyph into d's viewport. Terrain-class
// marks (bones, the disguised mimic) render from explored MEMORY; live
// entities additionally require current torch sight (live=true).
func (rm *room) plot(d *delver, wx, wy int, g rune, st kit.Style, live bool) {
	ox, oy := d.camera()
	vx, vy := wx-ox, wy-oy
	if vx < 0 || vx >= kit.Cols || vy < 0 || vy >= mapRows {
		return
	}
	if mem := d.explored[d.floor]; mem == nil || !mem[wy][wx] {
		return
	}
	if live && cheb(wx-d.x, wy-d.y) > d.sightRadius() {
		return
	}
	rm.frame.Cells[vy][vx] = kit.Cell{Rune: g, FG: st.FG, Attr: st.Attr}
}

// hud writes rows 21..23: vitals, world line, message log.
func (rm *room) hud(d *delver) {
	fr := rm.frame
	for r := mapRows; r < kit.Rows; r++ {
		for c := 0; c < kit.Cols; c++ {
			fr.Cells[r][c] = kit.Cell{}
		}
	}

	// Row 21: HP, depth, torch gauge, gold.
	col := fr.Text(21, 0, "HP ", stHUDDim)
	col = putInt(fr, 21, col, d.hp, stHUD)
	fr.SetRune(21, col, '/', stHUD)
	putInt(fr, 21, col+1, d.maxHP, stHUD)
	col = 14
	col = fr.Text(21, col, "B", kit.Style{FG: kit.Yellow, Attr: kit.AttrBold})
	putInt(fr, 21, col, d.floor, kit.Style{FG: kit.Yellow, Attr: kit.AttrBold})
	col = 22
	fr.Text(21, col, "Torch ", stHUDDim)
	rm.torchGauge(d, 28)
	col = 42
	col = fr.Text(21, col, "$ ", stHUDDim)
	putInt(fr, 21, col, d.gold, stTorch)
	// "banked B<n>" right-aligned to kit.Cols-1.
	bw := runeLen("banked B") + intWidth(d.banked)
	bc := fr.Text(21, kit.Cols-bw, "banked B", stHUDDim)
	putInt(fr, 21, bc, d.banked, stHUDDim)

	// Row 22: the world line — who else is down here.
	others := 0
	for _, o := range rm.delvers {
		if o != d && o.floor == d.floor {
			others++
		}
	}
	fr.Text(22, 0, "BONEYARD", stTitle)
	// "collapses in <cdCache>" right-aligned to kit.Cols-1, written as two
	// alloc-free frame writes (the literal+cdCache concat would allocate per
	// render under -gc=leaking).
	cdw := runeLen("collapses in ") + runeLen(rm.cdCache)
	cdc := fr.Text(22, kit.Cols-cdw, "collapses in ", stHUDDim)
	fr.Text(22, cdc, rm.cdCache, stHUDDim)
	if others > 0 {
		oc := putInt(fr, 22, 10, others, stMsg)
		fr.Text(22, oc, " other delver(s) on this floor", stMsg)
	} else {
		fr.Text(22, 10, "alone down here (you think)", stHUDDim)
	}

	// Row 23: the newest message line + hints.
	fr.Text(23, 0, d.msg[1], stMsg)
	fr.TextRight(23, kit.Cols-1, hintLegend, stHUDDim)
}

// torchGauge draws ▓-blocks for the remaining torch (600t = 5 blocks) + the
// numeric tail; returns nothing useful (layout is fixed-column).
func (rm *room) torchGauge(d *delver, col int) int {
	fr := rm.frame
	blocks := (d.torch + 119) / 120 // 0..5
	for i := 0; i < 5; i++ {
		st := stTorch
		g := '▓'
		if i >= blocks {
			g = '░'
			st = stHUDDim
		}
		fr.Cells[21][col+i] = kit.Cell{Rune: g, FG: st.FG}
	}
	numSt := stTorch
	if d.torch == 0 {
		numSt = stTorchOut
	}
	tc := putInt(fr, 21, col+6, d.torch, numSt)
	fr.SetRune(21, tc, 't', numSt)
	return col
}

// memorial renders the week's wall of the dead over the viewport: the most
// mourned, the deepest fallen, and the freshest graves (artboard: the
// Memorial Wall). [m] toggles.
func (rm *room) memorial(d *delver) {
	fr := rm.frame
	for r := 0; r < kit.Rows; r++ {
		for c := 0; c < kit.Cols; c++ {
			fr.Cells[r][c] = kit.Cell{}
		}
	}
	fr.Text(1, 2, "THE BONEYARD — ROLL OF THE DEAD", stTitle)
	mc := fr.Text(2, 2, "collapses in ", stHUDDim)
	fr.Text(2, mc, rm.cdCache, stHUDDim)

	// Top three by respects, then the deepest, then the freshest.
	row := 4
	var mostMourned, deepest *corpse
	for _, c := range rm.bones {
		if mostMourned == nil || c.respects > mostMourned.respects {
			mostMourned = c
		}
		if deepest == nil || c.floor > deepest.floor {
			deepest = c
		}
	}
	line := func(label string, c *corpse) {
		if c == nil {
			return
		}
		fr.Text(row, 2, label, stShrine)
		w := newClampWriter(fr, row, 18, 58, stHUD)
		w.strClamp(c.handle, 24).str(" — ").str(c.killer).str(", B").num(c.floor)
		w.done()
		row++
		ww := newClampWriter(fr, row, 18, 58, stMsg)
		ww.str("\"").str(c.words).str("\"")
		ww.done()
		row += 2
	}
	line("MOST MOURNED", mostMourned)
	line("DEEPEST DEATH", deepest)
	n := len(rm.bones)
	nc := putInt(fr, row+1, 2, n, stMsg)
	fr.Text(row+1, nc, " souls rest in this week's Ossuary.", stMsg)
	fr.Text(kit.Rows-3, 2, "YOUR LINEAGE", stShrine)
	lc := fr.Text(kit.Rows-2, 2, "this week: banked B", stHUD)
	lc = putInt(fr, kit.Rows-2, lc, d.banked, stHUD)
	lc = fr.Text(kit.Rows-2, lc, "   ", stHUD)
	fr.Text(kit.Rows-2, lc, delverBadge(d.banked), stHUD)
	fr.Text(kit.Rows-1, 2, "[m] back to the dark", stHUDDim)
}

// deathSummary is the YOU DIED card's frozen run stats.
type deathSummary struct {
	killer            string
	floor, banked     int
	kills, gold       int
	respects, avenges int
	deepestThisWeek   int
	deepestHandle     string
}

// deathCardScreen renders the artboard's YOU DIED card.
func (rm *room) deathCardScreen(d *delver) {
	fr := rm.frame
	clearFrame(fr)
	c := d.deathCard
	center := func(row int, s string, st kit.Style) {
		col := (kit.Cols - len([]rune(s))) / 2
		if col < 0 {
			col = 0
		}
		fr.Text(row, col, s, st)
	}
	center(4, "Y O U   D I E D", kit.Style{FG: kit.Red, Attr: kit.AttrBold})

	// "on B<floor>, slain by <killer>"
	w6 := runeLen("on B") + intWidth(c.floor) + runeLen(", slain by ") + runeLen(c.killer)
	c6 := centerStart(w6)
	c6 = fr.Text(6, c6, "on B", stHUD)
	c6 = putInt(fr, 6, c6, c.floor, stHUD)
	c6 = fr.Text(6, c6, ", slain by ", stHUD)
	fr.Text(6, c6, c.killer, stHUD)

	// "banked B<n>   <kills> kills   <gold> gold"
	w9 := runeLen("banked B") + intWidth(c.banked) + runeLen("   ") +
		intWidth(c.kills) + runeLen(" kills   ") + intWidth(c.gold) + runeLen(" gold")
	c9 := centerStart(w9)
	c9 = fr.Text(9, c9, "banked B", stMsg)
	c9 = putInt(fr, 9, c9, c.banked, stMsg)
	c9 = fr.Text(9, c9, "   ", stMsg)
	c9 = putInt(fr, 9, c9, c.kills, stMsg)
	c9 = fr.Text(9, c9, " kills   ", stMsg)
	c9 = putInt(fr, 9, c9, c.gold, stMsg)
	fr.Text(9, c9, " gold", stMsg)

	// "<respects> mourned   <avenges> avenged"
	w10 := intWidth(c.respects) + runeLen(" mourned   ") + intWidth(c.avenges) + runeLen(" avenged")
	c10 := centerStart(w10)
	c10 = putInt(fr, 10, c10, c.respects, stMsg)
	c10 = fr.Text(10, c10, " mourned   ", stMsg)
	c10 = putInt(fr, 10, c10, c.avenges, stMsg)
	fr.Text(10, c10, " avenged", stMsg)

	if c.deepestHandle != "" {
		// "deepest this week: B<n> by <handle clamped to 20>"
		w13 := runeLen("deepest this week: B") + intWidth(c.deepestThisWeek) +
			runeLen(" by ") + clampWidth(c.deepestHandle, 20)
		c13 := centerStart(w13)
		c13 = fr.Text(13, c13, "deepest this week: B", stShrine)
		c13 = putInt(fr, 13, c13, c.deepestThisWeek, stShrine)
		c13 = fr.Text(13, c13, " by ", stShrine)
		dw := newClampWriter(fr, 13, c13, 20, stShrine)
		dw.strClamp(c.deepestHandle, 20)
		dw.done()
	}
	center(16, "the Gate calls you back", stHUDDim)
	center(kit.Rows-2, "press any key", stHUDDim)
}

// gateScreen renders the surface hub (artboard: THE GATE).
func (rm *room) gateScreen(d *delver) {
	fr := rm.frame
	clearFrame(fr)
	center := func(row int, s string, st kit.Style) {
		col := (kit.Cols - len([]rune(s))) / 2
		if col < 0 {
			col = 0
		}
		fr.Text(row, col, s, st)
	}
	center(2, "T H E   G A T E", stTitle)
	// "The Sunken Ossuary  —  collapses in <cdCache>" centered, written as two
	// alloc-free frame writes (concat would allocate per render).
	gw := runeLen("The Sunken Ossuary  —  collapses in ") + runeLen(rm.cdCache)
	gc := centerStart(gw)
	gc = fr.Text(3, gc, "The Sunken Ossuary  —  collapses in ", stHUDDim)
	fr.Text(3, gc, rm.cdCache, stHUDDim)
	deep := 0
	var whoHandle string
	for _, c := range rm.bones {
		if c.floor > deep {
			deep, whoHandle = c.floor, c.handle
		}
	}
	// "deepest descent:  B<deep>  (<name clamped to 24>)" — name() == clampCols(handle,24).
	w6 := runeLen("deepest descent:  B") + intWidth(deep) + runeLen("  (") +
		clampWidth(whoHandle, 24) + runeLen(")")
	c6 := centerStart(w6)
	c6 = fr.Text(6, c6, "deepest descent:  B", stShrine)
	c6 = putInt(fr, 6, c6, deep, stShrine)
	c6 = fr.Text(6, c6, "  (", stShrine)
	nw := newClampWriter(fr, 6, c6, 24, stShrine)
	nw.strClamp(whoHandle, 24)
	nw.done()
	fr.Text(6, nw.col, ")", stShrine)

	// "<bones> bones rest below.  Your best: B<banked>"
	nb := len(rm.bones)
	w7 := intWidth(nb) + runeLen(" bones rest below.  Your best: B") + intWidth(d.banked)
	c7 := centerStart(w7)
	c7 = putInt(fr, 7, c7, nb, stMsg)
	c7 = fr.Text(7, c7, " bones rest below.  Your best: B", stMsg)
	putInt(fr, 7, c7, d.banked, stMsg)
	center(10, "[1] BLADE    [2] LANTERN    [3] FLASK", stHUD)
	center(11, "choose your kit, then step off the stairs to descend", stHUDDim)
	center(14, "[m] the Roll of the Dead", stHUDDim)
	center(kit.Rows-2, "wasd to move   >  to descend", stHUDDim)
}

func clearFrame(fr *kit.Frame) {
	for r := 0; r < kit.Rows; r++ {
		for c := 0; c < kit.Cols; c++ {
			fr.Cells[r][c] = kit.Cell{}
		}
	}
}

// delverBadge names the DELVER band a banked depth earns (design §11).
func delverBadge(banked int) string {
	switch {
	case banked >= 21:
		return "Of The Deep"
	case banked >= 16:
		return "Legend"
	case banked >= 10:
		return "Brutalist"
	case banked >= 4:
		return "Belt-Walker"
	default:
		return "(unproven)"
	}
}
