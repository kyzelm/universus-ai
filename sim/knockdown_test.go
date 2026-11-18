package sim

import "testing"

// The fixture's sweep and the input that selects it.
const (
	sweepIndex = int32(11)
	sweepInput = InDown | InHK
)

// sweep lands the fixture's knockdown strike on player 1 and returns the state on the frame it
// connected.
func sweep(t *testing.T) GameState {
	t.Helper()

	s := facing(30)
	s.Advance([2]uint16{sweepInput, 0})
	if s.Players[0].MoveIndex != sweepIndex {
		t.Fatalf("crouching HK gave move %d, want the sweep %d", s.Players[0].MoveIndex, sweepIndex)
	}

	for range char().Moves[sweepIndex].Total() {
		if s.Players[1].State == StateHitstun {
			return s
		}
		s.Advance([2]uint16{sweepInput, 0})
	}
	t.Fatal("the sweep never connected")
	return s
}

// knockedDown runs the sweep through its hitstun and returns the state on the first frame player 1
// is on the floor.
func knockedDown(t *testing.T) GameState {
	t.Helper()

	s := sweep(t)
	for range 200 {
		s.Advance([2]uint16{0, 0})
		if s.Players[1].State != StateHitstun {
			break
		}
	}
	if got := s.Players[1].State; got != StateKnockdown {
		t.Fatalf("player 1 is in state %d after the sweep's hitstun, want the knockdown", got)
	}
	return s
}

// The knockdown is owed at the *end* of the hitstun, not instead of it.
func TestASweepKnocksDownWhenItsHitstunEnds(t *testing.T) {
	s := sweep(t)
	if s.Players[1].State != StateHitstun {
		t.Fatalf("player 1 is in state %d on the connect, want hitstun", s.Players[1].State)
	}
	if s.Players[1].Down == 0 {
		t.Fatal("the sweep connected but owes no knockdown")
	}

	s = knockedDown(t)
	if got := s.Players[1].Stun; got != balance.KnockdownFrames {
		t.Errorf("knocked down for %d frames, want the balance value %d", got, balance.KnockdownFrames)
	}
}

// Invulnerable for the whole knockdown, and actionable on the frame the invulnerability ends.
func TestVulnerableAndActionableBeginTogether(t *testing.T) {
	s := knockedDown(t)

	var boxes [MaxBoxes]Box
	for range balance.KnockdownFrames + 10 {
		if s.Players[1].State != StateKnockdown {
			break
		}
		if n := s.Hurtboxes(1, &boxes); n != 0 {
			t.Fatalf("frame %d of the knockdown has %d hurtboxes, want none", s.Players[1].StateFrame, n)
		}
		if s.throwable(1) {
			t.Fatalf("frame %d of the knockdown is throwable", s.Players[1].StateFrame)
		}
		s.Advance([2]uint16{0, 0})
	}

	if got := s.Players[1].State; !Actionable(got) {
		t.Fatalf("player 1 is in state %d off the floor, want an actionable one", got)
	}
	if n := s.Hurtboxes(1, &boxes); n == 0 {
		t.Error("player 1 is actionable and still has no hurtboxes")
	}
}
