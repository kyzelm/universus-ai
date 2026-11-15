package sim

import "testing"

// facing puts the two players at a distance where move 0 reaches, both on the
// ground, facing each other.
func facing(gap int) GameState {
	s := New()
	s.Players[0].X = FromInt(-gap / 2)
	s.Players[1].X = FromInt(gap / 2)
	return s
}

func TestBoxMirrorsWithFacing(t *testing.T) {
	b := Box{X: FromInt(10), Y: FromInt(5), W: FromInt(20), H: FromInt(8)}

	r := b.World(0, 0, 1)
	if r.X != FromInt(10) || r.W != FromInt(20) {
		t.Errorf("facing right: %+v", r)
	}
	// Facing left, the same local box occupies the mirrored span.
	l := b.World(0, 0, -1)
	if l.X != FromInt(-30) || l.W != FromInt(20) {
		t.Errorf("facing left: %+v", l)
	}
	// Y is unaffected by facing.
	if l.Y != r.Y || l.H != r.H {
		t.Errorf("facing changed the vertical extent: %+v vs %+v", l, r)
	}
}

func TestBoxOverlapEdges(t *testing.T) {
	b := Box{X: 0, Y: 0, W: FromInt(10), H: FromInt(10)}

	for _, tc := range []struct {
		name string
		o    Box
		want bool
	}{
		{"identical", b, true},
		{"contained", Box{X: FromInt(2), Y: FromInt(2), W: FromInt(2), H: FromInt(2)}, true},
		{"overlapping corner", Box{X: FromInt(9), Y: FromInt(9), W: FromInt(5), H: FromInt(5)}, true},
		// Touching edges are a miss: a hit must not depend on the last bit of
		// a position, and "exactly adjacent" is the case that would.
		{"touching right edge", Box{X: FromInt(10), Y: 0, W: FromInt(5), H: FromInt(5)}, false},
		{"touching top edge", Box{X: 0, Y: FromInt(10), W: FromInt(5), H: FromInt(5)}, false},
		{"one unit apart", Box{X: FromInt(11), Y: 0, W: FromInt(5), H: FromInt(5)}, false},
		{"zero width", Box{X: 0, Y: 0, W: 0, H: FromInt(5)}, false},
		{"negative height", Box{X: 0, Y: 0, W: FromInt(5), H: -One}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.Overlaps(tc.o); got != tc.want {
				t.Errorf("Overlaps = %v, want %v", got, tc.want)
			}
			if got := tc.o.Overlaps(b); got != tc.want {
				t.Errorf("Overlaps is not symmetric: reverse = %v, want %v", got, tc.want)
			}
		})
	}
}

// A hitbox exists only during the active window, and a hurtbox almost always.
func TestBoxesFollowTheFrameData(t *testing.T) {
	s := New()
	mv := &char().Moves[0]

	var boxes [MaxBoxes]Box
	s.Advance([2]uint16{InLP, 0})

	for f := int32(0); f < mv.Total(); f++ {
		state := s.Players[0]
		if state.State != StateAttack {
			t.Fatalf("frame %d: left the attack early, state = %d", f, state.State)
		}
		active := state.StateFrame >= mv.Startup && state.StateFrame < mv.Startup+mv.Active

		n := s.Hitboxes(0, &boxes)
		if active && n == 0 {
			t.Errorf("move frame %d is active but has no hitbox", state.StateFrame)
		}
		if !active && n != 0 {
			t.Errorf("move frame %d is not active but has %d hitbox(es)", state.StateFrame, n)
		}
		if s.Hurtboxes(0, &boxes) == 0 {
			t.Errorf("move frame %d has no hurtbox", state.StateFrame)
		}
		s.Advance([2]uint16{0, 0})
	}
}

func TestAttackConnectsAndDealsDamage(t *testing.T) {
	s := facing(30)
	mv := &char().Moves[0]
	full := s.Players[1].Health

	// Press, then hold nothing; the move runs to completion on its own.
	s.Advance([2]uint16{InLP, 0})
	for range mv.Total() + int32(mv.Hitstop) + 2 {
		s.Advance([2]uint16{0, 0})
	}

	// The jab is a light, so its 100 base arrives as 80 — the starter scaling
	// applies to the opening hit of a combo as much as to the rest of it.
	if got := s.Players[1].Health; got != full-jabFirstHit {
		t.Errorf("health = %d, want %d after the jab", got, full-jabFirstHit)
	}
	if s.Players[1].State != StateHitstun && s.Players[1].Stun == 0 {
		t.Errorf("defender is not in hitstun: state = %d stun = %d", s.Players[1].State, s.Players[1].Stun)
	}
}

// One active window, one hit.
func TestOneActiveWindowHitsOnce(t *testing.T) {
	s := facing(30)
	full := s.Players[1].Health

	s.Advance([2]uint16{InLP, 0})
	for range 60 {
		s.Advance([2]uint16{0, 0})
	}

	if got := full - s.Players[1].Health; got != jabFirstHit {
		t.Errorf("total damage %d, want %d — the active window hit more than once", got, jabFirstHit)
	}
}

// Standing blocks mids and highs; a low must be blocked crouching.
func TestBlockLevels(t *testing.T) {
	low := &char().Moves[1] // crouching low, must be blocked crouching

	for _, tc := range []struct {
		name    string
		defend  uint16
		blocked bool
	}{
		{"standing against a low", InRight, false},
		{"crouching against a low", InRight | InDown, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := facing(30)
			full := s.Players[1].Health

			// Player 0 crouches to get the low out.
			s.Advance([2]uint16{InDown | InLK, tc.defend})
			for range low.Total() + int32(low.Hitstop) + 2 {
				s.Advance([2]uint16{0, tc.defend})
			}

			hurt := s.Players[1].Health < full
			if tc.blocked && hurt {
				t.Errorf("the low was not blocked: health %d -> %d", full, s.Players[1].Health)
			}
			if !tc.blocked && !hurt {
				t.Error("the low was blocked by a standing defender")
			}
		})
	}
}

// Hitstop freezes both fighters.
func TestHitstopFreezesBothFighters(t *testing.T) {
	s := facing(30)
	mv := &char().Moves[0]

	for range 60 {
		s.Advance([2]uint16{InLP, 0})
		if s.Hitstop > 0 {
			break
		}
	}
	if s.Hitstop == 0 {
		t.Fatal("no hitstop after a connect")
	}
	if s.Hitstop != mv.Hitstop {
		t.Errorf("hitstop = %d, want the move's %d", s.Hitstop, mv.Hitstop)
	}

	before := s
	for s.Hitstop > 0 {
		s.Advance([2]uint16{InRight, InLeft}) // both mashing: nothing may move
	}

	for i := range s.Players {
		if s.Players[i].X != before.Players[i].X || s.Players[i].StateFrame != before.Players[i].StateFrame {
			t.Errorf("player %d moved during hitstop: %+v -> %+v", i, before.Players[i], s.Players[i])
		}
	}
	// The frame counter still runs, or the network and the checksum stall.
	if s.Frame == before.Frame {
		t.Error("the frame counter stopped during hitstop")
	}
}

// A trade is both attacks landing on the same frame.
func TestSimultaneousHitsTrade(t *testing.T) {
	s := facing(30)
	mv := &char().Moves[0]
	full := [2]int32{s.Players[0].Health, s.Players[1].Health}

	// Both press on the same frame, so both active windows line up.
	s.Advance([2]uint16{InLP, InLP})
	for range mv.Total() + int32(mv.Hitstop) + 2 {
		s.Advance([2]uint16{0, 0})
	}

	// Both were attacking when they were hit, so both hits are counter hits:
	// the jab's 80 at the fixture's 150%.
	const traded = jabFirstHit * 150 / 100
	for i := range s.Players {
		if got := full[i] - s.Players[i].Health; got != traded {
			t.Errorf("player %d took %d in the trade, want %d — both should be hit", i, got, traded)
		}
	}
}

// Detection is separated from resolution so both directions see the same
// positions.
func TestTradeDoesNotDependOnPlayerOrder(t *testing.T) {
	mirror := func(s GameState) GameState {
		s.Players[0], s.Players[1] = s.Players[1], s.Players[0]
		return s
	}

	a := facing(30)
	b := mirror(facing(30))

	for range 40 {
		a.Advance([2]uint16{InLP, InLP})
		b.Advance([2]uint16{InLP, InLP})
	}

	if a.Players[0].Health != b.Players[1].Health || a.Players[1].Health != b.Players[0].Health {
		t.Errorf("swapping the players changed the outcome: %d/%d vs %d/%d",
			a.Players[0].Health, a.Players[1].Health, b.Players[0].Health, b.Players[1].Health)
	}
}

// Invincible frames are the absence of hurtboxes, and Hurtboxes is the one
// funnel every attack and every projectile asks.
func TestInvulnerableFramesHaveNoHurtboxes(t *testing.T) {
	s := New()
	mv := &char().Moves[7]
	s.Advance([2]uint16{InMP, 0})

	var boxes [MaxBoxes]Box
	for range mv.Total() {
		p := &s.Players[0]
		n := s.Hurtboxes(0, &boxes)
		if inside := p.StateFrame < mv.InvulnEnd; inside != (n == 0) {
			t.Fatalf("frame %d of the move: %d hurtboxes, invulnerable window is [%d, %d)",
				p.StateFrame, n, mv.InvulnStart, mv.InvulnEnd)
		}
		s.Advance([2]uint16{0, 0})
	}
}
