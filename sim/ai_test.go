package sim

import "testing"

// The rule tests run with the imperfection roll switched off.
func certain(t *testing.T) {
	t.Helper()
	b := testBalance()
	b.AIRandomPercent, b.AIBlockPercent = 0, 100
	LoadBalance(b)
	t.Cleanup(func() { LoadBalance(testBalance()) })
}

// The property the whole design rests on.
func TestAnAIMatchIsReproducible(t *testing.T) {
	play := func() uint32 {
		s := NewAIMatch()
		s.Players[0].AI = 1
		for f := 0; f < 2000; f++ {
			s.Advance([2]uint16{0, 0})
		}
		return s.Checksum()
	}
	if a, b := play(), play(); a != b {
		t.Errorf("two identical AI matches ended at %#x and %#x", a, b)
	}
}

// A rollback replays the AI's decisions rather than re-rolling them.
func TestAIDecisionsRollBack(t *testing.T) {
	s := NewAIMatch()
	s.Players[0].AI = 1

	for f := 0; f < 300; f++ {
		s.Advance([2]uint16{0, 0})
	}
	saved := s
	for f := 0; f < 60; f++ {
		s.Advance([2]uint16{0, 0})
	}
	want := s.Checksum()

	s = saved
	for f := 0; f < 60; f++ {
		s.Advance([2]uint16{0, 0})
	}
	if got := s.Checksum(); got != want {
		t.Errorf("replayed to %#x, want %#x — an AI decision did not roll back", got, want)
	}
}

// Non-degeneracy, the design note's own bar.
func TestAIvsAIDoesNotDegenerate(t *testing.T) {
	s := NewAIMatch()
	s.Players[0].AI = 1

	hits := false
	start := [2]int32{s.Players[0].Health, s.Players[1].Health}
	near, far := false, false

	for f := 0; f < 10000; f++ {
		s.Advance([2]uint16{0, 0})
		if s.Players[0].Health < start[0] || s.Players[1].Health < start[1] {
			hits = true
		}
		// Two fighters that never close and never separate are deadlocked, whatever their health
		// bars say.
		if d := s.aiDistance(0); d <= balance.AICloseRange {
			near = true
		} else if d > balance.AIMidRange {
			far = true
		}
	}

	// Rounds, not the match: a best-of-three of timeouts is longer than this, and it is ending *a
	// round* that says the loop is not stuck.
	if s.Round < 2 && s.Winner == RoundNobody {
		t.Errorf("still on round %d after 10000 frames, wins %v", s.Round, s.Wins)
	}
	// A round decided entirely by the clock with nobody ever landing a hit is two opponents
	// standing still, which is exactly the degenerate case the round check above would not notice.
	if !hits {
		t.Error("neither player took a hit in 10000 frames")
	}
	if !near || !far {
		t.Errorf("the gap never varied: closed %v, opened %v", near, far)
	}
}

// Anti-air is first in the priority list, and it is the rule most worth having.
func TestAIAntiAirsAJumpIn(t *testing.T) {
	certain(t)

	s := NewAIMatch()
	s.Players[0].X = FromInt(-20)
	s.Players[1].X = FromInt(20)
	s.Players[0].State = StateAir
	s.Players[0].Y = FromInt(30)
	s.Players[0].StateFrame = 20 // long enough for the AI to have noticed

	if got := aiDecide(&s, 1, &s); got != aiDP {
		t.Errorf("decided plan %d against a jump-in, want the dragon punch (%d)", got, aiDP)
	}
}

// The device is the same opponent played from outside the match, and the two properties that make
// it usable as a test opponent are that it repeats and that it leaves no trace.
func TestTheAIDeviceDecidesWithoutTouchingTheMatch(t *testing.T) {
	s := New()
	for f := 0; f < 40; f++ {
		s.Advance([2]uint16{0, 0})
	}

	before := s.Checksum()
	d := NewAIDevice(7)
	var pressed []uint16
	for f := 0; f < 40; f++ {
		pressed = append(pressed, d.Press(&s, 1))
	}
	if got := s.Checksum(); got != before {
		t.Errorf("the state changed under a device: %#x, was %#x", got, before)
	}
	if !pressedSomething(pressed) {
		t.Error("the device pressed nothing at all in 40 frames")
	}

	// Same tier, same seed, same state: the same opponent.
	again := NewAIDevice(7)
	for f, want := range pressed {
		if got := again.Press(&s, 1); got != want {
			t.Fatalf("frame %d: pressed %#b, want %#b", f, got, want)
		}
	}

	// A different seed is a different opponent, or two bots in a harness play the same match twice
	// and half the fuzz is wasted.
	other := NewAIDevice(99)
	same := true
	for _, want := range pressed {
		if other.Press(&s, 1) != want {
			same = false
			break
		}
	}
	if same {
		t.Error("two seeds pressed the same buttons")
	}
}

func pressedSomething(in []uint16) bool {
	for _, v := range in {
		if v != 0 {
			return true
		}
	}
	return false
}
