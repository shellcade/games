// Scratchies — a newsagent of instant scratch-it tickets for the shellcade
// arcade. Each player walks a counter of four price-tier stands ($1/$2/$5/$10),
// buys a themed ticket, and scratches the latex off panel by panel. Sixteen
// tickets ride on four reusable mechanic engines (match-3, key-number,
// multiplier, find-the-symbol); every card's outcome is drawn at purchase from
// the ticket's prize table, so scratching is honest reveal theatre. Money runs
// on the platform casino Credits ABI: one Wager per ticket, one gross-inclusive
// Settle when it resolves, Buyback for the broke-relief rebuy, and a peak
// account-wide "Credits" leaderboard.
//
// Build (dev profile):
//
//	tinygo build -o game.wasm -opt=1 -no-debug -gc=conservative \
//	  -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
