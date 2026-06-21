package main

// pawn is a player's presence on the lounge floor.
type pawn struct {
	x, y   int
	seated bool
	seat   int // machine id when seated, -1 when roaming
}

// pawnAt reports whether any roaming pawn (other than `self`) occupies (x,y).
func (rm *room) pawnAt(x, y int, self string) bool {
	for id, pw := range rm.pawns {
		if id == self || pw.seated {
			continue
		}
		if pw.x == x && pw.y == y {
			return true
		}
	}
	return false
}

// tryMove steps the pawn by (dx,dy) when the target tile is walkable and
// unoccupied; blocked moves are no-ops. Seated pawns do not move.
func (rm *room) tryMove(id string, dx, dy int) {
	pw := rm.pawns[id]
	if pw == nil || pw.seated {
		return
	}
	nx, ny := pw.x+dx, pw.y+dy
	if !rm.fmap.walkable(nx, ny) || rm.pawnAt(nx, ny, id) {
		return
	}
	pw.x, pw.y = nx, ny
}
