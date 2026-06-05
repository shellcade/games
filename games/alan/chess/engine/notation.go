package engine

// Long renders a move in long-algebraic notation against this position:
//
//	"e2e4"      a quiet move or push
//	"e4d5"      a capture (same form — the destination is implied)
//	"O-O"       king-side castle
//	"O-O-O"     queen-side castle
//	"e7e8=Q"    promotion (uppercase promotion letter)
//
// SAN is intentionally out of scope; this is what the move list displays.
func (p Position) Long(m Move) string {
	mover := p.Board[m.From]

	// Castling is the king moving two files; render it with the O-O forms.
	if mover.Type == King && abs(m.To.File()-m.From.File()) == 2 {
		if m.To.File() == 6 {
			return "O-O"
		}
		return "O-O-O"
	}

	s := squareName(m.From) + squareName(m.To)
	if m.Promo != Empty {
		s += "=" + string(rune(pieceLetter[m.Promo]))
	}
	return s
}
