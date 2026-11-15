package sim

import "testing"

// Move-driven velocity.

// The arc is not authored.
func TestALaunchingMoveRisesAndLands(t *testing.T) {
	s := New()
	s.Advance([2]uint16{InHP, 0})
	if s.Players[0].MoveIndex != 3 {
		t.Fatalf("the launcher did not come out: move %d", s.Players[0].MoveIndex)
	}

	mv := &char().Moves[3]
	peak, airborne := Fix(0), 0

	for range mv.Total() {
		s.Advance([2]uint16{0, 0})
		p := &s.Players[0]
		if p.Y > peak {
			peak = p.Y
		}
		if p.Y > GroundY {
			airborne++
		}
	}

	if peak <= 0 {
		t.Fatal("never left the ground")
	}
	if airborne < 10 {
		t.Errorf("airborne for %d frames, want an actual arc", airborne)
	}
	if got := s.Players[0].Y; got != GroundY {
		t.Errorf("ended at Y = %d, want back on the ground", got)
	}
	if s.Players[0].State == StateAttack {
		t.Error("the move outlasted its own recovery")
	}
}

// A move that runs out while the character is still in the air hands over to
// StateAir.
func TestAMoveEndingInTheAirFallsOutIntoAJump(t *testing.T) {
	s := New()
	s.Advance([2]uint16{InHK, 0}) // total 6 frames, ~32 frames of airtime

	for range char().Moves[4].Total() {
		s.Advance([2]uint16{0, 0})
	}

	p := &s.Players[0]
	if p.Y <= GroundY {
		t.Fatalf("setup: back on the ground at Y = %d before the move ended", p.Y)
	}
	if p.State != StateAir {
		t.Fatalf("state %d after the move ended in the air, want StateAir", p.State)
	}

	// Being in StateAir rather than idle is what keeps the ground options off:
	// no second jump, no walking, no dash.
	s.Advance([2]uint16{pad[8], 0})
	if got := s.Players[0].State; got != StateAir {
		t.Errorf("holding up in mid-air put the player in state %d, want a second jump to be impossible", got)
	}

	for range 60 {
		s.Advance([2]uint16{0, 0})
		if s.Players[0].State == StateIdle {
			return
		}
	}
	t.Error("never landed")
}
