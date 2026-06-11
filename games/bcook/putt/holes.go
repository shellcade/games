package main

// Hole geometry and the nine handcrafted courses.
//
// A hole is authored as ASCII art: courseH (22) rows of up to `cols` (80)
// characters, parsed into a tile grid at load. Short rows are padded with the
// out-of-bounds tile, so the art only needs to draw what matters.
//
// Tile legend (in the art strings):
//
//	space  fairway   — smooth green, normal rolling
//	#      wall       — solid rail; the ball bounces off it
//	= - | /\ +        — wall geometry too (drawn as-is, all bounce)
//	:      sand       — bunker; heavy friction
//	~      water      — hazard; stroke penalty + reset to the pre-shot spot
//	T      tee        — the ball's spawn (rendered as fairway)
//	H      cup        — the hole (sink the ball here)
//	*      windmill hub — anchor for a spinning arm (drawn dynamically)
//
// Everything outside the drawn art is also wall, so a ball can never escape.

type tile uint8

const (
	tileWall    tile = iota // solid; bounces
	tileFairway             // rollable green
	tileSand                // heavy friction
	tileWater               // hazard
)

// hole is a parsed, ready-to-play course.
type hole struct {
	name     string
	par      int
	tiles    [courseH][cols]tile
	teeX     float64
	teeY     float64
	cupX     int
	cupY     int
	windmill *windmillSpec // nil if the hole has none
}

// windmillSpec is a hub with a rotating arm of solid cells. The arm sweeps the
// course; a ball that overlaps an arm cell in a given frame is batted.
type windmillSpec struct {
	hubX, hubY int
	arms       int     // number of arms (spaced evenly around the hub)
	length     int     // arm length in horizontal cells
	rate       float64 // radians per second
}

// classifyArt maps an art rune to its tile. Wall geometry runes all bounce.
func classifyArt(r rune) (t tile, isWall bool) {
	switch r {
	case ' ', 'T', 'H', '*':
		return tileFairway, false
	case ':':
		return tileSand, false
	case '~':
		return tileWater, false
	default: // # = - | / \ + and anything else: a wall
		return tileWall, true
	}
}

// parseHole turns authored art into a hole. Rows shorter than cols (and rows
// beyond what the art provides) are walled, so the playfield is always closed.
func parseHole(name string, par int, art []string, wm *windmillSpec) hole {
	h := hole{name: name, par: par, windmill: wm}
	for ry := 0; ry < courseH; ry++ {
		var line string
		if ry < len(art) {
			line = art[ry]
		}
		runes := []rune(line)
		for rx := 0; rx < cols; rx++ {
			r := ' '
			inArt := rx < len(runes)
			if inArt {
				r = runes[rx]
			} else {
				// Beyond the drawn line: wall, so nothing leaks off the edge.
				h.tiles[ry][rx] = tileWall
				continue
			}
			t, _ := classifyArt(r)
			h.tiles[ry][rx] = t
			switch r {
			case 'T':
				h.teeX = float64(rx)
				h.teeY = float64(ry + top)
			case 'H':
				h.cupX = rx
				h.cupY = ry + top
			case '*':
				if wm != nil {
					wm.hubX = rx
					wm.hubY = ry + top
				}
			}
		}
	}
	return h
}

// at returns the tile at a course cell (row is a canvas row in [top,bottom]).
// Out-of-range cells read as wall.
func (h *hole) at(row, col int) tile {
	ry := row - top
	if ry < 0 || ry >= courseH || col < 0 || col >= cols {
		return tileWall
	}
	return h.tiles[ry][col]
}

// holes is the nine-hole round, built once at package init.
var holes = buildHoles()

func buildHoles() []hole {
	return []hole{
		// 1 — Straightaway. A gentle open box: learn aim, charge, putt.
		parseHole("Straightaway", 2, []string{
			"################################################################################",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#    T                                                                 H       #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 2 — Dogleg. Bounce off the angled rail to reach the cup.
		parseHole("Dogleg", 3, []string{
			"################################################################################",
			"#                                                                              #",
			"#    T                                                                         #",
			"#                                                                              #",
			"#                                                                              #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                          #############                                       #",
			"#                                                                              #",
			"#                                                              H               #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 3 — The Bunker. A sand pit guards the line to the cup.
		parseHole("The Bunker", 3, []string{
			"################################################################################",
			"#                                                                              #",
			"#                                                                              #",
			"#   T                                                                          #",
			"#                                                                              #",
			"#                                                                              #",
			"#                          ::::::::::::::::::                                  #",
			"#                          ::::::::::::::::::                                  #",
			"#                          ::::::::::::::::::                                  #",
			"#                          ::::::::::::::::::                          H       #",
			"#                          ::::::::::::::::::                                  #",
			"#                          ::::::::::::::::::                                  #",
			"#                          ::::::::::::::::::                                  #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 4 — Water Carry. A pond you must clear — splash = penalty + reset.
		parseHole("Water Carry", 3, []string{
			"################################################################################",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#   T                                                                          #",
			"#                                                                              #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                              #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                              #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                              #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~              H               #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                              #",
			"#                  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 5 — The Chicane. Two staggered walls force a controlled zigzag.
		parseHole("The Chicane", 4, []string{
			"################################################################################",
			"#                                                                              #",
			"#  T                                                                           #",
			"#                                                                              #",
			"#                       ###################                                    #",
			"#                       ###################                                    #",
			"#                       ###################                                    #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#               ###################                                            #",
			"#               ###################                                            #",
			"#               ###################                                            #",
			"#                                                                              #",
			"#                                                                  H           #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 6 — Sand Trap Alley. A narrow fairway flanked by bunkers.
		parseHole("Sand Trap Alley", 4, []string{
			"################################################################################",
			"#                                                                              #",
			"#                                                                              #",
			"#   T          :::::::::::::::::::::::::::::::::::::::::::                     #",
			"#              :::::::::::::::::::::::::::::::::::::::::::                     #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                       H      #",
			"#                                                                              #",
			"#                                                                              #",
			"#              :::::::::::::::::::::::::::::::::::::::::::                     #",
			"#              :::::::::::::::::::::::::::::::::::::::::::                     #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 7 — Windmill. Time the spinning arm to slip the ball through.
		parseHole("Windmill", 4, []string{
			"################################################################################",
			"#                                                                              #",
			"#                                                                              #",
			"#   T                                                                          #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                 *                                            #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                  H           #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, &windmillSpec{arms: 2, length: 6, rate: 1.3}),

		// 8 — Island Green. Water all around a fairway corridor to the cup.
		parseHole("Island Green", 5, []string{
			"################################################################################",
			"#                                                                              #",
			"#  T                                                                           #",
			"#           ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                  #",
			"#           ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                  #",
			"#           ~~~~~                                      ~~~~~                   #",
			"#           ~~~~~                                      ~~~~~                   #",
			"#           ~~~~~          ::::::::::::::::            ~~~~~                   #",
			"#           ~~~~~          ::::::::::::::::    H       ~~~~~                   #",
			"#           ~~~~~          ::::::::::::::::            ~~~~~                   #",
			"#           ~~~~~                                      ~~~~~                   #",
			"#           ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                 #",
			"#           ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~                 #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"################################################################################",
		}, nil),

		// 9 — The Gauntlet. Windmill, sand, and a tight bounce — the finale.
		parseHole("The Gauntlet", 5, []string{
			"################################################################################",
			"#                                                                              #",
			"#  T                                                                           #",
			"#                                                                              #",
			"#                  ::::::::::::::                                              #",
			"#                  ::::::::::::::                                              #",
			"#                  ::::::::::::::             *                                #",
			"#                  ::::::::::::::                                              #",
			"#                                                                              #",
			"#                                                                              #",
			"#                                       #########                              #",
			"#                                       #########                              #",
			"#                                       #########                  H           #",
			"#                                       #########                              #",
			"#                                                                              #",
			"#                              ~~~~~~~~~~~~~~~~                                #",
			"#                              ~~~~~~~~~~~~~~~~                                #",
			"#                                                                              #",
			"################################################################################",
		}, &windmillSpec{arms: 3, length: 5, rate: 1.7}),
	}
}
