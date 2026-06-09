package main

// The European single-zero wheel: pockets 0..36, one green zero, and the
// standard red/black split (house edge 2.7%). The American double-zero wheel is
// deliberately not modelled — single-zero is the fairer, cleaner table.

const pockets = 37 // 0..36

// color classifies a pocket.
type color uint8

const (
	green color = iota // only 0
	red
	black
)

// redSet is the eighteen red numbers on a European wheel; every other number
// 1..36 is black, and 0 is green.
var redSet = [...]bool{
	1: true, 3: true, 5: true, 7: true, 9: true, 12: true, 14: true, 16: true,
	18: true, 19: true, 21: true, 23: true, 25: true, 27: true, 30: true,
	32: true, 34: true, 36: true,
}

// colorOf returns the pocket's color.
func colorOf(n int) color {
	if n == 0 {
		return green
	}
	if n >= 1 && n < len(redSet) && redSet[n] {
		return red
	}
	return black
}

// wheelSeq is the physical clockwise order of the pockets on a European wheel,
// starting at 0. The spin animation scrolls this strip and decelerates the
// pointer onto the rolled result; the betting outcome itself is a uniform draw
// over 0..36 (wheelIndex maps a result to its seat on the strip).
var wheelSeq = [pockets]int{
	0, 32, 15, 19, 4, 21, 2, 25, 17, 34, 6, 27, 13, 36, 11, 30, 8, 23, 10,
	5, 24, 16, 33, 1, 20, 14, 31, 9, 22, 18, 29, 7, 28, 12, 35, 3, 26,
}

// wheelIndex returns the position of pocket n on the physical strip.
func wheelIndex(n int) int {
	for i, p := range wheelSeq {
		if p == n {
			return i
		}
	}
	return 0
}
