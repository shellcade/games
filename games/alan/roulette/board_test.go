package main

import "testing"

func TestSpotCount(t *testing.T) {
	// One navigable spot per master bet.
	if len(spots) != len(masterBets) {
		t.Fatalf("spots = %d, want %d (one per master bet)", len(spots), len(masterBets))
	}
}

// TestSpotsDistinct checks no two bets share a lattice point (which would make
// one of them unreachable / ambiguous to the cursor).
func TestSpotsDistinct(t *testing.T) {
	seen := map[[2]int]int{}
	for i, s := range spots {
		key := [2]int{s.fr, s.fc}
		if prev, ok := seen[key]; ok {
			t.Errorf("spots %q and %q share lattice point %v",
				masterBets[prev].label, masterBets[i].label, key)
		}
		seen[key] = i
	}
}

// TestLatticeParity checks each inside bet sits on the lattice class its family
// implies: numbers on odd/odd, splits/streets/lines on a line, corners on an
// intersection.
func TestLatticeParity(t *testing.T) {
	for i, b := range masterBets {
		if b.outside || involvesZero(b) {
			continue // zero region + outside boxes have their own placement
		}
		s := spots[i]
		oddR, oddC := s.fr%2 == 1, s.fc%2 == 1
		switch b.kind {
		case kStraight:
			if !(oddR && oddC) {
				t.Errorf("straight %q at %v is not a number centre", b.label, s)
			}
		case kSplit:
			if oddR == oddC { // a split must be on exactly one line (one odd, one even)
				t.Errorf("split %q at %v is not on a line", b.label, s)
			}
		case kStreet:
			if s.fr != 6 || !oddC {
				t.Errorf("street %q at %v is not on the outer column end", b.label, s)
			}
		case kCorner:
			if oddR || oddC {
				t.Errorf("corner %q at %v is not on an intersection", b.label, s)
			}
		case kLine:
			if s.fr != 6 || oddC {
				t.Errorf("six-line %q at %v is not on an outer intersection", b.label, s)
			}
		}
	}
}

var dirs = [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

// TestNavigationConnected checks every spot is reachable from the start cursor
// through arrow moves alone.
func TestNavigationConnected(t *testing.T) {
	seen := map[int]bool{startSpotIdx: true}
	queue := []int{startSpotIdx}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, d := range dirs {
			to := nextSpot(from, d[0], d[1])
			if to != from && !seen[to] {
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	if len(seen) != len(spots) {
		var missing []string
		for i := range spots {
			if !seen[i] {
				missing = append(missing, masterBets[i].label)
			}
		}
		t.Fatalf("reached %d/%d spots; unreachable: %v", len(seen), len(spots), missing)
	}
}

// TestNextSpotRespectsDirection ensures a move never lands off the pressed axis.
func TestNextSpotRespectsDirection(t *testing.T) {
	for from := range spots {
		cur := spots[from]
		for _, d := range dirs {
			to := nextSpot(from, d[0], d[1])
			if to == from {
				continue
			}
			s := spots[to]
			if d[0] != 0 && (s.fr-cur.fr)*d[0] <= 0 {
				t.Errorf("vertical move from %q landed off-direction", masterBets[from].label)
			}
			if d[1] != 0 && (s.fc-cur.fc)*d[1] <= 0 {
				t.Errorf("horizontal move from %q landed off-direction", masterBets[from].label)
			}
		}
	}
}

// TestStartCursor checks the cursor opens on the straight for 17.
func TestStartCursor(t *testing.T) {
	s := newSelection()
	b := masterBets[s.betIndex()]
	if b.kind != kStraight || b.nums[0] != 17 {
		t.Errorf("start cursor is %s %q, want STRAIGHT 17", b.kind.name(), b.label)
	}
}

// TestConcreteSpots pins a couple of lattice points so the layout doesn't drift:
// stepping right off a number lands on the split between it and its neighbour.
func TestConcreteSpots(t *testing.T) {
	// Straight 17 is master 17; right should land on the 17-20 split (17 and 20
	// are horizontal neighbours on the felt).
	to := masterBets[nextSpot(17, 0, 1)]
	if to.kind != kSplit || !containsN(to.nums, 17) || !containsN(to.nums, 20) {
		t.Errorf("right of 17 = %s %q, want SPLIT 17-20", to.kind.name(), to.label)
	}
}

// TestZeroColumnNavigation checks the left-margin zero lane: down steps
// 0 → 0-00 split → 00, and up reverses it.
func TestZeroColumnNavigation(t *testing.T) {
	zero := 0                            // master 0 = straight 0
	split := masterOf(t, kSplit, "0-00") // the 0-00 split
	dz := masterOf(t, kStraight, "00")   // straight on 00
	if got := nextSpot(zero, 1, 0); got != split {
		t.Errorf("down from 0 = %q, want the 0-00 split", masterBets[got].label)
	}
	if got := nextSpot(split, 1, 0); got != dz {
		t.Errorf("down from the split = %q, want 00", masterBets[got].label)
	}
	if got := nextSpot(dz, -1, 0); got != split {
		t.Errorf("up from 00 = %q, want the 0-00 split", masterBets[got].label)
	}
	if got := nextSpot(split, -1, 0); got != zero {
		t.Errorf("up from the split = %q, want 0", masterBets[got].label)
	}
}

// TestUpThroughOutsideRows checks that ascending from the even-money strip stops
// on the dozen row rather than leaping past it to a grid line in the same column.
func TestUpThroughOutsideRows(t *testing.T) {
	low := masterOf(t, kLow, "1-18")
	up := masterBets[nextSpot(low, -1, 0)]
	if up.kind != kDozen {
		t.Errorf("up from 1-18 = %s %q, want a DOZEN", up.kind.name(), up.label)
	}
	// And up from the dozen continues onto the grid (its six-line / numbers).
	dozenIdx := nextSpot(low, -1, 0)
	above := masterBets[nextSpot(dozenIdx, -1, 0)]
	if above.outside {
		t.Errorf("up from the dozen = %s %q, want a grid bet", above.kind.name(), above.label)
	}
}

// TestDownThroughDozens checks that descending off the felt from a bottom-row
// number passes through the street and the dozen strip before the even-money
// row — never skipping the dozen because a wide box doesn't line up column-wise.
func TestDownThroughDozens(t *testing.T) {
	// Number 1 is master 1 (bottom-left). Down -> its street (outer edge).
	s := masterBets[nextSpot(1, 1, 0)]
	if s.kind != kStreet {
		t.Fatalf("down from 1 = %s %q, want a STREET", s.kind.name(), s.label)
	}
	streetIdx := nextSpot(1, 1, 0)
	// Down from the street -> the first dozen, NOT the even-money strip.
	d := masterBets[nextSpot(streetIdx, 1, 0)]
	if d.kind != kDozen {
		t.Fatalf("down from the street = %s %q, want a DOZEN", d.kind.name(), d.label)
	}
	// Down from the dozen -> the even-money row.
	dozenIdx := nextSpot(streetIdx, 1, 0)
	e := masterBets[nextSpot(dozenIdx, 1, 0)]
	switch e.kind {
	case kLow, kHigh, kRed, kBlack, kOdd, kEven:
		// good — reached the even-money strip
	default:
		t.Fatalf("down from the dozen = %s %q, want an even-money bet", e.kind.name(), e.label)
	}
}
