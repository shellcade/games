// Pokies — a no-winner social slot machine for the shellcade arcade. Each
// player gets their own 3-reel cabinet; cabinets render side-by-side on the
// fixed 80x24 canvas. A wake-driven port of the native pokies game to the
// shellcade wasm ABI: the reel animation is a clock derived from CallContext
// time, reel landings are one-shot deadlines held in guest memory, odds are
// admin-tunable via config, and the durable wallet uses the casino kv pattern
// (balance summed, peak max-merged) with a peak-ranked leaderboard.
//
// Build (dev profile):
//
//	tinygo build -o game.wasm -opt=1 -no-debug -gc=leaking \
//	  -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
