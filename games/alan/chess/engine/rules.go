package engine

// HasLegalMove reports whether the side to move has at least one legal move. It
// short-circuits on the first legal move, so it is cheaper than len(LegalMoves)
// for the common "is this the end of the game?" question. No legal move means
// checkmate (if InCheck) or stalemate (if not).
func HasLegalMove(p Position) bool {
	us := p.Side
	for _, m := range PseudoLegalMoves(p) {
		if !InCheck(Apply(p, m), us) {
			return true
		}
	}
	return false
}

// InsufficientMaterial reports whether neither side has enough material to force
// checkmate, which is an automatic draw. The recognised dead positions are:
//
//   - King vs King.
//   - King vs King + a single minor piece (bishop or knight).
//   - King + Bishop vs King + Bishop with both bishops on the same square colour.
//
// Any pawn, rook, or queen — or two minor pieces on one side — can in principle
// mate, so those return false.
func InsufficientMaterial(p Position) bool {
	// Count minor pieces and track bishop square colours; any major piece or pawn
	// is immediately sufficient.
	var knights, bishops int
	var bishopSquareColors [2]bool // which square colours hold a bishop

	for s := Square(0); s < 64; s++ {
		pc := p.Board[s]
		switch pc.Type {
		case Empty, King:
			// Kings don't count toward material.
		case Pawn, Rook, Queen:
			return false
		case Knight:
			knights++
		case Bishop:
			bishops++
			// Square colour: (file+rank) parity. 0 = one colour, 1 = the other.
			bishopSquareColors[(s.File()+s.Rank())&1] = true
		}
	}

	minors := knights + bishops
	switch {
	case minors == 0:
		// K vs K.
		return true
	case minors == 1:
		// K vs K + single bishop or knight.
		return true
	case knights == 0 && bishops == 2:
		// K+B vs K+B is a draw only when both bishops sit on the same square
		// colour, i.e. exactly one of the two parity slots is occupied.
		return bishopSquareColors[0] != bishopSquareColors[1]
	}
	return false
}
