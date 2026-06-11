package main

import (
	"fmt"
	"math"

	kit "github.com/shellcade/kit/v2"
)

// shipGlyphs maps an 8-way facing (0=east, clockwise) to a nose arrow.
var shipGlyphs = [8]rune{'→', '↘', '↓', '↙', '←', '↖', '↑', '↗'}

// noseOffset is where the nose sits relative to the hull for each facing.
var noseOffset = [8][2]int{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

func headingSector(heading float64) int {
	twoPi := 2 * math.Pi
	h := math.Mod(heading, twoPi)
	if h < 0 {
		h += twoPi
	}
	return int(math.Round(h/(twoPi/8))) % 8
}

func shipGlyph(heading float64) rune { return shipGlyphs[headingSector(heading)] }

// render composes and sends a tailored frame to every connected pilot, reusing
// one long-lived frame buffer (Send copies immediately, so per-tick rendering
// is allocation-free regardless of player count).
func (rm *room) render(r kit.Room) {
	for _, p := range r.Members() {
		rm.frame.Clear()
		rm.composeFor(rm.frame, p)
		r.Send(p, rm.frame)
	}
}

func (rm *room) composeFor(f *kit.Frame, viewer kit.Player) {
	// Background starfield.
	for _, st := range rm.stars {
		fg := kit.DimGray
		ch := '·'
		if st.bright {
			fg = kit.Gray(0x55)
			ch = '*'
		}
		f.SetRune(st.y, st.x, ch, kit.Style{FG: fg})
	}

	rm.drawCraters(f)
	rm.drawBullets(f)
	rm.drawShips(f, viewer)
	rm.drawExplosions(f)
	rm.drawHUD(f, viewer)
}

func (rm *room) drawCraters(f *kit.Frame) {
	for _, c := range rm.craters {
		cx, cy := roundCell(c.x), roundCell(c.y)
		rad := craterRadius(c.size)
		span := c.size + 1
		for ry := -span; ry <= span; ry++ {
			for rx := -span; rx <= span; rx++ {
				du := float64(rx)
				dv := float64(ry) / aspect
				d := math.Hypot(du, dv)
				if d > rad+0.3 {
					continue
				}
				ch := '#'
				if d > rad-0.7 {
					ch = 'o'
				}
				f.SetRune(wrapRow(cy+ry), wrapCol(cx+rx), ch, kit.Style{FG: craterColor})
			}
		}
	}
}

func (rm *room) drawBullets(f *kit.Frame) {
	for _, b := range rm.bullets {
		f.SetRune(roundCell(b.y), roundCell(b.x), '•', kit.Style{FG: b.color, Attr: kit.AttrBold})
	}
}

func (rm *room) drawShips(f *kit.Frame, viewer kit.Player) {
	blinkOff := rm.now.UnixNano()/int64(150*1e6)%2 == 0
	for id, s := range rm.ships {
		if !s.alive {
			continue
		}
		// Invulnerable ships blink so respawned pilots read as protected.
		attr := kit.Attr(kit.AttrBold)
		if rm.now.Before(s.invulnUntil) {
			if blinkOff {
				continue
			}
			attr = kit.AttrDim
		}
		// The viewer's own ship is reverse-video so you can find yourself at a
		// glance while keeping your team color.
		if id == viewer.AccountID {
			attr |= kit.AttrReverse
		}
		// A two-cell craft: a hull with a directional nose, so the ship is big
		// enough to see and to hit (the nose marks where shots come from).
		st := kit.Style{FG: s.color, Attr: attr}
		sec := headingSector(s.heading)
		hr, hc := roundCell(s.y), roundCell(s.x)
		f.SetRune(hr, hc, '◆', st)
		f.SetRune(wrapRow(hr+noseOffset[sec][1]), wrapCol(hc+noseOffset[sec][0]), shipGlyphs[sec], st)
	}
}

func (rm *room) drawExplosions(f *kit.Frame) {
	for _, e := range rm.booms {
		frac := rm.now.Sub(e.start).Seconds() / explodeDur.Seconds()
		if frac < 0 || frac > 1 {
			continue
		}
		rad := 0.5 + frac*4
		fg := kit.Yellow
		switch {
		case frac > 0.66:
			fg = kit.Red
		case frac > 0.33:
			fg = kit.RGB(0xff, 0xa5, 0x33)
		}
		cx, cy := roundCell(e.x), roundCell(e.y)
		span := int(rad) + 1
		for ry := -span; ry <= span; ry++ {
			for rx := -span - 1; rx <= span+1; rx++ {
				d := math.Hypot(float64(rx), float64(ry)/aspect)
				if math.Abs(d-rad) < 0.8 {
					f.SetRune(wrapRow(cy+ry), wrapCol(cx+rx), '*', kit.Style{FG: fg, Attr: kit.AttrBold})
				}
			}
		}
	}
}

func (rm *room) drawHUD(f *kit.Frame, viewer kit.Player) {
	// Top: scoreboard in join order, each pilot in their color.
	col := 1
	for _, id := range rm.order {
		s := rm.ships[id]
		p := rm.names[id]
		if s == nil {
			continue
		}
		name := p.Handle
		if len([]rune(name)) > 8 {
			name = string([]rune(name)[:8])
		}
		col = f.Text(0, col, "● ", kit.Style{FG: s.color, Attr: kit.AttrBold})
		f.Set(0, col, kit.CharacterCell(p.Character)) // character tile + a space before the name
		col += 2
		seg := fmt.Sprintf("%s %d", name, s.kills)
		if id == viewer.AccountID {
			seg += "*"
		}
		col = f.Text(0, col, seg+"  ", kit.Style{FG: s.color})
		// 56, not 58: each segment grew two columns (tile + space), so the
		// break compensates to keep the strip clear of the right-aligned
		// K/D/BEST readout exactly as before.
		if col > 56 {
			break
		}
	}
	if vs := rm.ships[viewer.AccountID]; vs != nil {
		f.TextRight(0, cols-1, fmt.Sprintf("K %d  D %d  BEST %d", vs.kills, vs.deaths, vs.best),
			kit.Style{FG: kit.White, Attr: kit.AttrBold})
	}

	// Bottom: controls + status.
	f.Text(bottom+1, 1, "←/→ turn  ↑ thrust  ↓ brake  SPACE fire  Q quit",
		kit.Style{FG: kit.DimGray})

	if vs := rm.ships[viewer.AccountID]; vs != nil && !vs.alive {
		secs := int(vs.respawnAt.Sub(rm.now).Seconds()) + 1
		if secs < 0 {
			secs = 0
		}
		f.TextRight(bottom+1, cols-1, fmt.Sprintf("DESTROYED — respawn in %ds", secs),
			kit.Style{FG: kit.Red, Attr: kit.AttrBold})
		banner := "✦  YOU WERE DESTROYED  ✦"
		f.Text(11, (cols-len([]rune(banner)))/2, banner, kit.Style{FG: kit.Red, Attr: kit.AttrBold})
	}
}
