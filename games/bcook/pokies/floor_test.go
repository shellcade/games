package main

import "testing"

func TestFloorMapWalkable(t *testing.T) {
	fm := &floorMap{w: 4, h: 3, tiles: []tile{
		tileWall, tileWall, tileWall, tileWall,
		tileWall, tileFloor, tileFloor, tileWall,
		tileWall, tileWall, tileEntrance, tileWall,
	}}
	if !fm.walkable(1, 1) {
		t.Error("interior floor should be walkable")
	}
	if !fm.walkable(2, 2) {
		t.Error("entrance should be walkable")
	}
	if fm.walkable(0, 0) {
		t.Error("wall is not walkable")
	}
	if fm.walkable(-1, 1) || fm.walkable(4, 1) || fm.walkable(1, 3) {
		t.Error("out of bounds is not walkable")
	}
}
