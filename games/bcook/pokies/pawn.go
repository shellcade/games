package main

// pawn is a player's presence on the lounge floor.
type pawn struct {
	x, y   int
	seated bool
	seat   int // machine id when seated, -1 when roaming
}
