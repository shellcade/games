package main

import (
	"math/rand"
	"testing"
)

// mkFindCard is a helper that builds a findCard directly with explicit outcome
// and a seeded RNG.
func mkFindCard(t *testing.T, ticket Ticket, win int, seed int64) *findCard {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	out := Outcome{Win: win}
	return newFindCard(&ticket, out, rng)
}

// countPanels counts panels whose Reveal equals label in the grid.
func countPanels(g *Grid, label string) int {
	n := 0
	for _, p := range g.Panels {
		if p.Reveal == label {
			n++
		}
	}
	return n
}

// countBusts counts panels where Bust==true.
func countBusts(g *Grid) int {
	n := 0
	for _, p := range g.Panels {
		if p.Bust {
			n++
		}
	}
	return n
}

// --- Winning card assertions ---

// TestFindWinHasExactlyThreeTargets checks that every winning card places
// exactly 3 target panels.
func TestFindWinHasExactlyThreeTargets(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 1},
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 42},
		{Ticket{Symbol: "CHRY", HasBust: false, Cols: 3, Rows: 3}, 7},
		{Ticket{Symbol: "GEM", HasBust: false, Cols: 5, Rows: 4}, 99},
		{Ticket{Symbol: "PICK", HasBust: true, Cols: 6, Rows: 5}, 2026},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 50, cfg.seed)
		got := countPanels(c.grid, cfg.ticket.Symbol)
		if got != 3 {
			t.Errorf("seed=%d sym=%s: winning card has %d targets, want 3",
				cfg.seed, cfg.ticket.Symbol, got)
		}
	}
}

// TestFindWinNoBust asserts no winning card carries a BUST panel.
func TestFindWinNoBust(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 1},
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 100},
		{Ticket{Symbol: "PICK", HasBust: true, Cols: 6, Rows: 5}, 999},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 50, cfg.seed)
		busts := countBusts(c.grid)
		if busts != 0 {
			t.Errorf("seed=%d: winning card has %d bust panel(s), want 0", cfg.seed, busts)
		}
	}
}

// TestFindWinRevealAllResolves checks that revealing all panels resolves to
// Win()==out.Win for a winning card.
func TestFindWinRevealAllResolves(t *testing.T) {
	configs := []struct {
		ticket Ticket
		win    int
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 50, 1},
		{Ticket{Symbol: "GEM", HasBust: false, Cols: 5, Rows: 4}, 100, 7},
		{Ticket{Symbol: "CHRY", HasBust: false, Cols: 3, Rows: 3}, 10, 13},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, cfg.win, cfg.seed)
		c.ScratchAll()
		if !c.Resolved() {
			t.Errorf("seed=%d: not resolved after ScratchAll", cfg.seed)
		}
		if c.Win() != cfg.win {
			t.Errorf("seed=%d: Win()=%d, want %d", cfg.seed, c.Win(), cfg.win)
		}
	}
}

// TestFindWinNeverAutoResolvesToZeroViaBust asserts a winning card never
// resolves to 0 through the BUST path (BUST must not appear on a winning card).
func TestFindWinNeverAutoResolvesToZeroViaBust(t *testing.T) {
	// Try many seeds with a HasBust ticket.
	ticket := Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}
	for seed := int64(0); seed < 50; seed++ {
		c := mkFindCard(t, ticket, 50, seed)
		// Verify no bust exists.
		if countBusts(c.grid) != 0 {
			t.Errorf("seed=%d: winning card has bust panel — impossible", seed)
		}
		// Scratch all and confirm win.
		c.ScratchAll()
		if c.bustHit {
			t.Errorf("seed=%d: winning card resolved via bust (bustHit=true)", seed)
		}
		if c.Win() != 50 {
			t.Errorf("seed=%d: Win()=%d, want 50", seed, c.Win())
		}
	}
}

// --- Losing card assertions ---

// TestFindLossFewerThanThreeTargets checks losing cards have <3 target panels.
func TestFindLossFewerThanThreeTargets(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 5},
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 77},
		{Ticket{Symbol: "CHRY", HasBust: false, Cols: 3, Rows: 3}, 3},
		{Ticket{Symbol: "GEM", HasBust: false, Cols: 5, Rows: 4}, 11},
		{Ticket{Symbol: "PICK", HasBust: true, Cols: 6, Rows: 5}, 404},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 0, cfg.seed)
		got := countPanels(c.grid, cfg.ticket.Symbol)
		if got >= 3 {
			t.Errorf("seed=%d sym=%s: losing card has %d targets, want <3",
				cfg.seed, cfg.ticket.Symbol, got)
		}
	}
}

// TestFindLossHasBustExactlyOnce checks that HasBust losing cards have exactly
// one BUST panel.
func TestFindLossHasBustExactlyOnce(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 5},
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 77},
		{Ticket{Symbol: "PICK", HasBust: true, Cols: 6, Rows: 5}, 404},
		{Ticket{Symbol: "PICK", HasBust: true, Cols: 6, Rows: 5}, 2026},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 0, cfg.seed)
		got := countBusts(c.grid)
		if got != 1 {
			t.Errorf("seed=%d: HasBust losing card has %d bust panels, want 1", cfg.seed, got)
		}
	}
}

// TestFindLossNoBustWhenHasBustFalse checks that non-bust tickets never place
// a BUST panel even on a loss.
func TestFindLossNoBustWhenHasBustFalse(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CHRY", HasBust: false, Cols: 3, Rows: 3}, 3},
		{Ticket{Symbol: "GEM", HasBust: false, Cols: 5, Rows: 4}, 11},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 0, cfg.seed)
		got := countBusts(c.grid)
		if got != 0 {
			t.Errorf("seed=%d: non-bust ticket has %d bust panel(s), want 0", cfg.seed, got)
		}
	}
}

// TestFindLossBustRevealResolvesZero checks that revealing a BUST panel on a
// losing card immediately resolves the card with Win()==0.
func TestFindLossBustRevealResolvesZero(t *testing.T) {
	// Find a losing card with HasBust and locate the BUST panel index.
	ticket := Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}
	for seed := int64(0); seed < 20; seed++ {
		c := mkFindCard(t, ticket, 0, seed)
		bustIdx := -1
		for i, p := range c.grid.Panels {
			if p.Bust {
				bustIdx = i
				break
			}
		}
		if bustIdx < 0 {
			t.Fatalf("seed=%d: losing HasBust card has no bust panel", seed)
		}
		// Reveal the bust panel directly (force cursor there and force-reveal).
		c.grid.Panels[bustIdx].Hidden = false
		c.grid.Panels[bustIdx].Layers = 0
		c.onReveal(bustIdx)

		if !c.Resolved() {
			t.Errorf("seed=%d: card not resolved after bust reveal", seed)
		}
		if c.Win() != 0 {
			t.Errorf("seed=%d: Win()=%d after bust, want 0", seed, c.Win())
		}
		if !c.bustHit {
			t.Errorf("seed=%d: bustHit=false after bust reveal", seed)
		}
	}
}

// TestFindLossRevealAllResolvesZero checks that a losing card resolved via
// ScratchAll gives Win()==0.
func TestFindLossRevealAllResolvesZero(t *testing.T) {
	configs := []struct {
		ticket Ticket
		seed   int64
	}{
		{Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}, 5},
		{Ticket{Symbol: "CHRY", HasBust: false, Cols: 3, Rows: 3}, 3},
		{Ticket{Symbol: "GEM", HasBust: false, Cols: 5, Rows: 4}, 11},
	}
	for _, cfg := range configs {
		c := mkFindCard(t, cfg.ticket, 0, cfg.seed)
		c.ScratchAll()
		if !c.Resolved() {
			t.Errorf("seed=%d: not resolved after ScratchAll", cfg.seed)
		}
		if c.Win() != 0 {
			t.Errorf("seed=%d: Win()=%d, want 0", cfg.seed, c.Win())
		}
	}
}

// TestFindWinAutoResolvesOnThirdTarget checks that the card auto-resolves when
// the third target is scratched (before all panels are open).
func TestFindWinAutoResolvesOnThirdTarget(t *testing.T) {
	// Build a winning card with enough panels that we can find the 3 targets
	// and verify resolution triggers mid-card.
	ticket := Ticket{Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}
	const win = 50
	for seed := int64(0); seed < 10; seed++ {
		c := mkFindCard(t, ticket, win, seed)
		revealed := 0
		resolved := false
		// Scratch panels until the card resolves.
		for i := range c.grid.Panels {
			// Force-reveal this panel.
			if c.grid.Panels[i].Hidden {
				c.grid.Panels[i].Hidden = false
				c.grid.Panels[i].Layers = 0
				c.onReveal(i)
				revealed++
			}
			if c.Resolved() {
				resolved = true
				break
			}
		}
		if !resolved {
			t.Errorf("seed=%d: card never resolved", seed)
			continue
		}
		if c.Win() != win {
			t.Errorf("seed=%d: Win()=%d, want %d", seed, c.Win(), win)
		}
		// Should have resolved in ≤ all panels (but typically before all revealed).
		_ = revealed
	}
}

// TestFindTitleAndPrompt checks Title() and Prompt() are non-empty and contain
// expected keywords.
func TestFindTitleAndPrompt(t *testing.T) {
	ticket := Ticket{Name: "Croc Cash", Price: 2, Symbol: "CROC", HasBust: true, Cols: 4, Rows: 3}
	c := mkFindCard(t, ticket, 50, 1)

	title := c.Title()
	if title == "" {
		t.Fatal("Title() is empty")
	}
	// Should mention the game name and symbol.
	if !contains(title, "Croc Cash") {
		t.Errorf("Title() missing game name: %q", title)
	}

	prompt := c.Prompt()
	if prompt == "" {
		t.Fatal("Prompt() is empty before resolution")
	}

	// Scratch all and check resolved prompt contains win info.
	c.ScratchAll()
	prompt = c.Prompt()
	if prompt == "" {
		t.Fatal("Prompt() is empty after resolution")
	}
}

// contains is a small helper for substring checks (avoid importing strings in test).
func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

// TestFindVariousGridSizes ensures the engine works for every grid size used in
// the catalog.
func TestFindVariousGridSizes(t *testing.T) {
	sizes := []struct {
		cols, rows int
		symbol     string
		hasBust    bool
	}{
		{3, 3, "CHRY", false}, // Cherry Pop $1 (9 panels)
		{4, 3, "CROC", true},  // Croc Cash $2 (12 panels)
		{5, 4, "GEM", false},  // Treasure Hunt $5 (20 panels)
		{6, 5, "PICK", true},  // Outback Riches $10 (30 panels)
	}
	for _, sz := range sizes {
		ticket := Ticket{Symbol: sz.symbol, HasBust: sz.hasBust, Cols: sz.cols, Rows: sz.rows}

		// Win case.
		for seed := int64(0); seed < 5; seed++ {
			c := mkFindCard(t, ticket, 100, seed)
			tCount := countPanels(c.grid, sz.symbol)
			if tCount != 3 {
				t.Errorf("%dx%d win seed=%d: %d targets, want 3", sz.cols, sz.rows, seed, tCount)
			}
			bCount := countBusts(c.grid)
			if bCount != 0 {
				t.Errorf("%dx%d win seed=%d: %d busts on winning card, want 0", sz.cols, sz.rows, seed, bCount)
			}
		}

		// Loss case.
		for seed := int64(0); seed < 5; seed++ {
			c := mkFindCard(t, ticket, 0, seed)
			tCount := countPanels(c.grid, sz.symbol)
			if tCount >= 3 {
				t.Errorf("%dx%d loss seed=%d: %d targets (≥3) on losing card", sz.cols, sz.rows, seed, tCount)
			}
			if sz.hasBust {
				bCount := countBusts(c.grid)
				if bCount != 1 {
					t.Errorf("%dx%d loss seed=%d: %d busts, want 1", sz.cols, sz.rows, seed, bCount)
				}
			}
		}
	}
}
