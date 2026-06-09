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
	// Pose ships and a few bullets so the frame is lively.
	rm.ships[a.AccountID].x, rm.ships[a.AccountID].y, rm.ships[a.AccountID].heading = 20, 8, 0
	rm.ships[a.AccountID].invulnUntil = tr.Clock
	rm.ships[a.AccountID].kills = 7
	rm.ships[b.AccountID].x, rm.ships[b.AccountID].y, rm.ships[b.AccountID].heading = 55, 14, 3.14
	rm.ships[b.AccountID].invulnUntil = tr.Clock
	rm.ships[b.AccountID].kills = 3
	rm.ships[c.AccountID].x, rm.ships[c.AccountID].y, rm.ships[c.AccountID].heading = 40, 18, 1.57
	rm.ships[c.AccountID].invulnUntil = tr.Clock
	rm.fire(tr, a, rm.ships[a.AccountID])
	rm.addExplosion(48, 10, kit.Red)

	f := rm.composeFor(a)
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
