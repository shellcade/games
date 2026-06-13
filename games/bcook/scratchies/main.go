// Scratchies — a newsagent of instant scratch-it tickets for the shellcade
// arcade. Each player walks a counter of four price-tier stands ($1/$2/$5/$10),
// buys a themed ticket, and scratches the latex off panel by panel. Sixteen
// tickets ride on four reusable mechanic engines (match-3, key-number,
// multiplier, find-the-symbol); every card's outcome is drawn at purchase from
// the ticket's prize table, so scratching is honest reveal theatre. The durable
// wallet uses the casino kv pattern (balance summed, peak max-merged) with a
// peak-ranked "Credits" leaderboard and a rebuy-to-1000 safety net.
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
