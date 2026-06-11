// Salvo is turn-based tank artillery for the terminal: lob shells over rolling,
// destructible hills, read the wind, and blow your rivals off the map. Aim the
// angle, dial the power, pick a weapon, and fire — the shell arcs across the
// battlefield, craters the ground, and chips health off anyone in the blast.
// Last tank standing wins. Play solo against the CPU or share the field over SSH.
//
// Build (arcade artifact):
//
//	tinygo build -opt=2 -no-debug -gc=leaking \
//	  -o salvo.wasm -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .              # play it in your terminal (vs CPU)
//	go run . -seats 2     # hot-seat two players (Ctrl-T switches the active seat)
//	go test ./...         # logic + allocation tests
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
