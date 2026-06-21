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
	// Each delver renders as their arcade character tile (kit v2.9.0) — the
	// same glyph the player and everyone else sees — falling back to the
	// classic '@' when no character rides the connection.
	for _, o := range rm.delvers {
		if o == d || o.floor != d.floor || !o.online {
			continue
		}
		rm.plotCell(d, o.x, o.y, delverCell(o, stOther), true)
	}
	rm.plotCell(d, d.x, d.y, delverCell(d, stSelf), true)

	rm.hud(d)
}

// plot writes a world-coordinate glyph into d's viewport. Terrain-class
// marks (bones, the disguised mimic) render from explored MEMORY; live
// entities additionally require current torch sight (live=true).
func (rm *room) plot(d *delver, wx, wy int, g rune, st kit.Style, live bool) {
	rm.plotCell(d, wx, wy, kit.Cell{Rune: g, FG: st.FG, Attr: st.Attr}, live)
}

// plotCell is plot for a ready-made cell (a character tile keeps its own
// ink AND background, which the rune path has no slot for).
func (rm *room) plotCell(d *delver, wx, wy int, cell kit.Cell, live bool) {
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
	rm.frame.Cells[vy][vx] = cell
}

// delverCell is the one map cell a delver occupies: their arcade character
// tile when the connection carries one, else '@' in the classic style.
func delverCell(o *delver, fallback kit.Style) kit.Cell {
	if o.p.Character.Glyph != "" {
		return kit.CharacterCell(o.p.Character)
	}
	return kit.Cell{Rune: '@', FG: fallback.FG, Attr: fallback.Attr}
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
	col += fr.Text(21, col, itoa(d.hp)+"/"+itoa(d.maxHP), stHUD)
	col = 14
	col += fr.Text(21, col, "B"+itoa(d.floor), kit.Style{FG: kit.Yellow, Attr: kit.AttrBold})
	col = 22
	col += fr.Text(21, col, "Torch ", stHUDDim)
	col = rm.torchGauge(d, 28)
	col = 42
	col += fr.Text(21, col, "$ ", stHUDDim)
	col += fr.Text(21, col, itoa(d.gold), stTorch)
	fr.TextRight(21, kit.Cols-1, "banked B"+itoa(d.banked), stHUDDim)

	// Row 22: the world line — who else is down here.
	others := 0
	for _, o := range rm.delvers {
		if o != d && o.floor == d.floor {
			others++
		}
	}
	fr.Text(22, 0, "BONEYARD", stTitle)
	fr.TextRight(22, kit.Cols-1, "collapses in "+rm.cdCache, stHUDDim)
	if others > 0 {
		fr.Text(22, 10, itoa(others)+" other delver(s) on this floor", stMsg)
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
	fr.Text(21, col+6, itoa(d.torch)+"t", numSt)
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
	fr.Text(1, 2, "THE BONEYARD - ROLL OF THE DEAD", stTitle)
	fr.Text(2, 2, "collapses in "+rm.cdCache, stHUDDim)

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
		// The dead's character tile (frozen at death) rides immediately
		// before their name; ancestral bones carry none and leave the gap.
		if c.ch.Glyph != "" {
			fr.Cells[row][16] = kit.CharacterCell(c.ch)
		}
		fr.Text(row, 18, clampCols(c.name()+" - "+c.killer+", B"+itoa(c.floor), 58), stHUD)
		row++
		fr.Text(row, 18, clampCols("\""+c.words+"\"", 58), stMsg)
		row += 2
	}
	line("MOST MOURNED", mostMourned)
	line("DEEPEST DEATH", deepest)
	n := len(rm.bones)
	fr.Text(row+1, 2, itoa(n)+" souls rest in this week's Ossuary.", stMsg)
	fr.Text(kit.Rows-3, 2, "YOUR LINEAGE", stShrine)
	fr.Text(kit.Rows-2, 2, "this week: banked B"+itoa(d.banked)+"   "+delverBadge(d.banked), stHUD)
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
	deepestCh         kit.Character // the deepest fallen's character tile (zero if none)
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
	center(6, "on B"+itoa(c.floor)+", slain by "+c.killer, stHUD)
	center(9, "banked B"+itoa(c.banked)+"   "+itoa(c.kills)+" kills   "+itoa(c.gold)+" gold", stMsg)
	center(10, itoa(c.respects)+" mourned   "+itoa(c.avenges)+" avenged", stMsg)
	if c.deepestHandle != "" {
		centerWithTile(fr, 13, "deepest this week: B"+itoa(c.deepestThisWeek)+" by ", c.deepestCh, clampCols(c.deepestHandle, 20), stShrine)
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
	center(3, "The Sunken Ossuary  -  collapses in "+rm.cdCache, stHUDDim)
	deep, who := 0, ""
	var whoCh kit.Character
	for _, c := range rm.bones {
		if c.floor > deep {
			deep, who, whoCh = c.floor, c.name(), c.ch
		}
	}
	centerWithTile(fr, 6, "deepest descent:  B"+itoa(deep)+"  (", whoCh, who+")", stShrine)
	center(7, itoa(len(rm.bones))+" bones rest below.  Your best: B"+itoa(d.banked), stMsg)
	center(10, "[1] BLADE    [2] LANTERN    [3] FLASK", stHUD)
	center(11, "choose your kit, then step off the stairs to descend", stHUDDim)
	center(14, "[m] the Roll of the Dead", stHUDDim)
	center(kit.Rows-2, "wasd to move   >  to descend", stHUDDim)
}

// centerWithTile centres "<pre><character tile> <post>" on a row — the tile
// (one styled cell + one space) rides immediately before the name in post.
// A zero character degrades to the plain centered string, gap-free.
func centerWithTile(fr *kit.Frame, row int, pre string, ch kit.Character, post string, st kit.Style) {
	if ch.Glyph == "" {
		s := pre + post
		col := (kit.Cols - len([]rune(s))) / 2
		if col < 0 {
			col = 0
		}
		fr.Text(row, col, s, st)
		return
	}
	w := len([]rune(pre)) + 2 + len([]rune(post))
	col := (kit.Cols - w) / 2
	if col < 0 {
		col = 0
	}
	col = fr.Text(row, col, pre, st)
	fr.Cells[row][col] = kit.CharacterCell(ch)
	fr.Text(row, col+2, post, st)
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
