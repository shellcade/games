# Pokies Lounge — Resident Floor (PR2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn pokies into a shared, walkable resident lounge: roam a scrolling floor as your arcade character, see others, and sit at a themed machine to play (reusing the existing 3-reel engine).

**Architecture:** A per-player **mode** (`roaming`/`seated`) gates input and render. Roaming moves a pawn one tile per arrow press on a static tile map with a camera that follows and clamps; sitting on a machine's approach tile binds that machine's variant and switches to the seated cabinet (the existing `machine`/spin/gamble/free-spin logic, unchanged). Players render as their `kit.Character` tiles on the floor and at machines. The room is `LifecycleResident`, `MaxPlayers 32`.

**Tech Stack:** Go 1.25, shellcade kit v2.9.0, `kittest`. TinyGo wasip1 for publish.

**Working dir:** `games/bcook/pokies` in worktree `.claude/worktrees/pokies-aussie-features` (branch `bcook/pokies-lounge`). Run `go` commands from `games/bcook/pokies`.

**Conventions:** TDD — failing test, minimal code, green, commit. Zero per-render allocations (see `alloc_test.go`). gofmt after edits.

> **Scope note:** This is PR2 (the floor). The 6 machines are placed and named here but all bind to the existing default 3-reel variant; distinct per-theme **odds** and the **5-reel 243-ways** engine are PR3 (a separate plan). The seated view reuses today's `drawCard` cabinet, centered.

---

## File map

- `floor.go` *(new)* — `tile`, `floorMap`, `buildLounge` (programmatic map + machine placements), `walkable`, camera math.
- `pawn.go` *(new)* — `pawn` (position + mode), movement + collision, sit/stand transitions, occupancy.
- `floorlayout.go` *(new)* — floor camera render (tiles, machine icons + labels, avatars as character tiles + names).
- `room.go` — Meta (resident, scale, ctx flags), room fields (`fmap`, `fmachines`, `pawns`, `occupied`, `themes`), `OnJoin`/`OnLeave` (spawn/despawn), `OnInput`/`OnWake`/`compose` dispatch by mode. The machine/spin/gamble/free-spin logic is reused; `startSpin` reads the seated machine's bound variant.
- `layout.go` — `compose` dispatches roaming→floor / seated→cabinet; the seated cabinet draw is reused.
- `themes.go` *(new, minimal here)* — the 6 machine names + their variant binding (all → default variant in PR2; real PAR sheets in PR3).
- tests: `floor_test.go`, `pawn_test.go`, plus updates to `pokies_test.go` (meta, multi-cabinet → floor).

---

## Phase A — Meta: resident lifecycle + scale

### Task A1: Declare resident lifecycle, ctx flags, MaxPlayers 32

**Files:** Modify `room.go` (`Meta`); Modify `pokies_test.go` (`TestMetaIsBareName`).

- [ ] **Step 1: Update the meta test** in `pokies_test.go`:

```go
func TestMetaIsResidentLounge(t *testing.T) {
	m := Game{}.Meta()
	if m.Slug != "pokies" {
		t.Errorf("slug = %q, want pokies", m.Slug)
	}
	if m.MinPlayers != 1 || m.MaxPlayers != 32 {
		t.Errorf("players = %d..%d, want 1..32", m.MinPlayers, m.MaxPlayers)
	}
	if m.Lifecycle != kit.LifecycleResident {
		t.Errorf("lifecycle = %v, want resident", m.Lifecycle)
	}
	if m.CtxFeatures&kit.CtxFeatCharacter == 0 || m.CtxFeatures&kit.CtxFeatRosterEpoch == 0 {
		t.Errorf("ctx features = %d, want character + roster-epoch", m.CtxFeatures)
	}
}
```

Replace the old `TestMetaIsBareName` body's player assertion (it expected 1..5) — rename to the above, keeping the leaderboard check from the original in a separate retained assertion if desired.

- [ ] **Step 2: Run** `go test ./... -run TestMetaIsResidentLounge` → FAIL (still 5 / resumable).

- [ ] **Step 3: Update `Meta()`** in `room.go`:

```go
		MinPlayers:       1,
		MaxPlayers:       32,
		Tags:             []string{"slots", "casual", "social"},

		Lifecycle:   kit.LifecycleResident,
		CtxFeatures: kit.CtxFeatCharacter | kit.CtxFeatRosterEpoch,
```

(Remove the old `Lifecycle: kit.LifecycleEphemeral` and the lone `CtxFeatures: kit.CtxFeatCharacter`.)

- [ ] **Step 4: Run** the test → PASS. Then `go test ./...` — other tests may fail where they assume 5 cabinets; those are reworked in Phase H. Note failures, proceed.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): declare resident lounge meta (32 players, roster-epoch)"`

---

## Phase B — Floor map

### Task B1: `tile`, `floorMap`, `walkable`

**Files:** Create `floor.go`; Create `floor_test.go`.

- [ ] **Step 1: Failing test** (`floor_test.go`):

```go
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
```

- [ ] **Step 2: Run** → FAIL (undefined `floorMap`).

- [ ] **Step 3: Implement** `floor.go`:

```go
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
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): floor tile map + walkable"`

### Task B2: `buildLounge` — programmatic map + machine placements

**Files:** Modify `floor.go` (`floorMachine`, `buildLounge`); Modify `floor_test.go`.

- [ ] **Step 1: Failing test**:

```go
func TestBuildLoungeShape(t *testing.T) {
	fm, machines := buildLounge()
	if fm.w != loungeW || fm.h != loungeH {
		t.Fatalf("size = %dx%d, want %dx%d", fm.w, fm.h, loungeW, loungeH)
	}
	if fm.w <= kit.Cols { // must exceed the viewport to force horizontal scroll
		t.Fatalf("lounge width %d must exceed viewport %d", fm.w, kit.Cols)
	}
	// Border is wall all around.
	for x := 0; x < fm.w; x++ {
		if fm.at(x, 0) != tileWall || fm.at(x, fm.h-1) != tileWall {
			t.Fatalf("top/bottom border not wall at x=%d", x)
		}
	}
	if len(machines) != 6 {
		t.Fatalf("machines = %d, want 6", len(machines))
	}
	// Each machine's approach tile is walkable and the icon tile is not.
	for _, mc := range machines {
		if fm.walkable(mc.mx, mc.my) {
			t.Errorf("machine %q icon tile should block movement", mc.name)
		}
		if !fm.walkable(mc.ax, mc.ay) {
			t.Errorf("machine %q approach tile must be walkable", mc.name)
		}
	}
	// Spawn is walkable.
	sx, sy := loungeSpawn()
	if !fm.walkable(sx, sy) {
		t.Error("spawn must be walkable")
	}
}
```

(Add `import kit "github.com/shellcade/kit/v2"` to `floor_test.go`.)

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** in `floor.go`:

```go
import kit "github.com/shellcade/kit/v2"

// Lounge dimensions — wider and taller than the 80x24 viewport so the camera
// scrolls in both axes.
const (
	loungeW = 96
	loungeH = 36
)

// floorMachine is a placed cabinet on the floor: an icon tile you cannot walk
// onto, and the approach tile you sit from.
type floorMachine struct {
	id         int
	name       string
	mx, my     int // icon tile (blocks movement)
	ax, ay     int // approach tile (walk here, press Confirm to sit)
}

// machineIcon marks a machine's icon tile as a wall so pawns can't walk through it.
// (Stored in the tile grid as wall; the machine list carries the identity.)

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
	// Entrance gap in the bottom wall, centred.
	fm.tiles[(loungeH-1)*loungeW+loungeW/2] = tileEntrance

	names := []string{"LUCKY 7s", "GEM RUSH", "BELLS", "CHERRY POP", "CROWN", "GIFT DROP"}
	machines := make([]floorMachine, len(names))
	// Spread the six icons along the top interior row (y=2), approach at y=3.
	for i, name := range names {
		mx := (i+1)*loungeW/(len(names)+1)
		machines[i] = floorMachine{id: i, name: name, mx: mx, my: 2, ax: mx, ay: 3}
		fm.tiles[2*loungeW+mx] = tileWall // icon tile blocks movement
	}
	return fm, machines
}
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): buildLounge — bordered map with 6 machines + entrance"`

### Task B3: Camera math

**Files:** Modify `floor.go` (`cameraOrigin`); Modify `floor_test.go`.

- [ ] **Step 1: Failing test** — camera follows and clamps:

```go
func TestCameraClamps(t *testing.T) {
	// viewport vw x vh over a map of loungeW x loungeH.
	cases := []struct{ px, py, wantX, wantY int }{
		{0, 0, 0, 0},                                    // top-left: clamp to 0
		{loungeW - 1, loungeH - 1, loungeW - vpW, loungeH - vpH}, // bottom-right: clamp to max
		{loungeW / 2, loungeH / 2, loungeW/2 - vpW/2, loungeH/2 - vpH/2}, // centred
	}
	for _, c := range cases {
		ox, oy := cameraOrigin(c.px, c.py)
		if ox != c.wantX || oy != c.wantY {
			t.Errorf("cameraOrigin(%d,%d) = (%d,%d), want (%d,%d)", c.px, c.py, ox, oy, c.wantX, c.wantY)
		}
	}
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** in `floor.go`:

```go
// Viewport: the floor draws into rows [vpTop, vpTop+vpH) over all vpW columns.
// Row 0 is the title, the last row is controls/status.
const (
	vpW   = kit.Cols      // 80
	vpTop = 1
	vpH   = kit.Rows - 2  // 22 (rows 1..22)
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
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): floor camera origin (follow + clamp)"`

---

## Phase C — Pawns: movement, collision, seating

### Task C1: `pawn` + room wiring + spawn on join

**Files:** Create `pawn.go`; Modify `room.go` (room fields, `newRoom`, `OnStart`, `OnJoin`, `OnLeave`).

- [ ] **Step 1: Failing test** (`pawn_test.go`):

```go
package main

import (
	"testing"

	"github.com/shellcade/kit/v2/kittest"
)

func TestJoinSpawnsPawnAtEntrance(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	pw := rm.pawns[p.AccountID]
	if pw == nil {
		t.Fatal("join should create a pawn")
	}
	sx, sy := loungeSpawn()
	if pw.x != sx || pw.y != sy {
		t.Errorf("pawn at (%d,%d), want spawn (%d,%d)", pw.x, pw.y, sx, sy)
	}
	if pw.seated {
		t.Error("a fresh pawn should be roaming, not seated")
	}
}
```

- [ ] **Step 2: Run** → FAIL (no `pawns`).

- [ ] **Step 3: Implement** `pawn.go`:

```go
package main

// pawn is a player's presence on the lounge floor.
type pawn struct {
	x, y   int
	seated bool
	seat   int // machine id when seated, -1 when roaming
}
```

In `room.go` add fields to `room`:

```go
	fmap      *floorMap
	fmachines []floorMachine
	pawns     map[string]*pawn   // account id -> floor presence
	occupied  map[int]string     // machine id -> account id (exclusive seat)
	themes    []*variant         // machine id -> bound variant (PR2: all default)
```

In `newRoom`, build the floor and themes:

```go
	fmap, fmachines := buildLounge()
	themes := make([]*variant, len(fmachines))
	for i := range themes {
		themes[i] = defaultVariant() // PR2: every machine uses the default odds
	}
	return &room{
		cfg:       cfg,
		svc:       svc,
		machines:  map[string]*machine{},
		names:     map[string]kit.Player{},
		variant:   defaultVariant(),
		fmap:      fmap,
		fmachines: fmachines,
		pawns:     map[string]*pawn{},
		occupied:  map[int]string{},
		themes:    themes,
	}
```

In `OnJoin`, after seeding the machine, spawn a pawn:

```go
	sx, sy := loungeSpawn()
	rm.pawns[p.AccountID] = &pawn{x: sx, y: sy, seat: -1}
```

In `OnLeave`, release any seat and remove the pawn:

```go
	if pw := rm.pawns[p.AccountID]; pw != nil && pw.seated {
		delete(rm.occupied, pw.seat)
	}
	delete(rm.pawns, p.AccountID)
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): spawn a floor pawn per player on join"`

### Task C2: Movement + collision

**Files:** Modify `pawn.go` (`tryMove`, `pawnAt`); Modify `pawn_test.go`.

- [ ] **Step 1: Failing test**:

```go
func TestPawnMovementAndCollision(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	pa := rm.pawns[a.AccountID]
	pa.x, pa.y = 5, 5 // open floor

	rm.tryMove(a.AccountID, 1, 0) // east into floor
	if pa.x != 6 || pa.y != 5 {
		t.Fatalf("after east move pawn at (%d,%d), want (6,5)", pa.x, pa.y)
	}
	// Wall to the north of the top row: move into a wall is a no-op.
	pa.x, pa.y = 1, 1
	rm.tryMove(a.AccountID, 0, -1) // into top wall (y=0)
	if pa.x != 1 || pa.y != 1 {
		t.Fatalf("wall move should be a no-op, pawn at (%d,%d)", pa.x, pa.y)
	}
	// Another pawn blocks the tile.
	pb := rm.pawns[b.AccountID]
	pa.x, pa.y = 5, 5
	pb.x, pb.y = 6, 5
	rm.tryMove(a.AccountID, 1, 0)
	if pa.x != 5 {
		t.Fatalf("should not move onto another pawn, pawn at (%d,%d)", pa.x, pa.y)
	}
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** in `pawn.go`:

```go
// pawnAt reports whether any roaming pawn (other than `self`) occupies (x,y).
func (rm *room) pawnAt(x, y int, self string) bool {
	for id, pw := range rm.pawns {
		if id == self || pw.seated {
			continue
		}
		if pw.x == x && pw.y == y {
			return true
		}
	}
	return false
}

// tryMove steps the pawn by (dx,dy) when the target tile is walkable and
// unoccupied; blocked moves are no-ops. Seated pawns do not move.
func (rm *room) tryMove(id string, dx, dy int) {
	pw := rm.pawns[id]
	if pw == nil || pw.seated {
		return
	}
	nx, ny := pw.x+dx, pw.y+dy
	if !rm.fmap.walkable(nx, ny) || rm.pawnAt(nx, ny, id) {
		return
	}
	pw.x, pw.y = nx, ny
}
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): pawn movement with wall + pawn collision"`

### Task C3: Sit / stand / occupancy

**Files:** Modify `pawn.go` (`machineAtApproach`, `trySit`, `standUp`); Modify `pawn_test.go`.

- [ ] **Step 1: Failing test**:

```go
func TestSitAndStand(t *testing.T) {
	a, b := kittest.Player("a"), kittest.Player("b")
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	mc := rm.fmachines[0]

	// Put A on the machine's approach tile and sit.
	pa := rm.pawns[a.AccountID]
	pa.x, pa.y = mc.ax, mc.ay
	rm.trySit(a.AccountID)
	if !pa.seated || pa.seat != mc.id {
		t.Fatalf("A should be seated at machine %d, got seated=%v seat=%d", mc.id, pa.seated, pa.seat)
	}
	if rm.occupied[mc.id] != a.AccountID {
		t.Fatalf("machine %d should be occupied by A", mc.id)
	}
	// B tries the same machine — occupied, refused.
	pb := rm.pawns[b.AccountID]
	pb.x, pb.y = mc.ax, mc.ay
	rm.trySit(b.AccountID)
	if pb.seated {
		t.Fatal("B must not sit at an occupied machine")
	}
	// A stands; the seat frees and A returns to the approach tile.
	rm.standUp(a.AccountID)
	if pa.seated || rm.occupied[mc.id] != "" && rm.occupied[mc.id] == a.AccountID {
		t.Fatal("standing should free the seat")
	}
	if pa.x != mc.ax || pa.y != mc.ay {
		t.Fatalf("after standing pawn at (%d,%d), want approach (%d,%d)", pa.x, pa.y, mc.ax, mc.ay)
	}
}
```

- [ ] **Step 2: Run** → FAIL.

- [ ] **Step 3: Implement** in `pawn.go`:

```go
// machineAtApproach returns the machine whose approach tile is (x,y), or nil.
func (rm *room) machineAtApproach(x, y int) *floorMachine {
	for i := range rm.fmachines {
		if rm.fmachines[i].ax == x && rm.fmachines[i].ay == y {
			return &rm.fmachines[i]
		}
	}
	return nil
}

// trySit seats the pawn at the machine on its current approach tile, if any and
// unoccupied, binding that machine's variant to the player's session.
func (rm *room) trySit(id string) {
	pw := rm.pawns[id]
	if pw == nil || pw.seated {
		return
	}
	mc := rm.machineAtApproach(pw.x, pw.y)
	if mc == nil {
		return
	}
	if _, taken := rm.occupied[mc.id]; taken {
		return
	}
	pw.seated, pw.seat = true, mc.id
	rm.occupied[mc.id] = id
	if m := rm.machines[id]; m != nil {
		m.seatVar = rm.themes[mc.id] // the bound variant for this seat
	}
}

// standUp releases the seat and returns the pawn to the machine's approach tile.
func (rm *room) standUp(id string) {
	pw := rm.pawns[id]
	if pw == nil || !pw.seated {
		return
	}
	if mc := rm.machineByID(pw.seat); mc != nil {
		pw.x, pw.y = mc.ax, mc.ay
	}
	delete(rm.occupied, pw.seat)
	pw.seated, pw.seat = false, -1
}

func (rm *room) machineByID(id int) *floorMachine {
	for i := range rm.fmachines {
		if rm.fmachines[i].id == id {
			return &rm.fmachines[i]
		}
	}
	return nil
}
```

Add `seatVar *variant` to the `machine` struct in `room.go`. In `startSpin`, prefer the seated variant:

```go
	v := m.seatVar
	if v == nil {
		v = rm.variant
	}
```

(Replace the existing `v := rm.variant` in `startSpin`.)

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): sit/stand with exclusive machine occupancy + variant binding"`

---

## Phase D — Mode dispatch (input)

### Task D1: Route input by mode

**Files:** Modify `room.go` (`OnInput`); Modify `pawn_test.go`.

- [ ] **Step 1: Failing test** — roaming arrows move; seated arrows adjust bet:

```go
func TestInputRoutesByMode(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	pw := rm.pawns[p.AccountID]
	pw.x, pw.y = 5, 5

	rm.OnInput(r, p, keyRight()) // roaming: move east
	if pw.x != 6 {
		t.Fatalf("roaming right should move east, x=%d", pw.x)
	}
	// Seat the player, then Up should adjust bet, not move.
	m := rm.machines[p.AccountID]
	mc := rm.fmachines[0]
	pw.x, pw.y = mc.ax, mc.ay
	rm.trySit(p.AccountID)
	m.bet = betTiers[0]
	rm.OnInput(r, p, keyUp())
	if m.bet != betTiers[1] {
		t.Fatalf("seated up should raise bet to %d, got %d", betTiers[1], m.bet)
	}
}
```

(Add `keyRight()`/`keyLeft()` helpers to `pokies_test.go` mirroring `keyUp`: `kit.Input{Kind: kit.InputKey, Key: kit.KeyRight}`.)

- [ ] **Step 2: Run** → FAIL (`keyRight` undefined / roaming not handled).

- [ ] **Step 3: Implement** `OnInput` in `room.go`:

```go
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	id := p.AccountID
	pw := rm.pawns[id]
	if pw == nil {
		return
	}
	act := kit.Resolve(in, kit.CtxNav)
	if !pw.seated {
		// Roaming: arrows move, Confirm sits.
		switch act {
		case kit.ActUp:
			rm.tryMove(id, 0, -1)
		case kit.ActDown:
			rm.tryMove(id, 0, +1)
		case kit.ActLeft:
			rm.tryMove(id, -1, 0)
		case kit.ActRight:
			rm.tryMove(id, +1, 0)
		case kit.ActConfirm:
			rm.trySit(id)
		}
		rm.render(r)
		return
	}
	// Seated: existing machine controls; Back stands up when idle.
	m := rm.machines[id]
	if m == nil {
		rm.render(r)
		return
	}
	switch {
	case act == kit.ActBack && m.spin == nil && m.gamble == nil && m.freeSpins == 0:
		rm.standUp(id)
	case m.gamble != nil:
		rm.gambleInput(r, id, act)
	case m.freeSpins > 0:
		// auto-play; ignore
	default:
		switch act {
		case kit.ActUp:
			rm.adjustBet(m, +1)
		case kit.ActDown:
			rm.adjustBet(m, -1)
		case kit.ActConfirm:
			rm.startSpin(r, p)
		}
	}
	rm.render(r)
}
```

> The current `OnInput` (from PR #60) resolves act once and branches on gamble/free/default. This replaces it, adding the roaming branch and seated `Back`→stand. `kit.Resolve` maps Esc→`ActBack`.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): dispatch input by mode (roam vs seated)"`

### Task D2: Guard OnWake to seated machines only

**Files:** Modify `room.go` (`OnWake`); Test: `pawn_test.go`.

- [ ] **Step 1: Failing test** — a roaming player's machine never auto-spins / lands:

```go
func TestWakeIgnoresRoamingMachines(t *testing.T) {
	p := kittest.Player("alice")
	rm, r := newGame(t, p)
	rm.OnJoin(r, p)
	m := rm.machines[p.AccountID]
	// Force a stray spin while roaming; OnWake must not settle it into a feature.
	m.freeSpins = 3
	m.freeBet, m.freeVar = 10, rm.variant
	for i := 0; i < 10; i++ {
		r.Advance(300 * time.Millisecond)
		rm.OnWake(r)
	}
	if m.freeSpins != 3 {
		t.Fatalf("roaming machine should not auto-play free spins, freeSpins=%d", m.freeSpins)
	}
}
```

- [ ] **Step 2: Run** → FAIL (auto-play runs regardless of seating).

- [ ] **Step 3: Implement** — in `OnWake`, skip machines whose player is not seated. At the top of the per-machine loop body:

```go
		if pw := rm.pawns[id]; pw == nil || !pw.seated {
			continue // only seated machines animate / auto-play
		}
```

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit** `git commit -am "feat(pokies): only seated machines tick in OnWake"`

---

## Phase E — Rendering

### Task E1: Floor camera render with character-tile avatars

**Files:** Create `floorlayout.go`; Modify `pokies_test.go` (helpers if needed).

- [ ] **Step 1: Failing test** — a roaming viewer sees their own character tile and another player's:

```go
func TestFloorRendersCharacterAvatars(t *testing.T) {
	a := kittest.PlayerWithCharacter("anna", '@') // see helper note below
	b := kittest.PlayerWithCharacter("bert", '&')
	rm, r := newGame(t, a, b)
	rm.OnJoin(r, a)
	rm.OnJoin(r, b)
	// Place both near each other so both are in A's camera window.
	pa, pb := rm.pawns[a.AccountID], rm.pawns[b.AccountID]
	pa.x, pa.y = 20, 18
	pb.x, pb.y = 22, 18
	rm.render(r)
	if !frameContains(r, a, "@") {
		t.Error("viewer should see their own character glyph on the floor")
	}
	if !frameContains(r, a, "&") {
		t.Error("viewer should see another player's character glyph")
	}
	if !frameContains(r, a, "bert") {
		t.Error("other players should be name-labelled")
	}
}
```

> **Helper note:** `kittest` may not expose `PlayerWithCharacter`. In Step 1 first check the `kittest` API (`grep -r "func Player" $(go env GOMODCACHE)/github.com/shellcade/kit/v2@v2.9.0/kittest`). If there is no character-setting constructor, set it directly: build with `kittest.Player("anna")` then assign `p.Character = kit.Character{Glyph: "@"}` before `OnJoin` (a Character has a `Glyph string`; the cell uses its first rune). Adjust the test to whatever the kittest API supports — the assertion (own + other glyph + name visible) is the contract.

- [ ] **Step 2: Run** → FAIL (no floor render).

- [ ] **Step 3: Implement** `floorlayout.go`:

```go
package main

import kit "github.com/shellcade/kit/v2"

var (
	stWall  = kit.Style{FG: kit.DimGray}
	stFloorDecor = kit.Style{FG: kit.DimGray}
	stMachine    = kit.Style{FG: kit.Cyan, Attr: kit.AttrBold}
	stSelf       = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
	stOther      = kit.Style{FG: kit.White}
)

// drawFloor renders the camera window centred on the viewer's pawn: tiles,
// machine icons + labels, and every visible player as their character tile with
// a name label. Avatars are ALWAYS the player's kit.Character (never a generic
// glyph).
func (rm *room) drawFloor(f *kit.Frame, viewer kit.Player) {
	pw := rm.pawns[viewer.AccountID]
	if pw == nil {
		return
	}
	ox, oy := cameraOrigin(pw.x, pw.y)

	// Tiles.
	for sy := 0; sy < vpH; sy++ {
		for sx := 0; sx < vpW; sx++ {
			mx, my := ox+sx, oy+sy
			switch rm.fmap.at(mx, my) {
			case tileWall:
				f.SetRune(vpTop+sy, sx, '#', stWall)
			case tileEntrance:
				f.SetRune(vpTop+sy, sx, '=', stFloorDecor)
			}
		}
	}

	// Machines: an icon at its tile + a short label above/below within view.
	for _, mc := range rm.fmachines {
		sx, sy := mc.mx-ox, mc.my-oy
		if sx < 0 || sy < 0 || sx >= vpW || sy >= vpH {
			continue
		}
		f.SetRune(vpTop+sy, sx, '+', stMachine)
		// Label on the row below the icon if room.
		lx := mc.mx - ox - len(mc.name)/2
		f.Text(vpTop+sy+1, clampi(lx, 0, vpW-1), mc.name, stMachine)
		// Occupant's character tile sits ON the machine when taken.
		if acct, ok := rm.occupied[mc.id]; ok {
			if op, ok := rm.names[acct]; ok && op.Character.Glyph != "" {
				f.Set(vpTop+sy, sx, kit.CharacterCell(op.Character))
			}
		}
	}

	// Players (roaming) as character tiles + names.
	for _, id := range rm.order {
		op := rm.pawns[id]
		if op == nil || op.seated {
			continue
		}
		sx, sy := op.x-ox, op.y-oy
		if sx < 0 || sy < 0 || sx >= vpW || sy >= vpH {
			continue
		}
		st := stOther
		if id == viewer.AccountID {
			st = stSelf
		}
		if pl, ok := rm.names[id]; ok && pl.Character.Glyph != "" {
			f.Set(vpTop+sy, sx, kit.CharacterCell(pl.Character))
		} else {
			f.SetRune(vpTop+sy, sx, '?', st)
		}
		// Name label to the right if it fits.
		if pl, ok := rm.names[id]; ok {
			nm := pl.Handle
			if len(nm) > 8 {
				nm = nm[:8]
			}
			f.Text(vpTop+sy, clampi(sx+1, 0, vpW-1), nm, st)
		}
	}
}
```

> `rm.order` is the existing join-order slice of account ids; iterate it (not the map) for deterministic, hibernation-stable render order. If `order` is not maintained for all joined players, fall back to iterating `rm.pawns` keys sorted — but `order` is appended in `OnJoin` already.

- [ ] **Step 4: Wire** into `compose` (Phase E2) — until then this is unit-tested by calling `rm.drawFloor` directly. Adjust the test to call through `compose` once E2 lands. For Step 3, add a temporary direct call path or land E2 together. **Do E2 now**, then run.

- [ ] **Step 5: Commit** with E2.

### Task E2: `compose` dispatches floor vs seated cabinet

**Files:** Modify `layout.go` (`compose`).

- [ ] **Step 1: Implement** — replace the body of `compose` so it branches on the viewer's mode:

```go
func (rm *room) compose(v kit.Player) *kit.Frame {
	f := composeFrame
	f.Clear()

	pw := rm.pawns[v.AccountID]
	if pw != nil && pw.seated {
		rm.composeSeated(f, v)
		return f
	}
	rm.composeFloor(f, v)
	return f
}

// composeFloor draws the lounge title, the camera window, and the viewer's
// bankroll + controls.
func (rm *room) composeFloor(f *kit.Frame, v kit.Player) {
	f.Text(0, 2, "*** POKIES LOUNGE ***", stTitle)
	if rm.tickerActive(rm.lastNow) {
		rm.drawTicker(f) // extracted from the old compose ticker block
	}
	rm.drawFloor(f, v)
	f.Text(kit.Rows-1, 2, "Arrows move   SPACE sit   Esc leave", stDim)
	if m := rm.machines[v.AccountID]; m != nil {
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
}

// composeSeated draws the single seated cabinet centred, plus the paytable of the
// machine's bound variant and the seated controls.
func (rm *room) composeSeated(f *kit.Frame, v kit.Player) {
	id := v.AccountID
	pw := rm.pawns[id]
	mc := rm.machineByID(pw.seat)
	title := "*** POKIES ***"
	if mc != nil {
		title = "*** " + mc.name + " ***"
	}
	f.Text(0, 2, title, stTitle)
	if rm.tickerActive(rm.lastNow) {
		rm.drawTicker(f)
	}
	// Centre the existing 15-wide cabinet.
	col := (kit.Cols - cardW) / 2
	rm.drawCard(f, col, cardTop, id, true)
	rm.drawPaytableFor(f, payRowY, rm.seatVariant(id))
	controls := "Up/Down bet   SPACE spin   Esc stand"
	if m := rm.machines[id]; m != nil {
		switch {
		case m.gamble != nil:
			controls = "Arrows pick   SPACE lock/take   Esc stand"
		case m.freeSpins > 0:
			controls = "FREE SPINS auto-playing...   Esc stand"
		}
		f.TextRight(kit.Rows-1, kit.Cols-2, fmt.Sprintf("BAL %d   HI %d", m.balance, m.highScore), stDim)
	}
	f.Text(kit.Rows-1, 2, controls, stDim)
}

// seatVariant returns the variant bound to the player's current seat (default if
// unseated/unknown).
func (rm *room) seatVariant(id string) *variant {
	if pw := rm.pawns[id]; pw != nil && pw.seated && pw.seat >= 0 && pw.seat < len(rm.themes) {
		return rm.themes[pw.seat]
	}
	return rm.variant
}
```

Refactors required to support the above:
- Extract the ticker-drawing block from the old `compose` into `drawTicker(f *kit.Frame)`.
- Generalize `drawPaytable(f, row)` to `drawPaytableFor(f, row, v *variant)` (it currently reads `rm.variant`); keep a thin `drawPaytable` wrapper using `rm.variant` if other callers exist, or update them.
- `drawFloor` is named `composeFloor`'s helper; rename the Task E1 function to `drawFloor` consistently (used above).

- [ ] **Step 2: Run** `go test ./... -run TestFloorRendersCharacterAvatars` → PASS.

- [ ] **Step 3: Commit** `git commit -am "feat(pokies): render floor (character avatars) vs seated cabinet by mode"`

---

## Phase F — Rework existing tests + finish

### Task F1: Fix tests broken by the floor restructure

**Files:** Modify `pokies_test.go`.

The old model rendered up to 5 cabinets for every viewer; now a viewer sees the floor unless seated. Update/replace:

- [ ] **Step 1:** `TestFrameShowsTitleAndOwnBalance` — a fresh (roaming) player sees `POKIES LOUNGE` and their balance. Change the title assertion to `"POKIES LOUNGE"` and keep the `1000` balance check; drop the "machine label" check (no cabinet while roaming).
- [ ] **Step 2:** `TestFrameShowsAllMachines` — delete (no longer the model) or replace with `TestFloorRendersCharacterAvatars` (already added).
- [ ] **Step 3:** Reel/cabinet render tests (`TestReelFacesRenderAsWideGraphemes`, `TestScreenBoxFitsWideFaces`, `TestBlankFacesAreSingleWidthDashes`, `TestPaytableStripNamesSymbolsWithArt`, `TestGambleOwnerSeesSelector...`, `TestFreeSpinCabinetShowsFreeCount`, `TestControlsLineReflectsMode`) — these need the viewer **seated** to render the cabinet. Add a helper and seat the player first:

```go
// seatAt seats player p at machine 0 (so the cabinet renders) and returns its machine.
func seatAt0(t *testing.T, rm *room, p kit.Player) *machine {
	t.Helper()
	pw := rm.pawns[p.AccountID]
	mc := rm.fmachines[0]
	pw.x, pw.y = mc.ax, mc.ay
	rm.trySit(p.AccountID)
	return rm.machines[p.AccountID]
}
```

Insert `seatAt0(t, rm, p)` (after `OnJoin`) in each cabinet-render test before `rm.render`. The geometry helpers `soloCardCol`/`soloFaceCol` still hold because `composeSeated` centres the same 15-wide `drawCard` at `(kit.Cols-cardW)/2`.

- [ ] **Step 4:** `TestControlsLineReflectsMode` — update expected strings: roaming → "sit"; seated idle → "spin"; gamble → "lock"; free spins → "FREE SPINS". Seat the player for the seated cases.
- [ ] **Step 5: Run** `go test ./...` until green. **Commit** `git commit -am "test(pokies): adapt cabinet/render tests to the floor + seated model"`

### Task F2: Alloc, vet, build, smoke

**Files:** Modify `alloc_test.go`; verify build.

- [ ] **Step 1:** Add a floor-render alloc budget test (the floor render uses per-player name `Text`; pin a small budget, matching the existing discipline):

```go
func TestDrawFloorAllocBudget(t *testing.T) {
	a := kittest.Player("anna")
	rm, _ := newGame(t, a)
	rm.OnJoin(rmRoomUnused(), a) // see note: use the room+kittest.Room from newGame
	f := kit.NewFrame()
	if n := testing.AllocsPerRun(50, func() { rm.drawFloor(f, a) }); n > 8 {
		t.Fatalf("drawFloor allocates %.0f/call, want a small bounded budget", n)
	}
}
```

> Adjust the budget to the measured value (run once, read the number, pin at that or just above). Use the `newGame` harness pattern already in `pokies_test.go` for room+player setup; do not fabricate `rmRoomUnused` — seat/join via the real harness.

- [ ] **Step 2:** `gofmt -w *.go && go vet ./... && go test ./... -count=1` — all green/clean.
- [ ] **Step 3:** `go build ./...`; if `tinygo` present, the wasip1 c-shared build (from `main.go`'s documented command).
- [ ] **Step 4:** Update `smoke.yaml` for a roaming + sit sequence (arrows then space then a spin); keep `seats: 1`.
- [ ] **Step 5: Commit** `git commit -am "test(pokies): floor alloc budget; chore: smoke + build"`

### Task F3: PR

- [ ] **Step 1:** Full suite green, vet clean, native + wasm build.
- [ ] **Step 2:** Use `superpowers:finishing-a-development-branch` → push `bcook/pokies-lounge` and open a PR (base `main`, stacked on #60) summarizing the resident floor, character avatars, and the seated reuse of the 3-reel engine. Note PR3 (5-reel engine) follows.

---

## Self-review checklist (author)

- **Spec coverage:** resident/scale (A1), map (B1–B2), camera (B3), pawns/movement/collision (C1–C2), seating/occupancy/variant-bind (C3), mode dispatch input (D1) + wake (D2), floor render with **character avatars** (E1) and seated cabinet (E2), test rework (F1), alloc/build/smoke (F2), PR (F3). Distinct per-theme odds + 5-reel are explicitly PR3. ✓
- **Placeholder scan:** the two soft spots — the `kittest` character-setting API (E1) and the alloc budget number (F2) — each carry a concrete "measure/check then pin" instruction, not a vague TODO. ✓
- **Type consistency:** `pawn{x,y,seated,seat}`, `floorMachine{id,name,mx,my,ax,ay}`, `floorMap.at/walkable`, `cameraOrigin`, `tryMove/trySit/standUp/machineByID/machineAtApproach/pawnAt`, `seatVar` on `machine`, `themes []*variant`, `drawFloor/composeFloor/composeSeated/seatVariant/drawTicker/drawPaytableFor` — names used consistently across tasks. ✓
- **Determinism:** static map, discrete movement, `rm.order` iteration — hibernation-stable. ✓
