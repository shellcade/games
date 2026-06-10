// Roulette — a shared-table American double-zero roulette wheel for the
// shellcade arcade. Everyone at the table bets on ONE wheel: a timed betting
// window where each player places chips across the full felt (straight, split,
// street, corner, line, the zero-area trios and top line, plus the dozen/
// column/even-money outside bets), a single shared spin, then a payout beat
// before the next round opens.
//
// A wake-driven port to the shellcade wasm ABI: the betting window, the wheel
// deceleration, and the results hold are deadlines held in guest memory and
// landed in OnWake against CallContext time (no host timer survives a thaw);
// the spin outcome is rolled up front from the room's seeded RNG so a seeded
// room reproduces every result; and the durable bankroll uses the casino kv
// pattern (balance summed, peak max-merged) feeding a peak-ranked leaderboard.
//
// This is the dual-target entrypoint: `go run .` plays it in your terminal with
// normal Go tooling (and `go run . -smoke smoke.yaml -smoke-out shots/` writes
// the preview screens); the wasm artifact is built from the //go:export
// trampolines in exports.go.
//
// Build (dev profile):
//
//	tinygo build -o game.wasm -opt=1 -no-debug -gc=leaking \
//	  -target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
