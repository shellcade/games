package shellracer

import kit "github.com/shellcade/kit/v2"

// intWidth returns the number of bytes putInt would write for n (including a
// leading '-' for negatives).
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

// putInt writes n in base-10 starting at (row,col) and returns the next column.
// Alloc-free.
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

// putIntRight writes n right-justified in a field of `width` columns starting at
// col, left-padded with spaces. Returns col+width. Alloc-free.
func putIntRight(f *kit.Frame, row, col, n, width int, st kit.Style) int {
	start := col
	for pad := width - intWidth(n); pad > 0; pad-- {
		f.SetRune(row, col, ' ', st)
		col++
	}
	putInt(f, row, col, n, st)
	return start + width
}

// putIntLeft writes n left-justified in a field of `width` columns starting at
// col, right-padded with spaces. Returns col+width. Alloc-free.
func putIntLeft(f *kit.Frame, row, col, n, width int, st kit.Style) int {
	end := col + width
	col = putInt(f, row, col, n, st)
	for col < end {
		f.SetRune(row, col, ' ', st)
		col++
	}
	return end
}

// putTextLeft writes s left-justified in a field of `width` columns: writes s
// (truncated to width runes) then space-pads to fill width. Returns col+width.
// Alloc-free.
func putTextLeft(f *kit.Frame, row, col int, s string, width int, st kit.Style) int {
	end := col + width
	for _, r := range s {
		if col >= end {
			break
		}
		f.SetRune(row, col, r, st)
		col++
	}
	for col < end {
		f.SetRune(row, col, ' ', st)
		col++
	}
	return end
}

// runeLen returns the number of runes in s without allocating.
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
