package main

import (
	"fmt"
	"strings"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// TestSnapshot prints a readable rune grid of one composed frame. Run with:
//
//	go test -run TestSnapshot -v
func TestSnapshot(t *testing.T) {
	rm, tr := newTestRoom(t, "alice", "bob", "cleo")
	for _, p := range tr.Players {
		rm.OnJoin(tr, p)
	}
	a, b, c := tr.Players[0], tr.Players[1], tr.Players[2]

	// Pose the three contestants and rough up the floor so the snapshot shows a
	// mid-round arena: cracks, worn tiles, and a couple of holes.
	rm.players[a.AccountID].layer, rm.players[a.AccountID].row, rm.players[a.AccountID].col = 0, 8, 20
	rm.players[b.AccountID].layer, rm.players[b.AccountID].row, rm.players[b.AccountID].col = 0, 14, 50
	rm.players[c.AccountID].layer, rm.players[c.AccountID].row, rm.players[c.AccountID].col = 0, 18, 35

	for col := 18; col < 24; col++ {
		rm.floors[0][8-top][col] = tileCracked
	}
	for col := 30; col < 40; col++ {
		rm.floors[0][12-top][col] = tileWorn
	}
	rm.floors[0][10-top][45] = tileGone
	rm.floors[0][10-top][46] = tileGone
	rm.floors[0][11-top][45] = tileGone
	// A worn tile sitting above a hole on the next layer should glow as a warning.
	rm.floors[0][15-top][55] = tileWorn
	rm.floors[1][15-top][55] = tileGone

	f := kit.NewFrame()
	rm.composeFor(f, a)
	var sb strings.Builder
	sb.WriteString("+" + strings.Repeat("-", kit.Cols) + "+\n")
	for row := 0; row < kit.Rows; row++ {
		sb.WriteByte('|')
		for col := 0; col < kit.Cols; col++ {
			ru := f.Cells[row][col].Rune
			if ru == 0 {
				ru = ' '
			}
			sb.WriteRune(ru)
		}
		sb.WriteString("|\n")
	}
	sb.WriteString("+" + strings.Repeat("-", kit.Cols) + "+\n")
	fmt.Print(sb.String())
}
