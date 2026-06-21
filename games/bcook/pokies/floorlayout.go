package main

import kit "github.com/shellcade/kit/v2"

var (
	stWall       = kit.Style{FG: kit.DimGray}
	stFloorDecor = kit.Style{FG: kit.DimGray}
	stMachine    = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stSelf       = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stOther      = kit.Style{FG: kit.White}
)

// drawFloor renders the camera window centred on the viewer's pawn: tiles,
// machine icons + labels, and every visible player as their character tile with
// a name label. Avatars are ALWAYS the player's kit.Character (never a generic
// glyph).
func (rm *room) drawFloor(f *kit.Frame, viewer kit.Player) {
	pw := rm.pawns[viewer.AccountID]
	if pw == nil {
		return
	}
	ox, oy := cameraOrigin(pw.x, pw.y)

	for sy := 0; sy < vpH; sy++ {
		for sx := 0; sx < vpW; sx++ {
			mx, my := ox+sx, oy+sy
			switch rm.fmap.at(mx, my) {
			case tileWall:
				f.SetRune(vpTop+sy, sx, '#', stWall)
			case tileEntrance:
				f.SetRune(vpTop+sy, sx, '=', stFloorDecor)
			}
		}
	}

	for _, mc := range rm.fmachines {
		sx, sy := mc.mx-ox, mc.my-oy
		if sx < 0 || sy < 0 || sx >= vpW || sy >= vpH {
			continue
		}
		f.SetRune(vpTop+sy, sx, '+', stMachine)
		lx := mc.mx - ox - len(mc.name)/2
		f.Text(vpTop+sy+1, clampi(lx, 0, vpW-1), mc.name, stMachine)
		if acct, ok := rm.occupied[mc.id]; ok {
			if op, ok := rm.names[acct]; ok && op.Character.Glyph != "" {
				f.Set(vpTop+sy, sx, kit.CharacterCell(op.Character))
			}
		}
	}

	for _, id := range rm.order {
		op := rm.pawns[id]
		if op == nil || op.seated {
			continue
		}
		sx, sy := op.x-ox, op.y-oy
		if sx < 0 || sy < 0 || sx >= vpW || sy >= vpH {
			continue
		}
		st := stOther
		if id == viewer.AccountID {
			st = stSelf
		}
		if pl, ok := rm.names[id]; ok && pl.Character.Glyph != "" {
			f.Set(vpTop+sy, sx, kit.CharacterCell(pl.Character))
		} else {
			f.SetRune(vpTop+sy, sx, '?', st)
		}
		if pl, ok := rm.names[id]; ok {
			nm := pl.Handle
			if len(nm) > 8 {
				nm = nm[:8]
			}
			f.Text(vpTop+sy, clampi(sx+1, 0, vpW-1), nm, st)
		}
	}
}
