package main

import "strconv"

// The full American betting layout, generated once into a master list. Every
// chip a player places references a master bet by index, so settlement is a
// single coverage test.
//
// The number felt is the standard 3x12 grid: column index c = 0..11 runs along
// the table, row rr = 0 (top) holds the "3,6,9,…" line, rr = 1 the "2,5,8,…"
// line, rr = 2 (bottom) the "1,4,7,…" line. So a cell's number is:
//
//	num(rr, c) = 3*c + (3 - rr)
//
// with the two green zeros (0 and 00) sitting to the left.
func num(rr, c int) int { return 3*c + (3 - rr) }

// betKind is the wager family; it fixes the payout and names the bet.
type betKind uint8

const (
	kStraight betKind = iota // 1 number       35:1
	kSplit                   // 2 numbers       17:1
	kStreet                  // 3 (a row)       11:1
	kTrio                    // 3 incl. a zero  11:1
	kCorner                  // 4 numbers        8:1
	kLine                    // 6 numbers        5:1
	kTopLine                 // 0,00,1,2,3       6:1 (American five-number bet)
	kDozen                   // 12 numbers       2:1
	kColumn                  // 12 numbers       2:1
	kRed                     // 18 numbers       1:1
	kBlack                   // 18 numbers       1:1
	kOdd                     // 18 numbers       1:1
	kEven                    // 18 numbers       1:1
	kLow                     // 1..18            1:1
	kHigh                    // 19..36           1:1
)

// payout is the winnings paid per unit staked (a winning bet returns the stake
// plus stake*payout; a losing bet returns nothing).
func (k betKind) payout() int {
	switch k {
	case kStraight:
		return 35
	case kSplit:
		return 17
	case kStreet, kTrio:
		return 11
	case kCorner:
		return 8
	case kTopLine:
		return 6
	case kLine:
		return 5
	case kDozen, kColumn:
		return 2
	default: // the even-money outside bets
		return 1
	}
}

// name is the short uppercase family label used in the armed-bet readout.
func (k betKind) name() string {
	switch k {
	case kStraight:
		return "STRAIGHT"
	case kSplit:
		return "SPLIT"
	case kStreet:
		return "STREET"
	case kTrio:
		return "TRIO"
	case kCorner:
		return "CORNER"
	case kLine:
		return "LINE"
	case kTopLine:
		return "TOP LINE"
	case kDozen:
		return "DOZEN"
	case kColumn:
		return "COLUMN"
	case kRed:
		return "RED"
	case kBlack:
		return "BLACK"
	case kOdd:
		return "ODD"
	case kEven:
		return "EVEN"
	case kLow:
		return "LOW"
	case kHigh:
		return "HIGH"
	}
	return "?"
}

// bet is one wager position on the felt.
type bet struct {
	kind    betKind
	nums    []int // covered pockets, ascending
	anchor  int   // min covered number (inside bets); -1 for outside
	outside bool
	label   string // compact chip/menu label, e.g. "17", "2-3", "Cnr 1-5", "RED"
}

// covers reports whether the spun pocket wins this bet.
func (b bet) covers(result int) bool {
	for _, n := range b.nums {
		if n == result {
			return true
		}
	}
	return false
}

// settleReturn is the chips returned for staking `stake` on bet b when `result`
// is the spun pocket: the stake back plus stake*payout on a win, nothing on a
// loss. (The American wheel has no en-prison / la-partage rule, so either green
// pocket — 0 or 00 — simply loses every outside bet.)
func settleReturn(b bet, stake, result int) int {
	if b.covers(result) {
		return stake * (b.kind.payout() + 1)
	}
	return 0
}

// masterBets is the immutable full layout, built once.
var masterBets = buildBets()

func buildBets() []bet {
	var bs []bet
	add := func(k betKind, label string, nums ...int) {
		anchor := -1
		outside := k >= kDozen
		if !outside {
			anchor = nums[0]
			for _, n := range nums[1:] {
				if n < anchor {
					anchor = n
				}
			}
		}
		bs = append(bs, bet{kind: k, nums: nums, anchor: anchor, outside: outside, label: label})
	}

	// 1. Straights: 0, then 1..36, then 00 (so master i == straight i for the
	// numbered pockets, and 00 is the last straight, master doubleZero).
	for n := 0; n <= 36; n++ {
		add(kStraight, strconv.Itoa(n), n)
	}
	add(kStraight, "00", doubleZero)

	// 2. Splits. Inside the grid, horizontal neighbours differ by 3 and vertical
	// neighbours by 1; the only zero split on an American felt is 0-00.
	add(kSplit, "0-00", 0, doubleZero)
	for rr := 0; rr <= 2; rr++ { // horizontal: same row, adjacent columns
		for c := 0; c < 11; c++ {
			a, b := num(rr, c), num(rr, c+1)
			add(kSplit, splitLabel(a, b), a, b)
		}
	}
	for c := 0; c <= 11; c++ { // vertical: same column, adjacent rows
		for rr := 0; rr < 2; rr++ {
			a, b := num(rr+1, c), num(rr, c) // lower number first
			add(kSplit, splitLabel(a, b), a, b)
		}
	}

	// 3. Streets (each grid row of three).
	for c := 0; c <= 11; c++ {
		a := num(2, c)
		add(kStreet, "Str "+strconv.Itoa(a)+"-"+strconv.Itoa(a+2), a, a+1, a+2)
	}

	// 4. Corners (each 2x2 block of adjacent numbers).
	for c := 0; c < 11; c++ {
		for rr := 0; rr < 2; rr++ {
			ns := []int{num(rr, c), num(rr+1, c), num(rr, c+1), num(rr+1, c+1)}
			lo, hi := bounds(ns)
			add(kCorner, "Cnr "+strconv.Itoa(lo)+"-"+strconv.Itoa(hi), sortAsc(ns)...)
		}
	}

	// 5. Six-lines (two adjacent streets).
	for c := 0; c < 11; c++ {
		a := num(2, c)
		ns := []int{a, a + 1, a + 2, a + 3, a + 4, a + 5}
		add(kLine, "Line "+strconv.Itoa(a)+"-"+strconv.Itoa(a+5), ns...)
	}

	// 6. The American zero-area bets: the two corner trios and the five-number
	// top line (0-00-1-2-3, the worst bet on the table at 6:1). Numbers stay
	// ascending, so the 00 pocket (doubleZero) sorts last.
	add(kTrio, "Trio 0-1-2", 0, 1, 2)
	add(kTrio, "Trio 00-2-3", 2, 3, doubleZero)
	add(kTopLine, "Top line", 0, 1, 2, 3, doubleZero)

	// 7. Outside bets.
	add(kDozen, "1st 12", seq(1, 12)...)
	add(kDozen, "2nd 12", seq(13, 24)...)
	add(kDozen, "3rd 12", seq(25, 36)...)
	add(kColumn, "Col 1", column(2)...) // bottom row: 1,4,7,…,34
	add(kColumn, "Col 2", column(1)...) // middle row: 2,5,8,…,35
	add(kColumn, "Col 3", column(0)...) // top row: 3,6,9,…,36
	add(kRed, "RED", colorNums(red)...)
	add(kBlack, "BLACK", colorNums(black)...)
	add(kOdd, "ODD", parity(1)...)
	add(kEven, "EVEN", parity(0)...)
	add(kLow, "1-18", seq(1, 18)...)
	add(kHigh, "19-36", seq(19, 36)...)

	return bs
}

// --- small helpers -----------------------------------------------------------

func splitLabel(a, b int) string {
	if a > b {
		a, b = b, a
	}
	return strconv.Itoa(a) + "-" + strconv.Itoa(b)
}

func bounds(ns []int) (lo, hi int) {
	lo, hi = ns[0], ns[0]
	for _, n := range ns[1:] {
		if n < lo {
			lo = n
		}
		if n > hi {
			hi = n
		}
	}
	return lo, hi
}

func sortAsc(ns []int) []int {
	out := append([]int(nil), ns...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func seq(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for n := lo; n <= hi; n++ {
		out = append(out, n)
	}
	return out
}

// column returns the twelve numbers in grid row rr (rr=2 is column 1, the
// bottom "1,4,7,…" row; rr=0 is column 3).
func column(rr int) []int {
	out := make([]int, 0, 12)
	for c := 0; c <= 11; c++ {
		out = append(out, num(rr, c))
	}
	return out
}

func colorNums(want color) []int {
	out := make([]int, 0, 18)
	for n := 1; n <= 36; n++ {
		if colorOf(n) == want {
			out = append(out, n)
		}
	}
	return out
}

func parity(odd int) []int {
	out := make([]int, 0, 18)
	for n := 1; n <= 36; n++ {
		if n%2 == odd {
			out = append(out, n)
		}
	}
	return out
}
