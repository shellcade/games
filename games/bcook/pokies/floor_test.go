package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
)

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

func TestBuildLoungeShape(t *testing.T) {
	fm, machines := buildLounge()
	if fm.w != loungeW || fm.h != loungeH {
		t.Fatalf("size = %dx%d, want %dx%d", fm.w, fm.h, loungeW, loungeH)
	}
	if fm.w <= kit.Cols { // must exceed the viewport to force horizontal scroll
		t.Fatalf("lounge width %d must exceed viewport %d", fm.w, kit.Cols)
	}
	for x := 0; x < fm.w; x++ {
		if fm.at(x, 0) != tileWall || fm.at(x, fm.h-1) != tileWall {
			t.Fatalf("top/bottom border not wall at x=%d", x)
		}
	}
	if len(machines) != 6 {
		t.Fatalf("machines = %d, want 6", len(machines))
	}
	for _, mc := range machines {
		if fm.walkable(mc.mx, mc.my) {
			t.Errorf("machine %q icon tile should block movement", mc.name)
		}
		if !fm.walkable(mc.ax, mc.ay) {
			t.Errorf("machine %q approach tile must be walkable", mc.name)
		}
	}
	sx, sy := loungeSpawn()
	if !fm.walkable(sx, sy) {
		t.Error("spawn must be walkable")
	}
}
