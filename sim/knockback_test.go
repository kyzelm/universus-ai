package sim

import "testing"

// The fixture's anti-air launcher and the button that selects it.
const (
	launcherIndex = int32(12)
	launcherInput = InLK
)

// strike presses p0 until the move it selects lands on player 1, and returns
// the state on the frame of the connect.
func strike(t *testing.T, gap int, p0, p1 uint16) GameState {
	t.Helper()

	s := facing(gap)
	for range 60 {
		s.Advance([2]uint16{p0, p1})
		switch s.Players[1].State {
		case StateHitstun, StateBlockstun:
			return s
		}
	}
	t.Fatal("the move never connected")
	return GameState{}
}

// pushStep advances until player 1's position changes, and returns the state
// and how far they moved.
func pushStep(t *testing.T, s *GameState) Fix {
	t.Helper()

	for range 30 {
		before := s.Players[1].X
		s.Advance([2]uint16{0, 0})
		if d := s.Players[1].X - before; d != 0 {
			return d
		}
	}
	t.Fatal("the defender never moved")
	return 0
}

// A hit moves the defender.
func TestAHitPushesTheDefenderAway(t *testing.T) {
	s := strike(t, 40, InLP, 0)

	if got := pushStep(t, &s); got != balance.KnockbackHit {
		t.Errorf("first frame of pushback moved %v, want the balance value %v", got, balance.KnockbackHit)
	}
	// Half of it left the next frame: the fixture keeps 50%.
	if got, want := pushStep(t, &s), balance.KnockbackHit/2; got != want {
		t.Errorf("second frame moved %v, want %v", got, want)
	}
}

// The corner rule, and the reason pushback is worth having at all: the wall
// does not move, so the push the clamp would have eaten moves the attacker back
// instead.
func TestTheCornerPushesTheAttackerBack(t *testing.T) {
	s := facing(40)
	// Player 1's pushbox is 12 wide either side, so this is exactly the wall.
	s.Players[1].X = balance.StageHalfWidth - FromInt(12)
	s.Players[0].X = s.Players[1].X - FromInt(40)
	// Teleported, so move the camera with them or its edge is the wall.
	s.updateCamera()

	for range 60 {
		s.Advance([2]uint16{InLP, 0})
		if s.Players[1].State == StateHitstun {
			break
		}
	}
	if s.Players[1].State != StateHitstun {
		t.Fatal("the jab never connected")
	}

	wall, attacker := s.Players[1].X, s.Players[0].X
	for range 30 {
		s.Advance([2]uint16{0, 0})
		if s.Players[0].X != attacker {
			break
		}
	}

	if s.Players[1].X != wall {
		t.Errorf("the cornered defender moved to %v from %v, but the wall is behind them", s.Players[1].X, wall)
	}
	if got, want := attacker-s.Players[0].X, balance.KnockbackHit; got != want {
		t.Errorf("the attacker was pushed back %v, want the whole eaten push %v", got, want)
	}
}

// A launcher is a knockback with a vertical component and nothing else — no
// second field, no flag that could disagree with the velocity.
func TestALauncherPutsTheDefenderInTheAir(t *testing.T) {
	s := strike(t, 40, launcherInput, 0)
	if s.Players[0].MoveIndex != launcherIndex {
		t.Fatalf("standing LK gave move %d, want the launcher %d", s.Players[0].MoveIndex, launcherIndex)
	}

	if got, want := s.Players[1].VY, char().Moves[launcherIndex].KnockbackVY; got != want {
		t.Fatalf("the defender's vertical velocity is %v, want the move's %v", got, want)
	}

	airborne := false
	for range 20 {
		s.Advance([2]uint16{0, 0})
		if s.Players[1].Airborne() {
			airborne = true
			break
		}
	}
	if !airborne {
		t.Fatal("the launcher left the defender on the ground")
	}
}

// airborne puts player 1 in the air with n juggle hits already on them, still
// rising so they are still up there when the follow-up lands.
func airborne(n int32) GameState {
	s := facing(40)
	s.Players[1].enter(StateAir)
	s.Players[1].Y = FromInt(10)
	s.Players[1].VY = FromInt(4)
	s.Players[1].Juggle = n
	return s
}

// The limit refuses the connect.
func TestAMoveRefusesToConnectAtTheJuggleLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int32
		want bool
	}{
		{"one hit below the limit", balance.JuggleLimit - 1, true},
		{"at the limit", balance.JuggleLimit, false},
		{"past it", balance.JuggleLimit + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := airborne(tc.n)
			hit := false
			for range 20 {
				s.Advance([2]uint16{launcherInput, 0})
				if s.Players[1].State == StateHitstun {
					hit = true
					break
				}
			}
			if hit != tc.want {
				t.Errorf("the follow-up connected: %v, want %v", hit, tc.want)
			}
		})
	}
}
