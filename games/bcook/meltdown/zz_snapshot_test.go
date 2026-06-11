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
	// Pose the crew and some faults so the frame is lively.
	st := rm.stations
	standOn(rm.crew[a.AccountID], st[0][0], st[0][1])
	standOn(rm.crew[b.AccountID], st[3][0], st[3][1])
	standOn(rm.crew[c.AccountID], st[6][0], st[6][1])
	rm.crew[a.AccountID].fixes = 4
	rm.crew[b.AccountID].fixes = 2

	rm.faults = []*fault{
		{kind: faultLeak, row: st[0][0], col: st[0][1], born: tr.Clock, progress: 0.5},
		{kind: faultFire, row: st[2][0], col: st[2][1], born: tr.Clock},
		{kind: faultValve, row: st[4][0], col: st[4][1], born: tr.Clock, seq: []rune{'r', 'e', 'd'}, seqAt: 1, progress: 0.33},
		{kind: faultBreach, row: st[6][0], col: st[6][1], born: tr.Clock},
	}
	rm.core = 38

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
