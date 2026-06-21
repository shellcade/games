package main

import kit "github.com/shellcade/kit/v2"

// tile is one cell of the lounge floor.
type tile byte

const (
	tileFloor    tile = ' '
	tileWall     tile = '#'
	tileEntrance tile = 'E'
)

// floorMap is the static lounge layout: a row-major grid of tiles.
type floorMap struct {
	w, h  int
	tiles []tile
}

func (fm *floorMap) at(x, y int) tile {
	if x < 0 || y < 0 || x >= fm.w || y >= fm.h {
		return tileWall
	}
	return fm.tiles[y*fm.w+x]
}

// walkable reports whether a pawn may occupy (x,y): in bounds and not a wall.
func (fm *floorMap) walkable(x, y int) bool {
	t := fm.at(x, y)
	return t == tileFloor || t == tileEntrance
}

// Lounge dimensions — wider and taller than the 80x24 viewport so the camera
// scrolls in both axes.
const (
	loungeW = 96
	loungeH = 36
)

// floorMachine is a placed cabinet on the floor: an icon tile you cannot walk
// onto, and the approach tile you sit from.
type floorMachine struct {
	id     int
	name   string
	mx, my int // icon tile (blocks movement)
	ax, ay int // approach tile (walk here, press Confirm to sit)
}

func loungeSpawn() (int, int) { return loungeW / 2, loungeH - 3 }

// buildLounge constructs the static lounge: a bordered room with an entrance gap
// at the bottom centre and six named machines along the top wall, each with an
// approach tile one row below its icon.
func buildLounge() (*floorMap, []floorMachine) {
	tiles := make([]tile, loungeW*loungeH)
	for y := 0; y < loungeH; y++ {
		for x := 0; x < loungeW; x++ {
			t := tileFloor
			if x == 0 || y == 0 || x == loungeW-1 || y == loungeH-1 {
				t = tileWall
			}
			tiles[y*loungeW+x] = t
		}
	}
	fm := &floorMap{w: loungeW, h: loungeH, tiles: tiles}

	names := []string{"LUCKY 7s", "GEM RUSH", "BELLS", "CHERRY POP", "CROWN", "GIFT DROP"}
	machines := make([]floorMachine, len(names))
	for i, name := range names {
		mx := (i + 1) * loungeW / (len(names) + 1)
		machines[i] = floorMachine{id: i, name: name, mx: mx, my: 2, ax: mx, ay: 3}
		fm.tiles[2*loungeW+mx] = tileWall // icon tile blocks movement
	}
	return fm, machines
}

// Viewport: the floor draws into rows [vpTop, vpTop+vpH) over all vpW columns.
// Row 0 is the title, the last row is controls/status.
const (
	vpW   = kit.Cols // 80
	vpTop = 1
	vpH   = kit.Rows - 2 // 22 (rows 1..22)
)

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cameraOrigin returns the top-left map coordinate of the viewport so it is
// centred on (px,py) and clamped to the map bounds.
func cameraOrigin(px, py int) (int, int) {
	ox := clampi(px-vpW/2, 0, loungeW-vpW)
	oy := clampi(py-vpH/2, 0, loungeH-vpH)
	return ox, oy
}
