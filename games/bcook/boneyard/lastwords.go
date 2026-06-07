package main

// Last words (design §9): a CLOSED template system — template + word-bank
// slots, never free text. The dying delver gets a brief modal: [1-5] picks a
// tab's template (slots auto-filled from death context: the killer's slug,
// the depth); Enter (or any move key, or the 8s timer) accepts; the default
// is the panic-scrawl. The anti-grief cornerstone: every renderable string is
// assembled from these banks and live game facts only.

type lwTemplate struct {
	tab  string
	fill func(killer string, floor int, gaspDir string) string
}

// depthRef renders a closed-vocab depth reference.
func depthRef(floor int) string { return "B" + itoa(floor) }

var lwTemplates = []lwTemplate{
	{"Intel", func(k string, f int, g string) string { return "go " + g + " at the stairs." }},
	{"Lament", func(k string, f int, g string) string { return "so close to " + depthRef(f+1) + "." }},
	{"Warn", func(k string, f int, g string) string { return k + " is fast. turn back at " + depthRef(f) + "." }},
	{"Brag", func(k string, f int, g string) string { return depthRef(f) + " and still smiling." }},
	{"Dark", func(k string, f int, g string) string { return "all-in on the " + k + ". no regrets." }},
}

// panicScrawl is the unbanked default (closed vocab, grammatical).
func panicScrawl(killer, gaspDir string) string {
	return "ran " + gaspDir + "... " + killer + "."
}

// gaspName renders a last-gasp direction.
func gaspName(dx, dy int) string {
	switch {
	case dx > 0:
		return "east"
	case dx < 0:
		return "west"
	case dy > 0:
		return "south"
	case dy < 0:
		return "north"
	}
	return "nowhere"
}
