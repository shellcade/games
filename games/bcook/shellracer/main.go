// Shell Racer — a multiplayer typing race ported to the shellcade wasm game ABI.
// This is the thin entrypoint; the game logic lives in the importable
// ./shellracer package so kittest can drive the Handler directly.
//
// Build the artifact:
//
//	tinygo build -o game.wasm -opt=1 -no-debug -gc=leaking -target wasip1 -buildmode=c-shared .
package main

import (
	kit "github.com/shellcade/kit/v2"

	"shellracer.shellcade.example/shellracer"
)

func main() { kit.Main(shellracer.Game{}) }
