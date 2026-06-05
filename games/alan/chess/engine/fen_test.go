package engine

import "testing"

// allFENs are the start position plus the six perft reference positions — a good
// spread of placements, castling rights, en-passant targets, and clocks.
var allFENs = []string{
	"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
	"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
	"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
	"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
	"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
	"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
}

func TestFENRoundTrip(t *testing.T) {
	for _, fen := range allFENs {
		p, err := ParseFEN(fen)
		if err != nil {
			t.Fatalf("ParseFEN(%q): %v", fen, err)
		}
		if got := p.FEN(); got != fen {
			t.Fatalf("FEN round-trip:\n got %q\nwant %q", got, fen)
		}
	}
}

func TestStartPositionFEN(t *testing.T) {
	want := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	if got := StartPosition().FEN(); got != want {
		t.Fatalf("StartPosition().FEN() = %q, want %q", got, want)
	}
}

func TestParseFENWithEnPassant(t *testing.T) {
	// After 1.e4, the EP target is e3.
	p, err := ParseFEN("rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1")
	if err != nil {
		t.Fatalf("ParseFEN: %v", err)
	}
	if p.EP != SquareAt(4, 2) { // e3
		t.Fatalf("EP = %d, want e3 (%d)", p.EP, SquareAt(4, 2))
	}
	if p.Side != Black {
		t.Fatalf("Side = %d, want Black", p.Side)
	}
}

func TestRepetitionKeyIgnoresMoveCounters(t *testing.T) {
	// Identical placement/side/castling/EP but different clocks → same key.
	a, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	b, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 9 42")
	if a.RepetitionKey() != b.RepetitionKey() {
		t.Fatalf("repetition keys differ across move counters:\n %q\n %q",
			a.RepetitionKey(), b.RepetitionKey())
	}
	// The key carries no move counters.
	if got := a.RepetitionKey(); got != "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -" {
		t.Fatalf("RepetitionKey() = %q", got)
	}
}

func TestRepetitionKeyDistinguishesState(t *testing.T) {
	base, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")

	cases := []struct {
		name string
		fen  string
	}{
		{"different side", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1"},
		{"different castling", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w Qkq - 0 1"},
		{"different en passant", "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			other, err := ParseFEN(c.fen)
			if err != nil {
				t.Fatalf("ParseFEN: %v", err)
			}
			if base.RepetitionKey() == other.RepetitionKey() {
				t.Fatalf("repetition keys should differ:\n %q\n %q",
					base.RepetitionKey(), other.RepetitionKey())
			}
		})
	}
}

func TestParseFENRejectsGarbage(t *testing.T) {
	bad := []string{
		"",
		"only three fields here w -",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR x KQkq - 0 1", // bad side
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP w KQkq - 0 1",         // 7 ranks
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNX w KQkq - 0 1", // bad piece
	}
	for _, fen := range bad {
		if _, err := ParseFEN(fen); err == nil {
			t.Fatalf("ParseFEN(%q) should have failed", fen)
		}
	}
}
