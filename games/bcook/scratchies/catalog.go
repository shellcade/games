package main

import kit "github.com/shellcade/kit/v2"

// catalog.go — the 16-ticket catalog (SPEC §6). The prize tables here are rough
// first-pass ladders; Agent E tunes them to the per-tier RTP/odds bands (SPEC
// §7) and adds the stats() enumerator + asserting tests.

// tierTable builds a placeholder prize ladder topping out at `top` credits.
// Ascending in Credits; the long tail to `top` is the dream jackpot.
func tierTable(top int) PrizeTable {
	return PrizeTable{
		{Credits: 1, OneIn: 6},
		{Credits: 2, OneIn: 18},
		{Credits: 5, OneIn: 40},
		{Credits: 10, OneIn: 120},
		{Credits: 20, OneIn: 400},
		{Credits: 50, OneIn: 2000},
		{Credits: 100, OneIn: 10000},
		{Credits: top / 10, OneIn: 250000},
		{Credits: top, OneIn: 1250000},
	}
}

var themeDefault = Theme{Accent: kit.Yellow, Symbol: kit.Green}

// tickets is the live catalog. Four mechanics × four tiers, each its own theme.
var tickets = []Ticket{
	// --- $1 ---
	{Slug: "lucky-7s", Name: "Lucky 7s", Price: 1, Mechanic: MechMatch3, Theme: themeDefault, Cols: 3, Rows: 3, Prizes: tierTable(10000)},
	{Slug: "coin-toss", Name: "Coin Toss", Price: 1, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 3, Rows: 2, WinNumbers: 2, Prizes: tierTable(10000)},
	{Slug: "cherry-pop", Name: "Cherry Pop", Price: 1, Mechanic: MechFind, Theme: themeDefault, Cols: 3, Rows: 3, Symbol: "CHRY", Prizes: tierTable(10000)},
	{Slug: "tinnie-tripler", Name: "Tinnie Tripler", Price: 1, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 3, Prizes: tierTable(12000)},

	// --- $2 ---
	{Slug: "gold-rush", Name: "Gold Rush", Price: 2, Mechanic: MechMatch3, Theme: themeDefault, Cols: 4, Rows: 4, Prizes: tierTable(25000)},
	{Slug: "lucky-numbers", Name: "Lucky Numbers", Price: 2, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 3, Rows: 3, WinNumbers: 3, Prizes: tierTable(25000)},
	{Slug: "croc-cash", Name: "Croc Cash", Price: 2, Mechanic: MechFind, Theme: themeDefault, Cols: 4, Rows: 3, Symbol: "CROC", HasBust: true, Prizes: tierTable(25000)},
	{Slug: "double-trouble", Name: "Double Trouble", Price: 2, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 5, Prizes: tierTable(30000)},

	// --- $5 ---
	{Slug: "diamond-mine", Name: "Diamond Mine", Price: 5, Mechanic: MechMatch3, Theme: themeDefault, Cols: 5, Rows: 5, Prizes: tierTable(100000)},
	{Slug: "lotto-lanes", Name: "Lotto Lanes", Price: 5, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 4, Rows: 4, WinNumbers: 4, Prizes: tierTable(100000)},
	{Slug: "treasure-hunt", Name: "Treasure Hunt", Price: 5, Mechanic: MechFind, Theme: themeDefault, Cols: 5, Rows: 4, Symbol: "GEM", Prizes: tierTable(100000)},
	{Slug: "mega-multiplier", Name: "Mega Multiplier", Price: 5, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 10, Prizes: tierTable(120000)},

	// --- $10 ---
	{Slug: "platinum-sevens", Name: "Platinum Sevens", Price: 10, Mechanic: MechMatch3, Theme: themeDefault, Cols: 6, Rows: 6, Prizes: tierTable(250000)},
	{Slug: "fortune-50", Name: "Fortune 50", Price: 10, Mechanic: MechKeyNum, Theme: themeDefault, Cols: 4, Rows: 6, WinNumbers: 6, Prizes: tierTable(250000)},
	{Slug: "outback-riches", Name: "Outback Riches", Price: 10, Mechanic: MechFind, Theme: themeDefault, Cols: 6, Rows: 5, Symbol: "PICK", HasBust: true, Prizes: tierTable(250000)},
	{Slug: "cash-explosion", Name: "Cash Explosion", Price: 10, Mechanic: MechMult, Theme: themeDefault, Cols: 1, Rows: 2, MaxMult: 20, Prizes: tierTable(300000)},
}
