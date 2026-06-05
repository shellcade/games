package engine

import "testing"

func TestCheckmate(t *testing.T) {
	// Fool's mate: 1.f3 e5 2.g4 Qh4#. White is to move, in check, with no legal
	// move → checkmate (White loses).
	p := mustParse(t, "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	if !InCheck(p, p.Side) {
		t.Fatalf("expected the side to move to be in check")
	}
	if HasLegalMove(p) {
		t.Fatalf("expected no legal move (checkmate)")
	}
	if len(LegalMoves(p)) != 0 {
		t.Fatalf("LegalMoves should be empty in checkmate, got %v", LegalMoves(p))
	}
}

func TestStalemate(t *testing.T) {
	// Black king h8, White queen f7, White king g6: Black to move, not in check,
	// but every king move is into check → stalemate (draw).
	p := mustParse(t, "7k/5Q2/6K1/8/8/8/8/8 b - - 0 1")
	if InCheck(p, p.Side) {
		t.Fatalf("stalemate position should not be in check")
	}
	if HasLegalMove(p) {
		t.Fatalf("expected no legal move (stalemate)")
	}
	if len(LegalMoves(p)) != 0 {
		t.Fatalf("LegalMoves should be empty in stalemate, got %v", LegalMoves(p))
	}
}

func TestHasLegalMoveNormalPosition(t *testing.T) {
	if !HasLegalMove(StartPosition()) {
		t.Fatalf("the start position obviously has legal moves")
	}
}

func TestInsufficientMaterial(t *testing.T) {
	cases := []struct {
		name string
		fen  string
		want bool
	}{
		{"king vs king", "8/8/4k3/8/8/3K4/8/8 w - - 0 1", true},
		{"king vs king+bishop", "8/8/4k3/8/8/3K4/5B2/8 w - - 0 1", true},
		{"king vs king+knight", "8/8/4k3/8/8/3K4/5N2/8 w - - 0 1", true},
		// Both bishops on the same square colour: c1 (parity 0) and a3 (parity 0).
		{"KB vs KB same colour", "7k/8/8/8/8/b7/8/K1B5 w - - 0 1", true},
		// Bishops on opposite colours: c1 (parity 0) and a4 (parity 1) → can mate.
		{"KB vs KB opposite colour", "7k/8/8/8/b7/8/8/K1B5 w - - 0 1", false},
		// A lone pawn is sufficient (it can promote).
		{"king vs king+pawn", "8/8/4k3/8/8/3K4/5P2/8 w - - 0 1", false},
		{"rook is sufficient", "8/8/4k3/8/8/3K4/5R2/8 w - - 0 1", false},
		{"two knights one side", "8/8/4k3/8/8/3K4/4NN2/8 w - - 0 1", false},
		{"start position", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mustParse(t, c.fen)
			if got := InsufficientMaterial(p); got != c.want {
				t.Fatalf("InsufficientMaterial(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestFiftyMoveBoundary(t *testing.T) {
	// The fifty-move draw is claimed at HalfMove >= 100 (100 plies). The Handler
	// reads HalfMove; verify the counter increments on quiet moves and resets on a
	// pawn move or capture so a boundary check is meaningful.
	p := mustParse(t, "4k3/8/8/8/8/8/8/4K3 w - - 99 60")
	if p.HalfMove < 100 {
		// A quiet king move pushes it to the boundary.
		p = Apply(p, Move{From: SquareAt(4, 0), To: SquareAt(3, 0)}) // Ke1-d1
	}
	if p.HalfMove < 100 {
		t.Fatalf("HalfMove = %d, expected >= 100 after a quiet move at 99", p.HalfMove)
	}

	// A pawn move resets the clock.
	q := mustParse(t, "4k3/8/8/8/8/8/4P3/4K3 w - - 80 60")
	q = Apply(q, Move{From: SquareAt(4, 1), To: SquareAt(4, 3)}) // e2-e4
	if q.HalfMove != 0 {
		t.Fatalf("HalfMove should reset to 0 on a pawn move, got %d", q.HalfMove)
	}

	// A capture resets the clock too.
	r := mustParse(t, "4k3/8/8/8/3r4/3R4/8/4K3 w - - 75 60")
	r = Apply(r, Move{From: SquareAt(3, 2), To: SquareAt(3, 3)}) // Rd3xd4
	if r.HalfMove != 0 {
		t.Fatalf("HalfMove should reset to 0 on a capture, got %d", r.HalfMove)
	}
}
