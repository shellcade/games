package main

import (
	"math/rand"

	kit "github.com/shellcade/kit/v2"
)

// gamble.go is the double-up ladder: after a base-game line win the player may
// gamble the win, guessing the dealt card's colour (Red/Black, ×2) or its suit
// (×4). A correct guess advances the ladder (up to the variant's caps); a wrong
// guess forfeits the held win. The deal is fair (P(colour)=1/2, P(suit)=1/4), so
// the feature is RTP-neutral and does not affect the machine's RTP gate.

// suit indices; hearts/diamonds are red, spades/clubs black. Rendered as letters
// (S/H/D/C) — the Unicode suit glyphs ♠♥♦♣ are ambiguous-width and would desync
// the fixed cabinet layout, the same hazard the fullwidth-7 choice avoids.
const (
	suitSpades = iota
	suitHearts
	suitDiamonds
	suitClubs
)

// selector option indices. Navigation is linear (Left/Right and Up/Down cycle
// through all options); the options render over two rows in the cabinet.
const (
	selTake = iota
	selRed
	selBlack
	selSpades
	selHearts
	selDiamonds
	selClubs
	selCount
)

// gambleState holds an at-risk win on the double-up ladder.
type gambleState struct {
	atRisk int  // current win being gambled
	rungs  int  // doubles taken so far
	sel    int  // highlighted selector option (sel*)
	card   int  // last revealed suit (-1 = face down)
	last   bool // whether the last revealed guess won (for the reveal flash)
}

// dealCardSuit returns a uniformly random suit (0..3).
func dealCardSuit(rng *rand.Rand) int { return rng.Intn(4) }

// suitIsRed reports whether a suit is red (hearts or diamonds).
func suitIsRed(s int) bool { return s == suitHearts || s == suitDiamonds }

// suitOf maps a suit selector option to its suit index.
func suitOf(sel int) int { return sel - selSpades }

// enterGamble holds a base-game win at risk and opens the ladder, highlighting
// TAKE so an accidental confirm banks the win rather than risking it.
func (rm *room) enterGamble(r kit.Room, m *machine, win int) {
	m.gamble = &gambleState{atRisk: win, sel: selTake, card: -1}
}

// gambleInput moves the selector or confirms (called from OnInput while a gamble
// is active). Navigation is linear across all options.
func (rm *room) gambleInput(r kit.Room, id string, act kit.Action) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	switch act {
	case kit.ActLeft, kit.ActUp:
		m.gamble.sel = (m.gamble.sel + selCount - 1) % selCount
	case kit.ActRight, kit.ActDown:
		m.gamble.sel = (m.gamble.sel + 1) % selCount
	case kit.ActConfirm:
		rm.gambleConfirm(r, id)
	}
}

// gambleConfirm acts on the highlighted option: TAKE banks the win; a guess deals
// a card and resolves.
func (rm *room) gambleConfirm(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	if m.gamble.sel == selTake {
		rm.takeWin(r, id)
		return
	}
	rm.resolveGuess(r, id, dealCardSuit(r.Rand()))
}

// resolveGuess settles the highlighted guess against a dealt suit: a correct
// Red/Black doubles (×2), a correct Suit quadruples (×4); a wrong guess forfeits
// the held win. A win advances the ladder, auto-taking at a cap.
func (rm *room) resolveGuess(r kit.Room, id string, suit int) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	g := m.gamble
	g.card = suit
	win, mult := false, 0
	switch g.sel {
	case selRed:
		win, mult = suitIsRed(suit), 2
	case selBlack:
		win, mult = !suitIsRed(suit), 2
	case selSpades, selHearts, selDiamonds, selClubs:
		win, mult = suit == suitOf(g.sel), 4
	}
	g.last = win
	if !win {
		m.gamble = nil
		m.flash = "GAMBLED AWAY"
		m.flashUntil = r.Now().Add(flashDur)
		rm.creditWin(r, id, 0, true) // rebuy check; nothing credited
		return
	}
	g.atRisk *= mult
	g.rungs++
	gc := rm.gambleCap(m)
	if g.rungs >= gc.MaxRungs || g.atRisk >= gc.MaxWin {
		rm.takeWin(r, id)
	}
}

// takeWin banks the at-risk win through the normal credit path (peak, leaderboard,
// big-win ticker), then clears the gamble.
func (rm *room) takeWin(r kit.Room, id string) {
	m := rm.machines[id]
	if m == nil || m.gamble == nil {
		return
	}
	win := m.gamble.atRisk
	m.gamble = nil
	rm.creditWin(r, id, win, true)
	if win >= m.bet*tickerMult {
		rm.announce(r, id, win)
	}
}

// gambleCap returns the ladder caps of the variant the held win was won under,
// falling back to the compiled defaults.
func (rm *room) gambleCap(m *machine) gambleConfig {
	if m.lastVar != nil {
		return m.lastVar.gamble
	}
	return defaultGamble
}
