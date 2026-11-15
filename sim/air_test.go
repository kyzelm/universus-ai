package sim

import "testing"

// jump puts player 0 in the air and returns the state on the first airborne
// frame.
func jump(t *testing.T, dir uint8) GameState {
	t.Helper()
	s := New()
	s.Advance([2]uint16{pad[dir], 0})
	for s.Players[0].State != StateAir {
		s.Advance([2]uint16{pad[dir], 0})
	}
	return s
}

// An air move's recovery is the fall.
func TestLandingEndsAnAirNormal(t *testing.T) {
	s := jump(t, 8)

	// Press it on the way down and close to the ground, so the move is still
	// running when the jump ends — pressed at the apex it would simply expire
	// in mid-air and prove nothing about landing.
	for s.Players[0].VY > 0 || s.Players[0].Y > FromInt(20) {
		s.Advance([2]uint16{0, 0})
	}
	s.Advance([2]uint16{InLP, 0})
	if s.Players[0].MoveIndex != 6 {
		t.Fatalf("the air normal did not come out: move %d", s.Players[0].MoveIndex)
	}

	for range 60 {
		s.Advance([2]uint16{0, 0})
		if s.Players[0].Y == GroundY {
			break
		}
	}
	p := &s.Players[0]
	if p.MoveIndex != -1 || p.State == StateAttack {
		t.Errorf("state %d move %d on the ground: the air normal outlived the jump", p.State, p.MoveIndex)
	}
	// The move is over, but the landing is not free: the air normal's own data
	// says what coming down out of it costs.
	if p.State != StateLanding || p.Stun != char().Moves[6].Landing {
		t.Errorf("state %d stun %d on the ground, want %d frames of landing recovery",
			p.State, p.Stun, char().Moves[6].Landing)
	}
	for range char().Moves[6].Landing {
		if Actionable(s.Players[0].State) {
			t.Fatal("actionable during landing recovery")
		}
		s.Advance([2]uint16{0, 0})
	}
	if got := s.Players[0].State; got != StateIdle {
		t.Errorf("state %d after the landing recovery ran out, want idle", got)
	}
}

// The quarter-circle is still on the stick when the character leaves the
// ground.
func TestAGroundSpecialCannotComeOutInTheAir(t *testing.T) {
	s := New()
	feed(&s, 2, 3, 6) // the motion, then jump before the button
	s.Advance([2]uint16{pad[8], 0})
	for s.Players[0].State != StateAir {
		s.Advance([2]uint16{0, 0})
	}

	s.Advance([2]uint16{InLP, 0})
	if got := s.Players[0].MoveIndex; got != 6 {
		t.Errorf("move %d came out in the air, want the air normal 6", got)
	}
	for i := range s.Projectiles {
		if s.Projectiles[i].Active != 0 {
			t.Fatal("a fireball came out of a jump")
		}
	}
}

// One press is one jump.
func TestHoldingUpJumpsOnce(t *testing.T) {
	s := New()

	// Up, held for long enough to cover the jump, the landing and a good while
	// after it.
	landings := 0
	airborne := false
	for range 120 {
		s.Advance([2]uint16{InUp, 0})
		if up := s.Players[0].Airborne(); up {
			airborne = true
		} else if airborne {
			airborne = false
			landings++
		}
	}
	if landings != 1 {
		t.Fatalf("%d jumps while up was held for 120 frames, want 1", landings)
	}

	// Letting go arms the next one, whatever the player was doing when they
	// let go: the release is read from the raw input, not from a state.
	s.Advance([2]uint16{0, 0})
	s.Advance([2]uint16{InUp, 0})
	for range 10 {
		if s.Players[0].Airborne() {
			return
		}
		s.Advance([2]uint16{InUp, 0})
	}
	t.Error("releasing up and pressing it again did not jump")
}
