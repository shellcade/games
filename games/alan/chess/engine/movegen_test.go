package engine

import (
	"sort"
	"testing"
)

// mustParse parses a FEN and fails the test on error.
func mustParse(t *testing.T, fen string) Position {
	t.Helper()
	p, err := ParseFEN(fen)
	if err != nil {
		t.Fatalf("ParseFEN(%q): %v", fen, err)
	}
	return p
}

// longMoves returns the legal moves of p as a sorted set of long-algebraic
// strings, which is convenient for asserting presence/absence.
func longMoves(p Position) map[string]bool {
	out := map[string]bool{}
	for _, m := range LegalMoves(p) {
		out[p.Long(m)] = true
	}
	return out
}

// movesFrom returns the long-algebraic legal moves whose origin is the given
// square.
func movesFrom(p Position, from Square) []string {
	var out []string
	for _, m := range LegalMoves(p) {
		if m.From == from {
			out = append(out, p.Long(m))
		}
	}
	sort.Strings(out)
	return out
}

func TestCastlingAvailable(t *testing.T) {
	// White king on e1, rooks on a1/h1, all squares clear, full rights.
	p := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	mv := longMoves(p)
	if !mv["O-O"] {
		t.Errorf("expected king-side castle to be legal")
	}
	if !mv["O-O-O"] {
		t.Errorf("expected queen-side castle to be legal")
	}
}

func TestCastlingBlockedByPiece(t *testing.T) {
	// A bishop on f1 blocks king-side; a knight on b1 blocks queen-side.
	p := mustParse(t, "4k3/8/8/8/8/8/8/RN2KB1R w KQ - 0 1")
	mv := longMoves(p)
	if mv["O-O"] {
		t.Errorf("king-side castle should be blocked by the bishop on f1")
	}
	if mv["O-O-O"] {
		t.Errorf("queen-side castle should be blocked by the knight on b1")
	}
}

func TestQueensideRequiresBFileEmpty(t *testing.T) {
	// b1 occupied, c1/d1 empty: queen-side castling is blocked even though the
	// king never travels over b1.
	p := mustParse(t, "4k3/8/8/8/8/8/8/RN2K2R w KQ - 0 1")
	if longMoves(p)["O-O-O"] {
		t.Errorf("queen-side castle should require the b-file square empty")
	}
	// Now clear b1: queen-side becomes legal.
	p2 := mustParse(t, "4k3/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if !longMoves(p2)["O-O-O"] {
		t.Errorf("queen-side castle should be legal with b1 empty")
	}
}

func TestCastlingThroughCheckForbidden(t *testing.T) {
	// A black rook on f8 attacks f1, the square the white king passes through on
	// the king-side castle.
	p := mustParse(t, "4kr2/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if longMoves(p)["O-O"] {
		t.Errorf("king-side castle should be forbidden through the attacked f1")
	}
}

func TestCastlingIntoCheckForbidden(t *testing.T) {
	// A black rook on g8 attacks g1, the king's landing square king-side.
	p := mustParse(t, "4k1r1/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	if longMoves(p)["O-O"] {
		t.Errorf("king-side castle should be forbidden landing on the attacked g1")
	}
}

func TestCastlingWhileInCheckForbidden(t *testing.T) {
	// A black rook on e8 gives check along the e-file; the king may not castle.
	p := mustParse(t, "4r3/8/8/8/8/8/8/R3K2R w KQ - 0 1")
	mv := longMoves(p)
	if mv["O-O"] || mv["O-O-O"] {
		t.Errorf("castling should be forbidden while in check")
	}
}

func TestCastlingRightsLostAfterKingMove(t *testing.T) {
	p := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	// Move the king e1-e2 and back; rights are gone permanently.
	after := Apply(p, Move{From: SquareAt(4, 0), To: SquareAt(4, 1)})
	if after.Castle.Has(WhiteKing) || after.Castle.Has(WhiteQueen) {
		t.Fatalf("white should lose both castling rights after a king move: %v", after.Castle)
	}
	if !after.Castle.Has(BlackKing) || !after.Castle.Has(BlackQueen) {
		t.Fatalf("black should keep its rights: %v", after.Castle)
	}
}

func TestCastlingRightsLostAfterRookMove(t *testing.T) {
	p := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	// Move the h1 rook: white loses only the king-side right.
	after := Apply(p, Move{From: SquareAt(7, 0), To: SquareAt(7, 1)})
	if after.Castle.Has(WhiteKing) {
		t.Errorf("white should lose the king-side right after the h1 rook moves")
	}
	if !after.Castle.Has(WhiteQueen) {
		t.Errorf("white should keep the queen-side right")
	}
}

func TestCastlingRightLostWhenRookCaptured(t *testing.T) {
	// White knight on g6 can capture the black rook on h8, removing Black's
	// king-side right.
	p := mustParse(t, "r3k2r/8/6N1/8/8/8/8/4K3 w kq - 0 1")
	after := Apply(p, Move{From: SquareAt(6, 5), To: SquareAt(7, 7)}) // Ng6xh8
	if after.Castle.Has(BlackKing) {
		t.Errorf("black should lose the king-side right when its h8 rook is captured")
	}
	if !after.Castle.Has(BlackQueen) {
		t.Errorf("black should keep the queen-side right")
	}
}

func TestEnPassantAvailableAfterDoublePush(t *testing.T) {
	// White pawn on e5, black plays ...d7-d5; en passant exd6 is available.
	p := mustParse(t, "4k3/3p4/8/4P3/8/8/8/4K3 b - - 0 1")
	after := Apply(p, Move{From: SquareAt(3, 6), To: SquareAt(3, 4)}) // d7-d5
	if after.EP != SquareAt(3, 5) {
		t.Fatalf("EP target = %d, want d6 (%d)", after.EP, SquareAt(3, 5))
	}
	if !longMoves(after)["e5d6"] {
		t.Fatalf("en passant exd6 should be available, moves: %v", movesFrom(after, SquareAt(4, 4)))
	}
}

func TestEnPassantGoneNextMove(t *testing.T) {
	// Same as above, but White delays a move; the EP window closes.
	p := mustParse(t, "4k3/3p4/8/4P3/8/8/8/4K3 b - - 0 1")
	afterPush := Apply(p, Move{From: SquareAt(3, 6), To: SquareAt(3, 4)}) // d7-d5
	// White plays a king move instead of capturing.
	afterWait := Apply(afterPush, Move{From: SquareAt(4, 0), To: SquareAt(3, 0)}) // Ke1-d1
	if afterWait.EP != NoSquare {
		t.Fatalf("EP should be cleared, got %d", afterWait.EP)
	}
	// Black makes a quiet move; now it's White's turn again and exd6 is gone.
	afterBlack := Apply(afterWait, Move{From: SquareAt(4, 7), To: SquareAt(3, 7)}) // Ke8-d8
	if longMoves(afterBlack)["e5d6"] {
		t.Fatalf("en passant should no longer be available a move later")
	}
}

func TestEnPassantIllegalDiscoveredCheck(t *testing.T) {
	// The rare case where en passant is illegal because it discovers check on the
	// mover's own king. White king a5, white pawn e5, black pawn d5 (just pushed),
	// black rook h5 — all on the 5th rank. Capturing exd6 e.p. removes BOTH the
	// white e5 pawn and the black d5 pawn from rank 5, opening the h5 rook's check
	// on the a5 king, so the move is illegal.
	p := mustParse(t, "8/8/8/K2pP2r/8/8/8/8 w - d6 0 1")
	if longMoves(p)["e5d6"] {
		t.Fatalf("en passant exd6 must be illegal — it discovers check on the white king")
	}
	// Sanity: the EP capture is pseudo-legal (generated) but filtered out.
	found := false
	for _, m := range PseudoLegalMoves(p) {
		if p.Long(m) == "e5d6" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected exd6 to be pseudo-legal before the king-safety filter")
	}
}

func TestNoPhantomEnPassantTarget(t *testing.T) {
	// A double push with NO enemy pawn positioned to capture must not record an
	// en-passant target (strict FIDE / canonical FEN), so transpositions to the
	// same board share a RepetitionKey and threefold can be detected.
	p := StartPosition()
	after := Apply(p, Move{From: SquareAt(4, 1), To: SquareAt(4, 3)}) // 1. e4, no black pawn on d4/f4
	if after.EP != NoSquare {
		t.Fatalf("EP = %d, want NoSquare (no capturer present after 1. e4)", after.EP)
	}
	// The same board reached without a fresh double push must share the key.
	same := mustParse(t, "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1")
	if after.RepetitionKey() != same.RepetitionKey() {
		t.Fatalf("repetition keys differ:\n  %q\n  %q", after.RepetitionKey(), same.RepetitionKey())
	}
}

func TestPromotionEmitsFourMoves(t *testing.T) {
	// White pawn on a7 promotes by pushing to a8.
	p := mustParse(t, "4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	got := movesFrom(p, SquareAt(0, 6))
	want := []string{"a7a8=B", "a7a8=N", "a7a8=Q", "a7a8=R"}
	if len(got) != 4 {
		t.Fatalf("promotion should emit 4 moves, got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("promotion moves = %v, want %v", got, want)
		}
	}
}

func TestCapturePromotionEmitsFourMoves(t *testing.T) {
	// White pawn on b7 can capture the rook on a8 or c8, or push to b8 — each a
	// promotion → 12 moves from b7 total; the capture on a8 yields four.
	p := mustParse(t, "r1r1k3/1P6/8/8/8/8/8/4K3 w - - 0 1")
	var captures []string
	for _, s := range movesFrom(p, SquareAt(1, 6)) {
		if s[0:4] == "b7a8" || s[0:4] == "b7c8" {
			captures = append(captures, s)
		}
	}
	if len(captures) != 8 { // 4 underpromotions per capturable rook
		t.Fatalf("capture-promotions = %v, want 8", captures)
	}
	mv := longMoves(p)
	if !mv["b7a8=N"] || !mv["b7c8=N"] {
		t.Fatalf("underpromotion to knight on a capture should be present")
	}
}

func TestPinnedPieceCannotLeavePinLine(t *testing.T) {
	// White king e1, white bishop e2 pinned by a black rook on e8: the bishop may
	// not leave the e-file.
	p := mustParse(t, "4r3/8/8/8/8/8/4B3/4K3 w - - 0 1")
	for _, s := range movesFrom(p, SquareAt(4, 1)) {
		t.Fatalf("pinned bishop should have no legal move, got %v", s)
	}
	// A rook on the pin line, by contrast, may move along it.
	p2 := mustParse(t, "4r3/8/8/8/8/8/4R3/4K3 w - - 0 1")
	got := movesFrom(p2, SquareAt(4, 1))
	if len(got) == 0 {
		t.Fatalf("rook on the pin line should be able to move along it")
	}
	for _, s := range got {
		// Every move must stay on the e-file or capture the pinning rook on e8.
		if s[2] != 'e' {
			t.Fatalf("pinned rook moved off the e-file: %s", s)
		}
	}
}
