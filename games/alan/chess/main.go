// Chess — a strict two-player, turn-alternating duel on a self-contained,
// perft-verified rules engine, ported to the shellcade wasm ABI. The game layer
// adds pairing, seeded colour assignment, a wake-driven blitz clock, cursor move
// entry, draw/resign/abandon handling, and end detection, plus per-viewer ASCII
// rendering oriented so the viewer's colour sits at the bottom.
//
// This is the dual-target entrypoint: `go run .` plays it in your terminal with
// normal Go tooling; the wasm artifact is built from the //go:export trampolines
// in exports.go.
//
// Build: tinygo build -o chess.wasm -opt=1 -no-debug -gc=leaking \
//
//	-target wasip1 -buildmode=c-shared .
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
