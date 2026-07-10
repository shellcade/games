package main

import kit "github.com/shellcade/kit/v2"

// The rotating dealer crew: every shoe belongs to one dealer, and the two
// retire together. A busy table reaches the cut card well inside the hand cap;
// a slow (heads-up) shoe hits the cap first. Either way the swap lands at the
// next betting window — as on a real floor, where dealers rotate off on a
// schedule and the incoming dealer starts from a fresh shuffle.
const handsPerDealer = 20

// dealerNames is the dealer roster, cycled in order from a seeded random
// start — star names, for The Star's table. Names stay <= 6 letters so the
// spaced-caps nameplate centred over the dealer's cards never crowds the
// (wide, 30-col) left rules signage.
var dealerNames = [...]string{"Vega", "Nova", "Orion", "Luna", "Rigel", "Stella", "Cass", "Astra"}

// dealerName is the dealer currently working the table.
func (rm *room) dealerName() string { return dealerNames[rm.dealerIdx] }

// needsNewDealer reports whether the current dealer's shoe is done: the cut
// card was reached (or a drained round forced a recycle), or the dealer has
// dealt their full shift of hands from a slow-burning shoe.
func (rm *room) needsNewDealer() bool {
	return rm.sh.needsReshuffle() || rm.handsThisShoe >= handsPerDealer
}

// rotateDealer retires the current dealer along with the spent shoe and seats
// the next one off the roster with a freshly shuffled shoe. The note announces
// the change on the felt through the betting window; the incoming dealer's
// first deal clears it.
func (rm *room) rotateDealer(r kit.Room) {
	prev := rm.dealerName()
	rm.dealerIdx = (rm.dealerIdx + 1) % len(dealerNames)
	rm.sh.shuffle(r.Rand())
	rm.handsThisShoe = 0
	rm.dealerNote = prev + " steps away - " + rm.dealerName() + " takes the shoe"
}
