package main

import (
	"testing"

	kit "github.com/shellcade/kit/v2"
	"github.com/shellcade/kit/v2/kittest"
)

// stubCard is a Card whose outcome is fixed, so credits-path tests don't have
// to fish a rare jackpot out of the seeded RNG. The ticket's outcome is drawn
// at purchase anyway; here we just pin what Win() reports.
type stubCard struct{ win int }

func (c *stubCard) Title() string            { return "STUB" }
func (c *stubCard) Prompt() string           { return "" }
func (c *stubCard) Move(dx, dy int)          {}
func (c *stubCard) Scratch() bool            { return true }
func (c *stubCard) ScratchAll()              {}
func (c *stubCard) Resolved() bool           { return true }
func (c *stubCard) Win() int                 { return c.win }
func (c *stubCard) Render(f *Frame, top int) {}

// newTestRoom spins up a one-player casino room wired to the kittest credits
// double, with the payout ceiling mirroring the declared MaxPayoutMultiplier.
func newTestRoom(t *testing.T) (*room, *kittest.Room, kit.Player) {
	t.Helper()
	r := kittest.NewRoom(kittest.Player("p1"))
	r.CreditsSeed = 1000
	r.CreditsMaxPayoutMultiplier = maxPayoutMultiplier
	h := Game{}.NewRoom(r.Config(), r.Services())
	rm := h.(*room)
	rm.OnStart(r)
	rm.OnJoin(r, r.Players[0])
	return rm, r, r.Players[0]
}

// standAndIndex points a patron's cursor at the ticket with the given slug and
// returns its ticket pointer, so buy() computes the right stake.
func aimAt(pt *patron, slug string) *Ticket {
	for si, price := range standPrices {
		list := ticketsAtPrice(price)
		for ti, tk := range list {
			if tk.Slug == slug {
				pt.standIdx, pt.ticketIdx = si, ti
				return tk
			}
		}
	}
	return nil
}

// TestWagerEscrowsAndSettleWins checks the full happy path: buy escrows exactly
// the ticket price, and a winning Settle credits the gross onto the balance.
func TestWagerEscrowsAndSettleWins(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]

	tk := aimAt(pt, "lucky-7s") // $1 ticket
	rm.buy(r, pt, tk)
	if got := r.Credits[p.AccountID]; got != 999 {
		t.Fatalf("after $1 wager balance = %d, want 999", got)
	}
	if r.CreditsStakes[p.AccountID] != 1 {
		t.Fatalf("open stake = %d, want 1", r.CreditsStakes[p.AccountID])
	}
	if !pt.staked {
		t.Fatal("patron.staked should be true after a wager")
	}

	pt.card = &stubCard{win: 5} // a $5 gross on the $1 ticket
	rm.maybeSettle(r, pt)
	if got := r.Credits[p.AccountID]; got != 1004 {
		t.Fatalf("after settle balance = %d, want 1004 (999+5)", got)
	}
	if len(r.CreditsStakes) != 0 {
		t.Fatalf("stake should be cleared after settle, got %v", r.CreditsStakes)
	}
	if pt.staked {
		t.Fatal("patron.staked should be false after settle")
	}
	if pt.state != stateResult {
		t.Fatalf("state = %d, want stateResult", pt.state)
	}
}

// TestSettleLossBooksZero checks a losing card settles 0 (stake forfeited).
func TestSettleLossBooksZero(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]
	tk := aimAt(pt, "gold-rush") // $2
	rm.buy(r, pt, tk)
	pt.card = &stubCard{win: 0}
	rm.maybeSettle(r, pt)
	if got := r.Credits[p.AccountID]; got != 998 {
		t.Fatalf("loss balance = %d, want 998", got)
	}
	if len(r.CreditsStakes) != 0 {
		t.Fatalf("stake not cleared on loss: %v", r.CreditsStakes)
	}
}

// TestTopPrizeNotClamped is the guardrail: Cash Explosion's headline 300,000 on
// a $10 ticket is exactly 30,000x — the declared ceiling — so it must settle in
// full, un-clamped.
func TestTopPrizeNotClamped(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]
	tk := aimAt(pt, "cash-explosion") // $10, top 300,000
	if top := topPrize(tk); top != 300000 {
		t.Fatalf("cash-explosion top prize = %d, want 300000", top)
	}
	rm.buy(r, pt, tk)
	pt.card = &stubCard{win: 300000}
	rm.maybeSettle(r, pt)
	if pt.lastWin != 300000 {
		t.Fatalf("lastWin = %d, want 300000 (unclamped)", pt.lastWin)
	}
	if got := r.Credits[p.AccountID]; got != 300990 {
		t.Fatalf("balance = %d, want 300990 (990 + 300000)", got)
	}
}

// TestOverCeilingClamped is the defensive companion: a payout above stake x
// multiplier is clamped to the ceiling before Settle, so the UI never shows a
// number the host won't pay.
func TestOverCeilingClamped(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]
	tk := aimAt(pt, "cash-explosion") // $10 -> ceiling 300,000
	rm.buy(r, pt, tk)
	pt.card = &stubCard{win: 400000} // impossible, but proves the clamp
	rm.maybeSettle(r, pt)
	if pt.lastWin != 300000 {
		t.Fatalf("clamped lastWin = %d, want 300000", pt.lastWin)
	}
	if got := r.Credits[p.AccountID]; got != 300990 {
		t.Fatalf("balance = %d, want 300990 (clamped)", got)
	}
}

// TestAbandonMidCardSettles is the escrow-leak guard: hitting q on an
// unresolved card after a Wager must Settle(0), not silently drop the stake.
func TestAbandonMidCardSettles(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]

	// Drive it like a real player: Enter to the counter's $1 stand, Enter to buy.
	rm.OnInput(r, p, kit.Input{Kind: kit.InputKey, Key: kit.KeyEnter}) // stand
	rm.OnInput(r, p, kit.Input{Kind: kit.InputKey, Key: kit.KeyEnter}) // buy
	if pt.state != stateCard || !pt.staked {
		t.Fatalf("expected a staked card, state=%d staked=%v", pt.state, pt.staked)
	}
	if r.Credits[p.AccountID] != 999 {
		t.Fatalf("post-buy balance = %d, want 999", r.Credits[p.AccountID])
	}
	// Walk away with q while the card is still latex.
	rm.OnInput(r, p, kit.Input{Kind: kit.InputRune, Rune: 'q'})
	if len(r.CreditsStakes) != 0 {
		t.Fatalf("abandon leaked escrow: stakes=%v", r.CreditsStakes)
	}
	if r.Credits[p.AccountID] != 999 {
		t.Fatalf("abandon balance = %d, want 999 (bet booked as loss)", r.Credits[p.AccountID])
	}
	if pt.staked {
		t.Fatal("staked should be false after abandon")
	}
}

// TestLeaveMidCardSettles covers the same leak on a hard disconnect.
func TestLeaveMidCardSettles(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]
	tk := aimAt(pt, "lucky-7s")
	rm.buy(r, pt, tk)
	if r.Credits[p.AccountID] != 999 {
		t.Fatalf("post-buy balance = %d", r.Credits[p.AccountID])
	}
	rm.OnLeave(r, p)
	if len(r.CreditsStakes) != 0 {
		t.Fatalf("leave leaked escrow: stakes=%v", r.CreditsStakes)
	}
	if r.Credits[p.AccountID] != 999 {
		t.Fatalf("leave balance = %d, want 999", r.Credits[p.AccountID])
	}
}

// TestRebuyWhenBroke checks that trying to buy with no money triggers Buyback
// and enters the bust screen only on success.
func TestRebuyWhenBroke(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("p1"))
	r.Credits = map[string]int64{"p1": 0} // broke: below the buyback floor
	r.CreditsMaxPayoutMultiplier = maxPayoutMultiplier
	r.CreditsBuybackFloor = 100
	r.CreditsBuybackAmount = 1000
	h := Game{}.NewRoom(r.Config(), r.Services())
	rm := h.(*room)
	rm.OnStart(r)
	rm.OnJoin(r, r.Players[0])
	p := r.Players[0]
	pt := rm.patrons[p.AccountID]

	tk := aimAt(pt, "lucky-7s")
	rm.buy(r, pt, tk) // can't afford -> rebuy path
	if pt.state != stateBust {
		t.Fatalf("state = %d, want stateBust after successful rebuy", pt.state)
	}
	if r.Credits[p.AccountID] != 1000 {
		t.Fatalf("rebuy balance = %d, want 1000", r.Credits[p.AccountID])
	}
	if r.CreditsRebuys[p.AccountID] != 1 {
		t.Fatalf("rebuy count = %d, want 1", r.CreditsRebuys[p.AccountID])
	}
	if pt.staked {
		t.Fatal("a refused buy must not leave a stake open")
	}
}

// TestLeaderboardPostsPeak checks a new balance high is posted to the board.
func TestLeaderboardPostsPeak(t *testing.T) {
	rm, r, p := newTestRoom(t)
	pt := rm.patrons[p.AccountID]
	tk := aimAt(pt, "lucky-7s")
	rm.buy(r, pt, tk)
	pt.card = &stubCard{win: 500} // a real high over the 1000 seed
	rm.maybeSettle(r, pt)
	if len(r.Posted) == 0 {
		t.Fatal("expected a leaderboard post on a new peak")
	}
	last := r.Posted[len(r.Posted)-1].Rankings[0]
	if last.Metric != 1499 { // 999 + 500
		t.Fatalf("posted metric = %d, want 1499", last.Metric)
	}
}

// TestEconomyDisabledRendersOOS checks the game degrades to out-of-service
// rather than trapping when the host has no economy.
func TestEconomyDisabledRendersOOS(t *testing.T) {
	r := kittest.NewRoom(kittest.Player("p1"))
	r.CreditsDisabled = true
	h := Game{}.NewRoom(r.Config(), r.Services())
	rm := h.(*room)
	rm.OnStart(r)
	rm.OnJoin(r, r.Players[0])
	pt := rm.patrons[r.Players[0].AccountID]
	if !pt.oos {
		t.Fatal("expected out-of-service when the economy is disabled")
	}
	// A buy attempt must not panic and must stay out-of-service.
	tk := aimAt(pt, "lucky-7s")
	rm.buy(r, pt, tk)
	if pt.staked {
		t.Fatal("no stake should open with the economy off")
	}
}
