package main

import (
	kit "github.com/shellcade/kit/v2"
)

// Rendering: a centered, clamped viewport over the delver's floor with
// torch-radius sight and explored-memory fog; HUD and message log below.
// One reused frame; cells written directly (allocation-free steady state).

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
			if m.hidden {
				rm.plot(d, m.x, m.y, '%', kit.Style{FG: kit.Gray(0xb8)}, false)
			} else {
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
	if others > 0 {
		fr.Text(22, 10, itoa(others)+" other delver(s) on this floor", stMsg)
	} else {
		fr.Text(22, 10, "alone down here (you think)", stHUDDim)
	}

	// Row 23: the newest message line + hints.
	fr.Text(23, 0, d.msg[1], stMsg)
	fr.TextRight(23, kit.Cols-1, "[hjkl] [>]dn [B]ank [L]oot [R]espect [D]evour [q]uaff", stHUDDim)
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
