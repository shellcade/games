package main

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// TestSeatCharacterRendersBesideName verifies a seated player's character tile
// lands between their colour swatch and their name (one cell + one space).
func TestSeatCharacterRendersBesideName(t *testing.T) {
	p := kittest.Player("p1")
	p.Character = kit.Character{Glyph: "λ", InkR: 0x39, InkG: 0xFF, InkB: 0x14, BgR: 0x2D, BgG: 0x1B, BgB: 0x4E, Fallback: 'L'}
	r := kittest.NewRoom(p)
	rm, ok := Game{}.NewRoom(r.Config(), r.Services()).(*room)
	if !ok {
		t.Fatal("NewRoom did not return *room")
	}
	rm.OnStart(r)
	rm.OnJoin(r, p)

	f := r.LastFrame(p)
	if f == nil {
		t.Fatal("no frame sent")
	}
	x := -1
	for c := seatLeft; c <= seatRight; c++ {
		if f.Cells[seatsRow][c].Rune == '*' {
			x = c
			break
		}
	}
	if x < 0 {
		t.Fatalf("no seat swatch on the seats row: %q", kittest.String(f, seatsRow))
	}
	if got, want := f.Cells[seatsRow][x+1], kit.CharacterCell(p.Character); got != want {
		t.Errorf("cell after swatch = %+v, want character tile %+v", got, want)
	}
	if f.Cells[seatsRow][x+2].Rune != ' ' {
		t.Error("no space between character tile and name")
	}
	if f.Cells[seatsRow][x+3].Rune != 'p' || f.Cells[seatsRow][x+4].Rune != '1' {
		t.Errorf("name not beside the tile: %q", kittest.String(f, seatsRow))
	}
}

// TestWinnerHighlightTiming pins the reveal sequence: dark through the spin and
// the 1s pause after the ball lands, flashing on/off until settlement, then
// solid through the results board.
func TestWinnerHighlightTiming(t *testing.T) {
	rm := newRoom(kit.RoomConfig{}, kit.Services{})
	rm.spinStart = time.Unix(1_000_000, 0)

	rm.phase = phBetting
	if rm.winnerHighlightOn() {
		t.Error("highlight on during betting")
	}

	rm.phase = phSpinning
	at := func(sinceLanded time.Duration) bool {
		rm.lastNow = rm.spinStart.Add(spinAnimDur + sinceLanded)
		return rm.winnerHighlightOn()
	}
	if at(-time.Second) {
		t.Error("highlight on while the ball is still rolling")
	}
	if at(flashDelay - time.Millisecond) {
		t.Error("highlight on during the post-landing pause")
	}
	if !at(flashDelay) {
		t.Error("flash not on at the first half-cycle")
	}
	if at(flashDelay + flashPeriod) {
		t.Error("flash not off at the second half-cycle")
	}
	if !at(flashDelay + 2*flashPeriod) {
		t.Error("flash not on again at the third half-cycle")
	}

	rm.phase = phResults
	if !rm.winnerHighlightOn() {
		t.Error("highlight not solid at results")
	}
}

// TestChipPaletteCoversTable guards the palette against a MaxPlayers bump: every
// seated player must get a distinct chip colour.
func TestChipPaletteCoversTable(t *testing.T) {
	if max := (Game{}).Meta().MaxPlayers; len(chipColors) < max {
		t.Fatalf("chip palette has %d colours for %d seats", len(chipColors), max)
	}
}

func TestRoundSummary(t *testing.T) {
	cases := []struct {
		won, staked int
		want        string
	}{
		{600, 400, "up 200  (bet 400, back 600)"},
		{170, 200, "down 30  (bet 200, back 170)"},
		{0, 600, "down 600  (bet 600, back 0)"},
		{400, 400, "even  (bet 400, back 400)"},
		{0, 0, ""}, // sat the round out
	}
	for _, c := range cases {
		if got := roundSummary(c.won, c.staked); got != c.want {
			t.Errorf("roundSummary(%d,%d) = %q, want %q", c.won, c.staked, got, c.want)
		}
	}
}

func masterOf(t *testing.T, k betKind, label string) int {
	t.Helper()
	for i, b := range masterBets {
		if b.kind == k && b.label == label {
			return i
		}
	}
	t.Fatalf("no %s bet labelled %q", k.name(), label)
	return -1
}

// TestChipPositions pins where chips render for representative bets so the
// markers stay aligned with the felt (and the street/split chip stays centred
// on its line rather than drifting to the left grid line).
func TestChipPositions(t *testing.T) {
	// Straight on 17: the cell's left (chip) slot.
	if row, col := chipPos(17); row != rowOfRR(1) || col != colInterior(5) {
		t.Errorf("straight 17 chip at (%d,%d), want (%d,%d)", row, col, rowOfRR(1), colInterior(5))
	}
	// Straight on 0 / 00: in the chip slot left of the digit, never on it.
	if row, col := chipPos(0); row != zeroRow(0) || col >= zeroTextCol(0) {
		t.Errorf("0 chip at (%d,%d) not left of its digit at col %d", row, col, zeroTextCol(0))
	}
	dz := masterOf(t, kStraight, "00")
	if row, col := chipPos(dz); row != zeroRow(doubleZero) || col >= zeroTextCol(doubleZero) {
		t.Errorf("00 chip at (%d,%d) not left of its digits at col %d", row, col, zeroTextCol(doubleZero))
	}
	// Street 16-18 sits on the outer (bottom) edge of column 5, centred in the
	// cell's line segment — not on the left grid line.
	st := masterOf(t, kStreet, "Str 16-18")
	if row, col := chipPos(st); row != gridTop+6 || col != colInterior(5)+iw/2 {
		t.Errorf("street 16-18 chip at (%d,%d), want (%d,%d)", row, col, gridTop+6, colInterior(5)+iw/2)
	}
	// Corner 17-21 sits on the intersection between columns 5 and 6.
	cn := masterOf(t, kCorner, "Cnr 17-21")
	if row, col := chipPos(cn); row != gridTop+2 || col != lineCol(6) {
		t.Errorf("corner 17-21 chip at (%d,%d), want (%d,%d)", row, col, gridTop+2, lineCol(6))
	}
	// A horizontal split (17-20) sits on the vertical line right of 17.
	sp := masterOf(t, kSplit, "17-20")
	if row, col := chipPos(sp); row != rowOfRR(1) || col != lineCol(6) {
		t.Errorf("split 17-20 chip at (%d,%d), want (%d,%d)", row, col, rowOfRR(1), lineCol(6))
	}
}
