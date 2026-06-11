package main

import (
	"math"
	"math/rand"
	"testing"

	kit "github.com/shellcade/kit/v2"
)

func testRng() *rand.Rand { return rand.New(rand.NewSource(1)) }

func TestLaunchVelDirection(t *testing.T) {
	tk := &tank{col: 10, y: 15, power: 60}

	tk.angle = 90 // straight up
	vx, vy := tk.launchVel()
	if math.Abs(vx) > 0.01 || vy >= 0 {
		t.Errorf("straight-up launch = (%.2f,%.2f), want vx~0 vy<0", vx, vy)
	}

	tk.angle = 45 // up-right
	vx, vy = tk.launchVel()
	if vx <= 0 || vy >= 0 {
		t.Errorf("up-right launch = (%.2f,%.2f), want vx>0 vy<0", vx, vy)
	}

	tk.angle = 135 // up-left
	vx, _ = tk.launchVel()
	if vx >= 0 {
		t.Errorf("up-left launch vx = %.2f, want < 0", vx)
	}
}

func TestPowerScalesSpeed(t *testing.T) {
	lo := &tank{angle: 90, power: 10}
	hi := &tank{angle: 90, power: 100}
	_, vlo := lo.launchVel()
	_, vhi := hi.launchVel()
	if math.Abs(vhi) <= math.Abs(vlo) {
		t.Errorf("more power should mean more speed: |%.2f| vs |%.2f|", vhi, vlo)
	}
}

func TestAdjustClamps(t *testing.T) {
	tk := &tank{angle: 90, power: 50}
	for i := 0; i < 200; i++ {
		tk.adjustAngle(+5)
		tk.adjustPower(+5)
	}
	if tk.angle > 179 || tk.power > 100 {
		t.Errorf("clamp failed high: angle=%.0f power=%.0f", tk.angle, tk.power)
	}
	for i := 0; i < 400; i++ {
		tk.adjustAngle(-5)
		tk.adjustPower(-5)
	}
	if tk.angle < 1 || tk.power < 5 {
		t.Errorf("clamp failed low: angle=%.0f power=%.0f", tk.angle, tk.power)
	}
}

func TestCycleWeaponSkipsEmpty(t *testing.T) {
	tk := newTank("x", kit.Player{}, false, "X", 10, 0)
	tk.ammo[0] = -1 // SHELL unlimited
	tk.ammo[1] = 0  // HEAVY spent
	tk.ammo[2] = -1 // TRACER unlimited
	tk.weapon = 0
	tk.cycleWeapon()
	if tk.weapon == 1 {
		t.Error("cycle landed on the spent HEAVY")
	}
}

func TestIntegrateGravityPullsDown(t *testing.T) {
	_, _, _, vy := integrate(10, 5, 3, 0, 0, 0.1)
	if vy <= 0 {
		t.Errorf("gravity should add downward velocity, got vy=%.2f", vy)
	}
}

func TestIntegrateWindPushesSideways(t *testing.T) {
	_, _, vx, _ := integrate(10, 5, 0, 0, 8, 0.1)
	if vx <= 0 {
		t.Errorf("a rightward wind should add +vx, got %.2f", vx)
	}
}
