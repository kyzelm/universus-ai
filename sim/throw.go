package sim

// Throws and throw tech (03 Game Design/Movement and Defense.md).

// throwable reports whether the defender can be thrown at all.
func (s *GameState) throwable(i int) bool {
	p := &s.Players[i]
	return !p.Airborne() && !stunned(p.State)
}

// techFrame is the frame on which the defender pressed throw inside the tech
// window, or -1 for no escape.
func (s *GameState) techFrame(defender int, now uint32) int32 {
	mv := throwOf(s.Players[defender].Char)
	if mv == nil {
		return -1
	}
	return s.Players[defender].pressWithin(now, mv.Button, uint32(balance.ThrowTechFrames))
}

// throwOf is the character's *ordinary* throw — the one with no motion — or
// nil.
func throwOf(char int32) *Move {
	c := CharacterAt(char)
	for i := int32(0); i < c.NumMoves; i++ {
		if c.Moves[i].IsThrow() && c.Moves[i].Motion == MotionNone {
			return &c.Moves[i]
		}
	}
	return nil
}

// applyThrow resolves a throw that has connected.
func (s *GameState) applyThrow(attacker, defender int, mv *Move, now uint32) {
	ap, dp := &s.Players[attacker], &s.Players[defender]

	// A command throw cannot be teched, which is the genre's own rule and the
	// whole reason a grappler is frightening: the throw that beats blocking is
	// answerable, and the one that costs a motion is not.
	if mv.Motion == MotionNone && s.techFrame(defender, now) >= 0 {
		s.tech(attacker, defender)
		return
	}

	dp.enter(StateThrown)
	dp.Stun = mv.Hitstun
	dp.Events |= EventThrown
	// A throw ends on the floor if its data says so, which is the whole of what
	// changed when knockdowns arrived: the fixed count of helpless frames a
	// thrown player used to spend is now the throw's own animation, and the
	// knockdown after it is the same one a sweep gives.
	if mv.KnocksDown() {
		dp.Down = 1
	}

	// Through the same damage pipeline as everything else, so the scaling order
	// has one implementation (D79).
	dp.ComboStarter = starterScale(mv)
	dp.Combo++
	dp.Counter = CounterNone

	before := dp.Health
	dp.hurt(scaledDamage(mv, dp.ComboStarter, dp.Combo, CounterNone))
	s.Dealt[attacker] += before - dp.Health

	ap.gainSuper(mv.Damage * balance.SuperDealtPercent / 100)
	dp.gainSuper(mv.Damage * balance.SuperTakenPercent / 100)

	if mv.Hitstop > s.Hitstop {
		s.Hitstop = mv.Hitstop
	}
	ap.HasHit = 1
}

// tech is the escape: both players push apart, neither takes damage, and both
// owe the same recovery.
func (s *GameState) tech(attacker, defender int) {
	push := FromInt(int(balance.ThrowTechPush))

	for _, i := range [2]int{attacker, defender} {
		p := &s.Players[i]
		p.enter(StateThrown)
		p.Stun = balance.ThrowTechRecovery
		p.VX = 0
		p.VY = 0
	}

	// Apart along the axis they are actually on, not along either player's
	// facing: at the instant of a crossup the two disagree about which way
	// forward is, and pushing by facing would send them the same way.
	if s.Players[attacker].X <= s.Players[defender].X {
		push = -push
	}
	s.Players[attacker].X += push
	s.Players[defender].X -= push

	// The walls have the final say, as they do after any other movement — a
	// tech in the corner pushes the attacker out and leaves the cornered player
	// where they are.
	s.clampToStage()
}
