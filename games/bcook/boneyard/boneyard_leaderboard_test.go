package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// A delver who disconnects mid-run must not lose their banked depth from the
// board until a death or the weekly collapse banks them. OnLeave banks the
// CURRENT banked depth as a DNF — the same metric (d.banked) death and collapse
// post — while the run itself persists in-world for a possible rejoin.
func TestDisconnectBanksDepthDNF(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]

	// Bank B6 at the shrine — the same path the game uses, so banked is a real
	// depth, not a fabricated value (shrines sit on B3/B6/B9...).
	f6 := rm.floorAt(6)
	d.floor, d.x, d.y = 6, f6.shrineX, f6.shrineY
	d.bank(rm, tr)
	if d.banked != 6 {
		t.Fatalf("setup: banked=%d, want 6", d.banked)
	}
	postsAfterBank := len(tr.Posted)

	// Disconnect mid-run: OnLeave must post the banked depth as a DNF.
	rm.OnLeave(tr, a)

	if len(tr.Posted) != postsAfterBank+1 {
		t.Fatalf("OnLeave posted %d results, want exactly 1 (total %d)", len(tr.Posted)-postsAfterBank, len(tr.Posted))
	}
	last := tr.Posted[len(tr.Posted)-1]
	if len(last.Rankings) != 1 {
		t.Fatalf("disconnect post rankings = %+v", last.Rankings)
	}
	pr := last.Rankings[0]
	if pr.Metric != d.banked {
		t.Fatalf("disconnect post metric = %d, want banked %d", pr.Metric, d.banked)
	}
	if pr.Status != kit.StatusDNF {
		t.Fatalf("disconnect post status = %v, want StatusDNF", pr.Status)
	}
	if pr.Player.AccountID != a.AccountID {
		t.Fatalf("disconnect post player = %q, want %q", pr.Player.AccountID, a.AccountID)
	}

	// The run persists in-world for rejoin: the delver is NOT deleted, just
	// marked offline.
	if got, ok := rm.delvers[a.AccountID]; !ok || got != d {
		t.Fatal("OnLeave deleted the in-memory run — it must persist for rejoin")
	}
	if d.online {
		t.Fatal("OnLeave left the delver marked online")
	}
}

// A delver with no progress worth recording (banked == 0) should not pollute
// the board with a zero DNF on disconnect.
func TestDisconnectWithNoBankedDepthDoesNotPost(t *testing.T) {
	a := bp("ada")
	tr := kittest.NewRoom(a)
	rm := Game{}.NewRoom(tr.Cfg, tr.Services()).(*room)
	rm.OnStart(tr)
	rm.OnJoin(tr, a)
	d := rm.delvers[a.AccountID]
	if d.banked != 0 {
		t.Fatalf("setup: banked=%d, want 0", d.banked)
	}

	rm.OnLeave(tr, a)

	if len(tr.Posted) != 0 {
		t.Fatalf("OnLeave with banked=0 posted %d results, want 0", len(tr.Posted))
	}
	if _, ok := rm.delvers[a.AccountID]; !ok {
		t.Fatal("OnLeave deleted the in-memory run")
	}
}
