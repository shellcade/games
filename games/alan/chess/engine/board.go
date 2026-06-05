// Package engine is a self-contained chess rules engine: board model, FEN,
// legal move generation, and game-end detection. It imports only the standard
// library — no SDK, no canvas, no I/O — so it can be tested in isolation and
// proven correct by perft (see perft_test.go). A separate game layer renders
// and drives it.
package engine

// Color is the side a piece belongs to, and whose turn it is. White is 0 and
// Black is 1, so c^1 flips to the opponent.
type Color uint8

const (
	White Color = 0
	Black Color = 1
)

// PieceType is the kind of piece sitting on a square. Empty is the zero value,
// so a zero Piece is an empty square.
type PieceType uint8

const (
	Empty PieceType = iota
	Pawn
	Knight
	Bishop
	Rook
	Queen
	King
)

// Piece is one occupant of a square. The zero value (Empty, White) is an empty
// square — Color is meaningless when Type is Empty.
type Piece struct {
	Type  PieceType
	Color Color
}

// Square indexes the 64-square mailbox board, 0..63. The file is sq&7 (0=a,
// 7=h) and the rank is sq>>3 (0=rank 1, 7=rank 8), so a1=0 and h8=63. The
// sentinel NoSquare (-1) means "none" (e.g. no en-passant target).
type Square int

const NoSquare Square = -1

// File returns the 0-based file (0=a .. 7=h) of the square. Only meaningful for
// on-board squares.
func (s Square) File() int { return int(s) & 7 }

// Rank returns the 0-based rank (0=rank 1 .. 7=rank 8) of the square.
func (s Square) Rank() int { return int(s) >> 3 }

// SquareAt builds the square for a 0-based file and rank. The caller is
// responsible for keeping both in 0..7.
func SquareAt(file, rank int) Square { return Square(rank*8 + file) }

// onBoard reports whether a file/rank pair lands inside the 8x8 board. Off-board
// detection is by file/rank range (no 0x88 trickery), which keeps the generators
// obviously correct.
func onBoard(file, rank int) bool { return file >= 0 && file < 8 && rank >= 0 && rank < 8 }

// CastleRights is a bit set of the four castling rights still available.
type CastleRights uint8

const (
	WhiteKing CastleRights = 1 << iota
	WhiteQueen
	BlackKing
	BlackQueen
)

// Has reports whether the given right is still present.
func (c CastleRights) Has(r CastleRights) bool { return c&r != 0 }

// Position is a complete chess position: where every piece sits plus the small
// amount of state needed to generate legal moves (side to move, castling rights,
// en-passant target, and the two move clocks). It is copied by value freely —
// Apply returns a fresh Position rather than mutating.
type Position struct {
	Board    [64]Piece    // mailbox; index by Square
	Side     Color        // side to move
	Castle   CastleRights // remaining castling rights
	EP       Square       // en-passant target square (behind a double-pushed pawn), else NoSquare
	HalfMove int          // halfmove clock; resets on pawn move/capture (50-move rule)
	FullMove int          // full move number; starts at 1, increments after Black moves
}

// at returns the piece on a square (helper to keep call sites terse).
func (p *Position) at(s Square) Piece { return p.Board[s] }

// StartPosition returns the standard chess starting position with White to move
// and all castling rights intact.
func StartPosition() Position {
	var p Position
	// Back ranks, mirrored across the two colours.
	back := [8]PieceType{Rook, Knight, Bishop, Queen, King, Bishop, Knight, Rook}
	for file := 0; file < 8; file++ {
		p.Board[SquareAt(file, 0)] = Piece{back[file], White}
		p.Board[SquareAt(file, 1)] = Piece{Pawn, White}
		p.Board[SquareAt(file, 6)] = Piece{Pawn, Black}
		p.Board[SquareAt(file, 7)] = Piece{back[file], Black}
	}
	p.Side = White
	p.Castle = WhiteKing | WhiteQueen | BlackKing | BlackQueen
	p.EP = NoSquare
	p.HalfMove = 0
	p.FullMove = 1
	return p
}
