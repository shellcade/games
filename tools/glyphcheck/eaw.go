package main

// East Asian Width Wide (W) and Fullwidth (F) code-point ranges, merged from
// the Unicode Character Database's EastAsianWidth.txt (Unicode 15.1.0). These
// are the two widths that runewidth, uniseg, x/ansi and real terminals all
// agree render as TWO columns — the only safe inputs to a wide-glyph helper
// (see the package doc in main.go for the corruption class this guards).
//
// The table is embedded (not fetched) so the linter runs offline in fork CI.
// It is a flat, sorted, non-overlapping list of inclusive [lo, hi] ranges.
//
// Provenance / regenerating: these are exactly the `W` and `F` ranges of
// https://www.unicode.org/Public/15.1.0/ucd/EastAsianWidth.txt. When the kit's
// `shellcade-kit check` ships its own width contract, this table is retired in
// favour of it (see main.go). Keep it in sync with the kit's Unicode version
// if it must live longer.
type eawRange struct{ lo, hi rune }

var eawWideFull = []eawRange{
	{0x1100, 0x115F},   // Hangul Jamo
	{0x231A, 0x231B},   // watch, hourglass
	{0x2329, 0x232A},   // angle brackets
	{0x23E9, 0x23EC},   // black media controls
	{0x23F0, 0x23F0},   // alarm clock
	{0x23F3, 0x23F3},   // hourglass with flowing sand
	{0x25FD, 0x25FE},   // medium small squares
	{0x2614, 0x2615},   // umbrella with rain, hot beverage
	{0x2648, 0x2653},   // zodiac
	{0x267F, 0x267F},   // wheelchair
	{0x2693, 0x2693},   // anchor
	{0x26A1, 0x26A1},   // high voltage
	{0x26AA, 0x26AB},   // medium circles
	{0x26BD, 0x26BE},   // soccer, baseball
	{0x26C4, 0x26C5},   // snowman, sun behind cloud
	{0x26CE, 0x26CE},   // ophiuchus
	{0x26D4, 0x26D4},   // no entry
	{0x26EA, 0x26EA},   // church
	{0x26F2, 0x26F3},   // fountain, golf
	{0x26F5, 0x26F5},   // sailboat
	{0x26FA, 0x26FA},   // tent
	{0x26FD, 0x26FD},   // fuel pump
	{0x2705, 0x2705},   // white heavy check mark
	{0x270A, 0x270B},   // raised fist, hand
	{0x2728, 0x2728},   // sparkles
	{0x274C, 0x274C},   // cross mark
	{0x274E, 0x274E},   // negative squared cross mark
	{0x2753, 0x2755},   // question/exclamation
	{0x2757, 0x2757},   // heavy exclamation mark
	{0x2795, 0x2797},   // heavy plus/minus/division
	{0x27B0, 0x27B0},   // curly loop
	{0x27BF, 0x27BF},   // double curly loop
	{0x2B1B, 0x2B1C},   // black/white large square
	{0x2B50, 0x2B50},   // white medium star ⭐
	{0x2B55, 0x2B55},   // heavy large circle
	{0x2E80, 0x2E99},   // CJK Radicals Supplement
	{0x2E9B, 0x2EF3},   // CJK Radicals Supplement
	{0x2F00, 0x2FD5},   // Kangxi Radicals
	{0x2FF0, 0x2FFB},   // Ideographic Description Characters
	{0x3000, 0x303E},   // CJK Symbols and Punctuation
	{0x3041, 0x3096},   // Hiragana
	{0x3099, 0x30FF},   // Katakana
	{0x3105, 0x312F},   // Bopomofo
	{0x3131, 0x318E},   // Hangul Compatibility Jamo
	{0x3190, 0x31E3},   // Kanbun, CJK Strokes
	{0x31EF, 0x321E},   // Katakana Phonetic, Enclosed CJK
	{0x3220, 0x3247},   // Enclosed CJK Letters and Months
	{0x3250, 0x4DBF},   // Enclosed CJK .. CJK Unified Ext A
	{0x4E00, 0xA48C},   // CJK Unified Ideographs .. Yi Syllables
	{0xA490, 0xA4C6},   // Yi Radicals
	{0xA960, 0xA97C},   // Hangul Jamo Extended-A
	{0xAC00, 0xD7A3},   // Hangul Syllables
	{0xF900, 0xFAFF},   // CJK Compatibility Ideographs
	{0xFE10, 0xFE19},   // Vertical Forms
	{0xFE30, 0xFE52},   // CJK Compatibility Forms
	{0xFE54, 0xFE66},   // Small Form Variants
	{0xFE68, 0xFE6B},   // Small Form Variants
	{0xFF01, 0xFF60},   // Fullwidth Forms (incl. ７ U+FF17)
	{0xFFE0, 0xFFE6},   // Fullwidth signs
	{0x16FE0, 0x16FE4}, // Tangut/Khitan marks
	{0x16FF0, 0x16FF1}, // Vietnamese reading marks
	{0x17000, 0x187F7}, // Tangut
	{0x18800, 0x18CD5}, // Tangut Components / Khitan Small Script
	{0x18D00, 0x18D08}, // Tangut Supplement
	{0x1AFF0, 0x1AFF3}, // Katakana minor extensions
	{0x1AFF5, 0x1AFFB}, // Katakana minor extensions
	{0x1AFFD, 0x1AFFE}, // Katakana minor extensions
	{0x1B000, 0x1B122}, // Kana Supplement / Extended
	{0x1B132, 0x1B132}, // Hiragana small KO
	{0x1B150, 0x1B152}, // Small Kana Extension
	{0x1B155, 0x1B155}, // Katakana small KO
	{0x1B164, 0x1B167}, // Small Kana Extension
	{0x1B170, 0x1B2FB}, // Nushu
	{0x1F004, 0x1F004}, // mahjong red dragon
	{0x1F0CF, 0x1F0CF}, // playing card black joker
	{0x1F18E, 0x1F18E}, // negative squared AB
	{0x1F191, 0x1F19A}, // squared CL .. VS
	{0x1F200, 0x1F202}, // enclosed ideographic
	{0x1F210, 0x1F23B}, // enclosed ideographic
	{0x1F240, 0x1F248}, // tortoise shell bracketed
	{0x1F250, 0x1F251}, // circled ideographs
	{0x1F260, 0x1F265}, // rounded symbols
	{0x1F300, 0x1F320}, // Misc Symbols and Pictographs
	{0x1F32D, 0x1F335}, // food/plant pictographs
	{0x1F337, 0x1F37C}, // plant/food (incl. 🍀 U+1F340, 🍒 U+1F352)
	{0x1F37E, 0x1F393}, // food/celebration
	{0x1F3A0, 0x1F3CA}, // activity pictographs
	{0x1F3CF, 0x1F3D3}, // sport pictographs
	{0x1F3E0, 0x1F3F0}, // building pictographs
	{0x1F3F4, 0x1F3F4}, // waving black flag
	{0x1F3F8, 0x1F43E}, // misc pictographs (incl. 🐊 U+1F40A, 🐟 U+1F41F, 🐸 U+1F438)
	{0x1F440, 0x1F440}, // eyes
	{0x1F442, 0x1F4FC}, // pictographs (incl. 💎 U+1F48E, 💰 U+1F4B0, 🔔 U+1F514)
	{0x1F4FF, 0x1F53D}, // pictographs
	{0x1F54B, 0x1F54E}, // kaaba .. menorah
	{0x1F550, 0x1F567}, // clock faces
	{0x1F57A, 0x1F57A}, // man dancing
	{0x1F595, 0x1F596}, // reversed hand fingers
	{0x1F5A4, 0x1F5A4}, // black heart
	{0x1F5FB, 0x1F64F}, // map/emotion pictographs
	{0x1F680, 0x1F6C5}, // transport/map symbols
	{0x1F6CC, 0x1F6CC}, // sleeping accommodation
	{0x1F6D0, 0x1F6D2}, // place of worship .. shopping trolley
	{0x1F6D5, 0x1F6D7}, // hindu temple .. elevator
	{0x1F6DC, 0x1F6DF}, // wireless .. ring buoy
	{0x1F6EB, 0x1F6EC}, // airplane departure/arrival
	{0x1F6F4, 0x1F6FC}, // scooter .. roller skate
	{0x1F7E0, 0x1F7EB}, // large colored circles
	{0x1F7F0, 0x1F7F0}, // heavy equals sign
	{0x1F90C, 0x1F93A}, // supplemental symbols (incl. 🦀 U+1F980 below)
	{0x1F93C, 0x1F945}, // supplemental symbols
	{0x1F947, 0x1F9FF}, // supplemental symbols (incl. 🦀 U+1F980)
	{0x1FA70, 0x1FA7C}, // ballet shoes .. crutch
	{0x1FA80, 0x1FA88}, // yo-yo .. flute
	{0x1FA90, 0x1FABD}, // ringed planet .. wing
	{0x1FABF, 0x1FAC5}, // goose .. person with crown
	{0x1FACE, 0x1FADB}, // moose .. pea pod
	{0x1FAE0, 0x1FAE8}, // melting face .. shaking face
	{0x1FAF0, 0x1FAF8}, // hand pictographs
	{0x20000, 0x2FFFD}, // CJK Unified Ideographs Ext B..
	{0x30000, 0x3FFFD}, // CJK Unified Ideographs Ext G..
}

// isWideOrFullwidth reports whether r has East Asian Width Wide or Fullwidth.
func isWideOrFullwidth(r rune) bool {
	lo, hi := 0, len(eawWideFull)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < eawWideFull[mid].lo:
			hi = mid - 1
		case r > eawWideFull[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}
