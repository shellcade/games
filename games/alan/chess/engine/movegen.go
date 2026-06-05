package engine

// Move is a single ply: the from/to squares plus an optional promotion piece.
// Castling is encoded as the king moving two squares (e1->g1 / e1->c1, etc.);
// en passant as the pawn moving diagonally onto the empty EP square. Both are
// recognised by Apply from the piece and the geometry, so Move carries no flags.
// Promo is Empty except for a promotion, where it is one of Queen/Rook/Bishop/
// Knight.
type Move struct {
	From, To Square
	Promo    PieceType
}

// Direction offsets expressed as (file delta, rank delta) so off-board steps are
// caught by range-checking the resulting file/rank — never by index wraparound.
var (
	knightDeltas = [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingDeltas   = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	bishopDeltas = [4][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	rookDeltas   = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
)

// Attacked reports whether square sq is attacked by any piece of side by. It is
// the basis for check detection and the castling through-check test.
func Attacked(p Position, sq Square, by Color) bool {
	f, r := sq.File(), sq.Rank()

	// Pawns: a pawn of side by attacks diagonally forward. White pawns move up
	// (toward rank 8), so they attack the squares one rank below them; thus sq is
	// attacked by a white pawn sitting one rank below and one file to either side.
	pawnRankDelta := -1 // where the attacking white pawn sits relative to sq
	if by == Black {
		pawnRankDelta = 1 // a black pawn sits one rank above sq
	}
	for _, df := range [2]int{-1, 1} {
		af, ar := f+df, r+pawnRankDelta
		if onBoard(af, ar) {
			pc := p.Board[SquareAt(af, ar)]
			if pc.Type == Pawn && pc.Color == by {
				return true
			}
		}
	}

	// Knights.
	for _, d := range knightDeltas {
		af, ar := f+d[0], r+d[1]
		if onBoard(af, ar) {
			pc := p.Board[SquareAt(af, ar)]
			if pc.Type == Knight && pc.Color == by {
				return true
			}
		}
	}

	// King (adjacent).
	for _, d := range kingDeltas {
		af, ar := f+d[0], r+d[1]
		if onBoard(af, ar) {
			pc := p.Board[SquareAt(af, ar)]
			if pc.Type == King && pc.Color == by {
				return true
			}
		}
	}

	// Sliding rays. Bishop/queen along diagonals, rook/queen along files/ranks.
	if slideAttack(p, f, r, bishopDeltas[:], by, Bishop) {
		return true
	}
	if slideAttack(p, f, r, rookDeltas[:], by, Rook) {
		return true
	}
	return false
}

// slideAttack walks each direction until it leaves the board or hits a piece. A
// blocking piece of side by attacks sq if it is the matching slider (or a queen,
// which moves like both rook and bishop).
func slideAttack(p Position, f, r int, deltas [][2]int, by Color, slider PieceType) bool {
	for _, d := range deltas {
		af, ar := f+d[0], r+d[1]
		for onBoard(af, ar) {
			pc := p.Board[SquareAt(af, ar)]
			if pc.Type != Empty {
				if pc.Color == by && (pc.Type == slider || pc.Type == Queen) {
					return true
				}
				break // any other piece blocks the ray
			}
			af += d[0]
			ar += d[1]
		}
	}
	return false
}

// kingSquare finds the square of side c's king, or NoSquare if absent (only the
// case in malformed test positions).
func kingSquare(p Position, c Color) Square {
	for s := Square(0); s < 64; s++ {
		pc := p.Board[s]
		if pc.Type == King && pc.Color == c {
			return s
		}
	}
	return NoSquare
}

// InCheck reports whether side c's king is currently attacked by the opponent.
func InCheck(p Position, c Color) bool {
	ks := kingSquare(p, c)
	if ks == NoSquare {
		return false
	}
	return Attacked(p, ks, c^1)
}

// PseudoLegalMoves generates every move for the side to move ignoring whether
// the move leaves the mover's own king in check (that filter is LegalMoves). It
// includes double pawn pushes, en passant, castling, and promotions (four moves
// Q/R/B/N).
func PseudoLegalMoves(p Position) []Move {
	moves := make([]Move, 0, 48)
	us := p.Side
	for s := Square(0); s < 64; s++ {
		pc := p.Board[s]
		if pc.Type == Empty || pc.Color != us {
			continue
		}
		switch pc.Type {
		case Pawn:
			moves = pawnMoves(p, s, moves)
		case Knight:
			moves = stepMoves(p, s, knightDeltas[:], moves)
		case King:
			moves = stepMoves(p, s, kingDeltas[:], moves)
			moves = castleMoves(p, s, moves)
		case Bishop:
			moves = slideMoves(p, s, bishopDeltas[:], moves)
		case Rook:
			moves = slideMoves(p, s, rookDeltas[:], moves)
		case Queen:
			moves = slideMoves(p, s, bishopDeltas[:], moves)
			moves = slideMoves(p, s, rookDeltas[:], moves)
		}
	}
	return moves
}

// stepMoves appends single-step moves (knight/king) to empty or enemy squares.
func stepMoves(p Position, from Square, deltas [][2]int, moves []Move) []Move {
	us := p.Board[from].Color
	f, r := from.File(), from.Rank()
	for _, d := range deltas {
		af, ar := f+d[0], r+d[1]
		if !onBoard(af, ar) {
			continue
		}
		to := SquareAt(af, ar)
		dst := p.Board[to]
		if dst.Type == Empty || dst.Color != us {
			moves = append(moves, Move{From: from, To: to})
		}
	}
	return moves
}

// slideMoves appends sliding moves along the given rays until blocked.
func slideMoves(p Position, from Square, deltas [][2]int, moves []Move) []Move {
	us := p.Board[from].Color
	f, r := from.File(), from.Rank()
	for _, d := range deltas {
		af, ar := f+d[0], r+d[1]
		for onBoard(af, ar) {
			to := SquareAt(af, ar)
			dst := p.Board[to]
			if dst.Type == Empty {
				moves = append(moves, Move{From: from, To: to})
			} else {
				if dst.Color != us {
					moves = append(moves, Move{From: from, To: to})
				}
				break
			}
			af += d[0]
			ar += d[1]
		}
	}
	return moves
}

// pawnMoves appends pawn pushes, double pushes, captures, en passant, and
// promotions for the pawn on from.
func pawnMoves(p Position, from Square, moves []Move) []Move {
	us := p.Board[from].Color
	f, r := from.File(), from.Rank()

	fwd := 1        // rank delta for a forward push
	startRank := 1  // rank from which a double push is allowed
	promoRank := 7  // rank reached on promotion
	if us == Black {
		fwd, startRank, promoRank = -1, 6, 0
	}

	// Single push onto an empty square.
	if onBoard(f, r+fwd) {
		one := SquareAt(f, r+fwd)
		if p.Board[one].Type == Empty {
			moves = addPawnMove(moves, from, one, r+fwd == promoRank)
			// Double push from the start rank, both squares empty.
			if r == startRank {
				two := SquareAt(f, r+2*fwd)
				if p.Board[two].Type == Empty {
					moves = append(moves, Move{From: from, To: two})
				}
			}
		}
	}

	// Diagonal captures, including en passant onto the EP target square.
	for _, df := range [2]int{-1, 1} {
		af, ar := f+df, r+fwd
		if !onBoard(af, ar) {
			continue
		}
		to := SquareAt(af, ar)
		dst := p.Board[to]
		if dst.Type != Empty && dst.Color != us {
			moves = addPawnMove(moves, from, to, ar == promoRank)
		} else if to == p.EP && dst.Type == Empty {
			// En passant: the target square is empty; the captured pawn sits
			// beside us (handled in Apply).
			moves = append(moves, Move{From: from, To: to})
		}
	}
	return moves
}

// addPawnMove appends a pawn move, expanding to four promotion moves when the
// pawn reaches the last rank.
func addPawnMove(moves []Move, from, to Square, promotion bool) []Move {
	if !promotion {
		return append(moves, Move{From: from, To: to})
	}
	for _, pt := range [4]PieceType{Queen, Rook, Bishop, Knight} {
		moves = append(moves, Move{From: from, To: to, Promo: pt})
	}
	return moves
}

// castleMoves appends any legal castling moves for the king on from. The full
// castling legality is checked here (rights present, squares empty, king not in
// check and not passing through or landing on an attacked square), so LegalMoves
// can keep these without re-filtering.
func castleMoves(p Position, from Square, moves []Move) []Move {
	us := p.Board[from].Color
	them := us ^ 1
	var rank int
	var kingSide, queenSide CastleRights
	if us == White {
		rank, kingSide, queenSide = 0, WhiteKing, WhiteQueen
	} else {
		rank, kingSide, queenSide = 7, BlackKing, BlackQueen
	}
	// The king must be on its home square for castling to be possible.
	if from != SquareAt(4, rank) {
		return moves
	}
	// The king may not castle out of check.
	if Attacked(p, from, them) {
		return moves
	}

	// King-side: f and g empty; king passes through f and lands on g, neither
	// attacked.
	if p.Castle.Has(kingSide) &&
		p.Board[SquareAt(5, rank)].Type == Empty &&
		p.Board[SquareAt(6, rank)].Type == Empty &&
		!Attacked(p, SquareAt(5, rank), them) &&
		!Attacked(p, SquareAt(6, rank), them) {
		moves = append(moves, Move{From: from, To: SquareAt(6, rank)})
	}

	// Queen-side: b, c, d empty (b only needs to be empty, not safe); king passes
	// through d and lands on c, neither attacked.
	if p.Castle.Has(queenSide) &&
		p.Board[SquareAt(1, rank)].Type == Empty &&
		p.Board[SquareAt(2, rank)].Type == Empty &&
		p.Board[SquareAt(3, rank)].Type == Empty &&
		!Attacked(p, SquareAt(3, rank), them) &&
		!Attacked(p, SquareAt(2, rank), them) {
		moves = append(moves, Move{From: from, To: SquareAt(2, rank)})
	}
	return moves
}

// Apply returns the position reached by playing m. It is pure: p is unchanged.
// The move is assumed pseudo-legal. Apply handles the special cases (castle rook
// hop, en-passant capture removal, promotion) and updates side to move, castling
// rights, the en-passant target, and both clocks.
func Apply(p Position, m Move) Position {
	np := p // copy by value
	mover := np.Board[m.From]
	captured := np.Board[m.To]
	us := mover.Color

	// Reset the en-passant target by default; set it only on a double push below.
	np.EP = NoSquare

	isPawn := mover.Type == Pawn
	isCapture := captured.Type != Empty

	// En passant: a pawn moving diagonally onto an empty square captures the pawn
	// that just double-pushed, which sits beside the from-square on the to-file.
	if isPawn && m.To == p.EP && captured.Type == Empty && m.From.File() != m.To.File() {
		capSq := SquareAt(m.To.File(), m.From.Rank())
		np.Board[capSq] = Piece{}
		isCapture = true
	}

	// Move the piece.
	np.Board[m.From] = Piece{}
	if m.Promo != Empty {
		np.Board[m.To] = Piece{Type: m.Promo, Color: us}
	} else {
		np.Board[m.To] = mover
	}

	// Double pawn push: set the EP target to the skipped square — but only when an
	// enemy pawn is actually positioned to capture en passant next move. Setting a
	// "phantom" EP square with no capturer is legal-move-equivalent (no EP capture
	// is ever generated without an adjacent enemy pawn) but it perturbs FEN /
	// RepetitionKey, which would split otherwise-identical positions and mask a
	// threefold draw. This matches strict FIDE and canonical FEN.
	if isPawn && abs(m.To.Rank()-m.From.Rank()) == 2 && epHasCapturer(np, m.To, us) {
		np.EP = SquareAt(m.From.File(), (m.From.Rank()+m.To.Rank())/2)
	}

	// Castling: the king moved two files, so hop the matching rook.
	if mover.Type == King && abs(m.To.File()-m.From.File()) == 2 {
		rank := m.From.Rank()
		if m.To.File() == 6 { // king-side: rook h -> f
			np.Board[SquareAt(5, rank)] = np.Board[SquareAt(7, rank)]
			np.Board[SquareAt(7, rank)] = Piece{}
		} else { // queen-side: rook a -> d
			np.Board[SquareAt(3, rank)] = np.Board[SquareAt(0, rank)]
			np.Board[SquareAt(0, rank)] = Piece{}
		}
	}

	// Update castling rights: clear when the king moves, when a rook leaves its
	// home square, or when a rook is captured on its home square.
	np.Castle = updateCastle(np.Castle, mover, m.From, m.To, captured)

	// Clocks. Halfmove resets on a pawn move or any capture, else increments.
	if isPawn || isCapture {
		np.HalfMove = 0
	} else {
		np.HalfMove = p.HalfMove + 1
	}
	if us == Black {
		np.FullMove = p.FullMove + 1
	}

	np.Side = us ^ 1
	return np
}

// updateCastle clears castling rights affected by a move: the moving king or
// rook losing its right, and a rook captured on its home square losing the
// opponent's right.
func updateCastle(c CastleRights, mover Piece, from, to Square, captured Piece) CastleRights {
	clear := func(r CastleRights) { c &^= r }

	if mover.Type == King {
		if mover.Color == White {
			clear(WhiteKing | WhiteQueen)
		} else {
			clear(BlackKing | BlackQueen)
		}
	}
	// A rook leaving (or moving from) its home square forfeits that right.
	switch from {
	case SquareAt(0, 0):
		clear(WhiteQueen)
	case SquareAt(7, 0):
		clear(WhiteKing)
	case SquareAt(0, 7):
		clear(BlackQueen)
	case SquareAt(7, 7):
		clear(BlackKing)
	}
	// A rook captured on its home square forfeits the owner's right.
	if captured.Type == Rook {
		switch to {
		case SquareAt(0, 0):
			clear(WhiteQueen)
		case SquareAt(7, 0):
			clear(WhiteKing)
		case SquareAt(0, 7):
			clear(BlackQueen)
		case SquareAt(7, 7):
			clear(BlackKing)
		}
	}
	return c
}

// epHasCapturer reports whether, immediately after a double pawn push that landed
// a pawn on pawnSq, an enemy pawn sits beside it (same rank, adjacent file) able
// to capture en passant. Used to avoid recording a phantom en-passant target.
func epHasCapturer(p Position, pawnSq Square, us Color) bool {
	them := us ^ 1
	r := pawnSq.Rank()
	for _, df := range [2]int{-1, 1} {
		f := pawnSq.File() + df
		if onBoard(f, r) {
			pc := p.Board[SquareAt(f, r)]
			if pc.Type == Pawn && pc.Color == them {
				return true
			}
		}
	}
	return false
}

// LegalMoves returns the pseudo-legal moves filtered to those that do not leave
// the mover's own king in check.
func LegalMoves(p Position) []Move {
	pseudo := PseudoLegalMoves(p)
	legal := make([]Move, 0, len(pseudo))
	us := p.Side
	for _, m := range pseudo {
		if !InCheck(Apply(p, m), us) {
			legal = append(legal, m)
		}
	}
	return legal
}

// abs returns the absolute value of an int.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
