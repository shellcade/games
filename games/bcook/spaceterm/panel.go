package main

import (
	"math/rand"
	"time"
)

// ctrlKind is a widget's species. Every actuation is one keypress; the kinds
// differ only in how their state advances and how they draw.
type ctrlKind uint8

const (
	ckSwitch ctrlKind = iota // OFF/ON toggle
	ckDial                   // 0..dialMax, cycles +1 and wraps
	ckSlider                 // 1..sliderMax, cycles +1 and wraps
	ckButton                 // momentary; orders complete on the press
)

const (
	dialMax   = 4 // dial positions 0..4
	sliderMax = 4 // slider levels 1..4
	fogWipes  = 3 // presses to wipe a coolant-fogged control
)

// control is one widget on a crewmate's panel. Only its owner can actuate it.
type control struct {
	key      rune   // lowercase hotkey (one of panelKeys)
	adj, jot string // the two label lines (adjective, noun [+ suffix])
	kind     ctrlKind
	state    int
	fog      int       // coolant: presses left to wipe (0 = clear)
	litUntil time.Time // brief highlight after a press / order completion
}

// panelKeys is the hotkey block, reading order. 'q' is reserved as Back by
// the canonical vocabulary, so the block sits one column right of WASD-land.
var panelKeys = []rune{'w', 'e', 'r', 't', 's', 'd', 'f', 'g'}

// keysFor returns the hotkeys for an n-control panel laid out two rows deep:
// 6 controls use W E R / S D F, 8 use the full block.
func keysFor(n int) []rune {
	per := n / 2
	out := make([]rune, 0, n)
	out = append(out, panelKeys[:per]...)
	out = append(out, panelKeys[4:4+per]...)
	return out
}

// a balanced kind mix: every panel gets variety, shuffled per sector.
var kindMix = []ctrlKind{ckDial, ckSwitch, ckSlider, ckButton, ckDial, ckSwitch, ckSlider, ckButton}

// genPanel deals a fresh n-control panel, drawing names from the shared pool
// so they stay unique across the whole ship. sector >= 5 sprinkles suffixes.
func genPanel(rng *rand.Rand, used map[string]bool, n, sector int) []control {
	keys := keysFor(n)
	kinds := make([]ctrlKind, len(kindMix))
	copy(kinds, kindMix)
	rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })

	panel := make([]control, n)
	for i := 0; i < n; i++ {
		adj, jot := pickName(rng, used, sector)
		c := control{key: keys[i], adj: adj, jot: jot, kind: kinds[i]}
		switch c.kind {
		case ckDial:
			c.state = rng.Intn(dialMax + 1)
		case ckSlider:
			c.state = 1 + rng.Intn(sliderMax)
		case ckSwitch:
			c.state = rng.Intn(2)
		}
		panel[i] = c
	}
	return panel
}

func pickName(rng *rand.Rand, used map[string]bool, sector int) (string, string) {
	for {
		adj := adjectives[rng.Intn(len(adjectives))]
		jot := nouns[rng.Intn(len(nouns))]
		if sector >= 5 && rng.Intn(4) == 0 {
			suf := nameSuffixes[rng.Intn(len(nameSuffixes))]
			if len(jot)+len(suf) <= 17 {
				jot = jot + suf
			}
		}
		full := adj + " " + jot
		if used[full] {
			continue
		}
		used[full] = true
		return adj, jot
	}
}

// actuate advances the control one press. A fogged control eats the press as
// a wipe instead. Reports whether the control's state actually moved (button
// presses count as moved — orders complete on them).
func (c *control) actuate(now time.Time) bool {
	if c.fog > 0 {
		c.fog--
		return false
	}
	c.litUntil = now.Add(450 * time.Millisecond)
	switch c.kind {
	case ckSwitch:
		c.state = 1 - c.state
	case ckDial:
		c.state = (c.state + 1) % (dialMax + 1)
	case ckSlider:
		c.state = c.state%sliderMax + 1
	case ckButton:
		// momentary: state stays 0, the press itself is the event
	}
	return true
}

// statesOf returns the demandable state range for order generation:
// lo..hi inclusive (buttons have none — they demand a press, want = -1).
func (c *control) statesOf() (lo, hi int) {
	switch c.kind {
	case ckSwitch:
		return 0, 1
	case ckDial:
		return 0, dialMax
	case ckSlider:
		return 1, sliderMax
	}
	return 0, -1
}
