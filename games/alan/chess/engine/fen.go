package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// pieceLetter maps a piece type to its FEN/SAN letter (uppercase). White pieces
// use the uppercase letter, black the lowercase form.
var pieceLetter = map[PieceType]byte{
	Pawn: 'P', Knight: 'N', Bishop: 'B', Rook: 'R', Queen: 'Q', King: 'K',
}

// letterPiece is the inverse of pieceLetter (uppercase letters only).
var letterPiece = map[byte]PieceType{
	'P': Pawn, 'N': Knight, 'B': Bishop, 'R': Rook, 'Q': Queen, 'K': King,
}

// ParseFEN parses a FEN record into a Position. It accepts the standard six
// fields; the two move-counter fields are optional and default to 0/1 so the
// four-field repetition keys produced by RepetitionKey also parse.
func ParseFEN(fen string) (Position, error) {
	fields := strings.Fields(fen)
	if len(fields) < 4 {
		return Position{}, fmt.Errorf("fen: need at least 4 fields, got %d", len(fields))
	}

	var p Position
	p.EP = NoSquare

	// Field 1: piece placement, rank 8 down to rank 1, files a..h within a rank.
	ranks := strings.Split(fields[0], "/")
	if len(ranks) != 8 {
		return Position{}, fmt.Errorf("fen: placement needs 8 ranks, got %d", len(ranks))
	}
	for i, row := range ranks {
		rank := 7 - i // first row is rank 8 (rank index 7)
		file := 0
		for j := 0; j < len(row); j++ {
			ch := row[j]
			switch {
			case ch >= '1' && ch <= '8':
				file += int(ch - '0')
			default:
				color := White
				up := ch
				if ch >= 'a' && ch <= 'z' {
					color = Black
					up = ch - ('a' - 'A')
				}
				pt, ok := letterPiece[up]
				if !ok {
					return Position{}, fmt.Errorf("fen: bad piece %q", string(ch))
				}
				if file > 7 {
					return Position{}, fmt.Errorf("fen: too many squares in rank %d", rank+1)
				}
				p.Board[SquareAt(file, rank)] = Piece{pt, color}
				file++
			}
		}
		if file != 8 {
			return Position{}, fmt.Errorf("fen: rank %d describes %d files", rank+1, file)
		}
	}

	// Field 2: side to move.
	switch fields[1] {
	case "w":
		p.Side = White
	case "b":
		p.Side = Black
	default:
		return Position{}, fmt.Errorf("fen: bad side %q", fields[1])
	}

	// Field 3: castling rights ("-" for none).
	if fields[2] != "-" {
		for i := 0; i < len(fields[2]); i++ {
			switch fields[2][i] {
			case 'K':
				p.Castle |= WhiteKing
			case 'Q':
				p.Castle |= WhiteQueen
			case 'k':
				p.Castle |= BlackKing
			case 'q':
				p.Castle |= BlackQueen
			default:
				return Position{}, fmt.Errorf("fen: bad castling %q", fields[2])
			}
		}
	}

	// Field 4: en-passant target ("-" for none).
	if fields[3] != "-" {
		sq, err := parseSquare(fields[3])
		if err != nil {
			return Position{}, fmt.Errorf("fen: bad en passant %q", fields[3])
		}
		p.EP = sq
	}

	// Fields 5/6: halfmove clock and fullmove number (optional).
	p.FullMove = 1
	if len(fields) >= 5 {
		hm, err := strconv.Atoi(fields[4])
		if err != nil {
			return Position{}, fmt.Errorf("fen: bad halfmove %q", fields[4])
		}
		p.HalfMove = hm
	}
	if len(fields) >= 6 {
		fm, err := strconv.Atoi(fields[5])
		if err != nil {
			return Position{}, fmt.Errorf("fen: bad fullmove %q", fields[5])
		}
		p.FullMove = fm
	}

	return p, nil
}

// parseSquare reads an algebraic square like "e4" into a Square.
func parseSquare(s string) (Square, error) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return NoSquare, fmt.Errorf("bad square %q", s)
	}
	return SquareAt(int(s[0]-'a'), int(s[1]-'1')), nil
}

// squareName renders a square in algebraic form ("e4").
func squareName(s Square) string {
	return string(rune('a'+s.File())) + string(rune('1'+s.Rank()))
}

// FEN renders the position as a full six-field FEN record. ParseFEN(p.FEN())
// round-trips exactly.
func (p Position) FEN() string {
	var b strings.Builder
	b.WriteString(p.placement())
	b.WriteByte(' ')
	if p.Side == White {
		b.WriteByte('w')
	} else {
		b.WriteByte('b')
	}
	b.WriteByte(' ')
	b.WriteString(p.castleField())
	b.WriteByte(' ')
	if p.EP == NoSquare {
		b.WriteByte('-')
	} else {
		b.WriteString(squareName(p.EP))
	}
	fmt.Fprintf(&b, " %d %d", p.HalfMove, p.FullMove)
	return b.String()
}

// RepetitionKey returns the first four FEN fields (placement, side, castling,
// en passant) and omits the move counters. Two positions with the same key are
// identical for threefold-repetition purposes.
func (p Position) RepetitionKey() string {
	side := "w"
	if p.Side == Black {
		side = "b"
	}
	ep := "-"
	if p.EP != NoSquare {
		ep = squareName(p.EP)
	}
	return p.placement() + " " + side + " " + p.castleField() + " " + ep
}

// placement renders the piece-placement field of FEN (rank 8 down to rank 1).
func (p Position) placement() string {
	var b strings.Builder
	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			pc := p.Board[SquareAt(file, rank)]
			if pc.Type == Empty {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteByte(byte('0' + empty))
				empty = 0
			}
			letter := pieceLetter[pc.Type]
			if pc.Color == Black {
				letter += 'a' - 'A'
			}
			b.WriteByte(letter)
		}
		if empty > 0 {
			b.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			b.WriteByte('/')
		}
	}
	return b.String()
}

// castleField renders the castling-availability field ("-" if none).
func (p Position) castleField() string {
	if p.Castle == 0 {
		return "-"
	}
	var b strings.Builder
	if p.Castle.Has(WhiteKing) {
		b.WriteByte('K')
	}
	if p.Castle.Has(WhiteQueen) {
		b.WriteByte('Q')
	}
	if p.Castle.Has(BlackKing) {
		b.WriteByte('k')
	}
	if p.Castle.Has(BlackQueen) {
		b.WriteByte('q')
	}
	return b.String()
}
