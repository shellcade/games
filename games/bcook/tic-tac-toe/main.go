// Tic-Tac-Toe — a lean two-player shellcade game: classic noughts and
// crosses on the 80x24 canvas. The first two joiners take X and O (roster
// order); play cells 1-9 on your turn; three in a row wins, a full board
// draws. A player who leaves mid-game forfeits to the other, and a turn left
// idle for 60s forfeits on the wake heartbeat.
//
// This is the native entry point; the wasm exports live in exports.go. The
// game logic is in this same package (game.go) so `go run .` plays it.
//
// Build the artifact with TinyGo (dev profile):
// "tinygo build -opt=1 -no-debug -gc=leaking -o game.wasm -target wasip1 -buildmode=c-shared ."
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
