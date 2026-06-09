package main

import "testing"

// countByKind tallies the master list per family.
func countByKind() map[betKind]int {
	m := map[betKind]int{}
	for _, b := range masterBets {
		m[b.kind]++
	}
	return m
}

func TestBetCounts(t *testing.T) {
	want := map[betKind]int{
		// 38 straights (0, 00, 1..36); 58 splits (57 in-grid + 0-00); two trios;
		// one five-number top line; everything else as on a European felt.
		kStraight: 38, kSplit: 58, kStreet: 12, kTrio: 2, kCorner: 22,
		kTopLine: 1, kLine: 11, kDozen: 3, kColumn: 3,
		kRed: 1, kBlack: 1, kOdd: 1, kEven: 1, kLow: 1, kHigh: 1,
	}
	got := countByKind()
	for k, n := range want {
		if got[k] != n {
			t.Errorf("kind %s: got %d bets, want %d", k.name(), got[k], n)
		}
	}
	if len(masterBets) != 156 {
		t.Errorf("total bets = %d, want 156", len(masterBets))
	}
}

func TestPayouts(t *testing.T) {
	cases := map[betKind]int{
		kStraight: 35, kSplit: 17, kStreet: 11, kTrio: 11, kCorner: 8,
		kTopLine: 6, kLine: 5, kDozen: 2, kColumn: 2, kRed: 1, kHigh: 1,
	}
	for k, p := range cases {
		if k.payout() != p {
			t.Errorf("%s payout = %d, want %d", k.name(), k.payout(), p)
		}
	}
}

// TestBetIntegrity checks each bet's invariants: ascending nums, the right size
// for its family, and a correctly computed anchor.
func TestBetIntegrity(t *testing.T) {
	size := map[betKind]int{
		kStraight: 1, kSplit: 2, kStreet: 3, kTrio: 3, kCorner: 4, kTopLine: 5,
		kLine: 6, kDozen: 12, kColumn: 12, kRed: 18, kBlack: 18, kOdd: 18,
		kEven: 18, kLow: 18, kHigh: 18,
	}
	for _, b := range masterBets {
		if len(b.nums) != size[b.kind] {
			t.Errorf("%s %q has %d numbers, want %d", b.kind.name(), b.label, len(b.nums), size[b.kind])
		}
		min := b.nums[0]
		for i := 1; i < len(b.nums); i++ {
			if b.nums[i] <= b.nums[i-1] {
				t.Errorf("%s %q numbers not ascending: %v", b.kind.name(), b.label, b.nums)
			}
			if b.nums[i] < min {
				min = b.nums[i]
			}
		}
		if b.outside {
			if b.anchor != -1 {
				t.Errorf("outside %q has anchor %d, want -1", b.label, b.anchor)
			}
		} else if b.anchor != min {
			t.Errorf("inside %q anchor = %d, want %d", b.label, b.anchor, min)
		}
		for _, n := range b.nums {
			if n < 0 || (n > 36 && n != doubleZero) {
				t.Errorf("%s %q has out-of-range pocket %d", b.kind.name(), b.label, n)
			}
		}
	}
}

// TestNoDuplicateBets ensures the generation never emits the same covered set
// for the same family twice.
func TestNoDuplicateBets(t *testing.T) {
	seen := map[string]string{}
	for _, b := range masterBets {
		key := b.kind.name() + ":"
		for _, n := range b.nums {
			key += itoa(n) + ","
		}
		if prev, ok := seen[key]; ok {
			t.Errorf("duplicate bet %q and %q (key %s)", prev, b.label, key)
		}
		seen[key] = b.label
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestColorPartition verifies the red/black/green split.
func TestColorPartition(t *testing.T) {
	if colorOf(0) != green || colorOf(doubleZero) != green {
		t.Error("0 and 00 should both be green")
	}
	var reds, blacks int
	for n := 1; n <= 36; n++ {
		switch colorOf(n) {
		case red:
			reds++
		case black:
			blacks++
		default:
			t.Errorf("%d classified green", n)
		}
	}
	if reds != 18 || blacks != 18 {
		t.Errorf("reds=%d blacks=%d, want 18/18", reds, blacks)
	}
}

// TestOutsidePartitions checks the even-money/dozen/column groups partition the
// numbers 1..36 the way a real felt does.
func TestOutsidePartitions(t *testing.T) {
	get := func(k betKind) bet {
		for _, b := range masterBets {
			if b.kind == k {
				return b
			}
		}
		t.Fatalf("no bet of kind %s", k.name())
		return bet{}
	}
	// red ∪ black == {1..36}, disjoint.
	if !partitions(t, get(kRed).nums, get(kBlack).nums) {
		t.Error("red/black do not partition 1..36")
	}
	if !partitions(t, get(kOdd).nums, get(kEven).nums) {
		t.Error("odd/even do not partition 1..36")
	}
	if !partitions(t, get(kLow).nums, get(kHigh).nums) {
		t.Error("low/high do not partition 1..36")
	}
}

// partitions reports whether the slices are disjoint and together equal {1..36}.
func partitions(t *testing.T, a, b []int) bool {
	t.Helper()
	seen := map[int]int{}
	for _, n := range a {
		seen[n]++
	}
	for _, n := range b {
		seen[n]++
	}
	if len(seen) != 36 {
		return false
	}
	for n := 1; n <= 36; n++ {
		if seen[n] != 1 {
			return false
		}
	}
	return true
}

// TestSettlementMath checks the win/lose return for representative bets across
// every pocket.
func TestSettlementMath(t *testing.T) {
	straight17 := findBet(t, kStraight, "17")
	split23 := findBet(t, kSplit, "2-3")
	red := findBet(t, kRed, "RED")

	const stake = 100
	for result := 0; result <= doubleZero; result++ { // 0, 1..36, and 00 (=37)
		// Straight on 17 pays 35:1 only on 17.
		ret := settleReturn(straight17, stake, result)
		if result == 17 {
			if ret != stake*36 {
				t.Errorf("straight 17 on %d returned %d, want %d", result, ret, stake*36)
			}
		} else if ret != 0 {
			t.Errorf("straight 17 on %d returned %d, want 0", result, ret)
		}

		// Split 2-3 pays 17:1 on a 2 or a 3.
		ret = settleReturn(split23, stake, result)
		if result == 2 || result == 3 {
			if ret != stake*18 {
				t.Errorf("split 2-3 on %d returned %d, want %d", result, ret, stake*18)
			}
		} else if ret != 0 {
			t.Errorf("split 2-3 on %d returned %d, want 0", result, ret)
		}

		// RED pays 1:1 on a red number and loses on 0.
		ret = settleReturn(red, stake, result)
		if colorOf(result) == redCol() {
			if ret != stake*2 {
				t.Errorf("RED on %d returned %d, want %d", result, ret, stake*2)
			}
		} else if ret != 0 {
			t.Errorf("RED on %d returned %d, want 0", result, ret)
		}
	}
}

func redCol() color { return red }

func findBet(t *testing.T, k betKind, label string) bet {
	t.Helper()
	for _, b := range masterBets {
		if b.kind == k && b.label == label {
			return b
		}
	}
	t.Fatalf("no %s bet labelled %q", k.name(), label)
	return bet{}
}

// TestWheelSequence checks the physical strip is a permutation of all 38 pockets
// (0, 00, 1..36) and that wheelIndex round-trips.
func TestWheelSequence(t *testing.T) {
	seen := map[int]bool{}
	for _, n := range wheelSeq {
		if n < 0 || (n > 36 && n != doubleZero) {
			t.Fatalf("wheel pocket %d out of range", n)
		}
		if seen[n] {
			t.Fatalf("wheel pocket %d repeated", n)
		}
		seen[n] = true
	}
	if len(seen) != pockets {
		t.Fatalf("wheel has %d distinct pockets, want %d", len(seen), pockets)
	}
	for _, n := range wheelSeq {
		if wheelSeq[wheelIndex(n)] != n {
			t.Errorf("wheelIndex(%d) does not round-trip", n)
		}
	}
}
