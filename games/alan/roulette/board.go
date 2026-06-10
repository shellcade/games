package main

// The betting board the cursor roams. Every wager in the master list is a
// navigable "spot" sitting at its real position on the felt, so the cursor
// lands exactly where you'd drop a chip at a real table:
//
//   - a number's centre          -> straight up
//   - the line between two cells  -> split
//   - a four-number intersection  -> corner
//   - the outer end of a column   -> street (3) / six-line (6)
//   - the zero and its boundary    -> 0, its splits, the trios, the basket
//   - the boxes around the grid    -> dozens, columns, even-money
//
// Positions live on a fine integer lattice (fr, fc): odd/odd is a number,
// even coordinates fall on the lines and intersections between them. Arrow keys
// step to the nearest spot in the pressed direction, so moving right off a
// number lands on the split line, then the next number, and so on. Spot i is
// master bet i (buildSpots walks masterBets in order), so the cursor index is
// itself the master index — no separate mapping.

// fine lattice rows: number rows rr=0,1,2 sit at fr = 1,3,5; the lines between
// and the outer boundaries are the even rows 0,2,4,6. Outside boxes live below
// (dozens fr 8, even-money fr 10).
//
// fine lattice cols: number columns c=0..11 sit at fc = 1,3,…,23; the lines
// between are the even cols; the left boundary (the zero) is fc 0 / -1, and the
// column "2:1" boxes the right boundary fc 24.

type spot struct {
	fr, fc int // fine lattice coordinates
}

// gridRC maps a felt number 1..36 to its printed (row, col): row rr=0 is the
// top "3,6,9,…" line, rr=2 the bottom "1,4,7,…" line; column c=0..11 runs along
// the table. Inverse of num(rr, c).
func gridRC(n int) (rr, c int) { return 2 - (n-1)%3, (n - 1) / 3 }

// spots[i] is the felt position of master bet i.
var spots = buildSpots()

// startSpotIdx is where a fresh cursor sits: the straight on 17 (master 17, dead
// centre of the felt).
const startSpotIdx = 17

func buildSpots() []spot {
	ss := make([]spot, len(masterBets))
	for i, b := range masterBets {
		ss[i] = spot{}
		ss[i].fr, ss[i].fc = latticeOf(b)
	}
	return ss
}

// latticeOf places a bet on the fine lattice.
func latticeOf(b bet) (fr, fc int) {
	if b.outside {
		return outsideLattice(b)
	}
	if involvesZero(b) {
		return zeroLattice(b)
	}
	switch b.kind {
	case kStraight:
		rr, c := gridRC(b.nums[0])
		return 2*rr + 1, 2*c + 1
	case kSplit:
		rr0, c0 := gridRC(b.nums[0])
		rr1, c1 := gridRC(b.nums[1])
		if rr0 == rr1 { // horizontal neighbours -> vertical line between them
			return 2*rr0 + 1, 2*min(c0, c1) + 2
		}
		return 2*min(rr0, rr1) + 2, 2*c0 + 1 // vertical neighbours -> horizontal line
	case kStreet:
		_, c := gridRC(b.nums[0])
		return 6, 2*c + 1 // outer (bottom) end of the column
	case kCorner:
		rr, c := topLeft(b.nums)
		return 2*rr + 2, 2*c + 2
	case kLine:
		_, c := topLeft(b.nums)
		return 6, 2*c + 2 // outer corner between two columns
	}
	return 3, 1
}

// involvesZero reports whether a bet covers either green pocket.
func involvesZero(b bet) bool {
	return containsN(b.nums, 0) || containsN(b.nums, doubleZero)
}

// zeroLattice places the zeros and the bets that touch them down the left
// margin. The zero column (fc -1) is a clean vertical lane — 0 (fr 1), the 0-00
// split (fr 3), 00 (fr 5) — so up/down steps 0 → split → 00. The trios and the
// top line sit one column right (fc 0), on the same rows, so left/right reaches
// them on the way to the grid.
func zeroLattice(b bet) (fr, fc int) {
	switch b.kind {
	case kStraight:
		if b.nums[0] == doubleZero {
			return 5, -1
		}
		return 1, -1
	case kSplit: // 0-00
		return 3, -1
	case kTopLine: // 0-00-1-2-3, level with 0
		return 1, 0
	case kTrio:
		if containsN(b.nums, 1) { // 0-1-2, level with the split
			return 3, 0
		}
		return 5, 0 // 00-2-3, level with 00
	}
	return 1, -1
}

// outsideLattice places the dozen, column, and even-money boxes around the grid.
func outsideLattice(b bet) (fr, fc int) {
	switch b.kind {
	case kColumn:
		switch b.nums[0] {
		case 1:
			return 5, 24
		case 2:
			return 3, 24
		default:
			return 1, 24
		}
	case kDozen:
		switch b.nums[0] {
		case 1:
			return 8, 4
		case 13:
			return 8, 12
		default:
			return 8, 20
		}
	case kLow:
		return 10, 2
	case kEven:
		return 10, 6
	case kRed:
		return 10, 10
	case kBlack:
		return 10, 14
	case kOdd:
		return 10, 18
	case kHigh:
		return 10, 22
	}
	return 10, 12
}

// topLeft returns the (rr, c) of the upper-left number of a block (min rr, min c
// across its members) — the anchor for a corner/line lattice point.
func topLeft(nums []int) (rr, c int) {
	rr, c = 2, 11
	for _, n := range nums {
		r2, c2 := gridRC(n)
		rr = min(rr, r2)
		c = min(c, c2)
	}
	return rr, c
}

func containsN(ns []int, x int) bool {
	for _, n := range ns {
		if n == x {
			return true
		}
	}
	return false
}

// nextSpot returns the nearest spot in the pressed direction (dr or dc is ±1,
// the other 0), or `from` when nothing lies that way. Primary cost is the
// perpendicular offset (track straight along the line), secondary the distance
// travelled along the axis.
func nextSpot(from, dr, dc int) int {
	cur := spots[from]
	// A vertical move first tries to stay in the exact same column, so a number
	// column — and the zero lane (0 → 0-00 split → 00) — descends cleanly. The
	// jump is capped to a couple of fine rows: those lanes step at most 2 with
	// nothing between, but a longer same-column reach would skip over a whole row
	// of boxes (e.g. up from 1-18 must stop on the 1st 12 dozen, not leap to the
	// six-line above it). Beyond the cap we fall back to the nearest row below.
	const sameColMaxRows = 2
	if dr != 0 {
		best, bestDist := from, 1<<30
		for i, s := range spots {
			if i == from || s.fc != cur.fc || (s.fr-cur.fr)*dr <= 0 {
				continue
			}
			if d := abs(s.fr - cur.fr); d < bestDist {
				best, bestDist = i, d
			}
		}
		if best != from && bestDist <= sameColMaxRows {
			return best
		}
	}

	best, bestP, bestS := from, 1<<30, 1<<30
	for i, s := range spots {
		if i == from {
			continue
		}
		if dr != 0 {
			if (s.fr-cur.fr)*dr <= 0 {
				continue
			}
		} else {
			if (s.fc-cur.fc)*dc <= 0 {
				continue
			}
		}
		// Among the spots in the pressed direction, prefer the nearest row, then
		// the nearest column. For a horizontal move that keeps the cursor in its
		// row; for a vertical move it advances one row at a time, so going down
		// off the felt steps through the dozen strip before the even-money row
		// rather than skipping a wide box that doesn't line up column-wise.
		prim, sec := abs(s.fr-cur.fr), abs(s.fc-cur.fc)
		if prim < bestP || (prim == bestP && sec < bestS) {
			best, bestP, bestS = i, prim, sec
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// selection is one player's cursor: a spot index, which is also the master bet
// index it would place.
type selection struct {
	spot int
}

func newSelection() selection { return selection{spot: startSpotIdx} }

func (s *selection) move(dr, dc int) {
	if n := nextSpot(s.spot, dr, dc); n != s.spot {
		s.spot = n
	}
}

// betIndex is the master index of the bet under the cursor.
func (s *selection) betIndex() int { return s.spot }
