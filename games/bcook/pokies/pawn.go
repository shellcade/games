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

// machineAtApproach returns the machine whose approach tile is (x,y), or nil.
func (rm *room) machineAtApproach(x, y int) *floorMachine {
	for i := range rm.fmachines {
		if rm.fmachines[i].ax == x && rm.fmachines[i].ay == y {
			return &rm.fmachines[i]
		}
	}
	return nil
}

// trySit seats the pawn at the machine on its current approach tile, if any and
// unoccupied, binding that machine's variant to the player's session.
func (rm *room) trySit(id string) {
	pw := rm.pawns[id]
	if pw == nil || pw.seated {
		return
	}
	mc := rm.machineAtApproach(pw.x, pw.y)
	if mc == nil {
		return
	}
	if _, taken := rm.occupied[mc.id]; taken {
		return
	}
	pw.seated, pw.seat = true, mc.id
	rm.occupied[mc.id] = id
	if m := rm.machines[id]; m != nil {
		m.seatVar = rm.themes[mc.id]
	}
}

// standUp releases the seat and returns the pawn to the machine's approach tile.
func (rm *room) standUp(id string) {
	pw := rm.pawns[id]
	if pw == nil || !pw.seated {
		return
	}
	if mc := rm.machineByID(pw.seat); mc != nil {
		pw.x, pw.y = mc.ax, mc.ay
	}
	delete(rm.occupied, pw.seat)
	pw.seated, pw.seat = false, -1
}

func (rm *room) machineByID(id int) *floorMachine {
	for i := range rm.fmachines {
		if rm.fmachines[i].id == id {
			return &rm.fmachines[i]
		}
	}
	return nil
}
