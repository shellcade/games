package shellracer

import (
	"time"

	kit "github.com/shellcade/kit/v2"
)

// resultsHeader reproduces the old fmt.Sprintf("%-4s %-20s %-6s %-6s %s",
// "#", "Player", "WPM", "ACC", "Status"): "#" left-justified in 4, a space,
// "Player" left in 20, a space, "WPM" left in 6, a space, "ACC" left in 6, a
// space, then "Status". Precomputed once so the results header costs no
// per-render allocation.
const resultsHeader = "#    Player               WPM    ACC    Status"

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
	c := f.Text(0, 1, "Shell Racer  [", stHeader)
	c = f.Text(0, c, rm.pdiff, stHeader)
	f.Text(0, c, "]", stHeader)
	// "(%d/%d)" right-justified ending at kit.Cols-1, drawn alloc-free.
	n, capacity := len(members), rm.cfg.Capacity
	rw := 3 + intWidth(n) + intWidth(capacity) // '(' + n + '/' + cap + ')'
	hc := kit.Cols - 1 - rw + 1
	hc = f.Text(0, hc, "(", stHeader)
	hc = putInt(f, 0, hc, n, stHeader)
	hc = f.Text(0, hc, "/", stHeader)
	hc = putInt(f, 0, hc, capacity, stHeader)
	f.Text(0, hc, ")", stHeader)

	switch rm.phase {
	case phLobby:
		f.Text(10, 1, "Waiting for opponents...", stAccent)
		lc := putInt(f, 11, 1, len(members), stDim)
		f.Text(11, lc, " player(s) in the room.", stDim)
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
		rm.racerRowYou(f, row, v.DisplayName(), ps, stAccent, stAccent)
		row++
	}

	// Build the opponent list into a reusable buffer (no per-render alloc) and
	// stable-sort it by join order then account id with an in-place insertion
	// sort (sort.SliceStable's closure would allocate, leaking under -gc=leaking).
	opps := rm.oppBuf[:0]
	for _, p := range r.Members() {
		if p.AccountID != v.AccountID {
			opps = append(opps, p)
		}
	}
	rm.oppBuf = opps
	for i := 1; i < len(opps); i++ {
		for j := i; j > 0 && rm.oppLess(opps[j], opps[j-1]); j-- {
			opps[j], opps[j-1] = opps[j-1], opps[j]
		}
	}
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

// oppLess is the stable opponent ordering: by join order, then account id.
// Matches the previous sort.SliceStable comparator (nil states sort as equal).
func (rm *room) oppLess(a, b kit.Player) bool {
	oa, ob := rm.st[a.AccountID], rm.st[b.AccountID]
	if oa == nil || ob == nil {
		return false
	}
	if oa.joinOrder != ob.joinOrder {
		return oa.joinOrder < ob.joinOrder
	}
	return a.AccountID < b.AccountID
}

// racerRow draws one strip line: padded name, progress bar, WPM and accuracy.
func (rm *room) racerRow(f *kit.Frame, row int, name string, ps *pstate, nameSt, statSt kit.Style) {
	putTextLeft(f, row, 1, name, 18, nameSt)
	rm.racerRowTail(f, row, ps, statSt)
}

// racerRowYou draws the viewer's own strip line, where the name field holds
// "You (NAME)" left-justified in 18 columns (truncated to fit). Drawn without
// allocating the concatenated label.
func (rm *room) racerRowYou(f *kit.Frame, row int, name string, ps *pstate, nameSt, statSt kit.Style) {
	const end = 1 + 18 // exclusive column of the 18-wide name field at col 1
	col := f.Text(row, 1, "You (", nameSt)
	for _, r := range name {
		if col >= end {
			break
		}
		f.SetRune(row, col, r, nameSt)
		col++
	}
	if col < end {
		f.SetRune(row, col, ')', nameSt)
		col++
	}
	for col < end {
		f.SetRune(row, col, ' ', nameSt)
		col++
	}
	rm.racerRowTail(f, row, ps, statSt)
}

// racerRowTail draws the progress bar and the "WPM:nnn  ACC:nn%" stats that
// follow the name field, alloc-free.
func (rm *room) racerRowTail(f *kit.Frame, row int, ps *pstate, statSt kit.Style) {
	putProgressBar(f, row, 20, ps.cursor, len(rm.passage), 20)
	c := f.Text(row, 43, "WPM:", statSt)
	c = putIntRight(f, row, c, ps.wpmSnapOrLive(rm), 3, statSt)
	c = f.Text(row, c, "  ACC:", statSt)
	putAccuracy(f, row, c, ps, statSt)
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
	// "Starting in %d..." centered over the passage panel, drawn alloc-free.
	const prefix, suffix = "Starting in ", "..."
	msgLen := len(prefix) + intWidth(secs) + len(suffix)
	col := (kit.Cols - msgLen) / 2
	col = f.Text(9, col, prefix, stAccent)
	col = putInt(f, 9, col, secs, stAccent)
	f.Text(9, col, suffix, stAccent)
}

func (rm *room) composeResults(f *kit.Frame, v kit.Player) {
	f.Text(2, 1, "RESULTS", stHeader)
	f.Text(4, 2, resultsHeader, stDim)
	row := 5
	for _, pr := range rm.result.Rankings {
		if row > 20 {
			break
		}
		ps := rm.st[pr.Player.AccountID]
		st := stPlain
		if pr.Player.AccountID == v.AccountID {
			st = stAccent
		}
		// "%-4d %-20s %-6d %-6s %s": rank(4) sp name(20) sp metric(6) sp acc(6)
		// sp status — each field left-justified, a single space between them.
		col := putIntLeft(f, row, 2, pr.Rank, 4, st)
		col = f.Text(row, col, " ", st)
		col = putTextLeft(f, row, col, pr.Player.DisplayName(), 20, st)
		col = f.Text(row, col, " ", st)
		col = putIntLeft(f, row, col, pr.Metric, 6, st)
		col = f.Text(row, col, " ", st)
		col = putAccuracyField(f, row, col, ps, 6, st)
		col = f.Text(row, col, " ", st)
		f.Text(row, col, statusLabel(pr.Status), st)
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

// putAccuracy writes the accuracy ("--" or "NN%") at (row,col), returning the
// next column. Mirrors the old accuracyStr but draws inline without allocating.
func putAccuracy(f *kit.Frame, row, col int, ps *pstate, st kit.Style) int {
	denom := ps.cursor + ps.errorsTotal
	if denom == 0 {
		return f.Text(row, col, "--", st)
	}
	col = putInt(f, row, col, ps.cursor*100/denom, st)
	f.SetRune(row, col, '%', st)
	return col + 1
}

// putAccuracyField writes the accuracy left-justified in a field of `width`
// columns, space-padded. Returns col+width. Alloc-free.
func putAccuracyField(f *kit.Frame, row, col int, ps *pstate, width int, st kit.Style) int {
	end := col + width
	col = putAccuracy(f, row, col, ps, st)
	for col < end {
		f.SetRune(row, col, ' ', st)
		col++
	}
	return end
}

// putProgressBar draws "[####....]" of `width` body cells at (row,col),
// alloc-free, matching the old progressBar output.
func putProgressBar(f *kit.Frame, row, col, done, total, width int) {
	if total <= 0 {
		total = 1
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	f.SetRune(row, col, '[', stDone)
	col++
	for i := 0; i < width; i++ {
		if i < filled {
			f.SetRune(row, col, '#', stDone)
		} else {
			f.SetRune(row, col, '.', stDone)
		}
		col++
	}
	f.SetRune(row, col, ']', stDone)
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
