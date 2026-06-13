package main

import "math/rand"

// MechanicKind identifies which engine renders & resolves a ticket.
type MechanicKind int

const (
	MechMatch3 MechanicKind = iota // mech_match3.go
	MechKeyNum                     // mech_keynum.go
	MechMult                       // mech_mult.go
	MechFind                       // mech_find.go
)

// PrizeRow is one prize tier: Credits won at probability 1/OneIn per card.
type PrizeRow struct {
	Credits int
	OneIn   int
}

// PrizeTable is a ticket's full prize ladder. The sum of 1/OneIn is the any-win
// probability; the remainder is "no win". Rows are ascending in Credits.
type PrizeTable []PrizeRow

// Ticket is one buyable scratch-it: a pure data record. The catalog (catalog.go)
// is a slice of these; new tickets are new rows.
type Ticket struct {
	Slug     string
	Name     string
	Price    int // credits (= dollars 1:1): 1, 2, 5, 10
	Mechanic MechanicKind
	Theme    Theme
	Cols     int // grid width (<=6)
	Rows     int // grid height; Cols*Rows panels for grid mechanics
	Prizes   PrizeTable

	// Mechanic-specific knobs (ignored by mechanics that don't use them):
	WinNumbers int    // key-number: count of winning numbers
	HasBust    bool   // find-symbol: a BUST panel can end a losing card
	Symbol     string // find-symbol: target glyph label (e.g. "CROC")
	MaxMult    int    // multiplier: top multiplier (3,5,10,20)
}

// Outcome is the predetermined result drawn at purchase, before any scratch.
type Outcome struct {
	Win int // credits won (0 = no win)
}

// drawOutcome rolls one card's result from the prize table using rng. It walks
// the rows once; the first row whose 1/OneIn roll hits wins that prize.
func drawOutcome(t *Ticket, rng *rand.Rand) Outcome {
	for _, row := range t.Prizes {
		if row.OneIn > 0 && rng.Intn(row.OneIn) == 0 {
			return Outcome{Win: row.Credits}
		}
	}
	return Outcome{Win: 0}
}

// Card is what a Mechanic produces: a playable, scratchable instance. The room
// owns exactly one Card at a time.
type Card interface {
	Title() string            // header text, e.g. "LUCKY 7s · $1 · match three"
	Prompt() string           // running hint, e.g. "two $5 so far"
	Move(dx, dy int)          // move the coin cursor (and scroll the viewport)
	Scratch() (revealed bool) // rub the focused panel one layer; true if it just revealed
	ScratchAll()              // wear every panel through, then resolve
	Resolved() bool           // has the card settled?
	Win() int                 // credits won (valid once Resolved)
	Render(f *Frame, top int) // draw the card body starting at frame row `top`
}

// buildFn constructs a Card for a ticket from its drawn outcome.
type buildFn func(t *Ticket, out Outcome, rng *rand.Rand) Card

// builders is populated by each mechanic file's init().
var builders = map[MechanicKind]buildFn{}

// buildCard draws the outcome and dispatches to the registered engine.
func buildCard(t *Ticket, rng *rand.Rand) Card {
	out := drawOutcome(t, rng)
	b := builders[t.Mechanic]
	if b == nil {
		// Should never happen once all four engines register; fail safe.
		return newGenericGridCard(t, out, rng)
	}
	return b(t, out, rng)
}
