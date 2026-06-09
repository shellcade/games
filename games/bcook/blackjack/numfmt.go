package main

import kit "github.com/shellcade/kit/v2"

// intWidth returns how many columns putInt writes for n (digit count, plus one
// for a leading '-' when n is negative).
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

// putInt writes n in base-10 at (row, col) and returns the next column. It
// formats into a stack buffer and writes runes straight to the frame, so it
// allocates nothing — safe to call every render under -gc=leaking.
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

// runeLen counts runes without allocating (matches the compiler-optimized
// len([]rune(s)) form). Use it when you need a string literal's column width.
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// putClock writes a countdown as "0:%02d" at (row, col) and returns the next
// column, allocating nothing. It matches fmt.Sprintf("0:%02d", secs) exactly:
// "0:" then secs zero-padded to two digits (a leading '0' for secs<10), and the
// full number for secs>=100 (which %02d would also print in full).
func putClock(f *kit.Frame, row, col, secs int, st kit.Style) int {
	f.SetRune(row, col, '0', st)
	col++
	f.SetRune(row, col, ':', st)
	col++
	if secs < 0 {
		secs = 0
	}
	if secs < 10 {
		f.SetRune(row, col, '0', st)
		col++
		f.SetRune(row, col, rune('0'+secs), st)
		col++
		return col
	}
	return putInt(f, row, col, secs, st)
}

// clockWidth returns how many columns putClock writes for secs (matches
// "0:%02d": always at least "0:" + 2 digits).
func clockWidth(secs int) int {
	if secs < 0 {
		secs = 0
	}
	w := intWidth(secs)
	if w < 2 {
		w = 2
	}
	return 2 + w // "0:" + digits
}
