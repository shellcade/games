package shellracer

import (
	"fmt"
	"sort"
	"time"

	kit "github.com/shellcade/kit/v2"
)

var (
	stHeader = kit.Style{FG: kit.White, Attr: kit.AttrBold}
	stDim    = kit.Style{FG: kit.DimGray}
	stDone   = kit.Style{FG: kit.Green}
	stCursor = kit.Style{FG: kit.RGB(0, 0, 0), BG: kit.Cyan}
	stErr    = kit.Style{FG: kit.White, BG: kit.Red}
	stPlain  = kit.Style{}
	stAccent = kit.Style{FG: kit.Yellow, Attr: kit.AttrBold}
)

// render composes a per-viewer frame for every member and sends it. The native
// game used a per-viewer BroadcastFunc over a frozen Snapshot; the lean ABI has
// no snapshot, so each member's frame is composed from live state and r.Send-ed.
func (rm *room) render(r kit.Room) {
	rm.lastNow = r.Now()
	for _, v := range r.Members() {
		rm.compose(rm.frame, r, v)
		r.Send(v, rm.frame)
	}
}

func (rm *room) compose(f *kit.Frame, r kit.Room, v kit.Player) {
	f.Clear()
	members := r.Members()

	// Row 0: header
	f.Text(0, 1, fmt.Sprintf("Shell Racer  [%s]", rm.pdiff), stHeader)
	f.TextRight(0, kit.Cols-1, fmt.Sprintf("(%d/%d)", len(members), rm.cfg.Capacity), stHeader)

	switch rm.phase {
	case phLobby:
		f.Text(10, 1, "Waiting for opponents...", stAccent)
		f.Text(11, 1, fmt.Sprintf("%d player(s) in the room.", len(members)), stDim)
	case phResults:
		rm.composeResults(f, v)
	default: // countdown, racing
		rm.composePassage(f, v)
		rm.composeRacers(f, r, v)
		if rm.phase == phCountdown {
			rm.composeCountdown(f)
		}
	}

	// Row 23: status
	hint := "Esc: leave"
	if rm.phase == phResults {
		hint = "Enter: continue to lobby"
	}
	f.Text(23, 1, hint, stDim)
}

func (rm *room) composePassage(f *kit.Frame, v kit.Player) {
	ps := rm.st[v.AccountID]
	cursor, outstanding := 0, 0
	if ps != nil {
		cursor = ps.cursor
		outstanding = ps.outstanding
	}
	// Wrap is fixed for the room (passage + width constant); computed once in
	// OnStart. Guard for a resumed room whose snapshot predates the cache.
	if rm.plines == nil {
		rm.plines = wrap(rm.passage, passageWidth)
	}
	lines := rm.plines
	// which wrapped line holds the cursor
	curLine := 0
	for i, ln := range lines {
		if cursor >= ln[0] && cursor <= ln[1] {
			curLine = i
		}
	}
	const panelTop, panelRows = 2, 15
	// Auto-scroll keeps the cursor on the 3rd-from-bottom visible row.
	first := curLine - (panelRows - 3)
	if first < 0 {
		first = 0
	}
	if first > len(lines)-panelRows {
		first = len(lines) - panelRows
	}
	if first < 0 {
		first = 0
	}
	// While errors are outstanding the cursor cannot advance: the player has
	// mistyped. One passage character per outstanding error renders in the
	// error style (red) starting at the cursor — the red region's width IS the
	// number of backspaces needed, shrinking from the right as the player
	// corrects. (Errors typed past the end of the passage clamp at the last
	// character.) With no errors outstanding the cursor cell gets the cursor
	// highlight; style precedence is error region > cursor > done > plain.
	for row := 0; row < panelRows; row++ {
		li := first + row
		if li >= len(lines) {
			break
		}
		ln := lines[li]
		col := 2
		for idx := ln[0]; idx < ln[1]; idx++ {
			st := stPlain
			switch {
			case idx >= cursor && idx < cursor+outstanding:
				st = stErr
			case idx == cursor:
				st = stCursor
			case idx < cursor:
				st = stDone
			}
			f.SetRune(panelTop+row, col, rm.passage[idx], st)
			col++
		}
	}
}

// composeRacers draws the racer strip (spec rows 19–23, 0-based 18–22): the
// viewer's own accent-styled row first — name, progress, live WPM and accuracy
// — then up to four opponents in join order. The viewer always sees their own
// pace; the native game showed only opponents.
func (rm *room) composeRacers(f *kit.Frame, r kit.Room, v kit.Player) {
	const stripTop = 18
	row := stripTop
	if ps := rm.st[v.AccountID]; ps != nil {
		rm.racerRow(f, row, "You ("+v.DisplayName()+")", ps, stAccent, stAccent)
		row++
	}

	var opps []kit.Player
	for _, p := range r.Members() {
		if p.AccountID != v.AccountID {
			opps = append(opps, p)
		}
	}
	sort.SliceStable(opps, func(i, j int) bool {
		oi, oj := rm.st[opps[i].AccountID], rm.st[opps[j].AccountID]
		if oi == nil || oj == nil {
			return false
		}
		if oi.joinOrder != oj.joinOrder {
			return oi.joinOrder < oj.joinOrder
		}
		return opps[i].AccountID < opps[j].AccountID
	})
	for _, p := range opps {
		if row > 22 {
			break
		}
		ps := rm.st[p.AccountID]
		if ps == nil {
			continue
		}
		rm.racerRow(f, row, p.DisplayName(), ps, stPlain, stDim)
		row++
	}
}

// racerRow draws one strip line: padded name, progress bar, WPM and accuracy.
func (rm *room) racerRow(f *kit.Frame, row int, name string, ps *pstate, nameSt, statSt kit.Style) {
	if len(name) > 18 {
		name = name[:18]
	}
	f.Text(row, 1, fmt.Sprintf("%-18s", name), nameSt)
	f.Text(row, 20, progressBar(ps.cursor, len(rm.passage), 20), stDone)
	f.Text(row, 43, fmt.Sprintf("WPM:%3d  ACC:%s", ps.wpmSnapOrLive(rm), accuracyStr(ps)), statSt)
}

func (rm *room) composeCountdown(f *kit.Frame) {
	rem := rm.countdownDeadline.Sub(rm.lastNow)
	secs := int(rem.Seconds())
	if rem > time.Duration(secs)*time.Second {
		secs++ // ceil, so it counts 10..1 rather than 9..0
	}
	if secs < 1 {
		secs = 1
	}
	if secs > 99 {
		secs = 99
	}
	msg := fmt.Sprintf("Starting in %d...", secs)
	// a centered band over the passage panel
	f.Text(9, (kit.Cols-len(msg))/2, msg, stAccent)
}

func (rm *room) composeResults(f *kit.Frame, v kit.Player) {
	f.Text(2, 1, "RESULTS", stHeader)
	f.Text(4, 2, fmt.Sprintf("%-4s %-20s %-6s %-6s %s", "#", "Player", "WPM", "ACC", "Status"), stDim)
	row := 5
	for _, pr := range rm.result.Rankings {
		if row > 20 {
			break
		}
		ps := rm.st[pr.Player.AccountID]
		acc := "--"
		if ps != nil {
			acc = accuracyStr(ps)
		}
		name := pr.Player.DisplayName()
		if len(name) > 20 {
			name = name[:20]
		}
		st := stPlain
		if pr.Player.AccountID == v.AccountID {
			st = stAccent
		}
		f.Text(row, 2, fmt.Sprintf("%-4d %-20s %-6d %-6s %s", pr.Rank, name, pr.Metric, acc, statusLabel(pr.Status)), st)
		row++
	}
}

// ---- helpers --------------------------------------------------------------

func statusLabel(s kit.Status) string {
	switch s {
	case kit.StatusFinished:
		return "finished"
	case kit.StatusFlagged:
		return "flagged"
	default:
		return "dnf"
	}
}

func (ps *pstate) wpmSnapOrLive(rm *room) int {
	if ps.statusSet {
		return ps.wpmSnap
	}
	if rm.raceStart.IsZero() {
		return 0
	}
	return rm.netWPM(ps, rm.lastNow)
}

func accuracyStr(ps *pstate) string {
	denom := ps.cursor + ps.errorsTotal
	if denom == 0 {
		return "--"
	}
	return fmt.Sprintf("%d%%", ps.cursor*100/denom)
}

func progressBar(done, total, width int) string {
	if total <= 0 {
		total = 1
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	b := make([]byte, 0, width+2)
	b = append(b, '[')
	for i := 0; i < width; i++ {
		if i < filled {
			b = append(b, '#')
		} else {
			b = append(b, '.')
		}
	}
	b = append(b, ']')
	return string(b)
}

// wrap greedily word-wraps runes to width w, returning [start,end) ranges.
func wrap(runes []rune, w int) [][2]int {
	var lines [][2]int
	n := len(runes)
	i := 0
	for i < n {
		end := i + w
		if end >= n {
			lines = append(lines, [2]int{i, n})
			break
		}
		brk := -1
		for j := i; j <= end && j < n; j++ {
			if runes[j] == ' ' {
				brk = j
			}
		}
		if brk <= i {
			brk = end
		}
		lines = append(lines, [2]int{i, brk})
		i = brk
		if i < n && runes[i] == ' ' {
			i++
		}
	}
	if len(lines) == 0 {
		lines = append(lines, [2]int{0, 0})
	}
	return lines
}
