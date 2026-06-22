package main

import "fmt"

// themes.go defines the six named 5-reel PAR sheets bound one-per-machine on the
// lounge floor. Each is a distinct odds variant (weights / pays / scatter /
// gamble) tuned into the modern-pokie RTP band. Every symbol pays a small
// per-way amount (a spin credits ways/wayScale), so the reels show a varied mix
// of paying symbols rather than one dominant non-paying blank. Logical symbols
// and face art are shared across themes (themes differ by odds + name).

// themeDocs returns the six PAR-sheet documents in machine order. Each compiles
// to a total RTP in roughly [0.80, 0.92] (verified by TestThemesCompileInBand).
func themeDocs() []oddsVariant {
	scatter := []scatterEntry{{Count: 3, Spins: 6}, {Count: 4, Spins: 10}, {Count: 5, Spins: 15}}
	gamble := &gambleConfig{MaxRungs: 5, MaxWin: 1_000_000}
	return []oddsVariant{
		{
			Name:    "Lucky 7s", // high-variance, top-symbol focus (~0.88)
			Weights: map[string]int{"7": 3, "$": 5, "*": 6, "B": 8, "C": 9, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 2, Pay4: 4, Pay5: 14},
				{Faces: "$", Pay3: 1, Pay4: 3, Pay5: 8},
				{Faces: "*", Pay3: 1, Pay4: 2, Pay5: 4},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 2},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Gem Rush", // mid-symbol focus, frequent (~0.80)
			Weights: map[string]int{"7": 4, "$": 4, "*": 6, "B": 7, "C": 9, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 1, Pay4: 3, Pay5: 12},
				{Faces: "$", Pay3: 1, Pay4: 3, Pay5: 9},
				{Faces: "*", Pay3: 1, Pay4: 2, Pay5: 4},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 1},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Bells", // low-variance, frequent small wins (~0.83)
			Weights: map[string]int{"7": 5, "$": 6, "*": 6, "B": 6, "C": 7, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 1, Pay4: 3, Pay5: 9},
				{Faces: "$", Pay3: 1, Pay4: 2, Pay5: 6},
				{Faces: "*", Pay3: 1, Pay4: 2, Pay5: 4},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 2},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Cherry Pop", // frequent free spins (lower thresholds) (~0.88)
			Weights: map[string]int{"7": 4, "$": 5, "*": 6, "B": 7, "C": 8, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 1, Pay4: 3, Pay5: 14},
				{Faces: "$", Pay3: 1, Pay4: 3, Pay5: 8},
				{Faces: "*", Pay3: 1, Pay4: 2, Pay5: 5},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 2},
			},
			Scatter: []scatterEntry{{Count: 3, Spins: 5}, {Count: 4, Spins: 8}, {Count: 5, Spins: 12}},
			Gamble:  gamble,
		},
		{
			Name:    "Crown", // wild-leaning, big swings (~0.92)
			Weights: map[string]int{"7": 4, "$": 5, "*": 6, "B": 7, "C": 8, "W": 3, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 1, Pay4: 3, Pay5: 11},
				{Faces: "$", Pay3: 1, Pay4: 2, Pay5: 6},
				{Faces: "*", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 2},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 1},
			},
			Scatter: scatter, Gamble: gamble,
		},
		{
			Name:    "Gift Drop", // balanced all-rounder (~0.88)
			Weights: map[string]int{"7": 4, "$": 5, "*": 6, "B": 7, "C": 8, "W": 2, "S": 2},
			Paytable: []payEntry{
				{Faces: "7", Pay3: 1, Pay4: 3, Pay5: 15},
				{Faces: "$", Pay3: 1, Pay4: 3, Pay5: 9},
				{Faces: "*", Pay3: 1, Pay4: 2, Pay5: 6},
				{Faces: "B", Pay3: 1, Pay4: 1, Pay5: 3},
				{Faces: "C", Pay3: 1, Pay4: 1, Pay5: 1},
			},
			Scatter: scatter, Gamble: gamble,
		},
	}
}

// themeVariants compiles the six PAR sheets; a failure here is a programming bug
// (a theme PAR sheet that does not satisfy the RTP/convergence gates).
func themeVariants() []*variant {
	docs := themeDocs()
	vs := make([]*variant, len(docs))
	for i, doc := range docs {
		v, err := compileVariant(doc)
		if err != nil {
			panic(fmt.Sprintf("pokies: theme %q does not compile: %v", doc.Name, err))
		}
		vs[i] = v
	}
	return vs
}
