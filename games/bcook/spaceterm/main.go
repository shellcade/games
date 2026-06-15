// Spaceterm is frantic co-op bridge duty for the terminal: every crewmate
// stares at a panel of absurdly named controls, orders stream in that usually
// refer to controls on someone ELSE's panel, and the crew shouts (or hails)
// across the void to keep the hull in one piece long enough to clear another
// sector. An original homage to the party game Spaceteam, rebuilt for 80x24.
//
// Build (arcade artifact):
//
//	tinygo build -opt=2 -no-debug -gc=leaking \
//	  -o spaceterm.wasm -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .              # solo shift in your terminal
//	go run . -seats 3     # hot-seat a crew (Ctrl-T switches the active seat)
//	go test ./...         # logic + allocation tests
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
