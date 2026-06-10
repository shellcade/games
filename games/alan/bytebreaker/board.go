package main

import (
	"math"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// One player's board: a brick wall up top, a paddle down the bottom, and one or
// more bits bouncing between them. All motion is continuous, advanced against
// real elapsed time on every wake so it survives a hibernation pause.

// --- screen geometry (the arena is the full 80x24) --------------------------

const (
	hudRow    = 0            // score / level / lives
	wallTop   = 1            // top rail
	fieldTop  = 2            // first interior row a bit can occupy
	paddleRow = 21           // the paddle rides here
	floorRow  = 22           // a bit past this is lost
	statusRow = 23           // messages / controls
	colMin    = 1            // interior columns (walls at 0 and 79)
	colMax    = kit.Cols - 2 // 78

	brickTop  = 3
	brickW    = 6                              // a brick is six columns wide
	brickCols = (colMax - colMin + 1) / brickW // 13 across the wall
	maxRows   = 7

	startLives  = 3
	aspect      = 2.0  // a cell is ~twice as tall as wide; vertical speed is scaled by this
	paddleStep  = 3.0  // columns moved per left/right press
	maxSubStep  = 0.34 // a bit never advances more than this per collision check
	maxBalls    = 6
	bounceAngle = 1.15 // radians of english at the paddle's edge (~66°)
)

const (
	paddleHalfBase = 4 // paddle spans 2*half+1 = 9 cells…
	paddleHalfWide = 7 // …or 15 with the wide powerup
)

// rowPalette is the neon rainbow down the wall, brightest at the top (where the
// bytes are worth the most).
var rowPalette = []kit.Color{
	kit.RGB(0xff, 0x49, 0x6b), // red
	kit.RGB(0xff, 0x9f, 0x1c), // orange
	kit.RGB(0xff, 0xe0, 0x3a), // yellow
	kit.RGB(0x49, 0xe2, 0x6a), // green
	kit.RGB(0x3a, 0xd6, 0xff), // cyan
	kit.RGB(0x6b, 0x8a, 0xff), // blue
	kit.RGB(0xc8, 0x7a, 0xff), // violet
}

// --- pieces ------------------------------------------------------------------

type brick struct {
	alive bool
	hits  int // remaining hits (2 = an armoured byte)
	color kit.Color
}

type ball struct {
	x, y   float64 // centre, in screen cells
	vx, vy float64 // cols/sec, rows/sec
	stuck  bool    // resting on the paddle before launch
}

type puKind uint8

const (
	puWide puKind = iota
	puMulti
	puSlow
	puLife
	puKindCount
)

type powerup struct {
	x, y float64
	kind puKind
}

// particle is a short-lived spark thrown off a shattered byte — pure juice.
type particle struct {
	x, y   float64
	vx, vy float64
	until  time.Time
	color  kit.Color
	glyph  rune
}

type phase uint8

const (
	phReady   phase = iota // a bit sits on the paddle, awaiting launch
	phPlaying              // bits in flight
	phClear                // wall cleared, brief celebration before the next level
	phOver                 // out of lives — score on screen, SPACE to replay
)

type board struct {
	phase phase
	score int
	lives int
	level int

	paddleX  float64 // paddle centre column
	balls    []ball
	bricks   [][]brick
	left     int // live bricks remaining
	powerups []powerup
	parts    []particle

	speed float64 // bit speed magnitude (visual units/sec), grows per level

	// timed effects + holds, all derived from r.Now() so a thaw can't skip them.
	wide       bool
	slow       bool
	wideUntil  time.Time
	slowUntil  time.Time
	phaseUntil time.Time // phClear hold
	flashUntil time.Time // brief HUD/wall flash on a fresh level
	toast      string    // transient banner ("MULTIBALL", "BIT LOST")
	toastUntil time.Time
	clock      time.Time // last step time, for stamping spark expiries / toasts

	best   int // durable high score (loaded from KV)
	posted int // last value posted to the leaderboard
}

type rng interface {
	Intn(n int) int
	Float64() float64
}

func newBoard(best int) *board {
	b := &board{best: best, posted: best}
	b.reset()
	return b
}

// reset starts a brand-new game at level 1.
func (b *board) reset() {
	b.score = 0
	b.lives = startLives
	b.level = 1
	b.paddleX = float64(kit.Cols) / 2
	b.powerups = b.powerups[:0]
	b.parts = b.parts[:0]
	b.wide, b.slow = false, false
	b.buildLevel()
	b.serve()
}

func (b *board) paddleHalf() int {
	if b.wide {
		return paddleHalfWide
	}
	return paddleHalfBase
}

// buildLevel lays out the wall for the current level: more rows and some
// armoured bytes (and gaps for shape) as the levels climb.
func (b *board) buildLevel() {
	rows := 3 + b.level
	if rows > maxRows {
		rows = maxRows
	}
	b.speed = 20 + float64(b.level-1)*2.5
	b.bricks = make([][]brick, rows)
	b.left = 0
	for r := 0; r < rows; r++ {
		b.bricks[r] = make([]brick, brickCols)
		for c := 0; c < brickCols; c++ {
			if b.level >= 3 && (r+2*c)%9 == 0 {
				continue // a sprinkling of gaps from level 3
			}
			hits := 1
			if b.level >= 2 && r < (rows+2)/3 {
				hits = 2 // the top third armours up from level 2
			}
			b.bricks[r][c] = brick{alive: true, hits: hits, color: rowPalette[r%len(rowPalette)]}
			b.left++
		}
	}
}

// serve parks a fresh bit on the paddle, ready to launch.
func (b *board) serve() {
	b.balls = b.balls[:0]
	b.balls = append(b.balls, ball{x: b.paddleX, y: paddleRow - 1, stuck: true})
	b.phase = phReady
}

// setToast flashes a short banner for ~1.3s (stamped off the last step time).
func (b *board) setToast(s string) {
	b.toast = s
	b.toastUntil = b.clock.Add(1300 * time.Millisecond)
}

// --- input -------------------------------------------------------------------

func (b *board) movePaddle(dir float64) {
	half := float64(b.paddleHalf())
	b.paddleX += dir * paddleStep
	if b.paddleX < float64(colMin)+half {
		b.paddleX = float64(colMin) + half
	}
	if b.paddleX > float64(colMax)-half {
		b.paddleX = float64(colMax) - half
	}
	if b.phase == phReady {
		for i := range b.balls {
			if b.balls[i].stuck {
				b.balls[i].x = b.paddleX
			}
		}
	}
}

// launch flings every parked bit up at a lively angle; on game-over it replays.
func (b *board) launch(rng rng) {
	if b.phase == phOver {
		b.reset()
		return
	}
	any := false
	for i := range b.balls {
		if b.balls[i].stuck {
			a := (rng.Float64()*2 - 1) * 0.45 // +/- ~26 degrees
			setVel(&b.balls[i], b.speed, a)
			b.balls[i].stuck = false
			any = true
		}
	}
	if any && b.phase == phReady {
		b.phase = phPlaying
	}
}

// setVel points a bit up the wall at angle a from vertical (positive = right),
// keeping its visual speed magnitude (vertical scaled by the cell aspect).
func setVel(bl *ball, spd, a float64) {
	bl.vx = spd * math.Sin(a)
	bl.vy = -spd * math.Cos(a) / aspect
}

// --- the per-wake update -----------------------------------------------------

func (b *board) step(dt float64, now time.Time, rng rng) {
	b.clock = now
	b.wide = now.Before(b.wideUntil)
	b.slow = now.Before(b.slowUntil)
	b.bankScore()

	switch b.phase {
	case phClear:
		if now.After(b.phaseUntil) {
			b.level++
			b.buildLevel()
			b.serve()
		}
		b.advanceParticles(dt, now)
		return
	case phOver:
		b.advanceParticles(dt, now)
		return
	}

	mul := 1.0
	if b.slow {
		mul = 0.55
	}

	// Bits in flight (a parked bit just tracks the paddle).
	alive := b.balls[:0]
	for i := range b.balls {
		bl := b.balls[i]
		if bl.stuck {
			bl.x, bl.y = b.paddleX, paddleRow-1
			alive = append(alive, bl)
			continue
		}
		if b.moveBall(&bl, dt*mul, rng) {
			alive = append(alive, bl)
		}
	}
	b.balls = alive

	if len(b.balls) == 0 {
		b.loseLife()
	}

	b.advancePowerups(dt, now)
	b.advanceParticles(dt, now)

	if b.left == 0 && b.phase == phPlaying {
		b.phase = phClear
		b.phaseUntil = now.Add(1500 * time.Millisecond)
		b.setToast("WALL CLEARED!")
		b.flashUntil = now.Add(400 * time.Millisecond)
		b.bankScore()
	}
}

func (b *board) loseLife() {
	b.lives--
	if b.lives <= 0 {
		b.phase = phOver
		b.setToast("GAME OVER")
		return
	}
	b.setToast("BIT LOST")
	b.wide, b.slow = false, false
	b.wideUntil, b.slowUntil = time.Time{}, time.Time{}
	b.serve()
}

// moveBall advances one bit, resolving wall / brick / paddle hits in small
// sub-steps so a fast bit never tunnels through a byte. Returns false if the bit
// fell past the paddle.
func (b *board) moveBall(bl *ball, dt float64, rng rng) bool {
	dx, dy := bl.vx*dt, bl.vy*dt
	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Abs(dy)) / maxSubStep))
	if steps < 1 {
		steps = 1
	}
	sub := dt / float64(steps)
	for s := 0; s < steps; s++ {
		nx := bl.x + bl.vx*sub
		ny := bl.y + bl.vy*sub

		// Side + top walls.
		if nx <= float64(colMin) {
			nx = float64(colMin)
			bl.vx = math.Abs(bl.vx)
		} else if nx >= float64(colMax) {
			nx = float64(colMax)
			bl.vx = -math.Abs(bl.vx)
		}
		if ny <= float64(fieldTop) {
			ny = float64(fieldTop)
			bl.vy = math.Abs(bl.vy)
		}

		// Bricks: resolve against the cell we'd enter, reflecting off whichever
		// face we crossed (vertical neighbour -> flip vy, horizontal -> flip vx).
		cx, cy := int(math.Round(nx)), int(math.Round(ny))
		if b.brickAlive(cx, cy) {
			vBlock := b.brickAlive(int(math.Round(bl.x)), cy)
			hBlock := b.brickAlive(cx, int(math.Round(bl.y)))
			if vBlock || (!vBlock && !hBlock) {
				bl.vy = -bl.vy
				ny = bl.y
			}
			if hBlock {
				bl.vx = -bl.vx
				nx = bl.x
			}
			b.hitBrick(cx, cy, rng)
		}

		// Paddle.
		half := float64(b.paddleHalf())
		if bl.vy > 0 && ny >= float64(paddleRow)-0.5 && nx >= b.paddleX-half-0.5 && nx <= b.paddleX+half+0.5 {
			off := (nx - b.paddleX) / (half + 0.5)
			off = math.Max(-1, math.Min(1, off))
			setVel(bl, b.speed, off*bounceAngle)
			ny = float64(paddleRow) - 1
		}

		bl.x, bl.y = nx, ny
		if bl.y > float64(floorRow) {
			return false
		}
	}
	return true
}

func (b *board) brickIndex(col, row int) (r, c int, ok bool) {
	if row < brickTop || row >= brickTop+len(b.bricks) {
		return 0, 0, false
	}
	if col < colMin || col > colMax {
		return 0, 0, false
	}
	return row - brickTop, (col - colMin) / brickW, true
}

func (b *board) brickAlive(col, row int) bool {
	r, c, ok := b.brickIndex(col, row)
	if !ok || c >= brickCols {
		return false
	}
	return b.bricks[r][c].alive
}

// hitBrick damages the byte under (col,row): armoured ones crack first, the rest
// shatter into sparks, score, and the occasional powerup.
func (b *board) hitBrick(col, row int, rng rng) {
	r, c, ok := b.brickIndex(col, row)
	if !ok || c >= brickCols || !b.bricks[r][c].alive {
		return
	}
	br := &b.bricks[r][c]
	br.hits--
	if br.hits > 0 {
		b.score += 5
		return
	}
	br.alive = false
	b.left--
	b.score += 10 * (len(b.bricks) - r)
	b.spark(col, row, br.color, rng)
	if rng.Intn(100) < 14 {
		b.powerups = append(b.powerups, powerup{
			x: float64(colMin + c*brickW + brickW/2), y: float64(brickTop + r),
			kind: puKind(rng.Intn(int(puKindCount))),
		})
	}
}

func (b *board) spark(col, row int, color kit.Color, rng rng) {
	glyphs := []rune{'*', '+', '.', '\''}
	n := 4 + rng.Intn(3)
	for i := 0; i < n; i++ {
		ang := rng.Float64() * 2 * math.Pi
		spd := 6 + rng.Float64()*10
		b.parts = append(b.parts, particle{
			x: float64(col), y: float64(row),
			vx:    math.Cos(ang) * spd,
			vy:    math.Sin(ang) * spd / aspect,
			until: nowPlus(b, 320),
			color: color,
			glyph: glyphs[rng.Intn(len(glyphs))],
		})
	}
}

// nowPlus is a tiny helper so spark() can stamp an expiry without threading the
// clock everywhere — it derives off the most recent step time.
func nowPlus(b *board, ms int) time.Time { return b.clock.Add(time.Duration(ms) * time.Millisecond) }

func (b *board) advanceParticles(dt float64, now time.Time) {
	out := b.parts[:0]
	for _, p := range b.parts {
		if now.After(p.until) {
			continue
		}
		p.x += p.vx * dt
		p.y += p.vy * dt
		p.vy += 14 * dt // gravity
		out = append(out, p)
	}
	b.parts = out
}

func (b *board) advancePowerups(dt float64, now time.Time) {
	half := float64(b.paddleHalf())
	out := b.powerups[:0]
	for _, p := range b.powerups {
		p.y += 7 * dt
		if p.y >= float64(paddleRow)-0.5 && p.x >= b.paddleX-half-0.5 && p.x <= b.paddleX+half+0.5 {
			b.apply(p.kind, now)
			continue
		}
		if p.y > float64(floorRow) {
			continue
		}
		out = append(out, p)
	}
	b.powerups = out
}

func (b *board) apply(k puKind, now time.Time) {
	switch k {
	case puWide:
		b.wideUntil = now.Add(13 * time.Second)
		b.setToast("WIDE PADDLE")
	case puSlow:
		b.slowUntil = now.Add(9 * time.Second)
		b.setToast("SLOW BIT")
	case puLife:
		if b.lives < 6 {
			b.lives++
		}
		b.setToast("EXTRA LIFE")
	case puMulti:
		b.splitBalls()
		b.setToast("MULTIBALL")
	}
}

// splitBalls forks every bit in flight into two more, fanned out, up to the cap.
func (b *board) splitBalls() {
	add := []ball{}
	for _, bl := range b.balls {
		if bl.stuck {
			continue
		}
		spd := math.Hypot(bl.vx, bl.vy*aspect)
		base := math.Atan2(bl.vx, -bl.vy*aspect)
		for _, d := range []float64{-0.4, 0.4} {
			if len(b.balls)+len(add) >= maxBalls {
				break
			}
			nb := ball{x: bl.x, y: bl.y}
			setVel(&nb, spd, base+d)
			add = append(add, nb)
		}
	}
	b.balls = append(b.balls, add...)
}

// --- scoring / leaderboard hand-off -----------------------------------------

// bankScore lifts the high score; the room posts it to the leaderboard.
func (b *board) bankScore() {
	if b.score > b.best {
		b.best = b.score
	}
}
