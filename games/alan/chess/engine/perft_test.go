package engine

import "testing"

// perft counts the number of leaf nodes reachable in exactly depth plies from p,
// generating moves through LegalMoves and advancing with Apply. It is the gold
// standard for verifying a move generator: any bug (a missing or extra move, a
// botched special case) shows up as a node-count mismatch against the published
// reference values below.
func perft(p Position, depth int) uint64 {
	if depth == 0 {
		return 1
	}
	moves := LegalMoves(p)
	if depth == 1 {
		return uint64(len(moves))
	}
	var nodes uint64
	for _, m := range moves {
		nodes += perft(Apply(p, m), depth-1)
	}
	return nodes
}

// perftCase is one reference position and its expected node counts per depth
// (index 0 = depth 1).
type perftCase struct {
	name   string
	fen    string
	counts []uint64
}

func TestPerft(t *testing.T) {
	cases := []perftCase{
		{
			"start",
			"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			[]uint64{20, 400, 8902, 197281, 4865609},
		},
		{
			"kiwipete",
			"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
			[]uint64{48, 2039, 97862, 4085603},
		},
		{
			"pos3",
			"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
			[]uint64{14, 191, 2812, 43238, 674624},
		},
		{
			"pos4",
			"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
			[]uint64{6, 264, 9467, 422333},
		},
		{
			"pos5",
			"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
			[]uint64{44, 1486, 62379, 2103487},
		},
		{
			"pos6",
			"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
			[]uint64{46, 2079, 89890, 3894594},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			p, err := ParseFEN(c.fen)
			if err != nil {
				t.Fatalf("ParseFEN(%q): %v", c.fen, err)
			}
			for i, want := range c.counts {
				depth := i + 1
				if got := perft(p, depth); got != want {
					t.Fatalf("perft(%s, %d) = %d, want %d", c.name, depth, got, want)
				}
			}
		})
	}
}
