package sim

import "testing"

// The super meter.

// blocked runs player 0's jab into a player 1 who is holding away, and stops on
// the frame the block lands.
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

func TestSuperStartsEmpty(t *testing.T) {
	s := New()
	for i := range s.Players {
		if s.Players[i].Super != 0 {
			t.Errorf("p%d starts with %d Super, want none", i, s.Players[i].Super)
		}
	}
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

// Both fighters build Super from the same hit, at different rates: the one
// landing it is rewarded, the one eating it is compensated.
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

// A special pays a flat bonus for connecting, whether it hit or was blocked:
// it is paid for the connect, not for the damage behind it.
func TestALandedSpecialPaysItsBonus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		defend  uint16
		damaged bool
	}{
		{"on hit", 0, true},
		{"on block", InRight, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := facing(60)
			mv := &char().Moves[2]
			b := BalanceOf()

			feed(&s, 2, 3)
			s.Advance([2]uint16{pad[6] | InLP, tc.defend})
			for range 60 {
				s.Advance([2]uint16{0, tc.defend})
				if s.Players[1].State == StateHitstun || s.Players[1].State == StateBlockstun {
					break
				}
			}

			want := b.SuperOnSpecial
			if tc.damaged {
				want += mv.Damage * b.SuperDealtPercent / 100
			}
			if got := s.Players[0].Super; got != want {
				t.Errorf("the fireball built %d Super, want %d", got, want)
			}
		})
	}
}

// Super does not regenerate.
func TestSuperNeverRegenerates(t *testing.T) {
	s := New()
	run(&s, 300, [2]uint16{0, 0})
	if s.Players[0].Super != 0 {
		t.Errorf("Super regenerated to %d over 300 idle frames", s.Players[0].Super)
	}
}

func TestSuperIsCapped(t *testing.T) {
	var p PlayerState
	p.gainSuper(BalanceOf().SuperMax + BarUnits)
	if p.Super != BalanceOf().SuperMax {
		t.Errorf("Super = %d, want it capped at %d", p.Super, BalanceOf().SuperMax)
	}
}

// A bar is a round number of units so that a half-bar cost is an exact integer.
func TestABarIsAnEvenNumberOfUnits(t *testing.T) {
	if BarUnits%2 != 0 {
		t.Errorf("BarUnits is %d: a half-bar cost would not be an integer", BarUnits)
	}
}
