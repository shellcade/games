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
	a := rm.wells[tr.Players[0].AccountID]
	b := rm.wells[tr.Players[1].AccountID]
	c := rm.wells[tr.Players[2].AccountID]

	// Pose alice's well with a partial stack + a falling piece.
	for col := 0; col < wellW; col++ {
		if col == 4 {
			continue
		}
		a.grid[wellH-1][col] = garbageCell
		if col%2 == 0 {
			a.grid[wellH-2][col] = pieceIndex("Tee") + 1
		}
	}
	a.cur = active{kind: pieceIndex("Ell"), rot: 0, row: 2, col: 3}
	a.score = 2400
	a.lines = 12
	a.level = 1

	// Give rivals some stack to show miniatures tumbling.
	for r := wellH - 5; r < wellH; r++ {
		for col := 0; col < wellW-2; col++ {
			b.grid[r][col] = pieceIndex("Box") + 1
		}
	}
	for r := wellH - 8; r < wellH; r++ {
		c.grid[r][0] = garbageCell
		c.grid[r][1] = pieceIndex("Zig") + 1
	}

	f := kit.NewFrame()
	rm.composeFor(f, tr.Players[0])

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
