package main

import (
	"time"

	kit "github.com/shellcade/kit"
)

// Card dealing/reveal animation. Animations are purely cosmetic: every card's
// rank, suit and landing slot are fixed up front from the seeded shoe before any
// animation starts, and a `cardAnim` only carries room-clock timings that the
// compose pass interpolates from r.Now(). Nothing here ever consults the room
// RNG, so a seeded room reproduces every deal regardless of frame timing, a
// frame composed after an animation's end renders exactly the settled card, and
// a hibernation freeze/thaw replays identically (the schedule and the clock both
// live in guest memory / CallContext).
const (
	// dealStagger is the delay between successive cards starting their slide, so
	// the initial deal reads as a sweep around the table rather than a flash.
	dealStagger = 120 * time.Millisecond
	// slideDur is how long a card takes to glide from the right felt edge to its
	// slot, and flipDur is the back -> edge -> face reveal that follows.
	slideDur = 260 * time.Millisecond
	flipDur  = 240 * time.Millisecond
	// holePause is the one-beat hold before the dealer draws, covering the
	// hole-card flip on reveal.
	holePause = 600 * time.Millisecond
)

// animKind distinguishes where an animated card lands.
type animKind uint8

const (
	animSeat   animKind = iota // a player's hand card
	animDealer                 // a dealer row card
)

// cardAnim is one card's cosmetic schedule. The card itself already sits in the
// authoritative hand; this record only says when and from where it animates in.
//
//   - slide:  slideStart .. slideStart+slideDur  (right edge -> slot)
//   - flip:   flipStart  .. flipStart+flipDur     (back -> edge -> face); zero
//     flipStart means "no reveal flip" (the concealed dealer hole card until it
//     is turned over, which schedules its own flip).
type cardAnim struct {
	kind    animKind
	player  kit.Player // seat owner (zero for the dealer)
	handIdx int        // which split hand (0 for the dealer / unsplit)
	cardIdx int        // index within that hand

	slideStart time.Time
	flipStart  time.Time
}

// slideProgress returns how far the slide has advanced in [0,1] at now; 1 means
// the card has reached its slot.
func (a cardAnim) slideProgress(now time.Time) float64 {
	if a.slideStart.IsZero() {
		return 1
	}
	d := now.Sub(a.slideStart)
	if d <= 0 {
		return 0
	}
	if d >= slideDur {
		return 1
	}
	return float64(d) / float64(slideDur)
}

// flipFrame returns the reveal frame at now: 0 = back, 1 = edge, 2 = face, and
// reports whether a flip is still in progress (a settled card returns 2,false).
func (a cardAnim) flipFrame(now time.Time) (frame int, flipping bool) {
	if a.flipStart.IsZero() {
		return 2, false // no reveal scheduled (e.g. concealed hole card)
	}
	d := now.Sub(a.flipStart)
	switch {
	case d < 0:
		return 0, true // card has landed but not yet begun to turn
	case d < flipDur/3:
		return 0, true
	case d < 2*flipDur/3:
		return 1, true
	case d < flipDur:
		return 2, true
	default:
		return 2, false
	}
}

// settled reports whether the whole animation (slide + any flip) has finished by
// now, so a compose pass can draw the card exactly as the static layout would.
func (a cardAnim) settled(now time.Time) bool {
	if a.slideProgress(now) < 1 {
		return false
	}
	if _, flipping := a.flipFrame(now); flipping {
		return false
	}
	return true
}

// endsAt is the room-clock instant this card is fully settled.
func (a cardAnim) endsAt() time.Time {
	end := a.slideStart.Add(slideDur)
	if !a.flipStart.IsZero() {
		if fe := a.flipStart.Add(flipDur); fe.After(end) {
			end = fe
		}
	}
	return end
}
