package sim

import "testing"

// The super is three things at once: a motion, a price and a cancel target.

// qcfx2 inputs the double motion and presses button on the final frame.
func qcfx2(s *GameState, button uint16) {
	feed(s, 2, 3, 6, 2, 3)
	s.Advance([2]uint16{pad[6] | button, 0})
}

func TestSuperMotionsAreRecognised(t *testing.T) {
	for _, tc := range []struct {
		name string
		dirs []uint8
		want Motion
	}{
		{"double quarter-circle forward", []uint8{2, 3, 6, 2, 3, 6}, MotionQCFx2},
		// One quarter-circle is not two.
		{"a single quarter-circle is not a super motion", []uint8{2, 3, 6}, MotionQCF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			feed(&s, tc.dirs...)
			if got := s.Motion(0); got != tc.want {
				t.Errorf("motion = %d, want %d", got, tc.want)
			}
		})
	}
}

// The doubles must be scanned before DP, and this is the reason: read backward,
// ↓↘→↓↘→ offers →, ↓, ↘ in exactly the order a dragon punch wants.
func TestSuperMotionBeatsDP(t *testing.T) {
	s := New()
	feed(&s, 2, 3, 6, 2, 3, 6)
	if got := s.Motion(0); got != MotionQCFx2 {
		t.Fatalf("motion = %d, want the double quarter-circle (%d)", got, MotionQCFx2)
	}

	// And the plain dragon punch still is one.
	d := New()
	feed(&d, 6, 2, 3)
	if got := d.Motion(0); got != MotionDP {
		t.Errorf("a dragon punch reads as %d", got)
	}
}

// A double quarter-circle is a quarter-circle that kept going, so the same
// input has to be able to produce the cheaper move.
func TestDoubleMotionSatisfiesTheSingle(t *testing.T) {
	for _, tc := range []struct {
		got, want Motion
		ok        bool
	}{
		{MotionQCFx2, MotionQCF, true},
		{MotionQCFx2, MotionQCFx2, true},
		{MotionQCF, MotionQCFx2, false},
		{MotionQCFx2, MotionQCB, false},
		{MotionDP, MotionQCF, false},
	} {
		if got := tc.got.satisfies(tc.want); got != tc.ok {
			t.Errorf("Motion(%d).satisfies(%d) = %v, want %v", tc.got, tc.want, got, tc.ok)
		}
	}
}

// The category a move *is* — derived from its own data, never authored, so it
// cannot disagree with what the move actually does.
func TestCancelCategoryIsDerived(t *testing.T) {
	c := char()
	for _, tc := range []struct {
		move int32
		want uint16
	}{
		{0, CancelChain},   // a jab
		{2, CancelSpecial}, // a motion makes it a special
		{8, CancelSuper},
	} {
		if got := c.Moves[tc.move].Category(); got != tc.want {
			t.Errorf("move %d category = %#b, want %#b", tc.move, got, tc.want)
		}
	}
}

// The meter is checked when the move is *selected*, not after it comes out.
func TestASuperWithoutTheMeterIsTheSpecial(t *testing.T) {
	for _, tc := range []struct {
		name string
		bars int32
		want int32
	}{
		{"no meter at all", 0, 2},
		{"a fraction short", BarUnits - 1, 2},
		{"exactly the cost", BarUnits, 8},
		{"more than enough", 2 * BarUnits, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.Players[0].Super = tc.bars
			qcfx2(&s, InLP)

			if got := s.Players[0].MoveIndex; got != tc.want {
				t.Errorf("move %d came out, want %d", got, tc.want)
			}
		})
	}
}

// Any punch is the same super, and no kick is.
func TestTheSuperTakesAnyButtonOfItsKind(t *testing.T) {
	for _, tc := range []struct {
		button uint16
		super  bool
	}{
		{InLP, true}, {InMP, true}, {InHP, true},
		// Two punches with a full gauge is the super, not the EX.
		{InLP | InMP, true},
		{InLK, false}, {InHK, false},
	} {
		s := New()
		s.Players[0].Super = 2 * BarUnits
		qcfx2(&s, tc.button)
		if got := s.Players[0].MoveIndex == 8; got != tc.super {
			t.Errorf("buttons %#x: move %d came out, super = %v", tc.button, s.Players[0].MoveIndex, tc.super)
		}
	}
}

func TestPerformingASuperSpendsItsBar(t *testing.T) {
	s := New()
	s.Players[0].Super = 2 * BarUnits
	qcfx2(&s, InLP)

	if s.Players[0].MoveIndex != 8 {
		t.Fatalf("setup: move %d came out, not the super", s.Players[0].MoveIndex)
	}
	// A second punch landing late must not turn the paid super into an EX.
	s.Advance([2]uint16{pad[6] | InLP | InMP, 0})
	if s.Players[0].MoveIndex != 8 {
		t.Errorf("move %d: a late second punch took the super back", s.Players[0].MoveIndex)
	}
	// And it is spent once, not once per frame the move is on screen.
	run(&s, 10, [2]uint16{0, 0})
	if got, want := s.Players[0].Super, int32(BarUnits); got != want {
		t.Errorf("meter = %d after a one-bar super, want %d", got, want)
	}
}

// connect runs player 0's move until it has connected and the hitstop it caused
// has run out, which is when the cancel window is open *and* inputs are being
// read again.
func connect(t *testing.T, s *GameState, in uint16) {
	t.Helper()
	s.Advance([2]uint16{in, 0})
	for range 30 {
		if s.Players[0].HasHit != 0 && s.Hitstop == 0 {
			return
		}
		s.Advance([2]uint16{0, 0})
	}
	t.Fatal("the move never connected, so no cancel window opened")
}

// The cancel is data: a source move names the categories it feeds.
func TestCancelCategoriesComeFromTheSource(t *testing.T) {
	t.Run("a cancelable normal reaches the super", func(t *testing.T) {
		s := facing(30)
		s.Players[0].Super = 2 * BarUnits
		connect(t, &s, InLP)
		qcfx2(&s, InLP)

		if got := s.Players[0].MoveIndex; got != 8 {
			t.Errorf("move %d came out of the cancel, want the super", got)
		}
	})

	t.Run("a source that names only the super refuses the special", func(t *testing.T) {
		s := facing(30)
		s.Players[0].Super = 2 * BarUnits
		connect(t, &s, InDown|InLK)
		qcf(&s, InLP)

		if got := s.Players[0].MoveIndex; got != 1 {
			t.Errorf("move %d came out, want the low still running (1)", got)
		}
	})

	t.Run("and accepts the super", func(t *testing.T) {
		s := facing(30)
		s.Players[0].Super = 2 * BarUnits
		connect(t, &s, InDown|InLK)
		qcfx2(&s, InLP)

		if got := s.Players[0].MoveIndex; got != 8 {
			t.Errorf("move %d came out of the cancel, want the super", got)
		}
	})
}
