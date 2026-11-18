package sim

import "testing"

// Training mode (03 Game Design/Game Modes.md, D41).

func TestTrainingStopsTheClockAndTheRound(t *testing.T) {
	s := NewTraining()
	timer := s.Timer

	// Empty a health bar and run well past the point a round would end.
	s.Players[1].Health = 0
	for f := 0; f < 120; f++ {
		s.Advance([2]uint16{0, 0})
	}

	if s.Timer != timer {
		t.Errorf("timer moved from %d to %d: the lab has no clock", timer, s.Timer)
	}
	if s.Phase != PhaseFight {
		t.Errorf("phase = %d, want the fight: the round ended in training", s.Phase)
	}
	if s.Wins[0] != 0 {
		t.Errorf("player 0 was awarded %d rounds in training", s.Wins[0])
	}
}

// Off by default, and the flag is in the state, so the checksum covers it.
func TestTrainingIsOffByDefaultAndCoveredByTheChecksum(t *testing.T) {
	normal, lab := New(), NewTraining()
	if normal.Training != 0 {
		t.Error("an ordinary match started in training mode")
	}
	if normal.Checksum() == lab.Checksum() {
		t.Error("the checksum does not cover the mode")
	}
}
