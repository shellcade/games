// boneyard — a persistent shared ASCII dungeon for the shellcade arcade.
// Everyone on the arcade delves the SAME weekly dungeon; when you die, your
// corpse, gear, and last words stay in the world for the next delver to find.
// One resident room per week; the dungeon collapses and regenerates Mondays.
package main

import kit "github.com/shellcade/kit/v2"

func main() { kit.Main(Game{}) }
