package main

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// turnTimeout is how long a player has to move before forfeiting. Held as a
// deadline against r.Now() (the room clock) and checked on the wake heartbeat
// — the canonical wake idiom, fully deterministic under hibernation because
// the deadline lives in guest memory and r.Now() is the virtualized clock.
const turnTimeout = 60 * time.Second

// marks; 0 is an empty cell.
const (
	empty = 0
	markX = 'X'
	markO = 'O'
)

// winLines are the eight three-in-a-row lines over the 0..8 cell indices.
var winLines = [8][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // cols
	{0, 4, 8}, {2, 4, 6}, // diagonals
}

// Game is the registry entry: static metadata plus the per-room factory.
type Game struct{}

// Meta returns the static game metadata.
func (Game) Meta() kit.GameMeta {
	return kit.GameMeta{
		Slug:             "tic-tac-toe",
		Name:             "Tic-Tac-Toe",
		ShortDescription: "Classic two-player noughts and crosses; first to three in a row wins.",
		MinPlayers:       2,
		MaxPlayers:       2,
		Tags:             []string{"board", "two-player", "classic"},

		QuickModeLabel:    "Quick match",
		PrivateInviteLine: "Share the code; your opponent joins your board.",
	}
}

// NewRoom returns the per-room behavior.
func (Game) NewRoom(cfg kit.RoomConfig, svc kit.Services) kit.Handler {
	return &room{
		frame:   kit.NewFrame(),
		players: map[string]kit.Player{},
	}
}

// room is one match. All state lives here (the only durable place): the board,
// the two seats keyed by account id (hibernation-safe), whose turn it is, the
// settled outcome, and the per-turn forfeit deadline. One *kit.Frame is reused
// across renders for an allocation-free steady state.
type room struct {
	kit.Base

	frame   *kit.Frame
	players map[string]kit.Player // account id -> player, for display + result

	board [9]byte
	xID   string // account id of the X player ("" until seated)
	oID   string // account id of the O player
	turn  byte   // markX or markO — whose move it is
	moves int    // marks placed (9 == draw if no winner)

	over     bool
	winnerID string // account id of the winner; "" on draw or while playing

	deadline time.Time // current turn's forfeit deadline (zero until both seated)
}

func (rm *room) OnStart(r kit.Room) {
	r.SetInputContext(kit.CtxNav)
	rm.turn = markX
}

// OnJoin seats the first two joiners as X then O (roster order). A re-join of
// an already-seated account just re-renders. Once both seats are filled and the
// game is live, the turn timer starts.
func (rm *room) OnJoin(r kit.Room, p kit.Player) {
	rm.players[p.AccountID] = p

	switch p.AccountID {
	case rm.xID, rm.oID:
		// already seated (a re-join) — nothing to assign.
	default:
		if rm.xID == "" {
			rm.xID = p.AccountID
		} else if rm.oID == "" {
			rm.oID = p.AccountID
		}
	}

	if !rm.over && rm.bothSeated() && rm.deadline.IsZero() {
		rm.deadline = r.Now().Add(turnTimeout)
	}
	rm.render(r)
}

// OnLeave: if the match is still live, the player who left forfeits and the
// remaining player wins. If it is already settled (or the leaver was never a
// seated player), there is nothing to do.
func (rm *room) OnLeave(r kit.Room, p kit.Player) {
	if rm.over {
		return
	}
	leaver := p.AccountID
	if leaver != rm.xID && leaver != rm.oID {
		return // a non-seated viewer left; the match is unaffected.
	}
	winner := rm.oID
	if leaver == rm.oID {
		winner = rm.xID
	}
	if winner == "" {
		// The only seated player left before an opponent arrived: nothing to
		// settle, just clear the seat so a fresh opponent can take it.
		if leaver == rm.xID {
			rm.xID = ""
		} else {
			rm.oID = ""
		}
		rm.deadline = time.Time{}
		delete(rm.players, leaver)
		rm.render(r)
		return
	}
	rm.settleForfeit(r, winner, leaver)
}

// OnInput places a mark for the current player. Out-of-turn input, non-digit
// input, and moves onto an occupied cell are ignored (no re-render — we render
// on change only). A move that ends the game settles the room.
func (rm *room) OnInput(r kit.Room, p kit.Player, in kit.Input) {
	if rm.over || !rm.bothSeated() {
		return
	}
	mark := rm.markFor(p.AccountID)
	if mark == 0 || mark != rm.turn {
		return // not a seated player, or not their turn.
	}
	if in.Kind != kit.InputRune || in.Rune < '1' || in.Rune > '9' {
		return
	}
	cell := int(in.Rune - '1')
	if rm.board[cell] != empty {
		return // occupied.
	}

	rm.board[cell] = mark
	rm.moves++

	switch {
	case rm.hasWon(mark):
		rm.settleWin(r, rm.idOf(mark))
		return
	case rm.moves == 9:
		rm.settleDraw(r)
		return
	}
	rm.flipTurn()
	rm.deadline = r.Now().Add(turnTimeout)
	rm.render(r)
}

// OnWake forfeits the current mover if their turn deadline has passed. This is
// the only time-driven path; with no animation there is nothing else to do, so
// it renders only when the timeout actually fires (render on change).
func (rm *room) OnWake(r kit.Room) {
	if rm.over || !rm.bothSeated() || rm.deadline.IsZero() {
		return
	}
	if r.Now().After(rm.deadline) {
		loser := rm.idOf(rm.turn)
		winner := rm.xID
		if loser == rm.xID {
			winner = rm.oID
		}
		rm.settleForfeit(r, winner, loser)
	}
}

// --- settling ----------------------------------------------------------------

func (rm *room) settleWin(r kit.Room, winnerID string) {
	rm.over = true
	rm.winnerID = winnerID
	rm.deadline = time.Time{}
	rm.render(r)
	loserID := rm.xID
	if loserID == winnerID {
		loserID = rm.oID
	}
	r.End(kit.Result{Rankings: []kit.PlayerResult{
		{Player: rm.member(r, winnerID), Metric: 1, Rank: 1, Status: kit.StatusFinished},
		{Player: rm.member(r, loserID), Metric: 0, Rank: 2, Status: kit.StatusFinished},
	}})
}

func (rm *room) settleDraw(r kit.Room) {
	rm.over = true
	rm.winnerID = ""
	rm.deadline = time.Time{}
	rm.render(r)
	r.End(kit.Result{Rankings: []kit.PlayerResult{
		{Player: rm.member(r, rm.xID), Metric: 0, Rank: 1, Status: kit.StatusFinished},
		{Player: rm.member(r, rm.oID), Metric: 0, Rank: 1, Status: kit.StatusFinished},
	}})
}

// settleForfeit settles a match the loser abandoned (by leaving or by letting
// the turn timer expire): the winner takes rank 1, the loser is DNF at rank 2.
func (rm *room) settleForfeit(r kit.Room, winnerID, loserID string) {
	rm.over = true
	rm.winnerID = winnerID
	rm.deadline = time.Time{}
	rm.render(r)
	r.End(kit.Result{Rankings: []kit.PlayerResult{
		{Player: rm.member(r, winnerID), Metric: 1, Rank: 1, Status: kit.StatusFinished},
		{Player: rm.member(r, loserID), Metric: 0, Rank: 2, Status: kit.StatusDNF},
	}})
}

// --- board helpers -----------------------------------------------------------

func (rm *room) bothSeated() bool { return rm.xID != "" && rm.oID != "" }

func (rm *room) markFor(id string) byte {
	switch id {
	case rm.xID:
		return markX
	case rm.oID:
		return markO
	}
	return 0
}

func (rm *room) idOf(mark byte) string {
	if mark == markX {
		return rm.xID
	}
	return rm.oID
}

func (rm *room) flipTurn() {
	if rm.turn == markX {
		rm.turn = markO
	} else {
		rm.turn = markX
	}
}

func (rm *room) hasWon(mark byte) bool {
	for _, line := range winLines {
		if rm.board[line[0]] == mark && rm.board[line[1]] == mark && rm.board[line[2]] == mark {
			return true
		}
	}
	return false
}

// member resolves a seated account id to the live roster Player so End maps it
// to the right roster index. Falls back to the stored player (the leaver, who
// the host delivers as the final roster entry), then to a synthetic player.
func (rm *room) member(r kit.Room, id string) kit.Player {
	for _, p := range r.Members() {
		if p.AccountID == id {
			return p
		}
	}
	if p, ok := rm.players[id]; ok {
		return p
	}
	return kit.Player{AccountID: id}
}
