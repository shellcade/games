package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
)

// TestLeaderboardSpecDeclared verifies chess declares a Wins leaderboard with
// the career-tally shape: higher-is-better, summed across games, integer.
func TestLeaderboardSpecDeclared(t *testing.T) {
	lb := Game{}.Meta().Leaderboard
	if lb == nil {
		t.Fatal("Meta().Leaderboard is nil; want a Wins board")
	}
	if lb.MetricLabel != "Wins" {
		t.Errorf("MetricLabel=%q, want %q", lb.MetricLabel, "Wins")
	}
	if lb.Direction != kit.HigherBetter {
		t.Errorf("Direction=%v, want HigherBetter", lb.Direction)
	}
	if lb.Aggregation != kit.SumResults {
		t.Errorf("Aggregation=%v, want SumResults", lb.Aggregation)
	}
	if lb.Format != kit.Integer {
		t.Errorf("Format=%v, want Integer", lb.Format)
	}
}

// metricOf returns the posted Metric for player p in a settled result.
func metricOf(t *testing.T, res kit.Result, p kit.Player) int {
	t.Helper()
	for _, pr := range res.Rankings {
		if pr.Player.AccountID == p.AccountID {
			return pr.Metric
		}
	}
	t.Fatalf("player %v not in rankings %+v", p.Handle, res.Rankings)
	return 0
}

// TestCheckmateWinnerMetricIsOne drives a checkmate and asserts the winner posts
// Metric 1 and the loser 0.
func TestCheckmateWinnerMetricIsOne(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	// Fool's mate: Black mates White.
	mustPlay(t, rm, tr, "f2", "f3")
	mustPlay(t, rm, tr, "e7", "e5")
	mustPlay(t, rm, tr, "g2", "g4")
	mustPlay(t, rm, tr, "d8", "h4")

	settleAfterResults(tr, rm)
	if tr.Ended == nil {
		t.Fatal("game did not settle")
	}
	if got := metricOf(t, *tr.Ended, black); got != 1 {
		t.Errorf("winner (Black) metric=%d, want 1", got)
	}
	if got := metricOf(t, *tr.Ended, white); got != 0 {
		t.Errorf("loser (White) metric=%d, want 0", got)
	}
}

// TestResignWinnerMetricIsOne drives a resignation and asserts win-count metrics.
func TestResignWinnerMetricIsOne(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('r')) // arm
	rm.OnInput(tr, white, runeInput('r')) // confirm resign

	settleAfterResults(tr, rm)
	if got := metricOf(t, *tr.Ended, black); got != 1 {
		t.Errorf("winner (Black) metric=%d, want 1", got)
	}
	if got := metricOf(t, *tr.Ended, white); got != 0 {
		t.Errorf("resigner (White) metric=%d, want 0", got)
	}
}

// TestDrawPostsZeroMetrics verifies a draw is not a win for either side.
func TestDrawPostsZeroMetrics(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnInput(tr, white, runeInput('d')) // White offers
	rm.OnInput(tr, black, runeInput('y')) // Black accepts

	settleAfterResults(tr, rm)
	if got := metricOf(t, *tr.Ended, white); got != 0 {
		t.Errorf("draw: White metric=%d, want 0", got)
	}
	if got := metricOf(t, *tr.Ended, black); got != 0 {
		t.Errorf("draw: Black metric=%d, want 0", got)
	}
}

// TestForfeitByLeaveWinCounts verifies the leaver keeps DNF/Metric 0 and the
// opponent gets Metric 1 / Finished.
func TestForfeitByLeaveWinCounts(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	rm.OnLeave(tr, white) // White abandons mid-game
	settleAfterResults(tr, rm)
	if tr.Ended == nil {
		t.Fatal("forfeit did not settle")
	}
	checkRankStatus(t, *tr.Ended, white, kit.StatusDNF)
	checkRankStatus(t, *tr.Ended, black, kit.StatusFinished)
	if got := metricOf(t, *tr.Ended, black); got != 1 {
		t.Errorf("forfeit winner (Black) metric=%d, want 1", got)
	}
	if got := metricOf(t, *tr.Ended, white); got != 0 {
		t.Errorf("leaver (White) metric=%d, want 0", got)
	}
}

// TestTimeoutWinnerMetricIsOne verifies the flag-fall winner posts Metric 1.
func TestTimeoutWinnerMetricIsOne(t *testing.T) {
	rm, tr := newGame(t)
	a, b := pair(t, rm, tr)
	white, black := whiteBlack(rm, a, b)

	tr.Advance(mainClock + 1)
	rm.OnWake(tr)

	settleAfterResults(tr, rm)
	if got := metricOf(t, *tr.Ended, black); got != 1 {
		t.Errorf("flag-fall winner (Black) metric=%d, want 1", got)
	}
	if got := metricOf(t, *tr.Ended, white); got != 0 {
		t.Errorf("flagged (White) metric=%d, want 0", got)
	}
}
