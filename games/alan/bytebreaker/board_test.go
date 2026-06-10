package main

import (
	"math/rand"
	"testing"
	"time"
)

func testRng() *rand.Rand { return rand.New(rand.NewSource(1)) }

func brickCell(r, c int) (col, row int) { return colMin + c*brickW, brickTop + r }

func TestNewBoardStartsReady(t *testing.T) {
	b := newBoard(0)
	if b.phase != phReady {
		t.Errorf("phase = %v, want ready", b.phase)
	}
	if b.lives != startLives {
		t.Errorf("lives = %d, want %d", b.lives, startLives)
	}
	if b.level != 1 {
		t.Errorf("level = %d, want 1", b.level)
	}
	if len(b.balls) != 1 || !b.balls[0].stuck {
		t.Fatalf("want exactly one stuck bit, got %d", len(b.balls))
	}
	if b.left == 0 {
		t.Error("level built with no bricks")
	}
}

func TestLaunchUnsticks(t *testing.T) {
	b := newBoard(0)
	b.launch(testRng())
	if b.phase != phPlaying {
		t.Errorf("phase = %v, want playing", b.phase)
	}
	if b.balls[0].stuck {
		t.Error("bit still stuck after launch")
	}
	if b.balls[0].vy >= 0 {
		t.Errorf("bit launched downward: vy = %f", b.balls[0].vy)
	}
}

func TestPaddleClamps(t *testing.T) {
	b := newBoard(0)
	half := float64(b.paddleHalf())
	for i := 0; i < 200; i++ {
		b.movePaddle(-1)
	}
	if b.paddleX < float64(colMin)+half-0.001 {
		t.Errorf("paddle %f went past the left wall", b.paddleX)
	}
	for i := 0; i < 400; i++ {
		b.movePaddle(+1)
	}
	if b.paddleX > float64(colMax)-half+0.001 {
		t.Errorf("paddle %f went past the right wall", b.paddleX)
	}
}

func TestHitBrickDestroys(t *testing.T) {
	b := newBoard(0) // level 1: every byte is single-hit
	before := b.left
	col, row := brickCell(0, 0)
	b.hitBrick(col, row, testRng())
	if b.bricks[0][0].alive {
		t.Error("brick survived a clean hit")
	}
	if b.left != before-1 {
		t.Errorf("bricks left = %d, want %d", b.left, before-1)
	}
	if b.score == 0 {
		t.Error("destroying a byte scored nothing")
	}
}

func TestArmouredBrickTakesTwo(t *testing.T) {
	b := newBoard(0)
	b.level = 2
	b.buildLevel() // level 2 armours the top rows
	if b.bricks[0][0].hits != 2 {
		t.Fatalf("expected an armoured top-left byte, hits = %d", b.bricks[0][0].hits)
	}
	before := b.left
	col, row := brickCell(0, 0)
	b.hitBrick(col, row, testRng())
	if !b.bricks[0][0].alive || b.left != before {
		t.Error("armoured byte should crack, not shatter, on the first hit")
	}
	b.hitBrick(col, row, testRng())
	if b.bricks[0][0].alive || b.left != before-1 {
		t.Error("armoured byte should shatter on the second hit")
	}
}

func TestBallLostCostsALife(t *testing.T) {
	b := newBoard(0)
	b.launch(testRng())
	lives := b.lives
	b.balls = []ball{{x: 5, y: floorRow + 2, vx: 0, vy: 12}} // already past the floor
	b.step(0.05, time.Unix(100, 0), testRng())
	if b.lives != lives-1 {
		t.Errorf("lives = %d, want %d", b.lives, lives-1)
	}
	if b.phase != phReady {
		t.Errorf("phase = %v, want a fresh serve", b.phase)
	}
}

func TestGameOverAtZeroLives(t *testing.T) {
	b := newBoard(0)
	b.lives = 1
	b.launch(testRng())
	b.balls = b.balls[:0] // last bit gone
	b.step(0.05, time.Unix(100, 0), testRng())
	if b.phase != phOver {
		t.Errorf("phase = %v, want game over", b.phase)
	}
}

func TestSideWallReflects(t *testing.T) {
	b := newBoard(0)
	b.phase = phPlaying
	b.balls = []ball{{x: float64(colMin) + 0.1, y: 12, vx: -25, vy: 0}}
	b.step(0.05, time.Unix(100, 0), testRng())
	if b.balls[0].vx <= 0 {
		t.Errorf("left-wall bounce did not reverse vx: %f", b.balls[0].vx)
	}
}

func TestPaddleReflectsUp(t *testing.T) {
	b := newBoard(0)
	b.phase = phPlaying
	b.paddleX = 40
	b.balls = []ball{{x: 40, y: float64(paddleRow) - 0.6, vx: 0, vy: 22}}
	b.step(0.05, time.Unix(100, 0), testRng())
	if len(b.balls) != 1 {
		t.Fatalf("bit was lost instead of bouncing")
	}
	if b.balls[0].vy >= 0 {
		t.Errorf("paddle bounce did not send the bit up: vy = %f", b.balls[0].vy)
	}
}

func TestLevelClearAdvances(t *testing.T) {
	b := newBoard(0)
	b.launch(testRng())
	for r := range b.bricks {
		for c := range b.bricks[r] {
			b.bricks[r][c].alive = false
		}
	}
	b.left = 0
	now := time.Unix(100, 0)
	b.step(0.05, now, testRng())
	if b.phase != phClear {
		t.Fatalf("phase = %v, want clear", b.phase)
	}
	b.step(0.05, now.Add(2*time.Second), testRng())
	if b.level != 2 || b.phase != phReady {
		t.Errorf("after the hold: level %d phase %v, want level 2 / ready", b.level, b.phase)
	}
	if b.left == 0 {
		t.Error("level 2 built with no bricks")
	}
}

func TestMultiballForks(t *testing.T) {
	b := newBoard(0)
	b.phase = phPlaying
	b.balls = []ball{{x: 40, y: 12, vx: 8, vy: -8}}
	b.apply(puMulti, time.Unix(100, 0))
	if len(b.balls) < 3 {
		t.Errorf("multiball produced %d bits, want at least 3", len(b.balls))
	}
}

func TestExtraLifeCapsOut(t *testing.T) {
	b := newBoard(0)
	b.lives = 6
	b.apply(puLife, time.Unix(100, 0))
	if b.lives != 6 {
		t.Errorf("extra life exceeded the cap: %d", b.lives)
	}
}
