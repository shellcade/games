package shellracer

import (
	"testing"
	"time"

	kit "github.com/shellcade/kit/v2"
)

// When every racer disconnects mid-race the room must still settle (call End)
// so the DNF results reach the leaderboard, even though no one remains to drive
// the results-hold wake or press Enter.
func TestAllRacersLeaveSettlesRace(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second) // -> racing
	if d.rm.phase != phRacing {
		t.Fatalf("phase=%q after countdown, want racing", d.rm.phase)
	}
	d.advance(2 * time.Second) // non-zero WPM clock

	// Both racers disconnect, one after the other.
	d.leave(a)
	d.leave(b)

	if d.r.Ended == nil {
		t.Fatal("room did not settle (End not called) after all racers left mid-race")
	}
	res := *d.r.Ended
	if len(res.Rankings) != 2 {
		t.Fatalf("rankings=%d, want 2 (both racers present)", len(res.Rankings))
	}
	for _, pr := range res.Rankings {
		if pr.Status != kit.StatusDNF {
			t.Fatalf("player %s status=%v, want DNF", pr.Player.AccountID, pr.Status)
		}
	}
}

// Settling on the last leave must not double-settle (call End twice) if the
// room later receives another wake or input.
func TestAllRacersLeaveNoDoubleSettle(t *testing.T) {
	d := newDriver(kit.ModeQuick, 5)
	a, b := player("a"), player("b")
	d.join(a)
	d.join(b)
	d.advance(countdownDur + time.Second)
	d.advance(2 * time.Second)

	d.leave(a)
	d.leave(b)
	if d.r.Ended == nil {
		t.Fatal("room did not settle after all racers left")
	}
	first := d.r.Ended

	// A late wake on the now-empty, already-settled room must not End again.
	d.rm.OnWake(d.r)
	if d.r.Ended != first {
		t.Fatal("room settled a second time (End called twice)")
	}
}
