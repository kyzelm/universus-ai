package sim

import "testing"

// events runs the fixture's jab into player 1 and returns the state on the
// frame it connected, with p1 holding hold — which is what decides whether it
// is a hit or a block.
func events(t *testing.T, hold uint16) GameState {
	t.Helper()

	s := facing(40)
	for range 40 {
		s.Advance([2]uint16{InLP, hold})
		switch s.Players[1].State {
		case StateHitstun, StateBlockstun:
			return s
		}
	}
	t.Fatal("the jab never connected")
	return GameState{}
}

// The flag is on the defender, because that is where the effect happens: a hit
// spark is drawn on the fighter who was hit, not on the one who swung.
func TestAHitFlagsTheDefender(t *testing.T) {
	s := events(t, 0)

	if s.Players[1].Events&EventHit == 0 {
		t.Error("the defender has no hit event")
	}
	if s.Players[1].Events&EventBlock != 0 {
		t.Error("a clean hit also flagged a block")
	}
	if s.Players[0].Events != 0 {
		t.Errorf("the attacker has events %#x, want none", s.Players[0].Events)
	}
}

// A flag describes one frame and does not survive it.
func TestAnEventLastsOneFrame(t *testing.T) {
	s := events(t, 0)
	if s.Hitstop == 0 {
		t.Fatal("the fixture's jab has no hitstop, so this proves nothing")
	}

	for f := range 10 {
		s.Advance([2]uint16{0, 0})
		if e := s.Players[1].Events; e&EventHit != 0 {
			t.Fatalf("the hit event is still set %d frames later (hitstop %d)", f+1, s.Hitstop)
		}
	}
}

// session is a Session with the players close enough to hit each other, which
// New() deliberately is not: a match starts at a range where nothing connects.
func session() Session {
	s := NewSession()
	s.State().Players[0].X = FromInt(-20)
	s.State().Players[1].X = FromInt(20)
	return s
}

// hits runs the jab and returns the frame it connected on.
func hits(t *testing.T, s *Session, hold uint16) uint32 {
	t.Helper()

	for range 40 {
		s.Advance([2]uint16{InLP, hold})
		switch s.State().Players[1].State {
		case StateHitstun, StateBlockstun:
			return s.Frame() - 1
		}
	}
	t.Fatal("the jab never connected")
	return 0
}

// defender pulls player 1's flags out of a packed event word.
func defender(bits uint32) int32 { return int32(bits >> 16) }

// The mechanism, in one test.
func TestACorrectedReplayReplacesTheEvents(t *testing.T) {
	s := session()
	hit := hits(t, &s, 0)

	if e := defender(s.EventsAt(hit)); e&EventHit == 0 {
		t.Fatalf("frame %d: events %#x, want a hit to correct away", hit, e)
	}

	// The packet arrives: the defender was holding back all along, so that
	// frame was a block and not a hit.
	if !s.Adjust(hit, [2]uint16{InLP, InRight}) {
		t.Fatal("the rollback was rejected")
	}

	e := defender(s.EventsAt(hit))
	if e&EventBlock == 0 {
		t.Errorf("frame %d: events %#x after the correction, want a block", hit, e)
	}
	if e&EventHit != 0 {
		t.Errorf("frame %d: the corrected frame still reports the hit that was predicted (%#x)", hit, e)
	}
}

// A frame replayed nine times reports what one simulation of it reports.
func TestReplayingAFrameDoesNotRepeatItsEvents(t *testing.T) {
	const frames = 40
	in := inputSeq(frames)

	ref := session()
	for _, i := range in {
		ref.Advance(i)
	}

	s := session()
	for f, i := range in {
		s.Advance(i)
		// Roll back and replay the same eight frames, over and over: nine
		// simulations of every frame in the window.
		if f >= MaxRollback && f%3 == 0 {
			target := s.Frame() - MaxRollback
			if !s.Adjust(target, in[target]) {
				t.Fatalf("frame %d: rollback rejected", f)
			}
		}
	}

	var any uint32
	for f := uint32(0); f < frames; f++ {
		if got, want := s.EventsAt(f), ref.EventsAt(f); got != want {
			t.Errorf("frame %d: events %#x after replays, want %#x", f, got, want)
		}
		any |= ref.EventsAt(f)
	}
	if any == 0 {
		t.Fatal("no events at all in 40 frames, so this test is checking nothing")
	}
}

// Each new flag fires once over a jab, a jump and a round start.
func TestTheSoundEventsFireOnce(t *testing.T) {
	count := func(s *GameState, frames int, in [2]uint16, p int, bit int32) int {
		n := 0
		for range frames {
			s.Advance(in)
			if s.Players[p].Events&bit != 0 {
				n++
			}
		}
		return n
	}

	s := facing(40)
	if n := count(&s, 30, [2]uint16{InLP, 0}, 0, EventAttack); n != 1 {
		t.Errorf("a held jab flagged EventAttack %d times, want 1", n)
	}
	s = facing(40)
	if n := count(&s, 90, [2]uint16{InUp, 0}, 0, EventJump); n != 1 {
		t.Errorf("a held jump flagged EventJump %d times, want 1", n)
	}
	s = facing(40)
	if n := count(&s, 90, [2]uint16{InUp, 0}, 0, EventLand); n != 1 {
		t.Errorf("a held jump flagged EventLand %d times, want 1", n)
	}
	s = New()
	if n := count(&s, 600, [2]uint16{}, 0, EventFight); n != 1 {
		t.Errorf("the first round flagged EventFight %d times, want 1", n)
	}
}
