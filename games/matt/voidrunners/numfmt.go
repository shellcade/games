package main

import kit "github.com/shellcade/kit/v2"

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

// putInt writes n in base-10 at (row,col), returns next column. Alloc-free.
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

// putTextTrunc writes up to maxRunes runes of s at (row,col), returns next col.
// Alloc-free replacement for writing string([]rune(s)[:maxRunes]).
func putTextTrunc(f *kit.Frame, row, col int, s string, maxRunes int, st kit.Style) int {
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		f.SetRune(row, col, r, st)
		col++
		n++
	}
	return col
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
