package main

import "strconv"

// The American double-zero wheel: pockets 0, 00, and 1..36 — 38 in all, two
// green zeros, and the standard red/black split (house edge 5.26%, vs 2.70% for
// the single-zero European wheel). 00 is represented internally by the value
// doubleZero (37) so a pocket is a single int everywhere; only its label reads
// "00".

const (
	doubleZero = 37 // internal value for the "00" pocket
	pockets    = 38 // 0, 00 (=37), and 1..36
)

// pocketLabel is the printable name of a pocket: "00" for the double zero,
// otherwise the decimal number.
func pocketLabel(n int) string {
	if n == doubleZero {
		return "00"
	}
	return strconv.Itoa(n)
}

// color classifies a pocket.
type color uint8

const (
	green color = iota // 0 and 00
	red
	black
)

// redSet is the eighteen red numbers; every other number 1..36 is black, and
// both zeros are green (the red/black split is the same as the European wheel).
var redSet = [...]bool{
	1: true, 3: true, 5: true, 7: true, 9: true, 12: true, 14: true, 16: true,
	18: true, 19: true, 21: true, 23: true, 25: true, 27: true, 30: true,
	32: true, 34: true, 36: true,
}

// colorOf returns the pocket's color.
func colorOf(n int) color {
	if n == 0 || n == doubleZero {
		return green
	}
	if n >= 1 && n < len(redSet) && redSet[n] {
		return red
	}
	return black
}

// wheelSeq is the physical clockwise order of the pockets on an American wheel,
// starting at 0 (with 0 and 00 directly opposite). The spin animation scrolls
// this strip and decelerates the pointer onto the rolled result; the betting
// outcome itself is a uniform draw over the 38 pockets (wheelIndex maps a result
// to its seat on the strip).
var wheelSeq = [pockets]int{
	0, 28, 9, 26, 30, 11, 7, 20, 32, 17, 5, 22, 34, 15, 3, 24, 36, 13, 1,
	doubleZero, 27, 10, 25, 29, 12, 8, 19, 31, 18, 6, 21, 33, 16, 4, 23, 35, 14, 2,
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
