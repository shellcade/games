package main

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
