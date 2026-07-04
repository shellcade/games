// Blackjack Challenge — a no-winner, social multiplayer blackjack table on
// the 80x24 shellcade canvas: one shared auto-dealer, up to five seats, and
// rounds that loop while anyone is seated. The dealer's up card is face-up
// and stands on all 17 (S17); a tie hand loses instead of pushing, and a
// player blackjack is ranked, paying 2:1 up to 5:1 depending on its suits.
// Bet, hit, stand, double, and split; the round also auto-wins/auto-loses a
// hand outright without a turn where the outcome is already decided, and a
// Star Pairs side bet pays out on the first two cards dealt. Leaving with a
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
