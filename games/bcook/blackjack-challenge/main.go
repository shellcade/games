// Blackjack — a no-winner, social multiplayer blackjack table on the 80x24
// shellcade canvas: one shared auto-dealer, up to five seats, and rounds that
// loop while anyone is seated. Bet, hit, stand, double, split, surrender, and
// take insurance; the dealer stands on all 17 (S17) and blackjack pays 3:2.
// Chips are integers: an odd bet's half-chip on the 3:2 payout and on a
// surrender return rounds UP to the player; the insurance stake is half the
// bet rounded down and pays exactly 2:1 on what was staked. Leaving with a
// live hand forfeits its stake. Stakes are the platform's account-wide Credits
// (kit v2.16.0 casino ABI): every bet Wagers onto the seat's single open stake
// and the round Settles that stake exactly once with the gross payout; the host
// owns every balance and the board ranks your peak credits.
//
// The wasm ABI has no timers, ticks, or phases: every "later…" here is a
// deadline held in guest memory and checked against r.Now() inside OnWake (the
// host heartbeat). Card-dealing animation is a cosmetic schedule of timestamps
// the renderer interpolates from r.Now(); the authoritative cards are fixed up
// front from the room-seeded shoe, so a hibernation freeze/thaw and a -seed run
// both reproduce every deal.
//
// This is the native entry point; the wasm exports live in exports.go. The game
// logic shares this package so `go run .` plays it.
//
//	Build (artifact): tinygo build -opt=1 -no-debug -gc=conservative \
//	  -o game.wasm -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
