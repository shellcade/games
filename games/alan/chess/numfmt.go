package main

import kit "github.com/shellcade/kit/v2"

// intWidth returns the number of base-10 digits in n (plus one for a leading
// '-' when n is negative). Alloc-free.
func intWidth(n int) int {
	w := 1
	if n < 0 {
		w++
		n = -n
	}
	for n >= 10 {
		n /= 10
		w++
	}
	return w
}

// putInt writes n in base-10 at (row,col) and returns the next column.
// Alloc-free: the digits live in a stack buffer and are written cell-by-cell.
func putInt(f *kit.Frame, row, col, n int, st kit.Style) int {
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	if neg {
		i--
		b[i] = '-'
	}
	for ; i < len(b); i++ {
		f.SetRune(row, col, rune(b[i]), st)
		col++
	}
	return col
}

// runeLen returns the number of runes in s without allocating.
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
