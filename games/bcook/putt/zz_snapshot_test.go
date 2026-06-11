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
	rm, tr := newTestRoom(t, "alice", "bob")
	a, b := tr.Players[0], tr.Players[1]
	rm.OnJoin(tr, a)
	rm.OnJoin(tr, b)

	// Pose hole 7 (windmill) with the viewer charging a putt, the spinning arm
	// part-way round, and a rival ghost bogged down in... well, on the fairway,
	// for a lively frame that exercises the windmill, aim line, and ghost ball.
	rm.holeIdx = 6
	rm.placeAtTee(rm.golfers[a.AccountID])
	rm.placeAtTee(rm.golfers[b.AccountID])
	rm.hub = 0.7 // turn the arm off-axis so it reads as a diagonal blade
	ga := rm.golfers[a.AccountID]
	ga.state = stateCharge
	ga.power = 0.55
	ga.strokes = 1
	gb := rm.golfers[b.AccountID]
	gb.x, gb.y = 40, 12

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

// printFrame renders one composed frame as a plain rune grid.
func printFrame(f *kit.Frame) string {
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
	return sb.String()
}

// TestSnapshotHazards prints hole 8 (Island Green) so the sand (`:`) and water
// (`~`) hazards are visible in a readable frame. Run with:
//
//	go test -run TestSnapshotHazards -v
func TestSnapshotHazards(t *testing.T) {
	rm, tr := newTestRoom(t, "alice")
	a := tr.Players[0]
	rm.OnJoin(tr, a)
	rm.holeIdx = 7 // Island Green: water moat + a sand core
	rm.placeAtTee(rm.golfers[a.AccountID])
	g := rm.golfers[a.AccountID]
	// Park the ball just outside the moat with a charged aim line, mid-shot.
	g.state = stateAim
	g.x, g.y = 9, 7

	f := kit.NewFrame()
	rm.composeFor(f, a)
	fmt.Print(printFrame(f))
}
