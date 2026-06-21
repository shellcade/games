// Neon Snake — a neon two-snake duel for the terminal, built on shellcade/kit.
// Eat the star to grow and score, outlast the other snake (a friend, an AI bot,
// or your own second hand in solo co-op), and survive five modes. Run it right
// now: go run .  (add -seats 2 for hot-seat head-to-head).
package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	kit "github.com/shellcade/kit/v2"
)

func main() { kit.Main(Game{}) }

// Game is the registry entry: metadata + a per-room behavior factory.
type Game struct{}

func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "neon-snake",
		Name:             "Neon Snake",
		ShortDescription: "Grab glowing food, snatch shield and freeze power-ups, and outlast a rival snake across five modes.",
		MinPlayers:       1,
		MaxPlayers:       2,  // two snakes: solo co-op (one seat) or head-to-head (two seats)
		HeartbeatMS:      50, // 20 ticks per second

		// A real-time arcade round with no mid-game state worth resuming: when
		// everyone leaves, the room closes — no hibernation snapshot, no Resume entry.
		Lifecycle: kit.LifecycleEphemeral,

		Leaderboard: &kit.LeaderboardSpec{
			MetricLabel: "Score",
			Direction:   kit.HigherBetter,
			Aggregation: kit.BestResult,
			Format:      kit.Integer,
		},
	}
}

func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return &room{
		services: svc,
		// OnImprove: post only when a player tops their last posted score, so the
		// board sees the high-water mark from live play and the disconnect flush.
		sk: kit.NewScoreKeeper(kit.OnImprove),
	}
}

type Point struct {
	X, Y int
}

type ScorePopup struct {
	X, Y      int
	Text      string
	Color     kit.Color
	CreatedAt time.Time
}

type Particle struct {
	X, Y      float64
	VX, VY    float64
	Glyph     rune
	Color     kit.Color
	CreatedAt time.Time
	Duration  time.Duration
}

type Palette struct {
	Name        string
	Border      kit.Color
	Header      kit.Color
	Dot         kit.Color
	SnakeHead   kit.Color
	SnakeTail   kit.Color
	Snake2Head  kit.Color
	Snake2Tail  kit.Color
	Food        kit.Color
	Obstacle    kit.Color
	Footer      kit.Color
	Key         kit.Color
	ModalBorder kit.Color
	Hazard      kit.Color
}

// palettes is an immutable package-level table; themeIndex selects into it.
var palettes = []Palette{
	{
		Name:        "Cyberpunk",
		Border:      kit.RGB(0x8a, 0x2b, 0xe2), // Neon Violet
		Header:      kit.RGB(0x00, 0xff, 0xff), // Aqua/Cyan
		Dot:         kit.RGB(0x33, 0x33, 0x33), // Dark Gray
		SnakeHead:   kit.RGB(0x39, 0xff, 0x14), // Lime Green
		SnakeTail:   kit.RGB(0x00, 0xe5, 0xff), // Cyan
		Snake2Head:  kit.RGB(0xff, 0x00, 0x7f), // Neon Pink
		Snake2Tail:  kit.RGB(0xff, 0xa5, 0x00), // Neon Orange
		Food:        kit.RGB(0xff, 0x00, 0x7f), // Neon Pink
		Obstacle:    kit.RGB(0xff, 0x8c, 0x00), // Neon Orange/Amber
		Footer:      kit.RGB(0x00, 0xe5, 0xff), // Light Cyan
		Key:         kit.RGB(0xff, 0x00, 0x7f), // Neon Pink
		ModalBorder: kit.RGB(0xff, 0x00, 0x55), // Pink/Red
		Hazard:      kit.RGB(0xff, 0x00, 0xff), // Neon Magenta
	},
	{
		Name:        "Ocean",
		Border:      kit.RGB(0x00, 0x00, 0xcd), // Deep Blue
		Header:      kit.RGB(0xe0, 0xff, 0xff), // Light Cyan
		Dot:         kit.RGB(0x11, 0x22, 0x44), // Dark Navy
		SnakeHead:   kit.RGB(0x00, 0xff, 0xcc), // Teal/Cyan
		SnakeTail:   kit.RGB(0x00, 0x66, 0xff), // Blue
		Snake2Head:  kit.RGB(0xff, 0x7f, 0x50), // Coral
		Snake2Tail:  kit.RGB(0xff, 0xd7, 0x00), // Gold
		Food:        kit.RGB(0xff, 0x7f, 0x50), // Coral
		Obstacle:    kit.RGB(0xff, 0xd7, 0x00), // Gold
		Footer:      kit.RGB(0x00, 0xcd, 0xcd), // Teal
		Key:         kit.RGB(0xff, 0x7f, 0x50), // Coral
		ModalBorder: kit.RGB(0x00, 0xbf, 0xff), // Sky Blue
		Hazard:      kit.RGB(0xff, 0x55, 0x00), // Bright Orange-Red
	},
	{
		Name:        "Sunset",
		Border:      kit.RGB(0xdc, 0x14, 0x3c), // Crimson Red
		Header:      kit.RGB(0xff, 0xd7, 0x00), // Gold
		Dot:         kit.RGB(0x44, 0x22, 0x22), // Dark Rust
		SnakeHead:   kit.RGB(0xff, 0xa5, 0x00), // Yellow-Orange
		SnakeTail:   kit.RGB(0x8b, 0x00, 0x00), // Deep Red
		Snake2Head:  kit.RGB(0xda, 0x70, 0xd6), // Neon Purple/Orchid
		Snake2Tail:  kit.RGB(0xdc, 0x14, 0x3c), // Crimson
		Food:        kit.RGB(0xda, 0x70, 0xd6), // Neon Purple
		Obstacle:    kit.RGB(0xff, 0x00, 0xff), // Magenta
		Footer:      kit.RGB(0xff, 0xc0, 0xcb), // Light Orange/Pink
		Key:         kit.RGB(0xff, 0xd7, 0x00), // Gold
		ModalBorder: kit.RGB(0xff, 0x8c, 0x00), // Dark Orange
		Hazard:      kit.RGB(0xff, 0x24, 0x00), // Scarlet/Fiery Red
	},
	{
		Name:        "Matrix",
		Border:      kit.RGB(0x00, 0x64, 0x00), // Dark Green
		Header:      kit.RGB(0x00, 0xff, 0x00), // Neon Green
		Dot:         kit.RGB(0x00, 0x22, 0x00), // Very Dark Green
		SnakeHead:   kit.RGB(0xcc, 0xff, 0xcc), // White-Green
		SnakeTail:   kit.RGB(0x32, 0xcd, 0x32), // Lime Green
		Snake2Head:  kit.RGB(0xff, 0xff, 0x00), // Yellow
		Snake2Tail:  kit.RGB(0x00, 0x64, 0x00), // Forest Green
		Food:        kit.RGB(0xad, 0xff, 0x2f), // Matrix Yellow
		Obstacle:    kit.RGB(0x22, 0x8b, 0x22), // Forest Green
		Footer:      kit.RGB(0x98, 0xfb, 0x98), // Pale Green
		Key:         kit.RGB(0x00, 0xff, 0x00), // Neon Green
		ModalBorder: kit.RGB(0x7f, 0xff, 0x00), // Light Lime
		Hazard:      kit.RGB(0x00, 0xff, 0xff), // Neon Cyan (contrasts green)
	},
	{
		Name:        "Vaporwave",
		Border:      kit.RGB(0xda, 0x70, 0xd6), // Pastel Purple
		Header:      kit.RGB(0xff, 0xff, 0x00), // Bright Yellow
		Dot:         kit.RGB(0x33, 0x11, 0x44), // Pastel Violet
		SnakeHead:   kit.RGB(0xff, 0x69, 0xb4), // Hot Pink
		SnakeTail:   kit.RGB(0x00, 0xff, 0xff), // Cyan
		Snake2Head:  kit.RGB(0xee, 0x82, 0xee), // Soft Purple
		Snake2Tail:  kit.RGB(0xff, 0xff, 0x00), // Yellow
		Food:        kit.RGB(0xff, 0xa5, 0x00), // Neon Orange
		Obstacle:    kit.RGB(0x4b, 0x00, 0x82), // Deep Indigo
		Footer:      kit.RGB(0xee, 0x82, 0xee), // Soft Purple
		Key:         kit.RGB(0x00, 0xff, 0xff), // Cyan
		ModalBorder: kit.RGB(0xff, 0x00, 0xff), // Magenta
		Hazard:      kit.RGB(0x00, 0xff, 0x7f), // Spring Green
	},
}

type GameMode int

const (
	ModeClassic GameMode = iota
	ModeHazard
	ModeMaze
	ModePortal
	ModeBomb
	modeCount
)

// settingsCount is the number of rows in the settings menu (0..settingsCount-1,
// the last being "Close & Apply").
const settingsCount = 7

// startSpeeds are the selectable starting tick intervals in ms (index via
// startSpeedIdx). Faster (lower) values mean a quicker snake.
var startSpeeds = []int{100, 120, 150, 180, 200}

// maxObstacles caps obstacle growth. Without a cap, obstacles grow by one per
// food forever and never shrink, eventually crowding the board so food and
// power-ups can't find a free cell and fall back to a fixed point.
const maxObstacles = 12

type Hazard struct {
	Pos        Point
	Dir        Point
	MinX, MaxX int
	MinY, MaxY int
}

// room is one live room. ALL state lives here (and only here).
type room struct {
	kit.Base
	services    kit.Services
	sk          *kit.ScoreKeeper // tracks each player's current score for live/disconnect posting
	pb1         int
	pb2         int
	newPB1      bool
	newPB2      bool
	frame       *kit.Frame
	shakeFrame  *kit.Frame
	occupied    map[Point]bool // reused each render to mark cells that hide a grid dot
	lastTick    time.Time
	tickRate    time.Duration
	score1      int
	score2      int
	highScore   int
	gameStarted bool
	gameOver    bool

	snake1        []Point
	entityDir1    Point
	lastMovedDir1 Point

	snake2        []Point
	entityDir2    Point
	lastMovedDir2 Point

	crashed1 bool
	crashed2 bool

	food      Point
	obstacles []Point

	startedAt time.Time

	// Task 4 Aesthetics
	themeIndex      int
	popups          []ScorePopup
	lastCollisionAt time.Time

	// Task 5 Multiplayer & Active player tracking
	activePlayer    kit.Player
	activePlayerSet bool

	gameMode GameMode
	hazards  []Hazard

	// Task 8 Power-ups
	powerUpPos       Point
	powerUpType      string // "SHIELD" or "FREEZE"
	powerUpActive    bool
	powerUpSpawnedAt time.Time

	p1PowerUpType   string
	p1PowerUpExpiry time.Time

	p2PowerUpType   string
	p2PowerUpExpiry time.Time

	tickCount int
	lastWake  time.Time
	p2IsBot   bool

	portalA Point
	portalB Point

	settingsOpen    bool
	settingsCursor  int
	snake1SkinIdx   int
	snake2SkinIdx   int
	gridDotsEnabled bool
	startSpeedIdx   int

	// Task 13 Bomb Mode
	bombPos        Point
	bombActive     bool
	bombSpawnedAt  time.Time
	bombExploding  bool
	bombExplodedAt time.Time

	// Task 14 Screen FX & Particles
	particles      []Particle
	shakeOption    int // 0: OFF, 1: GENTLE, 2: STRONG
	flashOption    int // 0: OFF, 1: GENTLE, 2: STRONG
	shakeStartedAt time.Time
	shakeExpiry    time.Time
	flashExpiry    time.Time
}

func (rm *room) updateActivePlayer(r kit.Room) {
	members := r.Members()
	if len(members) == 0 {
		return
	}
	// If active player is not set or not in the room anymore, choose the first available member
	found := false
	if rm.activePlayerSet {
		for _, m := range members {
			if m.AccountID == rm.activePlayer.AccountID {
				found = true
				break
			}
		}
	}
	if !found {
		rm.activePlayer = members[0]
		rm.activePlayerSet = true
	}
}

func (rm *room) getActivePlayer(r kit.Room) kit.Player {
	rm.updateActivePlayer(r)
	return rm.activePlayer
}

func (rm *room) initHazards() {
	rm.hazards = []Hazard{}
	if rm.gameMode == ModeClassic {
		return
	}
	if rm.gameMode == ModeHazard {
		rm.hazards = []Hazard{
			{Pos: Point{X: 5, Y: 3}, Dir: Point{X: 1, Y: 0}, MinX: 5, MaxX: 33, MinY: 3, MaxY: 3},
			{Pos: Point{X: 33, Y: 14}, Dir: Point{X: -1, Y: 0}, MinX: 5, MaxX: 33, MinY: 14, MaxY: 14},
			{Pos: Point{X: 8, Y: 4}, Dir: Point{X: 0, Y: 1}, MinX: 8, MaxX: 8, MinY: 4, MaxY: 13},
			{Pos: Point{X: 30, Y: 13}, Dir: Point{X: 0, Y: -1}, MinX: 30, MaxX: 30, MinY: 4, MaxY: 13},
		}
	} else if rm.gameMode == ModeMaze {
		rm.hazards = []Hazard{
			{Pos: Point{X: 5, Y: 7}, Dir: Point{X: 1, Y: 0}, MinX: 5, MaxX: 33, MinY: 7, MaxY: 7},
			{Pos: Point{X: 33, Y: 10}, Dir: Point{X: -1, Y: 0}, MinX: 5, MaxX: 33, MinY: 10, MaxY: 10},
			{Pos: Point{X: 5, Y: 2}, Dir: Point{X: 0, Y: 1}, MinX: 5, MaxX: 5, MinY: 2, MaxY: 15},
			{Pos: Point{X: 33, Y: 15}, Dir: Point{X: 0, Y: -1}, MinX: 33, MaxX: 33, MinY: 2, MaxY: 15},
		}
	}
}

func (rm *room) isMazeWall(p Point) bool {
	if rm.gameMode != ModeMaze {
		return false
	}
	// Wall 1 & 3: X from 8 to 14, Y = 4 or 13
	if p.X >= 8 && p.X <= 14 && (p.Y == 4 || p.Y == 13) {
		return true
	}
	// Wall 2 & 4: X from 24 to 30, Y = 4 or 13
	if p.X >= 24 && p.X <= 30 && (p.Y == 4 || p.Y == 13) {
		return true
	}
	// Wall 5 & 6: X = 19, Y from 2 to 5 or 12 to 15
	if p.X == 19 && ((p.Y >= 2 && p.Y <= 5) || (p.Y >= 12 && p.Y <= 15)) {
		return true
	}
	return false
}

func (rm *room) getModeName() string {
	switch rm.gameMode {
	case ModeClassic:
		return "CLASSIC"
	case ModeHazard:
		return "HAZARDS"
	case ModeMaze:
		return "MAZE"
	case ModePortal:
		return "PORTALS"
	case ModeBomb:
		return "BOMB"
	default:
		return "CLASSIC"
	}
}

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.lastTick = r.Now()
	rm.startedAt = r.Now()

	rm.settingsOpen = false
	rm.settingsCursor = 0
	rm.snake1SkinIdx = 0
	rm.snake2SkinIdx = 0
	rm.gridDotsEnabled = true
	rm.startSpeedIdx = 2 // 150ms
	rm.shakeOption = 2   // default to STRONG
	rm.flashOption = 2   // default to STRONG
	rm.bombActive = false
	rm.bombExploding = false
	rm.particles = []Particle{}
	rm.shakeStartedAt = time.Time{}
	rm.shakeExpiry = time.Time{}
	rm.flashExpiry = time.Time{}

	speeds := []int{100, 120, 150, 180, 200}
	rm.tickRate = time.Duration(speeds[rm.startSpeedIdx]) * time.Millisecond
	rm.snake1 = []Point{
		{X: 10, Y: 9},
		{X: 9, Y: 9},
		{X: 8, Y: 9},
		{X: 7, Y: 9},
	}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = Point{X: 1, Y: 0}

	rm.snake2 = []Point{
		{X: 28, Y: 9},
		{X: 29, Y: 9},
		{X: 30, Y: 9},
		{X: 31, Y: 9},
	}
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}

	rm.crashed1 = false
	rm.crashed2 = false

	rm.gameStarted = true
	rm.score1 = 0
	rm.score2 = 0
	rm.gameOver = false
	rm.themeIndex = 0
	rm.gameMode = ModeClassic
	rm.popups = []ScorePopup{}
	rm.lastCollisionAt = time.Time{}
	rm.activePlayer = kit.Player{}
	rm.activePlayerSet = false

	rm.powerUpActive = false
	rm.p1PowerUpType = ""
	rm.p1PowerUpExpiry = time.Time{}
	rm.p2PowerUpType = ""
	rm.p2PowerUpExpiry = time.Time{}
	rm.tickCount = 0
	rm.lastWake = r.Now()
	rm.p2IsBot = false
	rm.portalA = Point{X: 9, Y: 9}
	rm.portalB = Point{X: 29, Y: 9}

	// Initialize hazards
	rm.initHazards()

	// Generate initial food & obstacles
	rm.food = rm.randomFreePoint(r, 0)
	rm.obstacles = []Point{}
	for i := 0; i < 3; i++ {
		rm.obstacles = append(rm.obstacles, rm.randomFreePoint(r, 5))
	}

	// Load PBs here too: a snapshot-revived room may run OnStart without a
	// following OnJoin, and we don't want PBs stuck at 0 until the first reset.
	rm.loadPersonalBests(r)
}

func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.updateActivePlayer(r)
	if len(r.Members()) >= 2 {
		rm.p2IsBot = false
	}
	rm.loadPersonalBests(r)
	rm.render(r)
}

// OnLeave records a leaving player's progress. A disconnect mid-game (before any
// crash) otherwise never reaches the game-over Post, so the player's current
// score would never be recorded. We flush it with StatusDNF and persist their
// personal best to KV for session resume.
func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	if rm.sk != nil && rm.gameStarted && !rm.gameOver {
		// Make sure the keeper has this player's current score even if they never
		// scored (Record is a no-op-ish update), then flush it as a DNF.
		rm.recordScores(r)
		rm.sk.FlushLeave(r, p, kit.StatusDNF)
	}
	rm.savePersonalBests(r)
	rm.render(r)
}

func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	rm.activePlayer = p
	rm.activePlayerSet = true

	if rm.settingsOpen {
		rm.handleSettingsInput(r, p, in)
		rm.render(r)
		return
	}

	// Identify player index
	members := r.Members()
	isPlayer1 := true
	isPlayer2 := false
	if len(members) >= 2 {
		if p.AccountID == members[0].AccountID {
			isPlayer1 = true
			isPlayer2 = false
		} else if p.AccountID == members[1].AccountID {
			isPlayer1 = false
			isPlayer2 = true
		} else {
			isPlayer1 = false
			isPlayer2 = false
		}
	}

	// Handle steering keys concurrently
	if !rm.gameOver && rm.gameStarted {
		if in.Kind == kit.InputRune {
			// Player 1 controls Snake 1 using WASD
			if isPlayer1 {
				switch in.Rune {
				case 'w', 'W':
					if rm.lastMovedDir1.Y != 1 {
						rm.entityDir1 = Point{X: 0, Y: -1}
					}
				case 's', 'S':
					if rm.lastMovedDir1.Y != -1 {
						rm.entityDir1 = Point{X: 0, Y: 1}
					}
				case 'a', 'A':
					if rm.lastMovedDir1.X != 1 {
						rm.entityDir1 = Point{X: -1, Y: 0}
					}
				case 'd', 'D':
					if rm.lastMovedDir1.X != -1 {
						rm.entityDir1 = Point{X: 1, Y: 0}
					}
				}
			}
		} else if in.Kind == kit.InputKey {
			// Player 2 controls Snake 2 using Arrow keys.
			// In single-player co-op (len(members) < 2), Player 1 controls Snake 2 using Arrow keys.
			canControlSnake2 := isPlayer2 || (isPlayer1 && len(members) < 2)
			if canControlSnake2 {
				if rm.p2IsBot {
					rm.p2IsBot = false
				}
				switch in.Key {
				case kit.KeyUp:
					if rm.lastMovedDir2.Y != 1 {
						rm.entityDir2 = Point{X: 0, Y: -1}
					}
				case kit.KeyDown:
					if rm.lastMovedDir2.Y != -1 {
						rm.entityDir2 = Point{X: 0, Y: 1}
					}
				case kit.KeyLeft:
					if rm.lastMovedDir2.X != 1 {
						rm.entityDir2 = Point{X: -1, Y: 0}
					}
				case kit.KeyRight:
					if rm.lastMovedDir2.X != -1 {
						rm.entityDir2 = Point{X: 1, Y: 0}
					}
				}
			}
		}
	}

	action := kit.Resolve(in, kit.CtxNav)
	switch action {
	case kit.ActConfirm:
		if rm.gameOver {
			rm.reset(r)
		} else {
			rm.gameStarted = !rm.gameStarted
		}
	}

	// Switch Theme support
	if in.Kind == kit.InputRune && (in.Rune == 't' || in.Rune == 'T') {
		rm.themeIndex = (rm.themeIndex + 1) % len(palettes)
	}

	// Switch Mode support. Cycling the mode resets the round, so gate it behind
	// the lobby/game-over states (same as Settings) — a stray 'm' mid-round must
	// not silently wipe an in-progress game.
	if in.Kind == kit.InputRune && (in.Rune == 'm' || in.Rune == 'M') {
		if !rm.gameStarted || rm.gameOver {
			rm.gameMode = (rm.gameMode + 1) % modeCount
			rm.reset(r)
		}
	}

	// Switch Bot support. Only meaningful in a single-seat room: with two human
	// players, seat 0 toggling the bot would hijack seat 1's snake.
	if in.Kind == kit.InputRune && (in.Rune == 'b' || in.Rune == 'B') {
		if len(members) < 2 {
			rm.p2IsBot = !rm.p2IsBot
		}
	}

	// Settings trigger support
	if in.Kind == kit.InputRune && (in.Rune == 's' || in.Rune == 'S') {
		if !rm.gameStarted || rm.gameOver {
			rm.settingsOpen = true
			rm.settingsCursor = 0
		}
	}

	rm.render(r)
}

// OnWake is the host heartbeat.
func (rm *room) OnWake(r kit.Room) {
	now := r.Now()
	if rm.lastTick.IsZero() {
		rm.lastTick = now
	}
	if rm.startedAt.IsZero() {
		rm.startedAt = now
	}
	if rm.lastWake.IsZero() {
		rm.lastWake = now
	}

	elapsedSinceLastWake := now.Sub(rm.lastWake)
	rm.lastWake = now

	// If paused or game over or settings menu is open, shift power-up timers forward by the elapsed real time
	isPausedOrGameOver := !rm.gameStarted || rm.gameOver || rm.settingsOpen
	if isPausedOrGameOver && elapsedSinceLastWake > 0 {
		if !rm.p1PowerUpExpiry.IsZero() {
			rm.p1PowerUpExpiry = rm.p1PowerUpExpiry.Add(elapsedSinceLastWake)
		}
		if !rm.p2PowerUpExpiry.IsZero() {
			rm.p2PowerUpExpiry = rm.p2PowerUpExpiry.Add(elapsedSinceLastWake)
		}
		if rm.powerUpActive && !rm.powerUpSpawnedAt.IsZero() {
			rm.powerUpSpawnedAt = rm.powerUpSpawnedAt.Add(elapsedSinceLastWake)
		}
		if rm.bombActive && !rm.bombSpawnedAt.IsZero() {
			rm.bombSpawnedAt = rm.bombSpawnedAt.Add(elapsedSinceLastWake)
		}
		if rm.bombExploding && !rm.bombExplodedAt.IsZero() {
			rm.bombExplodedAt = rm.bombExplodedAt.Add(elapsedSinceLastWake)
		}
	}

	// Update screen shake, flash, and particles
	isPaused := !rm.gameStarted || rm.settingsOpen
	if isPaused && elapsedSinceLastWake > 0 {
		if !rm.shakeExpiry.IsZero() {
			rm.shakeExpiry = rm.shakeExpiry.Add(elapsedSinceLastWake)
		}
		if !rm.flashExpiry.IsZero() {
			rm.flashExpiry = rm.flashExpiry.Add(elapsedSinceLastWake)
		}
		for i := range rm.particles {
			rm.particles[i].CreatedAt = rm.particles[i].CreatedAt.Add(elapsedSinceLastWake)
		}
	} else if elapsedSinceLastWake > 0 {
		// Update particles with gravity and drag/friction
		var activeParticles []Particle
		for _, p := range rm.particles {
			age := now.Sub(p.CreatedAt)
			if age < p.Duration {
				dt := elapsedSinceLastWake.Seconds()

				// Apply drag/friction (realistic deceleration)
				dragFactor := 3.0
				p.VX -= p.VX * dragFactor * dt
				p.VY -= p.VY * dragFactor * dt

				// Apply gravity (particles fall down slightly over time)
				gravity := 12.0
				p.VY += gravity * dt

				p.X += p.VX * dt
				p.Y += p.VY * dt

				if p.X >= 0 && p.X < 39 && p.Y >= 0 && p.Y < 18 {
					activeParticles = append(activeParticles, p)
				}
			}
		}
		rm.particles = activeParticles
	}

	// Update bomb countdown and explosion lifecycle
	if rm.gameStarted && !rm.gameOver && !rm.settingsOpen && rm.gameMode == ModeBomb {
		if rm.bombActive {
			elapsed := now.Sub(rm.bombSpawnedAt)
			if elapsed >= 5*time.Second {
				rm.bombActive = false
				rm.bombExploding = true
				rm.bombExplodedAt = now

				theme := palettes[rm.themeIndex]
				rm.spawnParticles(r, rm.bombPos.X, rm.bombPos.Y, theme.Obstacle, 35, "bomb")
				rm.triggerFlash(r, 400*time.Millisecond)
				rm.triggerShake(r, 600*time.Millisecond)
			}
		} else if rm.bombExploding {
			elapsed := now.Sub(rm.bombExplodedAt)
			if elapsed >= 1500*time.Millisecond {
				rm.bombExploding = false
				rm.spawnBomb(r)
			}
		}
	}

	// Advance game state based on tickRate
	if rm.gameStarted && !rm.gameOver && !rm.settingsOpen && now.Sub(rm.lastTick) >= rm.tickRate {
		rm.lastTick = now
		rm.tick(r)
	}

	rm.render(r)
}

func (rm *room) loadPersonalBests(r kit.Room) {
	if rm.services.Accounts == nil {
		return
	}
	members := r.Members()
	if len(members) >= 1 {
		p1Acct := rm.services.Accounts.For(members[0])
		if p1Acct != nil {
			store := p1Acct.Store()
			if store != nil {
				val, ok, err := store.Get(context.Background(), "personal_best")
				if err == nil && ok {
					rm.pb1, _ = strconv.Atoi(string(val))
				}
				// On a miss or read error, preserve the in-memory PB rather than
				// clobbering it with 0 (it defaults to 0 for a fresh account).
			}
		}
	}
	if len(members) >= 2 {
		p2Acct := rm.services.Accounts.For(members[1])
		if p2Acct != nil {
			store := p2Acct.Store()
			if store != nil {
				val, ok, err := store.Get(context.Background(), "personal_best")
				if err == nil && ok {
					rm.pb2, _ = strconv.Atoi(string(val))
				}
				// On a miss or read error, preserve the in-memory PB.
			}
		}
	}
}

func (rm *room) savePersonalBests(r kit.Room) {
	if rm.services.Accounts == nil {
		return
	}
	members := r.Members()
	if len(members) >= 2 {
		// Multiplayer
		if rm.score1 > rm.pb1 {
			rm.pb1 = rm.score1
			rm.newPB1 = true
			p1Acct := rm.services.Accounts.For(members[0])
			if p1Acct != nil {
				store := p1Acct.Store()
				if store != nil {
					_ = store.Set(context.Background(), "personal_best", []byte(strconv.Itoa(rm.score1)), kit.MergeMax)
				}
			}
		}
		if rm.score2 > rm.pb2 {
			rm.pb2 = rm.score2
			rm.newPB2 = true
			p2Acct := rm.services.Accounts.For(members[1])
			if p2Acct != nil {
				store := p2Acct.Store()
				if store != nil {
					_ = store.Set(context.Background(), "personal_best", []byte(strconv.Itoa(rm.score2)), kit.MergeMax)
				}
			}
		}
	} else if len(members) == 1 {
		// Single player / Co-op
		maxScore := rm.score1
		if rm.score2 > maxScore {
			maxScore = rm.score2
		}
		if maxScore > rm.pb1 {
			rm.pb1 = maxScore
			rm.newPB1 = true
			p1Acct := rm.services.Accounts.For(members[0])
			if p1Acct != nil {
				store := p1Acct.Store()
				if store != nil {
					_ = store.Set(context.Background(), "personal_best", []byte(strconv.Itoa(maxScore)), kit.MergeMax)
				}
			}
		}
	}
}

func (rm *room) reset(r kit.Room) {
	rm.snake1 = []Point{
		{X: 10, Y: 9},
		{X: 9, Y: 9},
		{X: 8, Y: 9},
		{X: 7, Y: 9},
	}
	rm.entityDir1 = Point{X: 1, Y: 0}
	rm.lastMovedDir1 = Point{X: 1, Y: 0}

	rm.snake2 = []Point{
		{X: 28, Y: 9},
		{X: 29, Y: 9},
		{X: 30, Y: 9},
		{X: 31, Y: 9},
	}
	rm.entityDir2 = Point{X: -1, Y: 0}
	rm.lastMovedDir2 = Point{X: -1, Y: 0}

	rm.crashed1 = false
	rm.crashed2 = false

	rm.score1 = 0
	rm.score2 = 0
	rm.gameOver = false
	rm.gameStarted = true
	rm.settingsOpen = false
	rm.lastTick = r.Now()
	rm.startedAt = r.Now()

	speeds := []int{100, 120, 150, 180, 200}
	startSpeed := 150
	if rm.startSpeedIdx >= 0 && rm.startSpeedIdx < len(speeds) {
		startSpeed = speeds[rm.startSpeedIdx]
	}
	rm.tickRate = time.Duration(startSpeed) * time.Millisecond
	rm.popups = []ScorePopup{}
	rm.lastCollisionAt = time.Time{}
	rm.particles = []Particle{}
	rm.shakeStartedAt = time.Time{}
	rm.shakeExpiry = time.Time{}
	rm.flashExpiry = time.Time{}

	rm.powerUpActive = false
	rm.p1PowerUpType = ""
	rm.p1PowerUpExpiry = time.Time{}
	rm.p2PowerUpType = ""
	rm.p2PowerUpExpiry = time.Time{}
	rm.tickCount = 0
	rm.lastWake = r.Now()
	rm.portalA = Point{X: 9, Y: 9}
	rm.portalB = Point{X: 29, Y: 9}

	if rm.gameMode == ModeBomb {
		rm.spawnBomb(r)
	} else {
		rm.bombActive = false
		rm.bombExploding = false
	}

	// Initialize hazards
	rm.initHazards()

	// Regenerate food and obstacles
	rm.food = rm.randomFreePoint(r, 0)
	rm.obstacles = []Point{}
	for i := 0; i < 3; i++ {
		rm.obstacles = append(rm.obstacles, rm.randomFreePoint(r, 5))
	}

	rm.newPB1 = false
	rm.newPB2 = false
	rm.loadPersonalBests(r)
}

// isFreePoint reports whether p is a valid spawn cell: not on a wall, snake,
// food, obstacle, power-up, hazard, portal, or bomb. When avoidHeadRange > 0 it
// also rejects cells within that Manhattan distance of either snake head.
func (rm *room) isFreePoint(p Point, avoidHeadRange int) bool {
	if rm.isMazeWall(p) {
		return false
	}
	for _, sp := range rm.snake1 {
		if sp == p {
			return false
		}
	}
	for _, sp := range rm.snake2 {
		if sp == p {
			return false
		}
	}
	if p == rm.food {
		return false
	}
	for _, op := range rm.obstacles {
		if op == p {
			return false
		}
	}
	if rm.powerUpActive && p == rm.powerUpPos {
		return false
	}
	for _, hp := range rm.hazards {
		if hp.Pos == p {
			return false
		}
	}
	if rm.gameMode == ModePortal && (p == rm.portalA || p == rm.portalB) {
		return false
	}
	if rm.gameMode == ModeBomb {
		if rm.bombActive && p == rm.bombPos {
			return false
		}
		if rm.bombExploding {
			dx := p.X - rm.bombPos.X
			dy := p.Y - rm.bombPos.Y
			if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
				return false
			}
		}
	}
	if avoidHeadRange > 0 {
		heads := make([]Point, 0, 2)
		if len(rm.snake1) > 0 {
			heads = append(heads, rm.snake1[0])
		}
		if len(rm.snake2) > 0 {
			heads = append(heads, rm.snake2[0])
		}
		for _, h := range heads {
			dx := h.X - p.X
			if dx < 0 {
				dx = -dx
			}
			dy := h.Y - p.Y
			if dy < 0 {
				dy = -dy
			}
			if dx+dy <= avoidHeadRange {
				return false
			}
		}
	}
	return true
}

func (rm *room) randomFreePoint(r kit.Room, avoidHeadRange int) Point {
	// Attempt up to 100 random samples for a free spot.
	for attempt := 0; attempt < 100; attempt++ {
		p := Point{X: r.Rand().Intn(39), Y: r.Rand().Intn(18)}
		if rm.isFreePoint(p, avoidHeadRange) {
			return p
		}
	}
	// Deterministic fallback: scan every cell for the first genuinely free one,
	// relaxing the head-distance preference (which is a nicety, not a hard rule)
	// so we never spawn on top of a snake or wall on a crowded board.
	for y := 0; y < 18; y++ {
		for x := 0; x < 39; x++ {
			p := Point{X: x, Y: y}
			if rm.isFreePoint(p, 0) {
				return p
			}
		}
	}
	// Board is entirely full (not reachable in normal play).
	return Point{X: 10, Y: 10}
}

func (rm *room) tick(r kit.Room) {
	if len(rm.snake1) == 0 || len(rm.snake2) == 0 {
		return
	}
	rm.tickCount++

	now := r.Now()
	p1ShieldActive := !rm.p1PowerUpExpiry.IsZero() && now.Before(rm.p1PowerUpExpiry) && rm.p1PowerUpType == "SHIELD"
	p1FreezeActive := !rm.p1PowerUpExpiry.IsZero() && now.Before(rm.p1PowerUpExpiry) && rm.p1PowerUpType == "FREEZE"

	p2ShieldActive := !rm.p2PowerUpExpiry.IsZero() && now.Before(rm.p2PowerUpExpiry) && rm.p2PowerUpType == "SHIELD"
	p2FreezeActive := !rm.p2PowerUpExpiry.IsZero() && now.Before(rm.p2PowerUpExpiry) && rm.p2PowerUpType == "FREEZE"

	// Save tail segment positions
	tail1 := rm.snake1[len(rm.snake1)-1]
	tail2 := rm.snake2[len(rm.snake2)-1]
	oldHead1 := rm.snake1[0]
	oldHead2 := rm.snake2[0]

	move1 := true
	if p2FreezeActive && rm.tickCount%2 == 0 {
		move1 = false
	}

	move2 := true
	if p1FreezeActive && rm.tickCount%2 == 0 {
		move2 = false
	}

	if move1 {
		// Move Snake 1 body
		for i := len(rm.snake1) - 1; i > 0; i-- {
			rm.snake1[i] = rm.snake1[i-1]
		}
		// Update Snake 1 head position
		head1 := rm.snake1[0]
		if rm.gameMode == ModePortal && head1 == rm.portalA {
			rm.snake1[0].X = (rm.portalB.X + rm.entityDir1.X + 39) % 39
			rm.snake1[0].Y = (rm.portalB.Y + rm.entityDir1.Y + 18) % 18
		} else if rm.gameMode == ModePortal && head1 == rm.portalB {
			rm.snake1[0].X = (rm.portalA.X + rm.entityDir1.X + 39) % 39
			rm.snake1[0].Y = (rm.portalA.Y + rm.entityDir1.Y + 18) % 18
		} else {
			rm.snake1[0].X += rm.entityDir1.X
			rm.snake1[0].Y += rm.entityDir1.Y

			// Wrap boundaries for Snake 1
			if rm.snake1[0].X < 0 {
				rm.snake1[0].X = 38
			} else if rm.snake1[0].X > 38 {
				rm.snake1[0].X = 0
			}
			if rm.snake1[0].Y < 0 {
				rm.snake1[0].Y = 17
			} else if rm.snake1[0].Y > 17 {
				rm.snake1[0].Y = 0
			}
		}
		rm.lastMovedDir1 = rm.entityDir1
	}

	if move2 {
		if rm.p2IsBot {
			oppositeDir := Point{X: -rm.lastMovedDir2.X, Y: -rm.lastMovedDir2.Y}
			target := rm.food
			if rm.powerUpActive {
				target = rm.powerUpPos
			}
			dir, ok := rm.findShortestDir(now, rm.snake2[0], target, oppositeDir, p1FreezeActive, p2FreezeActive)
			if ok {
				rm.entityDir2 = dir
			}
		}
		// Move Snake 2 body
		for i := len(rm.snake2) - 1; i > 0; i-- {
			rm.snake2[i] = rm.snake2[i-1]
		}
		// Update Snake 2 head position
		head2 := rm.snake2[0]
		if rm.gameMode == ModePortal && head2 == rm.portalA {
			rm.snake2[0].X = (rm.portalB.X + rm.entityDir2.X + 39) % 39
			rm.snake2[0].Y = (rm.portalB.Y + rm.entityDir2.Y + 18) % 18
		} else if rm.gameMode == ModePortal && head2 == rm.portalB {
			rm.snake2[0].X = (rm.portalA.X + rm.entityDir2.X + 39) % 39
			rm.snake2[0].Y = (rm.portalA.Y + rm.entityDir2.Y + 18) % 18
		} else {
			rm.snake2[0].X += rm.entityDir2.X
			rm.snake2[0].Y += rm.entityDir2.Y

			// Wrap boundaries for Snake 2
			if rm.snake2[0].X < 0 {
				rm.snake2[0].X = 38
			} else if rm.snake2[0].X > 38 {
				rm.snake2[0].X = 0
			}
			if rm.snake2[0].Y < 0 {
				rm.snake2[0].Y = 17
			} else if rm.snake2[0].Y > 17 {
				rm.snake2[0].Y = 0
			}
		}
		rm.lastMovedDir2 = rm.entityDir2
	}

	// Store old hazard positions for swap collision check
	oldHazards := make([]Point, len(rm.hazards))
	for i, h := range rm.hazards {
		oldHazards[i] = h.Pos
	}

	// Update Patrolling Hazards (frozen if freeze is active)
	hazardsMove := !p1FreezeActive && !p2FreezeActive
	if hazardsMove {
		for i := range rm.hazards {
			h := &rm.hazards[i]
			nextX := h.Pos.X + h.Dir.X
			nextY := h.Pos.Y + h.Dir.Y

			// Check if out of bounds
			if nextX < h.MinX || nextX > h.MaxX || nextY < h.MinY || nextY > h.MaxY {
				h.Dir.X = -h.Dir.X
				h.Dir.Y = -h.Dir.Y
				nextX = h.Pos.X + h.Dir.X
				nextY = h.Pos.Y + h.Dir.Y
			}
			h.Pos.X = nextX
			h.Pos.Y = nextY
		}
	}

	// Check collisions for Snake 1
	c1Self := false
	for _, sp := range rm.snake1[1:] {
		if rm.snake1[0] == sp {
			c1Self = true
		}
	}
	c1Obstacle := false
	for _, op := range rm.obstacles {
		if rm.snake1[0] == op {
			c1Obstacle = true
		}
	}
	c1Maze := rm.isMazeWall(rm.snake1[0])
	c1Hazard := false
	for i, hp := range rm.hazards {
		if rm.snake1[0] == hp.Pos || (rm.snake1[0] == oldHazards[i] && oldHead1 == hp.Pos) {
			c1Hazard = true
		}
	}
	c1Snake2 := false
	for _, sp := range rm.snake2 {
		if rm.snake1[0] == sp {
			c1Snake2 = true
		}
	}

	c1Bomb := false
	c2Bomb := false
	if rm.gameMode == ModeBomb {
		if rm.bombActive {
			if rm.snake1[0] == rm.bombPos {
				rm.bombActive = false
				rm.bombExploding = true
				rm.bombExplodedAt = now
				c1Bomb = true

				theme := palettes[rm.themeIndex]
				rm.spawnParticles(r, rm.bombPos.X, rm.bombPos.Y, theme.Obstacle, 35, "bomb")
				rm.triggerFlash(r, 400*time.Millisecond)
				rm.triggerShake(r, 600*time.Millisecond)
			}
			if rm.snake2[0] == rm.bombPos {
				rm.bombActive = false
				rm.bombExploding = true
				rm.bombExplodedAt = now
				c2Bomb = true

				theme := palettes[rm.themeIndex]
				rm.spawnParticles(r, rm.bombPos.X, rm.bombPos.Y, theme.Obstacle, 35, "bomb")
				rm.triggerFlash(r, 400*time.Millisecond)
				rm.triggerShake(r, 600*time.Millisecond)
			}
		}
		if rm.bombExploding {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					bx := (rm.bombPos.X + dx + 39) % 39
					by := (rm.bombPos.Y + dy + 18) % 18
					if rm.snake1[0].X == bx && rm.snake1[0].Y == by {
						c1Bomb = true
					}
					if rm.snake2[0].X == bx && rm.snake2[0].Y == by {
						c2Bomb = true
					}
				}
			}
		}
	}

	if (c1Self || c1Obstacle || c1Maze || c1Hazard || c1Snake2 || c1Bomb) && !p1ShieldActive {
		rm.crashed1 = true
	}

	// Check collisions for Snake 2
	c2Self := false
	for _, sp := range rm.snake2[1:] {
		if rm.snake2[0] == sp {
			c2Self = true
		}
	}
	c2Obstacle := false
	for _, op := range rm.obstacles {
		if rm.snake2[0] == op {
			c2Obstacle = true
		}
	}
	c2Maze := rm.isMazeWall(rm.snake2[0])
	c2Hazard := false
	for i, hp := range rm.hazards {
		if rm.snake2[0] == hp.Pos || (rm.snake2[0] == oldHazards[i] && oldHead2 == hp.Pos) {
			c2Hazard = true
		}
	}
	c2Snake1 := false
	for _, sp := range rm.snake1 {
		if rm.snake2[0] == sp {
			c2Snake1 = true
		}
	}

	if (c2Self || c2Obstacle || c2Maze || c2Hazard || c2Snake1 || c2Bomb) && !p2ShieldActive {
		rm.crashed2 = true
	}

	// If either crashed, it's Game Over!
	if rm.crashed1 || rm.crashed2 {
		rm.gameOver = true
		rm.lastCollisionAt = r.Now()
		rm.savePersonalBests(r)

		rm.triggerFlash(r, 300*time.Millisecond)
		rm.triggerShake(r, 500*time.Millisecond)
		theme := palettes[rm.themeIndex]
		if rm.crashed1 {
			rm.spawnParticles(r, rm.snake1[0].X, rm.snake1[0].Y, theme.SnakeHead, 25, "crash")
		}
		if rm.crashed2 {
			rm.spawnParticles(r, rm.snake2[0].X, rm.snake2[0].Y, theme.Snake2Head, 25, "crash")
		}

		// Post results to the leaderboard
		members := r.Members()
		if len(members) >= 2 {
			p1Result := kit.PlayerResult{
				Player: members[0],
				Metric: rm.score1,
				Status: kit.StatusFinished,
			}
			p2Result := kit.PlayerResult{
				Player: members[1],
				Metric: rm.score2,
				Status: kit.StatusFinished,
			}
			r.Post(kit.Result{
				Rankings: []kit.PlayerResult{p1Result, p2Result},
			})
		} else {
			// Single player controls both snakes (co-op), so report the better of
			// the two scores — matching savePersonalBests, which maxes both for PB.
			soloScore := rm.score1
			if rm.score2 > soloScore {
				soloScore = rm.score2
			}
			r.Post(kit.Result{
				Rankings: []kit.PlayerResult{
					{
						Player: rm.getActivePlayer(r),
						Metric: soloScore,
						Status: kit.StatusFinished,
					},
				},
			})
		}
		return
	}

	// Check power-up collision
	if rm.powerUpActive {
		// Check if 8 seconds expired
		if now.Sub(rm.powerUpSpawnedAt) >= 8*time.Second {
			rm.powerUpActive = false
		} else {
			// Check Snake 1 head
			if rm.snake1[0] == rm.powerUpPos {
				rm.p1PowerUpType = rm.powerUpType
				rm.p1PowerUpExpiry = now.Add(6 * time.Second)
				rm.powerUpActive = false

				// Trigger score popup / text popup
				theme := palettes[rm.themeIndex]
				popupText := "+" + rm.powerUpType
				rm.popups = append(rm.popups, ScorePopup{
					X:         rm.powerUpPos.X,
					Y:         rm.powerUpPos.Y,
					Text:      popupText,
					Color:     theme.SnakeHead,
					CreatedAt: now,
				})
			} else if rm.snake2[0] == rm.powerUpPos {
				// Check Snake 2 head
				rm.p2PowerUpType = rm.powerUpType
				rm.p2PowerUpExpiry = now.Add(6 * time.Second)
				rm.powerUpActive = false

				// Trigger score popup / text popup
				theme := palettes[rm.themeIndex]
				popupText := "+" + rm.powerUpType
				rm.popups = append(rm.popups, ScorePopup{
					X:         rm.powerUpPos.X,
					Y:         rm.powerUpPos.Y,
					Text:      popupText,
					Color:     theme.Snake2Head,
					CreatedAt: now,
				})
			}
		}
	}

	// Check food collision for both snakes independently. Tie rule: if both
	// heads land on the food the same tick (possible with shields up), both
	// score and grow; the food is then relocated once.
	ate1 := rm.snake1[0] == rm.food
	ate2 := rm.snake2[0] == rm.food
	if ate1 {
		rm.snake1 = append(rm.snake1, tail1)
		rm.score1 += 10
		if rm.score1 > rm.highScore {
			rm.highScore = rm.score1
		}
	}
	if ate2 {
		rm.snake2 = append(rm.snake2, tail2)
		rm.score2 += 10
		if rm.score2 > rm.highScore {
			rm.highScore = rm.score2
		}
	}
	if ate1 || ate2 {
		eater := 1
		if ate2 && !ate1 {
			eater = 2
		}
		rm.onFoodEaten(r, rm.food, eater)
	}
	if ate1 || ate2 {
		// Keep the ScoreKeeper's view of each player's current score current, so a
		// mid-game disconnect (OnLeave) can flush it. Mirrors the seat→snake map:
		// members[0] drives snake1, members[1] snake2; solo controls both.
		rm.recordScores(r)
	}
}

// recordScores feeds each player's current score into the ScoreKeeper. In a
// head-to-head room members[0] owns snake1's score and members[1] owns snake2's;
// in a solo/co-op room the single member controls both snakes, so we record the
// better of the two (matching the game-over Post and PB rules).
func (rm *room) recordScores(r kit.Room) {
	if rm.sk == nil {
		return
	}
	members := r.Members()
	if len(members) >= 2 {
		rm.sk.Record(r, members[0], rm.score1)
		rm.sk.Record(r, members[1], rm.score2)
	} else if len(members) == 1 {
		soloScore := rm.score1
		if rm.score2 > soloScore {
			soloScore = rm.score2
		}
		rm.sk.Record(r, members[0], soloScore)
	}
}

// speedForScore returns the tick interval for the current combined score,
// ramping down from the configured start speed to a 60ms floor.
func (rm *room) speedForScore(speeds []int) time.Duration {
	startSpeed := 150
	if rm.startSpeedIdx >= 0 && rm.startSpeedIdx < len(speeds) {
		startSpeed = speeds[rm.startSpeedIdx]
	}
	totalScore := rm.score1 + rm.score2
	speedMs := startSpeed - (totalScore/10)*5
	if speedMs < 60 {
		speedMs = 60
	}
	return time.Duration(speedMs) * time.Millisecond
}

func (rm *room) onFoodEaten(r kit.Room, foodPos Point, snakeNum int) {
	rm.tickRate = rm.speedForScore(startSpeeds)

	theme := palettes[rm.themeIndex]
	var popupColor kit.Color
	if snakeNum == 1 {
		popupColor = theme.SnakeHead
	} else {
		popupColor = theme.Snake2Head
	}
	rm.popups = append(rm.popups, ScorePopup{
		X:         foodPos.X,
		Y:         foodPos.Y,
		Text:      "+10",
		Color:     popupColor,
		CreatedAt: r.Now(),
	})

	// Spawn particles and screen FX
	rm.spawnParticles(r, foodPos.X, foodPos.Y, popupColor, 12, "food")
	rm.triggerFlash(r, 80*time.Millisecond)
	rm.triggerShake(r, 80*time.Millisecond)

	rm.food = rm.randomFreePoint(r, 0)
	// Grow obstacles by one per food, but cap the total so the board can't fill
	// up and strand future spawns.
	if len(rm.obstacles) < maxObstacles {
		rm.obstacles = append(rm.obstacles, rm.randomFreePoint(r, 4))
	}

	// Spawn a power-up if not already active (25% chance per food eaten).
	if !rm.powerUpActive {
		if r.Rand().Intn(100) < 25 {
			pType := "SHIELD"
			if r.Rand().Intn(2) == 0 {
				pType = "FREEZE"
			}
			rm.powerUpPos = rm.randomFreePoint(r, 2)
			rm.powerUpType = pType
			rm.powerUpActive = true
			rm.powerUpSpawnedAt = r.Now()
		}
	}
}

func centerText(text string, width int) string {
	// Measure and slice by runes, not bytes: multibyte content (emoji, the ✨ in
	// "NEW PERSONAL BEST!", long player handles) would otherwise be mis-centered
	// and a byte slice could cut mid-rune.
	rs := []rune(text)
	if len(rs) >= width {
		return string(rs[:width])
	}
	pad := (width - len(rs)) / 2
	return fmt.Sprintf("%*s%s", pad, "", text)
}

func (rm *room) render(r kit.Room) {
	if rm.frame == nil {
		rm.frame = kit.NewFrame()
	}
	f := rm.frame
	f.Clear()

	now := r.Now()
	theme := palettes[rm.themeIndex]

	// Styles
	headerStyle := kit.Style{FG: theme.Header, Attr: kit.AttrBold}
	valueStyle := kit.Style{FG: kit.RGB(0xff, 0xff, 0xff)}
	dotStyle := kit.Style{FG: theme.Dot}
	footerStyle := kit.Style{FG: theme.Footer}
	keyStyle := kit.Style{FG: theme.Key, Attr: kit.AttrBold}

	// 1. Draw Border (Animates dynamically)
	rm.drawBorder(f, now)

	members := r.Members()

	// 2. Draw Header Content
	f.Text(1, 2, "▲▼ NEON DUEL ▲▼", headerStyle)

	if len(members) >= 2 {
		f.Text(1, 19, "P1:", headerStyle)
		f.Text(1, 22, fmt.Sprintf("%04d", rm.score1), valueStyle)
		f.Text(1, 27, "P2:", headerStyle)
		f.Text(1, 30, fmt.Sprintf("%04d", rm.score2), valueStyle)
	} else {
		f.Text(1, 19, "S1:", headerStyle)
		f.Text(1, 22, fmt.Sprintf("%04d", rm.score1), valueStyle)
		f.Text(1, 27, "S2:", headerStyle)
		f.Text(1, 30, fmt.Sprintf("%04d", rm.score2), valueStyle)
	}

	f.Text(1, 36, "HI:", headerStyle)
	f.Text(1, 39, fmt.Sprintf("%04d", rm.highScore), valueStyle)

	f.Text(1, 54, "THEME:", headerStyle)
	f.Text(1, 60, theme.Name, valueStyle)

	// Pulsing status effect (right-aligned to column 78)
	var statusText string
	var statusStyle kit.Style
	if rm.gameOver {
		statusText = "GAME OVER"
		statusStyle = kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
	} else if !rm.gameStarted {
		statusText = "PAUSED"
		statusStyle = kit.Style{FG: kit.RGB(0xff, 0xff, 0x00), Attr: kit.AttrBold}
	} else {
		statusText = "PLAYING"
		elapsed := now.Sub(rm.startedAt)
		pulse := (elapsed.Milliseconds() / 150) % 10
		if pulse < 5 {
			statusStyle = kit.Style{FG: theme.Key, Attr: kit.AttrBold}
		} else {
			statusStyle = kit.Style{FG: brightenColor(theme.Key, 0.6)}
		}
	}
	f.Text(1, 78-len(statusText), statusText, statusStyle)

	// Create lookup for coordinates to skip drawing grid dots
	// Reuse a cleared map across frames instead of allocating one every render
	// (~20fps in-guest).
	if rm.occupied == nil {
		rm.occupied = make(map[Point]bool)
	} else {
		clear(rm.occupied)
	}
	occupied := rm.occupied
	for _, sp := range rm.snake1 {
		occupied[sp] = true
	}
	for _, sp := range rm.snake2 {
		occupied[sp] = true
	}
	occupied[rm.food] = true
	for _, op := range rm.obstacles {
		occupied[op] = true
	}
	if rm.powerUpActive {
		occupied[rm.powerUpPos] = true
	}
	if rm.gameMode == ModePortal {
		occupied[rm.portalA] = true
		occupied[rm.portalB] = true
	}
	if rm.gameMode == ModeBomb {
		if rm.bombActive {
			occupied[rm.bombPos] = true
		}
		if rm.bombExploding {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					bx := (rm.bombPos.X + dx + 39) % 39
					by := (rm.bombPos.Y + dy + 18) % 18
					occupied[Point{X: bx, Y: by}] = true
				}
			}
		}
	}

	p1ShieldActive := !rm.p1PowerUpExpiry.IsZero() && now.Before(rm.p1PowerUpExpiry) && rm.p1PowerUpType == "SHIELD"
	p1FreezeActive := !rm.p1PowerUpExpiry.IsZero() && now.Before(rm.p1PowerUpExpiry) && rm.p1PowerUpType == "FREEZE"

	p2ShieldActive := !rm.p2PowerUpExpiry.IsZero() && now.Before(rm.p2PowerUpExpiry) && rm.p2PowerUpType == "SHIELD"
	p2FreezeActive := !rm.p2PowerUpExpiry.IsZero() && now.Before(rm.p2PowerUpExpiry) && rm.p2PowerUpType == "FREEZE"

	// 3. Draw Grid Dots
	if rm.gridDotsEnabled {
		for y := 0; y < 18; y++ {
			for x := 0; x < 39; x++ {
				p := Point{X: x, Y: y}
				if occupied[p] || rm.isMazeWall(p) {
					continue
				}
				dStyle := dotStyle
				if rm.flashOption > 0 && !rm.flashExpiry.IsZero() && now.Before(rm.flashExpiry) {
					flashCycle := (now.UnixNano() / 150000000) % 2
					if flashCycle == 0 {
						if rm.flashOption == 1 {
							// Gentle flash: use theme border color (muted)
							dStyle = kit.Style{FG: theme.Border}
						} else {
							// Strong flash: flash bright white
							dStyle = kit.Style{FG: kit.RGB(0xff, 0xff, 0xff)}
						}
					} else {
						dStyle = kit.Style{FG: theme.Dot}
					}
				}
				f.SetRune(3+y, 1+x*2, '·', dStyle)
			}
		}
	}

	// 3.5 Draw Maze Walls if in ModeMaze
	if rm.gameMode == ModeMaze {
		wallStyle := kit.Style{FG: theme.Border, Attr: kit.AttrBold}
		for y := 0; y < 18; y++ {
			for x := 0; x < 39; x++ {
				p := Point{X: x, Y: y}
				if rm.isMazeWall(p) {
					f.SetRune(3+y, 1+x*2, '▒', wallStyle)
				}
			}
		}
	}

	// 3.6 Draw Portals if in ModePortal
	if rm.gameMode == ModePortal {
		elapsed := now.Sub(rm.startedAt)
		pulseA := 0.75 + 0.25*math.Sin(float64(elapsed.Milliseconds())*0.01)
		pulseB := 0.75 + 0.25*math.Sin(float64(elapsed.Milliseconds())*0.01+math.Pi)

		// Portal A: neon blue/cyan
		colorA := brightenColor(kit.RGB(0x00, 0xd2, 0xff), pulseA)
		styleA := kit.Style{FG: colorA, Attr: kit.AttrBold}

		// Portal B: neon orange/gold
		colorB := brightenColor(kit.RGB(0xff, 0x9d, 0x00), pulseB)
		styleB := kit.Style{FG: colorB, Attr: kit.AttrBold}

		f.SetRune(3+rm.portalA.Y, 1+rm.portalA.X*2, '◎', styleA)
		f.SetRune(3+rm.portalB.Y, 1+rm.portalB.X*2, '◎', styleB)
	}

	// 3.7 Draw Bomb if in ModeBomb
	if rm.gameMode == ModeBomb {
		if rm.bombActive {
			elapsed := now.Sub(rm.bombSpawnedAt)
			secs := 5 - int(elapsed.Seconds())
			if secs < 1 {
				secs = 1
			}
			if secs > 5 {
				secs = 5
			}
			bombColor := kit.RGB(0xff, 0x55, 0x00)
			if secs <= 2 {
				flash := (now.UnixNano() / 200000000) % 2
				if flash == 0 {
					bombColor = kit.RGB(0xff, 0xff, 0xff)
				} else {
					bombColor = kit.RGB(0xff, 0x00, 0x00)
				}
			}
			bombStyle := kit.Style{FG: bombColor, Attr: kit.AttrBold}
			f.Text(3+rm.bombPos.Y, 1+rm.bombPos.X*2, fmt.Sprintf("✹%d", secs), bombStyle)
		} else if rm.bombExploding {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					bx := (rm.bombPos.X + dx + 39) % 39
					by := (rm.bombPos.Y + dy + 18) % 18

					var fireColor kit.Color
					if dx == 0 && dy == 0 {
						fireColor = kit.RGB(0xff, 0xff, 0xff)
					} else {
						flash := (now.UnixNano() / 100000000) % 2
						if flash == 0 {
							fireColor = kit.RGB(0xff, 0xa5, 0x00)
						} else {
							fireColor = kit.RGB(0xff, 0x00, 0x00)
						}
					}
					fireStyle := kit.Style{FG: fireColor, Attr: kit.AttrBold}
					f.SetRune(3+by, 1+bx*2, '░', fireStyle)
				}
			}
		}
	}

	// 4. Draw Food (Pulsing glowing neon star, rotating/twinkling glyph)
	elapsed := now.Sub(rm.startedAt)
	foodPulse := (elapsed.Milliseconds() / 150) % 6
	var foodColor kit.Color
	switch foodPulse {
	case 0, 5:
		foodColor = theme.Food
	case 1, 4:
		foodColor = brightenColor(theme.Food, 1.2)
	default:
		foodColor = brightenColor(theme.Food, 1.5)
	}
	foodStyle := kit.Style{FG: foodColor, Attr: kit.AttrBold}

	foodGlyphs := []rune{'★', '☆', '✦', '✧'}
	foodGlyphIdx := (elapsed.Milliseconds() / 250) % int64(len(foodGlyphs))
	foodGlyph := foodGlyphs[foodGlyphIdx]
	f.SetRune(3+rm.food.Y, 1+rm.food.X*2, foodGlyph, foodStyle)

	// 4.5 Draw Power-Up on field if active
	if rm.powerUpActive {
		var powerUpStyle kit.Style
		var powerUpGlyph rune
		if rm.powerUpType == "SHIELD" {
			shieldPulse := 0.75 + 0.25*math.Sin(float64(elapsed.Milliseconds())*0.01)
			powerUpStyle = kit.Style{FG: brightenColor(kit.RGB(0xff, 0xd7, 0x00), shieldPulse), Attr: kit.AttrBold}
			powerUpGlyph = '🛡'
		} else {
			freezePulse := 0.75 + 0.25*math.Sin(float64(elapsed.Milliseconds())*0.01)
			powerUpStyle = kit.Style{FG: brightenColor(kit.RGB(0x00, 0xff, 0xff), freezePulse), Attr: kit.AttrBold}
			powerUpGlyph = '❄'
		}
		f.SetWide(3+rm.powerUpPos.Y, 1+rm.powerUpPos.X*2, powerUpGlyph, powerUpStyle)
	}

	// 5. Draw Obstacles (Neon triangles with warning flash when head is close)
	for _, op := range rm.obstacles {
		var dist1, dist2 int
		if len(rm.snake1) > 0 {
			head1 := rm.snake1[0]
			dx := head1.X - op.X
			dy := head1.Y - op.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			dist1 = dx + dy
		} else {
			dist1 = 999
		}
		if len(rm.snake2) > 0 {
			head2 := rm.snake2[0]
			dx := head2.X - op.X
			dy := head2.Y - op.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			dist2 = dx + dy
		} else {
			dist2 = 999
		}

		minDist := dist1
		if dist2 < minDist {
			minDist = dist2
		}

		var obstacleStyle kit.Style
		if minDist <= 2 && rm.gameStarted && !rm.gameOver {
			flashCycle := (elapsed.Milliseconds() / 100) % 2
			if flashCycle == 0 {
				obstacleStyle = kit.Style{FG: kit.RGB(0xff, 0xff, 0xff), Attr: kit.AttrBold}
			} else {
				obstacleStyle = kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
			}
		} else {
			obstacleStyle = kit.Style{FG: theme.Obstacle, Attr: kit.AttrBold}
		}

		f.SetRune(3+op.Y, 1+op.X*2, '▲', obstacleStyle)
	}

	// 5.5 Draw Patrolling Hazards (Neon diamond with pulse and head warning flash)
	for _, hp := range rm.hazards {
		var dist1, dist2 int
		if len(rm.snake1) > 0 {
			head1 := rm.snake1[0]
			dx := head1.X - hp.Pos.X
			dy := head1.Y - hp.Pos.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			dist1 = dx + dy
		} else {
			dist1 = 999
		}
		if len(rm.snake2) > 0 {
			head2 := rm.snake2[0]
			dx := head2.X - hp.Pos.X
			dy := head2.Y - hp.Pos.Y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			dist2 = dx + dy
		} else {
			dist2 = 999
		}

		minDist := dist1
		if dist2 < minDist {
			minDist = dist2
		}

		var hazardStyle kit.Style
		if minDist <= 2 && rm.gameStarted && !rm.gameOver {
			flashCycle := (elapsed.Milliseconds() / 100) % 2
			if flashCycle == 0 {
				hazardStyle = kit.Style{FG: kit.RGB(0xff, 0xff, 0xff), Attr: kit.AttrBold}
			} else {
				hazardStyle = kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
			}
		} else {
			hazardPulse := 0.75 + 0.25*math.Sin(float64(elapsed.Milliseconds())*0.008)
			hazardStyle = kit.Style{FG: brightenColor(theme.Hazard, hazardPulse), Attr: kit.AttrBold}
		}

		f.SetRune(3+hp.Pos.Y, 1+hp.Pos.X*2, '❖', hazardStyle)
	}

	// 6. Draw Snake 1 (gradient from SnakeHead to SnakeTail, flowing dynamically)
	skins := []rune{'█', '◆', '●', '■', '★'}
	n1 := len(rm.snake1)
	timeShift := float64(now.Sub(rm.startedAt).Milliseconds()%2000) / 2000.0
	for i := n1 - 1; i >= 0; i-- {
		p := rm.snake1[i]
		var segmentStyle kit.Style
		if p1ShieldActive {
			shieldPulse := 0.75 + 0.25*math.Sin(float64(now.Sub(rm.startedAt).Milliseconds()-int64(i*50))*0.01)
			segmentStyle = kit.Style{FG: brightenColor(kit.RGB(0xff, 0xd7, 0x00), shieldPulse), Attr: kit.AttrBold}
		} else if p2FreezeActive {
			segmentStyle = kit.Style{FG: interpolateColor(theme.SnakeHead, kit.RGB(0x00, 0xbf, 0xff), 0.5), Attr: kit.AttrDim}
		} else {
			if i == 0 {
				headPulse := 0.85 + 0.15*math.Sin(float64(now.Sub(rm.startedAt).Milliseconds())*0.006)
				segmentStyle = kit.Style{FG: brightenColor(theme.SnakeHead, headPulse)}
			} else {
				ratio := float64(i) / float64(n1-1)
				shiftedRatio := ratio + timeShift
				if shiftedRatio > 1.0 {
					shiftedRatio -= 1.0
				}
				segmentStyle = kit.Style{FG: interpolateColor(theme.SnakeHead, theme.SnakeTail, shiftedRatio)}
			}
		}
		skinGlyph1 := '█'
		if rm.snake1SkinIdx >= 0 && rm.snake1SkinIdx < len(skins) {
			skinGlyph1 = skins[rm.snake1SkinIdx]
		}
		f.SetRune(3+p.Y, 1+p.X*2, skinGlyph1, segmentStyle)
	}

	// Draw Snake 2 (gradient from Snake2Head to Snake2Tail, flowing dynamically)
	n2 := len(rm.snake2)
	for i := n2 - 1; i >= 0; i-- {
		p := rm.snake2[i]
		var segmentStyle kit.Style
		if p2ShieldActive {
			shieldPulse := 0.75 + 0.25*math.Sin(float64(now.Sub(rm.startedAt).Milliseconds()-int64(i*50))*0.01)
			segmentStyle = kit.Style{FG: brightenColor(kit.RGB(0xff, 0xd7, 0x00), shieldPulse), Attr: kit.AttrBold}
		} else if p1FreezeActive {
			segmentStyle = kit.Style{FG: interpolateColor(theme.Snake2Head, kit.RGB(0x00, 0xbf, 0xff), 0.5), Attr: kit.AttrDim}
		} else {
			if i == 0 {
				headPulse := 0.85 + 0.15*math.Sin(float64(now.Sub(rm.startedAt).Milliseconds())*0.006)
				segmentStyle = kit.Style{FG: brightenColor(theme.Snake2Head, headPulse)}
			} else {
				ratio := float64(i) / float64(n2-1)
				shiftedRatio := ratio + timeShift
				if shiftedRatio > 1.0 {
					shiftedRatio -= 1.0
				}
				segmentStyle = kit.Style{FG: interpolateColor(theme.Snake2Head, theme.Snake2Tail, shiftedRatio)}
			}
		}
		skinGlyph2 := '█'
		if rm.snake2SkinIdx >= 0 && rm.snake2SkinIdx < len(skins) {
			skinGlyph2 = skins[rm.snake2SkinIdx]
		}
		f.SetRune(3+p.Y, 1+p.X*2, skinGlyph2, segmentStyle)
	}

	// 6.5 Draw Particles
	for _, p := range rm.particles {
		px := int(math.Round(p.X))
		py := int(math.Round(p.Y))
		if px >= 0 && px < 39 && py >= 0 && py < 18 {
			style := kit.Style{FG: p.Color, Attr: kit.AttrBold}
			age := now.Sub(p.CreatedAt)
			lifeRatio := age.Seconds() / p.Duration.Seconds()
			if lifeRatio > 0.6 {
				style.Attr = kit.AttrDim
			}
			f.SetRune(3+py, 1+px*2, p.Glyph, style)
		}
	}

	// 7. Draw Floating Score Popups
	var activePopups []ScorePopup
	for _, p := range rm.popups {
		age := now.Sub(p.CreatedAt)
		if age < 1000*time.Millisecond {
			yOffset := int(age.Milliseconds() / 200)
			drawY := 3 + p.Y - yOffset
			drawX := 1 + p.X*2
			if drawX > 75 {
				drawX = 75
			}
			if drawY > 2 && drawY < 21 {
				style := kit.Style{FG: p.Color, Attr: kit.AttrBold}
				if age > 500*time.Millisecond {
					style.Attr = kit.AttrDim
				}
				f.Text(drawY, drawX, p.Text, style)
			}
			activePopups = append(activePopups, p)
		}
	}
	rm.popups = activePopups

	// 7.5 Draw Divider on row 21 with Mode and player text
	// Divider status uses all-narrow ASCII tokens (not the 🛡/❄ emoji) so the
	// centered text measures one column per rune and never overruns the border.
	var dividerText string
	p1Status := ""
	if p1ShieldActive {
		remSec := int(math.Ceil(rm.p1PowerUpExpiry.Sub(now).Seconds()))
		p1Status = fmt.Sprintf(" [SHIELD %ds]", remSec)
	} else if p1FreezeActive {
		remSec := int(math.Ceil(rm.p1PowerUpExpiry.Sub(now).Seconds()))
		p1Status = fmt.Sprintf(" [FREEZE %ds]", remSec)
	}

	p2Status := ""
	if p2ShieldActive {
		remSec := int(math.Ceil(rm.p2PowerUpExpiry.Sub(now).Seconds()))
		p2Status = fmt.Sprintf(" [SHIELD %ds]", remSec)
	} else if p2FreezeActive {
		remSec := int(math.Ceil(rm.p2PowerUpExpiry.Sub(now).Seconds()))
		p2Status = fmt.Sprintf(" [FREEZE %ds]", remSec)
	}

	if len(members) >= 2 {
		dividerText = fmt.Sprintf(" MODE: %s │ P1: %s%s VS P2: %s%s ", rm.getModeName(), members[0].Handle, p1Status, members[1].Handle, p2Status)
	} else {
		botText := "CO-OP"
		if rm.p2IsBot {
			botText = "AI-BOT"
		}
		dividerText = fmt.Sprintf(" MODE: %s │ PLAYSTYLE: %s %s%s ", rm.getModeName(), botText, p1Status, p2Status)
	}
	rm.drawDividerWithText(f, 21, dividerText, now)

	// 8. Draw Footer Content
	if rm.settingsOpen {
		f.Text(22, 2, "CONTROLS:", footerStyle)
		col := 12
		col = f.Text(22, col, " [", footerStyle)
		col = f.Text(22, col, "▲▼/WASD", keyStyle)
		col = f.Text(22, col, "] Navigate", footerStyle)

		col = f.Text(22, col+1, " [", footerStyle)
		col = f.Text(22, col, "◀▶/AD", keyStyle)
		col = f.Text(22, col, "] Change", footerStyle)

		col = f.Text(22, col+1, " [", footerStyle)
		col = f.Text(22, col, "Spc", keyStyle)
		col = f.Text(22, col, "] Close", footerStyle)

		for c := col; c < 79; c++ {
			f.SetRune(22, c, ' ', footerStyle)
		}
	} else {
		f.Text(22, 2, "CONTROLS:", footerStyle)
		col := 12
		col = f.Text(22, col, " [", footerStyle)
		if len(members) >= 2 {
			col = f.Text(22, col, "P1:WASD P2:Arrows", keyStyle)
		} else {
			if rm.p2IsBot {
				col = f.Text(22, col, "WASD/Bot", keyStyle)
			} else {
				col = f.Text(22, col, "WASD/Arrows", keyStyle)
			}
		}
		col = f.Text(22, col, "] Move", footerStyle)

		col = f.Text(22, col+1, " [", footerStyle)
		col = f.Text(22, col, "T", keyStyle)
		col = f.Text(22, col, "]Theme", footerStyle)

		col = f.Text(22, col+1, " [", footerStyle)
		col = f.Text(22, col, "M", keyStyle)
		col = f.Text(22, col, "]Mode", footerStyle)

		if len(members) < 2 {
			col = f.Text(22, col+1, " [", footerStyle)
			col = f.Text(22, col, "B", keyStyle)
			col = f.Text(22, col, "]Bot", footerStyle)
		}

		col = f.Text(22, col+1, " [", footerStyle)
		if !rm.gameStarted || rm.gameOver {
			col = f.Text(22, col, "S", keyStyle)
			col = f.Text(22, col, "]Settings", footerStyle)
		} else {
			col = f.Text(22, col, "Spc", keyStyle)
			col = f.Text(22, col, "]Pause", footerStyle)
		}

		col = f.Text(22, col+1, " [", footerStyle)
		col = f.Text(22, col, "Esc", keyStyle)
		col = f.Text(22, col, "]Quit", footerStyle)
	}

	// 9. Draw Game Over Overlay
	if rm.gameOver && !rm.settingsOpen {
		modalStyle := kit.Style{FG: theme.ModalBorder, Attr: kit.AttrBold}
		textStyle := kit.Style{FG: kit.RGB(0xff, 0xff, 0xff), Attr: kit.AttrBold}
		subTextStyle := kit.Style{FG: theme.Border}

		f.Text(8, 20, "╔══════════════════════════════════════╗", modalStyle)
		f.Text(9, 20, "║                                      ║", modalStyle)
		f.Text(10, 20, "║              GAME OVER               ║", modalStyle)
		f.Text(11, 20, "║                                      ║", modalStyle)
		f.Text(12, 20, "║                                      ║", modalStyle)
		f.Text(13, 20, "║                                      ║", modalStyle)
		f.Text(14, 20, "╚══════════════════════════════════════╝", modalStyle)

		gameOverPulse := (elapsed.Milliseconds() / 250) % 2
		var titleStyle kit.Style
		if gameOverPulse == 0 {
			titleStyle = kit.Style{FG: theme.ModalBorder, Attr: kit.AttrBold}
		} else {
			titleStyle = kit.Style{FG: theme.Key, Attr: kit.AttrBold}
		}
		f.Text(10, 35, "GAME OVER", titleStyle)

		var winnerMsg string
		if rm.crashed1 && rm.crashed2 {
			winnerMsg = "DRAW / MUTUAL CRASH"
		} else if rm.crashed1 {
			if len(members) >= 2 {
				winnerMsg = fmt.Sprintf("%s WINS!", members[1].Handle)
			} else {
				winnerMsg = "SNAKE 2 WINS!"
			}
		} else {
			if len(members) >= 2 {
				winnerMsg = fmt.Sprintf("%s WINS!", members[0].Handle)
			} else {
				winnerMsg = "SNAKE 1 WINS!"
			}
		}

		scoreText := fmt.Sprintf("S1: %04d  VS  S2: %04d", rm.score1, rm.score2)

		f.Text(11, 21, centerText(winnerMsg, 38), textStyle)
		f.Text(12, 21, centerText(scoreText, 38), textStyle)

		f.Text(13, 25, "Press [SPACE] to Restart", subTextStyle)
	}

	// 9.5 Draw Settings Overlay
	if rm.settingsOpen {
		rm.drawSettingsModal(f, now)
	}

	// Send viewport to each member
	for _, p := range r.Members() {
		pb := rm.pb1
		isNewPB := rm.newPB1
		if len(members) >= 2 && p.AccountID == members[1].AccountID {
			pb = rm.pb2
			isNewPB = rm.newPB2
		}

		currentPB := pb
		isCurrentlyNewPB := isNewPB

		if len(members) >= 2 {
			if p.AccountID == members[0].AccountID {
				if rm.score1 > pb {
					currentPB = rm.score1
					isCurrentlyNewPB = true
				}
			} else {
				if rm.score2 > pb {
					currentPB = rm.score2
					isCurrentlyNewPB = true
				}
			}
		} else {
			maxScore := rm.score1
			if rm.score2 > maxScore {
				maxScore = rm.score2
			}
			if maxScore > pb {
				currentPB = maxScore
				isCurrentlyNewPB = true
			}
		}

		var pbText string
		var pbValStyle kit.Style
		if isCurrentlyNewPB {
			elapsed := now.Sub(rm.startedAt)
			pulse := (elapsed.Milliseconds() / 200) % 2
			if pulse == 0 {
				pbText = "*NEW*"
				pbValStyle = kit.Style{FG: kit.RGB(0xff, 0xff, 0x00), Attr: kit.AttrBold}
			} else {
				pbText = fmt.Sprintf("%04d", currentPB)
				pbValStyle = kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
			}
		} else {
			pbText = fmt.Sprintf("%04d", currentPB)
			pbValStyle = valueStyle
		}

		f.Text(1, 45, "PB:", headerStyle)
		f.Text(1, 48, pbText, pbValStyle)

		if rm.gameOver {
			var pbMsg string
			var pbMsgStyle kit.Style
			if isCurrentlyNewPB {
				pbMsg = "✨ NEW PERSONAL BEST! ✨"
				pulse := (now.Sub(rm.startedAt).Milliseconds() / 150) % 3
				switch pulse {
				case 0:
					pbMsgStyle = kit.Style{FG: kit.RGB(0xff, 0xd7, 0x00), Attr: kit.AttrBold}
				case 1:
					pbMsgStyle = kit.Style{FG: kit.RGB(0x00, 0xff, 0xff), Attr: kit.AttrBold}
				default:
					pbMsgStyle = kit.Style{FG: kit.RGB(0xff, 0x00, 0x7f), Attr: kit.AttrBold}
				}
			} else {
				pbMsg = fmt.Sprintf("Personal Best: %04d", pb)
				pbMsgStyle = kit.Style{FG: theme.Footer}
			}
			f.Text(9, 21, centerText(pbMsg, 38), pbMsgStyle)
		}

		rm.applyShakeAndSend(r, p, f, now)
	}
}

func (rm *room) getBorderStyle(r, c int, now time.Time) kit.Style {
	theme := palettes[rm.themeIndex]

	// Check if flashing (e.g. from screen flash effect)
	if rm.flashOption > 0 && !rm.flashExpiry.IsZero() && now.Before(rm.flashExpiry) {
		flashCycle := (now.UnixNano() / 75000000) % 2 // 75ms toggle
		if flashCycle == 0 {
			if rm.flashOption == 1 {
				// Gentle flash: flash border with slightly brightened theme border
				return kit.Style{FG: brightenColor(theme.Border, 1.4), Attr: kit.AttrBold}
			} else {
				// Strong flash: bright white/red flash colors
				return kit.Style{FG: kit.RGB(0xff, 0xff, 0xff), Attr: kit.AttrBold}
			}
		} else {
			if rm.flashOption == 1 {
				return kit.Style{FG: theme.Border}
			} else {
				return kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
			}
		}
	}

	// Fallback/compatibility check for collision flash
	if rm.flashOption == 0 && !rm.lastCollisionAt.IsZero() && now.Sub(rm.lastCollisionAt) < 300*time.Millisecond {
		return kit.Style{FG: theme.Border}
	}

	if rm.flashOption > 0 && !rm.lastCollisionAt.IsZero() && now.Sub(rm.lastCollisionAt) < 300*time.Millisecond {
		flashCycle := (now.Sub(rm.lastCollisionAt).Milliseconds() / 75) % 2
		if flashCycle == 0 {
			if rm.flashOption == 1 {
				return kit.Style{FG: brightenColor(theme.Border, 1.4), Attr: kit.AttrBold}
			} else {
				return kit.Style{FG: kit.RGB(0xff, 0xff, 0xff), Attr: kit.AttrBold}
			}
		} else {
			if rm.flashOption == 1 {
				return kit.Style{FG: theme.Border}
			} else {
				return kit.Style{FG: kit.RGB(0xff, 0x00, 0x55), Attr: kit.AttrBold}
			}
		}
	}

	baseStyle := kit.Style{FG: theme.Border}
	// Dynamic wave animation along the border
	timeShift := int(now.Sub(rm.startedAt).Milliseconds() / 80)
	pos := r + c - timeShift
	if pos < 0 {
		pos = -pos
	}
	wave := pos % 16
	if wave < 3 {
		return kit.Style{FG: brightenColor(theme.Border, 1.8), Attr: kit.AttrBold}
	}
	return baseStyle
}

func (rm *room) drawBorder(f *kit.Frame, now time.Time) {
	// Top border
	f.SetRune(0, 0, '╔', rm.getBorderStyle(0, 0, now))
	for c := 1; c < 79; c++ {
		f.SetRune(0, c, '═', rm.getBorderStyle(0, c, now))
	}
	f.SetRune(0, 79, '╗', rm.getBorderStyle(0, 79, now))

	// Row 2 divider
	f.SetRune(2, 0, '╠', rm.getBorderStyle(2, 0, now))
	for c := 1; c < 79; c++ {
		f.SetRune(2, c, '═', rm.getBorderStyle(2, c, now))
	}
	f.SetRune(2, 79, '╣', rm.getBorderStyle(2, 79, now))

	// Row 21 divider
	f.SetRune(21, 0, '╠', rm.getBorderStyle(21, 0, now))
	for c := 1; c < 79; c++ {
		f.SetRune(21, c, '═', rm.getBorderStyle(21, c, now))
	}
	f.SetRune(21, 79, '╣', rm.getBorderStyle(21, 79, now))

	// Bottom border
	f.SetRune(23, 0, '╚', rm.getBorderStyle(23, 0, now))
	for c := 1; c < 79; c++ {
		f.SetRune(23, c, '═', rm.getBorderStyle(23, c, now))
	}
	f.SetRune(23, 79, '╝', rm.getBorderStyle(23, 79, now))

	// Vertical borders
	for r := 1; r < 23; r++ {
		if r == 2 || r == 21 {
			continue
		}
		f.SetRune(r, 0, '║', rm.getBorderStyle(r, 0, now))
		f.SetRune(r, 79, '║', rm.getBorderStyle(r, 79, now))
	}
}

func (rm *room) drawDividerWithText(f *kit.Frame, row int, text string, now time.Time) {
	f.SetRune(row, 0, '╠', rm.getBorderStyle(row, 0, now))
	f.SetRune(row, 79, '╣', rm.getBorderStyle(row, 79, now))

	// Index by runes, not bytes: the text always contains '│' (U+2502, 3 bytes)
	// and may carry other multibyte glyphs. Byte-indexing rendered 0xE2 as 'â'.
	rs := []rune(text)
	textLen := len(rs)
	startCol := 40 - textLen/2
	endCol := startCol + textLen

	for c := 1; c < 79; c++ {
		if c >= startCol && c < endCol {
			f.SetRune(row, c, rs[c-startCol], kit.Style{FG: palettes[rm.themeIndex].Header, Attr: kit.AttrBold})
		} else {
			f.SetRune(row, c, '═', rm.getBorderStyle(row, c, now))
		}
	}
}

func brightenColor(c kit.Color, factor float64) kit.Color {
	r, g, b := c.RGBVals()
	newR := float64(r) * factor
	newG := float64(g) * factor
	newB := float64(b) * factor
	if newR > 255 {
		newR = 255
	}
	if newG > 255 {
		newG = 255
	}
	if newB > 255 {
		newB = 255
	}
	return kit.RGB(uint8(newR), uint8(newG), uint8(newB))
}

func interpolateColor(c1, c2 kit.Color, t float64) kit.Color {
	r1, g1, b1 := c1.RGBVals()
	r2, g2, b2 := c2.RGBVals()
	r := uint8(float64(r1) + t*float64(int(r2)-int(r1)))
	g := uint8(float64(g1) + t*float64(int(g2)-int(g1)))
	b := uint8(float64(b1) + t*float64(int(b2)-int(b1)))
	return kit.RGB(r, g, b)
}

func (rm *room) spawnBomb(r kit.Room) {
	rm.bombPos = rm.randomFreePoint(r, 0)
	rm.bombActive = true
	rm.bombExploding = false
	rm.bombSpawnedAt = r.Now()
}

func (rm *room) findShortestDir(now time.Time, start Point, target Point, oppositeDir Point, p1FreezeActive, p2FreezeActive bool) (Point, bool) {
	// BFS pathfinding queue
	type QueueNode struct {
		pos  Point
		path []Point
	}

	queue := []QueueNode{
		{pos: start, path: []Point{}},
	}
	visited := make(map[Point]bool)
	visited[start] = true

	isSafeBFS := func(p Point, isStart bool, prevDir Point) bool {
		if rm.isMazeWall(p) {
			return false
		}
		for _, op := range rm.obstacles {
			if op == p {
				return false
			}
		}
		for _, sp := range rm.snake1 {
			if sp == p {
				return false
			}
		}
		for _, sp := range rm.snake2 {
			if sp == p {
				return false
			}
		}
		for _, hp := range rm.hazards {
			if hp.Pos == p {
				return false
			}
			if !p1FreezeActive && !p2FreezeActive {
				nextHX := hp.Pos.X + hp.Dir.X
				nextHY := hp.Pos.Y + hp.Dir.Y
				if nextHX < hp.MinX || nextHX > hp.MaxX || nextHY < hp.MinY || nextHY > hp.MaxY {
					nextHX = hp.Pos.X - hp.Dir.X
					nextHY = hp.Pos.Y - hp.Dir.Y
				}
				if p.X == nextHX && p.Y == nextHY {
					return false
				}
			}
		}
		if rm.gameMode == ModeBomb {
			if rm.bombActive {
				if p == rm.bombPos {
					return false
				}
				if now.Sub(rm.bombSpawnedAt) >= 3*time.Second {
					dx := p.X - rm.bombPos.X
					dy := p.Y - rm.bombPos.Y
					if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
						return false
					}
				}
			}
			if rm.bombExploding {
				dx := p.X - rm.bombPos.X
				dy := p.Y - rm.bombPos.Y
				if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
					return false
				}
			}
		}
		return true
	}

	dirs := []Point{
		{X: 0, Y: -1}, // Up
		{X: 0, Y: 1},  // Down
		{X: -1, Y: 0}, // Left
		{X: 1, Y: 0},  // Right
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.pos == target {
			if len(curr.path) > 0 {
				return curr.path[0], true
			}
			return Point{X: 0, Y: 0}, false
		}

		for _, d := range dirs {
			if curr.pos == start && d == oppositeDir {
				continue
			}

			var nextP Point
			if rm.gameMode == ModePortal && curr.pos == rm.portalA {
				nextP = Point{
					X: (rm.portalB.X + d.X + 39) % 39,
					Y: (rm.portalB.Y + d.Y + 18) % 18,
				}
			} else if rm.gameMode == ModePortal && curr.pos == rm.portalB {
				nextP = Point{
					X: (rm.portalA.X + d.X + 39) % 39,
					Y: (rm.portalA.Y + d.Y + 18) % 18,
				}
			} else {
				nextP = Point{
					X: (curr.pos.X + d.X + 39) % 39,
					Y: (curr.pos.Y + d.Y + 18) % 18,
				}
			}

			if !visited[nextP] {
				if nextP == target || isSafeBFS(nextP, curr.pos == start, d) {
					visited[nextP] = true
					newPath := make([]Point, len(curr.path)+1)
					copy(newPath, curr.path)
					newPath[len(curr.path)] = d
					queue = append(queue, QueueNode{pos: nextP, path: newPath})
				}
			}
		}
	}

	// Fallback to survival space maximization if no path to target found
	var bestDir Point
	bestSpace := -1
	hasSafeMove := false

	for _, d := range dirs {
		if d == oppositeDir {
			continue
		}
		var nextP Point
		if rm.gameMode == ModePortal && start == rm.portalA {
			nextP = Point{
				X: (rm.portalB.X + d.X + 39) % 39,
				Y: (rm.portalB.Y + d.Y + 18) % 18,
			}
		} else if rm.gameMode == ModePortal && start == rm.portalB {
			nextP = Point{
				X: (rm.portalA.X + d.X + 39) % 39,
				Y: (rm.portalA.Y + d.Y + 18) % 18,
			}
		} else {
			nextP = Point{
				X: (start.X + d.X + 39) % 39,
				Y: (start.Y + d.Y + 18) % 18,
			}
		}

		if isSafeBFS(nextP, true, d) {
			hasSafeMove = true
			space := rm.countReachableSpace(now, nextP, p1FreezeActive, p2FreezeActive)
			if space > bestSpace {
				bestSpace = space
				bestDir = d
			}
		}
	}

	if hasSafeMove {
		return bestDir, true
	}

	return Point{X: 0, Y: 0}, false
}

func (rm *room) countReachableSpace(now time.Time, start Point, p1FreezeActive, p2FreezeActive bool) int {
	visited := make(map[Point]bool)
	visited[start] = true
	queue := []Point{start}
	count := 0
	maxDepth := 50

	isSafeBFS := func(p Point) bool {
		if rm.isMazeWall(p) {
			return false
		}
		for _, op := range rm.obstacles {
			if op == p {
				return false
			}
		}
		for _, sp := range rm.snake1 {
			if sp == p {
				return false
			}
		}
		for _, sp := range rm.snake2 {
			if sp == p {
				return false
			}
		}
		for _, hp := range rm.hazards {
			if hp.Pos == p {
				return false
			}
			if !p1FreezeActive && !p2FreezeActive {
				nextHX := hp.Pos.X + hp.Dir.X
				nextHY := hp.Pos.Y + hp.Dir.Y
				if nextHX < hp.MinX || nextHX > hp.MaxX || nextHY < hp.MinY || nextHY > hp.MaxY {
					nextHX = hp.Pos.X - hp.Dir.X
					nextHY = hp.Pos.Y - hp.Dir.Y
				}
				if p.X == nextHX && p.Y == nextHY {
					return false
				}
			}
		}
		if rm.gameMode == ModeBomb {
			if rm.bombActive {
				if p == rm.bombPos {
					return false
				}
				if now.Sub(rm.bombSpawnedAt) >= 3*time.Second {
					dx := p.X - rm.bombPos.X
					dy := p.Y - rm.bombPos.Y
					if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
						return false
					}
				}
			}
			if rm.bombExploding {
				dx := p.X - rm.bombPos.X
				dy := p.Y - rm.bombPos.Y
				if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
					return false
				}
			}
		}
		return true
	}

	dirs := []Point{
		{X: 0, Y: -1},
		{X: 0, Y: 1},
		{X: -1, Y: 0},
		{X: 1, Y: 0},
	}

	for len(queue) > 0 && count < maxDepth {
		curr := queue[0]
		queue = queue[1:]
		count++

		for _, d := range dirs {
			var nextP Point
			if rm.gameMode == ModePortal && curr == rm.portalA {
				nextP = Point{
					X: (rm.portalB.X + d.X + 39) % 39,
					Y: (rm.portalB.Y + d.Y + 18) % 18,
				}
			} else if rm.gameMode == ModePortal && curr == rm.portalB {
				nextP = Point{
					X: (rm.portalA.X + d.X + 39) % 39,
					Y: (rm.portalA.Y + d.Y + 18) % 18,
				}
			} else {
				nextP = Point{
					X: (curr.X + d.X + 39) % 39,
					Y: (curr.Y + d.Y + 18) % 18,
				}
			}
			if !visited[nextP] && isSafeBFS(nextP) {
				visited[nextP] = true
				queue = append(queue, nextP)
			}
		}
	}
	return count
}

func (rm *room) handleSettingsInput(r kit.Room, p kit.Player, in kit.Input) {
	action := kit.Resolve(in, kit.CtxNav)
	if action == kit.ActConfirm {
		rm.settingsOpen = false
		return
	}

	speeds := []int{100, 120, 150, 180, 200}
	skins := []rune{'█', '◆', '●', '■', '★'}

	if in.Kind == kit.InputRune {
		switch in.Rune {
		case 'w', 'W':
			rm.settingsCursor = (rm.settingsCursor - 1 + settingsCount) % settingsCount
		case 's', 'S':
			rm.settingsCursor = (rm.settingsCursor + 1) % settingsCount
		case 'a', 'A':
			rm.changeSetting(skins, speeds, -1)
		case 'd', 'D':
			rm.changeSetting(skins, speeds, 1)
		}
	} else if in.Kind == kit.InputKey {
		switch in.Key {
		case kit.KeyUp:
			rm.settingsCursor = (rm.settingsCursor - 1 + settingsCount) % settingsCount
		case kit.KeyDown:
			rm.settingsCursor = (rm.settingsCursor + 1) % settingsCount
		case kit.KeyLeft:
			rm.changeSetting(skins, speeds, -1)
		case kit.KeyRight:
			rm.changeSetting(skins, speeds, 1)
		}
	}
}

func (rm *room) changeSetting(skins []rune, speeds []int, dir int) {
	switch rm.settingsCursor {
	case 0: // Snake 1 Skin
		rm.snake1SkinIdx = (rm.snake1SkinIdx + dir + len(skins)) % len(skins)
	case 1: // Snake 2 Skin
		rm.snake2SkinIdx = (rm.snake2SkinIdx + dir + len(skins)) % len(skins)
	case 2: // Grid Dots
		rm.gridDotsEnabled = !rm.gridDotsEnabled
	case 3: // Start Speed
		rm.startSpeedIdx = (rm.startSpeedIdx + dir + len(speeds)) % len(speeds)
		// Apply the new start speed to the live tick rate immediately. tick()
		// recomputes from the current total score, so this stays correct even
		// when changed mid-round (paused) rather than only at score 0.
		rm.tickRate = rm.speedForScore(speeds)
	case 4: // Screen Shake
		rm.shakeOption = (rm.shakeOption + dir + 3) % 3
	case 5: // Screen Flash
		rm.flashOption = (rm.flashOption + dir + 3) % 3
	case 6: // Close / Back
		// Left/Right on Close does nothing
	}
}

func (rm *room) drawSettingsModal(f *kit.Frame, now time.Time) {
	theme := palettes[rm.themeIndex]
	modalStyle := kit.Style{FG: theme.ModalBorder, Attr: kit.AttrBold}
	textStyle := kit.Style{FG: kit.RGB(0xff, 0xff, 0xff)}
	labelStyle := kit.Style{FG: theme.Header, Attr: kit.AttrBold}
	selectedStyle := kit.Style{FG: theme.Key, Attr: kit.AttrBold}
	dimStyle := kit.Style{FG: kit.RGB(0x77, 0x77, 0x77)}

	// Draw box (bounds sized for the 7 settings options)
	f.Text(4, 18, "╔══════════════════════════════════════════╗", modalStyle)
	for r := 5; r <= 16; r++ {
		f.Text(r, 18, "║                                          ║", modalStyle)
	}
	f.Text(17, 18, "╚══════════════════════════════════════════╝", modalStyle)

	// Title
	f.Text(5, 32, "── SETTINGS ──", labelStyle)

	skins := []rune{'█', '◆', '●', '■', '★'}
	speeds := []int{100, 120, 150, 180, 200}

	renderOption := func(row int, label string, value string, isSelected bool) {
		r := 8 + row
		for c := 20; c <= 59; c++ {
			f.SetRune(r, c, ' ', textStyle)
		}

		var prefix string
		if isSelected {
			prefix = "▶ "
		} else {
			prefix = "  "
		}

		var lStyle kit.Style
		if isSelected {
			lStyle = selectedStyle
		} else {
			lStyle = textStyle
		}

		f.Text(r, 20, prefix+label, lStyle)

		valStr := fmt.Sprintf("[ %s ]", value)
		valCol := 60 - len(valStr)

		var vStyle kit.Style
		if isSelected {
			vStyle = selectedStyle
		} else {
			vStyle = dimStyle
		}
		f.Text(r, valCol, valStr, vStyle)
	}

	// Option 0: Snake 1 Skin
	skin1Val := string(skins[rm.snake1SkinIdx])
	renderOption(0, "Snake 1 Skin", skin1Val, rm.settingsCursor == 0)

	// Option 1: Snake 2 Skin
	skin2Val := string(skins[rm.snake2SkinIdx])
	renderOption(1, "Snake 2 Skin", skin2Val, rm.settingsCursor == 1)

	// Option 2: Grid Dots
	dotsVal := "ON"
	if !rm.gridDotsEnabled {
		dotsVal = "OFF"
	}
	renderOption(2, "Grid Dots", dotsVal, rm.settingsCursor == 2)

	// Option 3: Start Speed
	speedVal := fmt.Sprintf("%dms", speeds[rm.startSpeedIdx])
	renderOption(3, "Start Speed", speedVal, rm.settingsCursor == 3)

	// Option 4: Screen Shake
	shakeVal := "STRONG"
	if rm.shakeOption == 0 {
		shakeVal = "OFF"
	} else if rm.shakeOption == 1 {
		shakeVal = "GENTLE"
	}
	renderOption(4, "Screen Shake", shakeVal, rm.settingsCursor == 4)

	// Option 5: Screen Flash
	flashVal := "STRONG"
	if rm.flashOption == 0 {
		flashVal = "OFF"
	} else if rm.flashOption == 1 {
		flashVal = "GENTLE"
	}
	renderOption(5, "Screen Flash", flashVal, rm.settingsCursor == 5)

	// Option 6: Close & Apply
	r := 8 + 6
	for c := 20; c <= 59; c++ {
		f.SetRune(r, c, ' ', textStyle)
	}
	closeText := "Close & Apply"
	if rm.settingsCursor == 6 {
		closeText = "▶ Close & Apply ◀"
		f.Text(r, 40-len([]rune(closeText))/2, closeText, selectedStyle)
	} else {
		f.Text(r, 40-len(closeText)/2, closeText, textStyle)
	}

	// Instructions
	instText := "Press [SPACE] to Close"
	f.Text(16, 40-len(instText)/2, instText, dimStyle)
}

func (rm *room) spawnParticles(r kit.Room, x, y int, color kit.Color, count int, pType string) {
	now := r.Now()
	var glyphs []rune
	if pType == "food" {
		glyphs = []rune{'·', '+', '*', 'o'}
	} else if pType == "crash" {
		glyphs = []rune{'░', '▒', '*', '·', 'x', 'o', '+', '✦'}
	} else if pType == "bomb" {
		glyphs = []rune{'✹', '░', '▒', '*', '+', 'x'}
	} else {
		glyphs = []rune{'*'}
	}

	for i := 0; i < count; i++ {
		angle := 2.0 * math.Pi * float64(i) / float64(count)
		speed := 5.0 + r.Rand().Float64()*10.0
		vx := math.Cos(angle) * speed
		vy := math.Sin(angle) * speed * 0.5

		glyph := glyphs[r.Rand().Intn(len(glyphs))]
		duration := time.Duration(300+r.Rand().Intn(400)) * time.Millisecond

		rm.particles = append(rm.particles, Particle{
			X:         float64(x),
			Y:         float64(y),
			VX:        vx,
			VY:        vy,
			Glyph:     glyph,
			Color:     color,
			CreatedAt: now,
			Duration:  duration,
		})
	}
}

func (rm *room) triggerShake(r kit.Room, duration time.Duration) {
	if rm.shakeOption > 0 {
		rm.shakeStartedAt = r.Now()
		rm.shakeExpiry = r.Now().Add(duration)
	}
}

func (rm *room) triggerFlash(r kit.Room, duration time.Duration) {
	if rm.flashOption > 0 {
		rm.flashExpiry = r.Now().Add(duration)
	}
}

// fillCells sets every cell of s to b. A slice loop (not a fixed-bound,
// constant-index array loop) is the point: the optimizer does not scalarize it.
func fillCells(s []kit.Cell, b kit.Cell) {
	for i := range s {
		s[i] = b
	}
}

func (rm *room) applyShakeAndSend(r kit.Room, p kit.Player, f *kit.Frame, now time.Time) {
	// Skip shake while the game-over modal is up (shaking it just jitters the
	// modal and previously clipped the outer border).
	if rm.shakeOption > 0 && !rm.gameOver && !rm.shakeExpiry.IsZero() && now.Before(rm.shakeExpiry) {
		elapsed := now.Sub(rm.shakeStartedAt)
		phase := elapsed.Milliseconds() / 40

		var offsetsX, offsetsY []int
		if rm.shakeOption == 1 {
			// Gentle shake: offset stays within 1 cell
			offsetsX = []int{1, -1, 0, 0, 1, -1, 0, 0}
			offsetsY = []int{0, 0, 1, -1, 0, 0, 1, -1}
		} else {
			// Strong shake: offset up to 2 cells
			offsetsX = []int{2, -2, 0, 1, -1, 2, -2, 1}
			offsetsY = []int{0, 1, -2, 2, 0, -1, 1, -2}
		}

		idx := int(phase) % len(offsetsX)
		dx := offsetsX[idx]
		dy := offsetsY[idx]

		// Render the shaken view into a SEPARATE frame and send that, leaving
		// f.Cells untouched. Move cells with copy() on row slices — never assign
		// or cell-index the whole [kit.Rows][kit.Cols]kit.Cell grid by value in a
		// fixed-bound loop: the optimizer scalarizes that into ~20k locals and
		// wasm-opt's coalesce-locals pass then balloons past 20 GB and OOMs the CI
		// runner. Only the playfield (rows 3..20, cols 1..78) shakes, so the outer
		// border box stays rock-steady and never clips off-screen.
		if rm.shakeFrame == nil {
			rm.shakeFrame = kit.NewFrame()
		}
		sf := rm.shakeFrame
		for row := 0; row < kit.Rows; row++ {
			copy(sf.Cells[row][:], f.Cells[row][:])
		}
		blank := kit.Cell{Rune: ' '}
		for row := 3; row <= 20; row++ {
			dst := sf.Cells[row][1:79] // play columns 1..78
			sr := row - dy
			if sr < 3 || sr > 20 {
				fillCells(dst, blank)
				continue
			}
			src := f.Cells[sr][1:79]
			n := len(dst)
			switch {
			case dx == 0:
				copy(dst, src)
			case dx > 0:
				if dx < n {
					copy(dst[dx:], src[:n-dx])
				}
				fillCells(dst[:min(dx, n)], blank)
			default: // dx < 0 — shift left
				k := -dx
				if k < n {
					copy(dst[:n-k], src[k:])
				}
				fillCells(dst[max(0, n-k):], blank)
			}
		}
		r.Send(p, sf)
	} else {
		r.Send(p, f)
	}
}
