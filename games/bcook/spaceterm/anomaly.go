package main

import (
	kit "github.com/shellcade/kit/v2"
	"math/rand"
	"time"
)

// anomalyKind is a sector hazard. Each maps a gesture from the mobile party
// game onto something a terminal can demand: mashing, memory, mirrored eyes.
type anomalyKind uint8

const (
	anNone     anomalyKind = iota
	anMeteor               // orders pause; everyone mashes their assigned key
	anFlare                // control labels render as static; orders keep flowing
	anWormhole             // the panel draws mirrored; hotkeys stay bound
	anLeak                 // random controls fog over until wiped (3 presses)
)

type anStage uint8

const (
	asNone anStage = iota
	asWarn         // the 3s INBOUND banner
	asLive         // the effect is on
)

const (
	anWarnDur  = 3 * time.Second
	meteorDur  = 4 * time.Second
	flareDur   = 6 * time.Second
	wormDur    = 6 * time.Second
	leakBanner = 2500 * time.Millisecond
	mashNeed   = 12 // presses to ride out a meteor storm
	meteorCap  = 2  // max hull lost to one storm, however many crew fumble it
	fogPerCrew = 2  // controls fogged per panel by a coolant leak
)

var anomalyNames = []string{"", "METEOR STORM", "SOLAR FLARE", "WORMHOLE TRANSIT", "COOLANT LEAK"}

func (rm *room) meteorActive() bool { return rm.anKind == anMeteor && rm.anStage == asLive }
func (rm *room) flareActive() bool  { return rm.anKind == anFlare && rm.anStage == asLive }
func (rm *room) wormActive() bool   { return rm.anKind == anWormhole && rm.anStage == asLive }

// scheduleAnomalies precomputes this sector's trigger points: charge counts
// between 30% and 70% of the warp bar. Sector 1 is calm; 5+ throws two.
func (rm *room) scheduleAnomalies(rng *rand.Rand) {
	rm.schedule = rm.schedule[:0]
	count := 0
	switch {
	case rm.sector >= 5:
		count = 2
	case rm.sector >= 2:
		count = 1
	}
	prev := 0
	for i := 0; i < count; i++ {
		at := int(float64(rm.need) * (0.30 + rng.Float64()*0.40))
		if at <= prev {
			at = prev + 1
		}
		rm.schedule = append(rm.schedule, at)
		prev = at
	}
}

// stepAnomaly runs the hazard state machine inside the sector heartbeat.
func (rm *room) stepAnomaly(r kit.Room) {
	switch rm.anStage {
	case asNone:
		if len(rm.schedule) > 0 && rm.charges >= rm.schedule[0] {
			rm.schedule = rm.schedule[1:]
			rm.beginWarn(r)
		}
	case asWarn:
		if rm.now.After(rm.anWarnAt) {
			rm.activateAnomaly(r)
		}
	case asLive:
		if rm.now.After(rm.anEndAt) {
			rm.finishAnomaly(r)
		}
	}
}

func (rm *room) beginWarn(r kit.Room) {
	rng := r.Rand()
	// Mirroring and fog are noise without comms pressure — solo gets the
	// anomalies that still play well alone.
	pool := []anomalyKind{anMeteor, anFlare, anWormhole, anLeak}
	if rm.boardedCount() < 2 {
		pool = pool[:2]
	}
	k := pool[rng.Intn(len(pool))]
	for k == rm.lastAn && len(pool) > 1 {
		k = pool[rng.Intn(len(pool))]
	}
	rm.anKind = k
	rm.anStage = asWarn
	rm.anWarnAt = rm.now.Add(anWarnDur)
}

func (rm *room) activateAnomaly(r kit.Room) {
	rng := r.Rand()
	rm.anStage = asLive
	switch rm.anKind {
	case anMeteor:
		rm.anEndAt = rm.now.Add(meteorDur)
		for _, c := range rm.crews {
			if !c.boarded {
				continue
			}
			// bank each order's remaining time; the storm doesn't eat it
			if c.ord.active {
				c.ord.paused = c.ord.expires.Sub(rm.now)
			}
			c.mashKey = c.panel[rng.Intn(len(c.panel))].key
			c.mashN = 0
		}
	case anFlare:
		rm.anEndAt = rm.now.Add(flareDur)
	case anWormhole:
		rm.anEndAt = rm.now.Add(wormDur)
	case anLeak:
		rm.anEndAt = rm.now.Add(leakBanner)
		for _, c := range rm.crews {
			if !c.boarded {
				continue
			}
			for n := 0; n < fogPerCrew; n++ {
				c.panel[rng.Intn(len(c.panel))].fog = fogWipes
			}
		}
	}
}

func (rm *room) finishAnomaly(r kit.Room) {
	kind := rm.anKind
	rm.lastAn = kind
	rm.anKind, rm.anStage = anNone, asNone
	if kind != anMeteor {
		return
	}
	// Resume the banked order clocks, then settle the storm's bill.
	missed := 0
	for _, c := range rm.crews {
		if !c.boarded {
			continue
		}
		if c.ord.active {
			c.ord.expires = rm.now.Add(c.ord.paused)
			c.ord.paused = 0
		}
		if c.mashN < mashNeed {
			missed++
		}
	}
	if missed > meteorCap {
		missed = meteorCap
	}
	if missed > 0 {
		rm.loseHull(r, missed)
	}
}
