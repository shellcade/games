package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
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

	// The dealer's reveal is paced far more deliberately than the initial deal
	// sweep: players need to watch the hole card turn over and read each hit as
	// it lands, one card at a time, rather than the cards flashing in back to
	// back at dealStagger.
	//
	//   - holeRevealDelay is the lead-in beat after the dealer's turn begins and
	//     before the hole card starts to turn, so the reveal reads as the dealer
	//     pausing then flipping the card rather than it snapping over the instant
	//     play ends. The card sits face down through this beat, then flips.
	//   - holeRevealHold is the beat the table holds on the just-turned hole card
	//     (the dealer's two-card total) before the first hit slides in.
	//   - dealerDrawGap is the pause between one hit fully landing and the next
	//     beginning to slide — the dealer reading the table and deciding to draw
	//     again. Far longer than dealStagger so hits never overlap.
	//   - dealerDoneHold is the final beat on the completed dealer hand (its made
	//     total, blackjack, or bust) before the round settles to results.
	holeRevealDelay = 500 * time.Millisecond
	holeRevealHold  = 700 * time.Millisecond
	dealerDrawGap   = 450 * time.Millisecond
	dealerDoneHold  = 800 * time.Millisecond
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
