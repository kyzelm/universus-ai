package sim

// Input bitfield — one uint16 per player per frame.
const (
	InUp    uint16 = 1 << 0
	InDown  uint16 = 1 << 1
	InLeft  uint16 = 1 << 2
	InRight uint16 = 1 << 3
	InLP    uint16 = 1 << 4
	InMP    uint16 = 1 << 5
	InHP    uint16 = 1 << 6
	InLK    uint16 = 1 << 8
	InMK    uint16 = 1 << 9
	InHK    uint16 = 1 << 10
)

// Stage geometry.
const (
	GroundY = Fix(0)
)

// The stage and screen widths are balance data (Balance.StageHalfWidth,
// Balance.CameraHalfWidth), so the data hash covers them: two builds that
// disagreed on where the walls are would otherwise handshake and desync.

// PlayerState is one fighter.
type PlayerState struct {
	X, Y   Fix
	VX, VY Fix

	// Facing is +1 (right) or -1 (left).
	Facing int32

	// Char indexes the loaded roster.
	Char int32

	Health int32

	// Drive and Super are the two resources (03 Game Design/Resource
	// System.md), in units of a thousandth of a bar — see resource.go.
	Drive int32
	Super int32

	// Combo is how many hits the player has taken without recovering, and
	// ComboStarter the scaling multiplier captured from the move that began it
	// — captured, because the starter scales the whole combo and the move that
	// started it is gone by the third hit.
	Combo        int32
	ComboStarter int32
	Counter      int32

	// Burnout is 1 while the Drive gauge is refilling from empty.
	Burnout int32

	// State and StateFrame are the state machine.
	State      int32
	StateFrame int32

	// MoveIndex is the active move while State is StateAttack, else -1.
	MoveIndex int32

	// HasHit is the HitNum of the last hit the current move connected with, 0
	// for none, so one hit cannot connect twice and a multi-hit move's next hit
	// can.
	HasHit int32

	// Armor is how many hits the current move has already absorbed.
	Armor int32

	// Stun is the remaining hitstun, blockstun or landing recovery.
	Stun int32

	// Landing is the recovery this player owes the moment they touch down — the
	// debt of a move that ended while they were still in the air, carried
	// because the move itself is gone by then.
	Landing int32

	// Down is 1 while this player owes a knockdown at the end of their current
	// stun.
	Down int32

	// Eaten is the newest frame whose button presses have already been spent,
	// or -1.
	Eaten int32

	// Events is this frame's effect flags — see events.go.
	Events int32

	// Juggle is how many times this player has been hit while already in the
	// air, reset the moment they touch the ground.
	Juggle int32

	// JumpVX is the horizontal velocity committed at pre-jump.
	JumpVX Fix

	// JumpHeld is 1 while the up that produced a jump is *still* being held.
	JumpHeld int32

	// AI is the difficulty tier driving this seat, AIOff for a human, and
	// AIPlan the action the last decision picked — see ai.go.
	AI     int32
	AIPlan int32

	// Inputs is the input history ring: Inputs[f%InputHistory] is the bitfield
	// that advanced frame f, widened to uint32 to keep GameState padding-free.
	Inputs [InputHistory]uint32
}

// GameState is all gameplay state.
type GameState struct {
	Frame uint32
	// RNG is the whole generator — see rand.go.
	RNG uint32

	// Hitstop freezes both fighters for a few frames on a connect.
	Hitstop int32

	// Round flow — see round.go.
	Phase      int32
	PhaseFrame int32

	// Timer is the round clock, counted in frames because the sim has no other
	// clock and is never allowed one.
	Timer int32

	// Training is 1 in training mode (03 Game Design/Game Modes.md): resources
	// refill, the clock does not run and the round never ends, so the lab stays
	// a lab.
	Training int32

	// Round is the 1-based number of the round being played and Wins the rounds
	// each player has taken.
	Round int32
	Wins  [2]int32

	// RoundWinner is who took the round just decided, RoundNobody for a draw;
	// Winner is the match winner once Phase is PhaseMatchEnd and RoundNobody
	// until then.
	RoundWinner int32
	Winner      int32

	// Dealt is cumulative damage dealt across the whole match, per player.
	Dealt [2]int32

	// CamX is the camera centre.
	CamX Fix

	Players [2]PlayerState

	// Projectiles is the pool.
	Projectiles [MaxProjectiles]Projectile
}

// New returns the starting state with both players on character 0.
func New() GameState { return NewMatch(0, 0) }

// NewTraining is New in training mode: the same match, with the clock stopped,
// the round never ending and both fighters kept topped up.
func NewTraining() GameState {
	s := New()
	s.Training = 1
	return s
}

// NewAIMatch is New with seat 2 played by the scripted opponent at the given
// difficulty (03 Game Design/AI Opponent.md).
func NewAIMatch(tier int32) GameState {
	s := New()
	s.Players[1].AI = tier
	return s
}

// Setup is everything about a match that is chosen before frame 0 and cannot be
// read back out of an input log: which characters, which mode, and whether seat
// 2 is the scripted opponent.
type Setup struct {
	Training bool
	AI       int32
	Chars    [2]int32
}

// NewStateOf is the one constructor the others are special cases of.
func NewStateOf(u Setup) GameState {
	s := NewMatch(u.Chars[0], u.Chars[1])
	if u.Training {
		s.Training = 1
	}
	s.Players[1].AI = u.AI
	return s
}

// NewMatch starts a match between two roster entries: players apart, on the
// ground, facing each other, at full health.
func NewMatch(c0, c1 int32) GameState {
	s := GameState{RNG: seed, Players: [2]PlayerState{
		{X: Fix(-60) << FracBits, Facing: 1, Char: c0, Health: CharacterAt(c0).Health},
		{X: Fix(60) << FracBits, Facing: -1, Char: c1, Health: CharacterAt(c1).Health},
	}}
	for i := range s.Players {
		s.Players[i].MoveIndex = -1
		s.Players[i].Eaten = -1
		// Drive starts full and Super starts empty.
		s.Players[i].Drive = DriveMax
	}

	// Round 1, fighting.
	s.Round = 1
	s.Timer = balance.RoundFrames
	s.RoundWinner = RoundNobody
	s.Winner = RoundNobody
	s.centreCamera()
	return s
}

// Advance runs one frame.
func (s *GameState) Advance(in [2]uint16) {
	// Effect flags describe *this* frame, so they are cleared before anything
	// can set them — above the hitstop and phase returns below, or a hit's flag
	// would survive the whole freeze and be fired once per frame of it.
	s.clearEvents()

	// A seat the AI is driving replaces whatever was passed for it, above
	// everything else so its presses are recorded, buffered and read as motions
	// exactly like a human's — the AI has no other way into the sim (ai.go).
	for i := range s.Players {
		if s.Players[i].AI != AIOff {
			in[i] = s.aiInput(i)
		}
	}

	// 1. Resolve inputs.
	for i := range s.Players {
		s.Players[i].Inputs[s.Frame%InputHistory] = uint32(in[i])
	}

	// Hitstop freezes the whole match: no input, no movement, no new hits.
	if s.Hitstop > 0 {
		s.Hitstop--
		s.Frame++
		return
	}

	// Between rounds nothing moves.
	if s.Phase != PhaseFight {
		s.advancePhase()
		s.Frame++
		return
	}

	for i := range s.Players {
		s.resolveInputs(i, in[i])
	}

	// 2. Advance state machines.
	for i := range s.Players {
		s.advanceState(i)
	}

	// 3. Movement and gravity.
	for i := range s.Players {
		p := &s.Players[i]

		// Whether the player was in the air is decided before the position
		// moves, and both the gravity and the landing below read that one
		// answer.
		air := p.Airborne()
		if air {
			p.VY += p.gravity()
		}
		p.X += p.VX
		p.Y += p.VY

		if p.Y <= GroundY {
			p.Y = GroundY
			p.VY = 0
			if air {
				// Landing kills horizontal momentum, so a move that came down
				// does not slide through its recovery.
				p.VX = 0

				// The floor is what ends a juggle, whoever was juggled and
				// however they got up there.
				p.Juggle = 0

				// Landing ends a jump, and it ends an air normal with it: an
				// air move's recovery is the fall, and it has no business
				// continuing on the ground.
				mv := p.move()
				switch {
				case mv != nil && mv.Stance == StanceAir:
					p.land(mv.Landing)
				case p.State == StateAir:
					p.land(p.Landing)
				case p.State == StateHitstun:
					// A juggle ends on the floor.
					p.knockdown()
				}
			}
		}
	}

	// 4. Pushboxes and stage bounds, then turn to face the opponent.
	s.separate()
	s.clampToStage()
	s.updateFacing()

	// 5. Projectiles: they move before hit detection reads their boxes, and
	// after the players have been separated and clamped, so a fireball is
	// tested against final positions like everything else.
	s.advanceProjectiles()

	// 6. Hit detection, then 7.
	mv0, mv1 := s.Players[0].move(), s.Players[1].move()
	n0, n1 := s.hitNum(0), s.hitNum(1)
	hit0 := s.connects(0, 1)
	hit1 := s.connects(1, 0)
	if hit0 {
		s.resolveHit(0, 1, mv0, n0, in[1])
	}
	if hit1 {
		s.resolveHit(1, 0, mv1, n1, in[0])
	}

	// Projectiles resolve after both players, so a fireball and the punch that
	// beat it to the same frame trade in a fixed order rather than in whichever
	// order the loop happened to reach them.
	s.resolveProjectiles(in)

	// 8. Timers, resources, round state.
	s.updateResources()

	// A combo lasts exactly as long as the hitstun holding it together.
	for i := range s.Players {
		if p := &s.Players[i]; p.State != StateHitstun {
			p.Combo = 0
			p.ComboStarter = 0
		}
	}
	s.updateCamera()

	// The round is resolved last of all, after the damage that might have ended
	// it and after the combo bookkeeping: a KO is a fact about the health bar
	// this frame left behind, not about the hit that was being applied.
	s.updateRound()

	// 9. Increment frame.
	s.Frame++
}

// connects reports whether attacker's hitboxes overlap defender's hurtboxes.
func (s *GameState) connects(attacker, defender int) bool {
	// A throw refuses a defender it is not allowed to take, and it refuses them
	// *here* rather than at resolution: a throw that whiffs has to actually
	// whiff.
	if mv := s.Players[attacker].move(); mv != nil && mv.IsThrow() && !s.throwable(defender) {
		return false
	}

	// The juggle limit refuses the connect rather than softening it: a move
	// that has run out of juggles whiffs, and whiffing is what ends the combo.
	if s.juggled(defender, s.Players[attacker].move()) {
		return false
	}

	var hits, hurts [MaxBoxes]Box
	nh := s.Hitboxes(attacker, &hits)
	if nh == 0 {
		return false
	}
	nd := s.Hurtboxes(defender, &hurts)

	for a := int32(0); a < nh; a++ {
		for d := int32(0); d < nd; d++ {
			if hits[a].Overlaps(hurts[d]) {
				return true
			}
		}
	}
	return false
}

// resolveHit applies one connect.
func (s *GameState) resolveHit(attacker, defender int, mv *Move, hit int32, defenderIn uint16) {
	ap := &s.Players[attacker]

	if mv == nil {
		return
	}
	// A throw is resolved whole, by its own path: it cannot be blocked, cannot
	// counter-hit, and may be escaped by a tech that ends the attacker's move
	// too — which is why it marks the connect itself rather than being marked
	// here and then possibly undone.
	if mv.IsThrow() {
		s.applyThrow(attacker, defender, mv, s.Frame)
		return
	}

	ap.HasHit = hit
	s.applyHit(attacker, defender, mv, defenderIn)
}

// hitNum is the HitNum of the hitboxes the player's move has on this frame, or
// 0 when it has none.
func (s *GameState) hitNum(i int) int32 {
	p := &s.Players[i]
	if mv := p.move(); mv != nil {
		if k := mv.BoxesAt(p.StateFrame); k != nil && k.NumHit > 0 {
			return k.HitNumber()
		}
	}
	return 0
}

// applyHit is the half of a connect that lands on the defender: block or hit,
// stun, damage, resources, hitstop.
func (s *GameState) applyHit(attacker, defender int, mv *Move, defenderIn uint16) {
	ap, dp := &s.Players[attacker], &s.Players[defender]

	// Whether the defender was already off the ground, read before anything
	// here changes their state: entering hitstun ends StateAir, and the answer
	// decides both whether this hit counts as a juggle and whether it is
	// allowed to launch them again.
	airborne := dp.Airborne()

	// Damage dealt is measured off the health bar at the end of this function
	// rather than taken from the formula, so the match total the round cap's
	// tiebreak reads is what actually happened: chip that stopped at 1 health
	// counts what it removed, and a hit that overkills counts what was left.
	before := dp.Health

	if dp.absorbs() {
		// Armour. The defender is mid-armoured-move and this hit is one it
		// eats: no hitstun, no knockback, no combo, no counter — their move
		// carries on, which is what they paid for.
		dp.Armor++
		dp.chip(mv.Damage * balance.ArmorDamagePercent / 100)
		dp.Events |= EventBlock

		// Meter still moves: a hit that was absorbed is a hit that landed, and
		// both sides felt it.
		ap.gainSuper(mv.Damage * balance.SuperDealtPercent / 100)
		dp.gainSuper(mv.Damage * balance.SuperTakenPercent / 100)
	} else if s.blocking(defender, defenderIn, mv.Level) {
		// A blocked hit is not a hit: it ends no combo and starts none, and the
		// counter class of the last real hit stands until the next one.
		dp.enter(StateBlockstun)
		dp.Stun = mv.Blockstun
		dp.Events |= EventBlock

		// Block pushback is its own number and it is the larger one.
		s.applyKnockback(attacker, defender, mv, true)

		if dp.Burnout != 0 {
			// The Burnout penalties, and the only place in the game where
			// blocking deals damage (D32).
			dp.Stun += balance.BurnoutBlockstun
			if mv.Motion != MotionNone {
				// Chip, and chip cannot kill.
				dp.chip(mv.Damage * balance.BurnoutChipPercent / 100)
			}
		} else {
			// Blocking spends Drive.
			dp.spendDrive(balance.DriveBlockCost)
		}
	} else {
		// The counter class is read before anything is applied: it is a fact
		// about the defender's own attack, and entering hitstun destroys it.
		counter := s.counterClass(defender)

		// The starter scales the whole combo, so it is captured on the hit that
		// begins one and never recomputed.
		if dp.Combo == 0 {
			dp.ComboStarter = starterScale(mv)
		}
		dp.Combo++
		dp.Counter = counter

		dp.enter(StateHitstun)
		dp.Stun = mv.Hitstun + counterHitstun(counter)
		dp.Events |= EventHit

		// Owed after the hitstun, not instead of it, and set after enter
		// because entering a state is what clears the previous debt.
		if mv.KnocksDown() {
			dp.Down = 1
		}

		dp.hurt(scaledDamage(mv, dp.ComboStarter, dp.Combo, counter))

		// Super is built from the move's base damage, not the scaled figure.
		ap.gainSuper(mv.Damage * balance.SuperDealtPercent / 100)
		dp.gainSuper(mv.Damage * balance.SuperTakenPercent / 100)

		// A hit taken in the air is a juggle hit.
		if airborne {
			dp.Juggle++
		}

		// Pushback, and the launcher with it: one field family, two behaviours.
		s.applyKnockback(attacker, defender, mv, false)
	}

	// A special that connects pays a flat bonus whether it hit or was blocked.
	if mv.Motion != MotionNone {
		ap.gainSuper(balance.SuperOnSpecial)
	}

	// Hitstop is one value for the match, not one per player: both fighters
	// freeze together, which is the whole effect.
	if mv.Hitstop > s.Hitstop {
		s.Hitstop = mv.Hitstop
	}

	// One place, so every source of damage — hits, chip, projectiles, whatever
	// comes next — is counted without anyone having to remember to.
	s.Dealt[attacker] += before - dp.Health
}

// separate pushes overlapping pushboxes apart, half the overlap each, then the
// stage clamp runs after.
func (s *GameState) separate() {
	a, b := s.Pushbox(0), s.Pushbox(1)

	// Vertical miss: one player is cleanly above the other, so they pass.
	if a.Y >= b.Y+b.H || b.Y >= a.Y+a.H {
		return
	}

	overlap := min(a.X+a.W, b.X+b.W) - max(a.X, b.X)
	if overlap <= 0 {
		return
	}

	// The tie at identical positions breaks by player index, which is the only
	// property that matters: it breaks the same way on every machine.
	push := overlap / 2
	if s.Players[0].X <= s.Players[1].X {
		s.Players[0].X -= push
		s.Players[1].X += push
	} else {
		s.Players[0].X += push
		s.Players[1].X -= push
	}
}

// clampToStage keeps both pushboxes inside the walls.
func (s *GameState) clampToStage() {
	for i := range s.Players {
		d := s.wallCorrection(i)
		if d == 0 {
			continue
		}
		s.Players[i].X += d
		if !pushedIntoWall(&s.Players[i], d) {
			continue
		}

		// The push has to go somewhere and the wall does not move, so the
		// attacker takes it — and is clamped in turn, because a stage narrower
		// than two fighters plus a push is a reason to stop, not a reason to
		// leave someone outside the walls.
		o := 1 - i
		s.Players[o].X += d
		s.Players[o].X += s.wallCorrection(o)
	}
}

// wallCorrection is how far player i has to move to be inside the walls, or
// zero if they already are.
func (s *GameState) wallCorrection(i int) Fix {
	box := s.Pushbox(i)
	lo := max(-balance.StageHalfWidth, s.CamX-balance.CameraHalfWidth)
	hi := min(balance.StageHalfWidth, s.CamX+balance.CameraHalfWidth)
	if d := lo - box.X; d > 0 {
		return d
	}
	if d := (box.X + box.W) - hi; d > 0 {
		return -d
	}
	return 0
}

// updateFacing turns each player toward the other.
func (s *GameState) updateFacing() {
	for i := range s.Players {
		p := &s.Players[i]
		if !Actionable(p.State) {
			continue
		}
		switch other := s.Players[1-i].X; {
		case other > p.X:
			p.Facing = 1
		case other < p.X:
			p.Facing = -1
			// Exactly level: keep the current facing.
		}
	}
}

// centreCamera puts the camera between the players, inside the stage.
func (s *GameState) centreCamera() {
	s.CamX = s.clampCamera((s.Players[0].X + s.Players[1].X) / 2)
}

// updateCamera moves the camera only as far as it has to.
func (s *GameState) updateCamera() {
	a, b := s.Players[0].X, s.Players[1].X
	lo, hi := min(a, b), max(a, b)
	reach := balance.CameraHalfWidth - balance.CameraMargin
	c := s.CamX
	switch {
	case hi-lo > 2*reach:
		c = (lo + hi) / 2
	case hi > c+reach:
		c = hi - reach
	case lo < c-reach:
		c = lo + reach
	}
	s.CamX = s.clampCamera(c)
}

// clampCamera keeps the screen inside the stage: never show past a wall, or
// the corner does not look like a corner.
func (s *GameState) clampCamera(c Fix) Fix {
	return min(max(c, -balance.StageHalfWidth+balance.CameraHalfWidth),
		balance.StageHalfWidth-balance.CameraHalfWidth)
}
