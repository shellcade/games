// Bytebreaker is a neon brick-breaker for the terminal: bounce a bit off your
// paddle to smash a wall of glowing bytes, catch the powerups they drop, and
// chase a high score across faster and faster levels. Each player runs their
// own board; with friends at the table it's a race for the top of the board.
//
// Build (arcade artifact):
//
//	tinygo build -opt=2 -no-debug -gc=leaking \
//	  -o bytebreaker.wasm -target wasip1 -buildmode=c-shared .
//
// Native dev loop:
//
//	go run .              # play it in your terminal
//	go run . -seats 2     # hot-seat two players (Ctrl-T switches the active seat)
//	go test ./...         # logic + allocation tests
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
