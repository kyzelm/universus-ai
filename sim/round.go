package sim

// Round flow — 03 Game Design/Round Flow.md.

// Phases. Values are part of the state's byte layout, so they are appended to,
// never reordered.
const (
	// PhaseFight is the only one that runs the game.
	PhaseFight int32 = iota
	PhaseKO
	PhaseRoundEnd
	PhaseIntro
	// PhaseMatchEnd is terminal: the match is over and nothing further happens
	// until the caller starts another one.
	PhaseMatchEnd
)

// RoundNobody is "no winner": a draw round, and the match while it is still
// being played.
const RoundNobody int32 = -1

// updateRound is the round half of step 8: the clock, the KO check, and the
// decision to stop fighting.
func (s *GameState) updateRound() {
	if s.Phase != PhaseFight {
		return
	}
	// The lab has no clock and no end.
	if s.Training != 0 {
		return
	}

	if s.Timer > 0 {
		s.Timer--
	}

	dead0, dead1 := s.Players[0].Health <= 0, s.Players[1].Health <= 0
	switch {
	// A double KO on the same frame is a draw, and it has to be checked before
	// either single KO or the player the loop reached first would win it.
	case dead0 && dead1:
		s.endRound(RoundNobody)
	case dead0:
		s.endRound(1)
	case dead1:
		s.endRound(0)
	case s.Timer == 0:
		s.endRound(s.aheadOnHealth())
	}
}

// aheadOnHealth is who wins a timeout: the higher percentage of their own
// maximum, not the higher number.
func (s *GameState) aheadOnHealth() int32 {
	a := s.Players[0].Health * CharacterAt(s.Players[1].Char).Health
	b := s.Players[1].Health * CharacterAt(s.Players[0].Char).Health
	switch {
	case a > b:
		return 0
	case b > a:
		return 1
	}
	return RoundNobody
}

// endRound awards the round and starts the KO freeze.
func (s *GameState) endRound(winner int32) {
	s.RoundWinner = winner
	if winner != RoundNobody {
		s.Wins[winner]++
	}
	// The freeze replaces whatever hitstop the killing blow set rather than
	// queueing behind it: two freezes in a row read as one long stutter, and
	// the hit that ends a round is the one moment nobody wants to sit through
	// twice.
	s.Hitstop = 0
	s.enterPhase(PhaseKO)
}

func (s *GameState) enterPhase(phase int32) {
	s.Phase = phase
	s.PhaseFrame = 0
}

// phaseLength is how long a between-rounds phase lasts.
func phaseLength(phase int32) int32 {
	switch phase {
	case PhaseKO:
		return balance.KOFreeze
	case PhaseRoundEnd:
		return balance.RoundEndHold
	case PhaseIntro:
		return balance.IntroFrames
	}
	return 0
}

// advancePhase runs one frame of a between-rounds phase.
func (s *GameState) advancePhase() {
	if s.Phase == PhaseMatchEnd {
		return // terminal — there is no next phase to count towards
	}

	s.PhaseFrame++
	if s.PhaseFrame < phaseLength(s.Phase) {
		return
	}

	switch s.Phase {
	case PhaseKO:
		s.enterPhase(PhaseRoundEnd)

	case PhaseRoundEnd:
		if w, over := s.matchWinner(); over {
			s.Winner = w
			s.enterPhase(PhaseMatchEnd)
			return
		}
		// The fighters are reset *before* the intro rather than after it, so
		// the intro is the players standing at their starting positions on full
		// health — which is what the intro is for.
		s.startRound()
		s.enterPhase(PhaseIntro)

	case PhaseIntro:
		s.enterPhase(PhaseFight)
	}
}

// matchWinner reports the match winner and whether the match is over at all.
func (s *GameState) matchWinner() (int32, bool) {
	for i := int32(0); i < 2; i++ {
		if s.Wins[i] >= balance.RoundsToWin {
			return i, true
		}
	}
	if s.Round < balance.MaxRounds {
		return RoundNobody, false
	}

	switch {
	case s.Dealt[0] > s.Dealt[1]:
		return 0, true
	case s.Dealt[1] > s.Dealt[0]:
		return 1, true
	}
	// Arbitrary, and that is fine: an arbitrary rule both machines agree on is
	// correct, and a fair rule they could disagree about is a desync.
	return 0, true
}

// startRound resets the fighters for the next round.
func (s *GameState) startRound() {
	fresh := NewMatch(s.Players[0].Char, s.Players[1].Char)
	for i := range s.Players {
		super := s.Players[i].Super
		// The AI tier is match setup, like the character it is playing — not
		// round state.
		ai := s.Players[i].AI
		s.Players[i] = fresh.Players[i]
		s.Players[i].Super = super
		s.Players[i].AI = ai
	}

	s.Round++
	s.Timer = balance.RoundFrames
	s.RoundWinner = RoundNobody
	s.Hitstop = 0
	// A fireball must not outlive the round that fired it: the pool is state
	// like everything else, and a projectile left in it would arrive during the
	// next round's intro.
	clear(s.Projectiles[:])
	s.centreCamera()
}
