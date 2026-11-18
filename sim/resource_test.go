package sim

import "testing"

// The super meter.

// blocked runs player 0's jab into a player 1 who is holding away, and stops on the frame the block
// lands.
func blocked(t *testing.T, s *GameState) {
	t.Helper()
	s.Advance([2]uint16{InLP, InRight})
	for range 60 {
		s.Advance([2]uint16{0, InRight})
		if s.Players[1].State == StateBlockstun {
			return
		}
		if s.Players[1].State == StateHitstun {
			t.Fatal("holding away produced hitstun, not blockstun")
		}
	}
	t.Fatal("the attack never connected, so nothing was blocked")
}

// fireballBlocked has player 1 block player 0's fireball.
func fireballBlocked(t *testing.T, s *GameState) {
	t.Helper()
	feed(s, 2, 3)
	s.Advance([2]uint16{pad[6] | InLP, InRight})
	for range 60 {
		s.Advance([2]uint16{0, InRight})
		if s.Players[1].State == StateBlockstun {
			return
		}
	}
	t.Fatal("the fireball was never blocked")
}

// Chip: a blocked special still hurts, a blocked normal does not.
func TestChipOnlyOnSpecials(t *testing.T) {
	s := facing(30)
	full := s.Players[1].Health
	blocked(t, &s)
	if s.Players[1].Health != full {
		t.Errorf("a blocked normal chipped %d", full-s.Players[1].Health)
	}

	s = facing(60)
	fireballBlocked(t, &s)
	want := char().Moves[2].Damage * BalanceOf().ChipPercent / 100
	if got := full - s.Players[1].Health; got != want {
		t.Errorf("chip damage = %d, want %d", got, want)
	}
}

// Chip can kill.
func TestChipCanKill(t *testing.T) {
	s := facing(60)
	s.Players[1].Health = 1
	fireballBlocked(t, &s)

	if got := s.Players[1].Health; got != 0 {
		t.Errorf("health = %d after chip on one point of health, want 0", got)
	}
	if s.Phase != PhaseKO {
		t.Errorf("phase = %d, want the KO", s.Phase)
	}
}

// Both fighters build Super from the same hit, at different rates.
func TestSuperBuildsForBothFightersOnAHit(t *testing.T) {
	s := facing(30)
	mv := &char().Moves[0]
	b := BalanceOf()

	for range 60 {
		s.Advance([2]uint16{InLP, 0})
		if s.Players[1].State == StateHitstun {
			break
		}
	}
	if s.Players[1].State != StateHitstun {
		t.Fatal("the jab never connected")
	}

	if got, want := s.Players[0].Super, mv.Damage*b.SuperDealtPercent/100; got != want {
		t.Errorf("the attacker built %d Super, want %d", got, want)
	}
	if got, want := s.Players[1].Super, mv.Damage*b.SuperTakenPercent/100; got != want {
		t.Errorf("the defender built %d Super, want %d", got, want)
	}
}

func TestSuperIsCapped(t *testing.T) {
	var p PlayerState
	p.gainSuper(BalanceOf().SuperMax + BarUnits)
	if p.Super != BalanceOf().SuperMax {
		t.Errorf("Super = %d, want it capped at %d", p.Super, BalanceOf().SuperMax)
	}
}
