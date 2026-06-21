package main

import (
	"fmt"

	kit "github.com/shellcade/kit/v2"
)

// tile rendering palette. Solid floor is bright; each decay stage dims and
// lightens the glyph so a crumbling tile visibly ages under your feet.
var (
	colSolid    = kit.RGB(0x6c, 0x7a, 0x9c) // slate — firm footing
	colCracked  = kit.RGB(0x8a, 0x6a, 0x4a) // browning — stepped off
	colWorn     = kit.RGB(0xb0, 0x55, 0x3a) // hot — about to give way
	colWarn     = kit.RGB(0xff, 0x6b, 0x3a) // worn tile with a hole right below
	colHoleHint = kit.RGB(0x33, 0x2a, 0x3a) // a hole on this layer — the dark below
)

var tileGlyph = [4]rune{'█', '▓', '░', ' '}

// render composes and sends a tailored frame to every connected player, reusing
// one long-lived frame buffer (Send copies immediately, so per-tick rendering is
// allocation-free regardless of player count).
func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		rm.frame.Clear()
		rm.composeFor(rm.frame, p)
		r.Send(p, rm.frame)
	}
}

func (rm *room) composeFor(f *kit.Frame, viewer kit.Player) {
	vp := rm.players[viewer.AccountID]
	layer := 0
	if vp != nil {
		layer = vp.layer
		if layer >= layers {
			layer = layers - 1 // an eliminated viewer watches the bottom floor
		}
	}
	rm.drawFloor(f, layer)
	rm.drawPlayers(f, viewer, layer)
	rm.drawHUD(f, viewer, vp, layer)
}

// drawFloor paints the viewer's current layer. Holes show the dark below; worn
// tiles sitting directly above a hole on the next layer glow as a warning — the
// faint hint of what is underneath.
func (rm *room) drawFloor(f *kit.Frame, layer int) {
	for row := top; row <= bottom; row++ {
		for col := 0; col < arenaW; col++ {
			stage := rm.tileAt(layer, row, col)
			switch stage {
			case tileSolid:
				f.SetRune(row, col, tileGlyph[tileSolid], kit.Style{FG: colSolid})
			case tileCracked:
				f.SetRune(row, col, tileGlyph[tileCracked], kit.Style{FG: colCracked})
			case tileWorn:
				fg := colWorn
				if layer+1 < layers && rm.tileAt(layer+1, row, col) == tileGone {
					fg = colWarn // a hole waits one floor down — telegraph the drop
				}
				f.SetRune(row, col, tileGlyph[tileWorn], kit.Style{FG: fg})
			default: // tileGone — a hole: the dark of the layer below
				f.SetRune(row, col, '·', kit.Style{FG: colHoleHint})
			}
		}
	}
}

// drawPlayers draws every living contestant standing on the viewed layer. The
// viewer's own avatar is reverse-video so they can spot themselves instantly.
func (rm *room) drawPlayers(f *kit.Frame, viewer kit.Player, layer int) {
	for id, pl := range rm.players {
		if !pl.alive || pl.layer != layer {
			continue
		}
		attr := kit.Attr(kit.AttrBold)
		if id == viewer.AccountID {
			attr |= kit.AttrReverse
		}
		f.SetRune(pl.row, pl.col, pl.glyph, kit.Style{FG: pl.color, Attr: attr})
	}
}

func (rm *room) drawHUD(f *kit.Frame, viewer kit.Player, vp *player, layer int) {
	// Top-left: scoreboard in join order, each contestant in their colour, with
	// a small floor-depth marker so you can read who has fallen how far.
	col := 1
	for _, id := range rm.order {
		pl := rm.players[id]
		p := rm.names[id]
		if pl == nil {
			continue
		}
		name := p.Handle
		if len([]rune(name)) > 7 {
			name = string([]rune(name)[:7])
		}
		marker := '●'
		if !pl.alive {
			marker = '○' // hollow once they have fallen out
		}
		col = f.Text(0, col, string(marker)+" ", kit.Style{FG: pl.color, Attr: kit.AttrBold})
		f.Set(0, col, kit.CharacterCell(p.Character)) // character tile + a space before the name
		col += 2
		seg := name
		if id == viewer.AccountID {
			seg += "*"
		}
		col = f.Text(0, col, seg+"  ", kit.Style{FG: pl.color})
		if col > 52 {
			break
		}
	}

	// Top-right: this viewer's depth + survival readout, and the population alive.
	alive := rm.aliveCount()
	if vp != nil {
		secs := rm.runSecs(vp)
		f.TextRight(0, cols-1,
			fmt.Sprintf("FLOOR %d/%d  ALIVE %d  %ds  BEST %ds", layer+1, layers, alive, secs, vp.bestSecs),
			kit.Style{FG: kit.White, Attr: kit.AttrBold})
	} else {
		f.TextRight(0, cols-1, fmt.Sprintf("ALIVE %d", alive), kit.Style{FG: kit.White, Attr: kit.AttrBold})
	}

	// Bottom: controls.
	f.Text(bottom+1, 1, "←/→/↑/↓ (hjkl) move - step off and the floor crumbles    Q quit",
		kit.Style{FG: kit.DimGray})

	rm.drawOverlays(f, viewer, vp)
}

// drawOverlays paints the transient juice: a fall flash when you drop a layer, a
// DROPPED notice once you are out, and the round-over / win banner during the
// intermission.
func (rm *room) drawOverlays(f *kit.Frame, viewer kit.Player, vp *player) {
	// Fall flash: a bright streak across the middle right after dropping a layer.
	if vp != nil && vp.alive && !vp.fellAt.IsZero() && rm.now.Sub(vp.fellAt) < fallFlash {
		flash := "▼ ▼ ▼  YOU FELL A FLOOR  ▼ ▼ ▼"
		f.Text(11, (cols-len([]rune(flash)))/2, flash, kit.Style{FG: kit.Yellow, Attr: kit.AttrBold})
	}

	if vp != nil && !vp.alive && rm.playing {
		f.TextRight(bottom+1, cols-1, fmt.Sprintf("ELIMINATED - survived %ds", vp.lastSecs),
			kit.Style{FG: kit.Red, Attr: kit.AttrBold})
		banner := "✖  YOU DROPPED OUT  ✖"
		f.Text(11, (cols-len([]rune(banner)))/2, banner, kit.Style{FG: kit.Red, Attr: kit.AttrBold})
	}

	if !rm.playing {
		rm.drawIntermission(f, viewer, vp)
	}
}

func (rm *room) drawIntermission(f *kit.Frame, viewer kit.Player, vp *player) {
	var banner string
	var fg kit.Color
	if rm.winnerID != "" {
		w := rm.names[rm.winnerID]
		name := w.Handle
		if rm.winnerID == viewer.AccountID {
			banner = "★  YOU WIN THE ROUND  ★"
			fg = kit.Yellow
		} else {
			banner = fmt.Sprintf("★  %s WINS THE ROUND  ★", name)
			fg = kit.Cyan
		}
	} else {
		// Solo (or everyone fell): show the survival result.
		fg = kit.Cyan
		if vp != nil {
			banner = fmt.Sprintf("CRUMBLE GOT YOU - survived %ds", vp.lastSecs)
		} else {
			banner = "ROUND OVER"
		}
	}
	f.Text(10, (cols-len([]rune(banner)))/2, banner, kit.Style{FG: fg, Attr: kit.AttrBold})

	secs := int(rm.resumeAt.Sub(rm.now).Seconds()) + 1
	if secs < 0 {
		secs = 0
	}
	sub := fmt.Sprintf("next arena in %ds", secs)
	f.Text(12, (cols-len([]rune(sub)))/2, sub, kit.Style{FG: kit.DimGray})
}

func (rm *room) aliveCount() int {
	n := 0
	for _, pl := range rm.players {
		if pl.alive {
			n++
		}
	}
	return n
}

// runSecs is how long the player has survived the current run so far (or their
// final time once they are out).
func (rm *room) runSecs(pl *player) int {
	if !pl.alive {
		return pl.lastSecs
	}
	s := int(rm.now.Sub(pl.roundStart).Seconds())
	if s < 0 {
		s = 0
	}
	return s
}
