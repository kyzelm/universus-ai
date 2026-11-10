package sim

// The character state machine.

// States. Values are part of the state's byte layout, so they are appended to,
// never reordered.
const (
	StateIdle int32 = iota
	StateWalkF
	StateWalkB
	StateCrouch
	StateDash
	StateBackdash
	StatePreJump // grounded startup — this is what makes throws beat jumps
	StateAir
	StateAttack
	StateHitstun
	StateBlockstun
	// StateLanding is the recovery owed for coming down out of a move.
	StateLanding
	// StateThrown is being thrown, and it is also the recovery both players owe
	// after a throw is teched.
	StateThrown
	// StateKnockdown is lying on the ground after a sweep or a throw, and it is
	// the state okizeme is played against: the attacker knows exactly which
	// frame the defender rises on, and that shared knowledge is the mechanic.
	StateKnockdown
)

// absorbs reports whether this player's current move eats the hit about to land
// on them: it has armour left, and it is still on the frames that carry it.
func (p *PlayerState) absorbs() bool {
	mv := p.move()
	return mv != nil && mv.Armored() && p.Armor < mv.Armor &&
		p.StateFrame < mv.Startup+mv.Active
}

// Airborne reports whether the player is off the ground.
func (p *PlayerState) Airborne() bool { return p.State == StateAir || p.Y > GroundY }

// Actionable reports whether a state accepts a new action this frame.
func Actionable(state int32) bool {
	switch state {
	case StateIdle, StateWalkF, StateWalkB, StateCrouch:
		return true
	}
	return false
}

// stunned reports the states that are a fixed count of frames accepting no
// input — hitstun, blockstun, landing recovery, a throw and its tech, and lying
// on the floor.
func stunned(state int32) bool {
	switch state {
	case StateHitstun, StateBlockstun, StateLanding, StateThrown, StateKnockdown:
		return true
	}
	return false
}

// enter puts the player into a state, resetting its frame counter.
func (p *PlayerState) enter(state int32) {
	p.State = state
	p.StateFrame = 0
	p.MoveIndex = -1
	// The debt belongs to the move that incurred it.
	p.Landing = 0
	// The knockdown owed by the hit currently being served, cleared for the
	// same reason and by the same rule: being hit out of hitstun replaces that
	// hit's consequences with the new one's, so a sweep that was interrupted
	// does not still knock down at the end of somebody else's combo.
	p.Down = 0
}

// knockdown puts the player on the ground.
func (p *PlayerState) knockdown() {
	p.enter(StateKnockdown)
	p.Stun = balance.KnockdownFrames
	p.Events |= EventKnockdown
}

// land ends an airborne action on touchdown, owing n frames of recovery.
func (p *PlayerState) land(n int32) {
	if n <= 0 {
		p.enter(StateIdle)
		return
	}
	p.enter(StateLanding)
	p.Stun = n
}

// hurt takes n damage, floored at zero.
func (p *PlayerState) hurt(n int32) { p.Health = max(p.Health-n, 0) }

// forward turns a speed authored facing right into the direction the player faces.
func (p *PlayerState) forward(v Fix) Fix { return v * Fix(p.Facing) }

// stay is enter for a state the player may already be in: holding forward for
// twenty frames is one walk, not twenty.
func (p *PlayerState) stay(state int32) {
	if p.State != state {
		p.enter(state)
	}
}

// enterMove starts an attack on frame now, and spends every press up to and
// including that frame: the press that started this move must not start another
// one while it is still inside the buffer window.
// ponytail: one frame marker, not a per-button flag.
func (p *PlayerState) enterMove(index int32, now uint32) {
	p.enter(StateAttack)
	p.MoveIndex = index
	p.HasHit = 0
	p.Eaten = int32(now)

	// A grounded move owns the character's velocity from here.
	mv := &CharacterAt(p.Char).Moves[index]

	// The meter is spent here, on the frame the move starts, and nowhere else.
	p.Super -= mv.SuperCost()
	if mv.Super > 0 {
		p.Events |= EventSuper
	}

	// Drive is spent here for the same reason and in the same place.
	if cost := mv.DriveCost(); cost > 0 {
		p.spendDrive(cost)
	}

	// Armour is counted per move, so the count resets with the move rather than
	// carrying absorbed hits into the next one.
	p.Armor = 0

	if !p.Airborne() || mv.Launches() {
		p.VX = p.forward(mv.LaunchVX)
		p.VY = mv.LaunchVY
	}
}

// resolveInputs is step 1: turn this frame's bitfield into a state transition.
func (s *GameState) resolveInputs(i int, in uint16) {
	p := &s.Players[i]
	c := CharacterAt(p.Char)
	now := s.Frame

	// Letting go of up is what arms the next jump.
	if in&InUp == 0 {
		p.JumpHeld = 0
	}

	// A move that has connected stops being a commitment if its data says so:
	// the cancel window accepts one thing, a move in a category the move being
	// cancelled names.
	cancel := s.cancelMask(i)

	dir := direction(in, p.Facing)
	crouching := dir == DirDown || dir == DirDownBack || dir == DirDownFwd

	stance := int32(StanceStand)
	switch {
	// Airborne rather than StateAir, for the cancel window's sake: a rising
	// uppercut is off the ground without being in StateAir, and what it could
	// cancel into up there is air moves.
	case p.Airborne():
		stance = StanceAir
	case crouching:
		stance = StanceCrouch
	}

	// The second button of a two-button input lands late, and the move it was
	// meant to be still comes out.
	if p.State == StateAttack {
		if m := s.moveFor(i, stance, now, 0); m >= 0 && s.pairLate(i, in, c.Moves[m].Button) {
			p.enterMove(m, now)
			return
		}
	}

	// Stun and commitment states tick down elsewhere; they accept no input.
	if !Actionable(p.State) && p.State != StateAir && cancel == 0 {
		return
	}

	// Attacks first: a button beats a direction on the same frame, which is
	// what lets a crouching attack come out of a walk without a spare frame.
	if m := s.moveFor(i, stance, now, cancel); m >= 0 {
		p.enterMove(m, now)
		return
	}

	// A cancel window is not an actionable state.
	if p.State == StateAttack {
		return
	}

	// Dashes: two taps of the same direction inside the dash window.
	if p.doubleTapped(now, DirFwd) {
		p.enter(StateDash)
		return
	}
	if p.doubleTapped(now, DirBack) {
		p.enter(StateBackdash)
		return
	}

	if p.State == StateAir {
		return
	}

	switch {
	// One press, one jump.
	case (dir == DirUp || dir == DirUpFwd || dir == DirUpBack) && p.JumpHeld == 0:
		p.JumpHeld = 1
		p.enter(StatePreJump)
		// The jump direction is committed here, at the *start* of pre-jump.
		switch dir {
		case DirUpFwd:
			p.JumpVX = p.forward(c.JumpForwardVX)
		case DirUpBack:
			p.JumpVX = -p.forward(c.JumpForwardVX)
		default:
			p.JumpVX = 0
		}

	case crouching:
		p.stay(StateCrouch)

	case dir == DirFwd:
		p.stay(StateWalkF)

	case dir == DirBack:
		p.stay(StateWalkB)

	default:
		p.stay(StateIdle)
	}
}

// dashWindow is how close two taps must be to dash.
const dashWindow = 10

// doubleTapped reports two clean presses of dir inside dashWindow.
func (p *PlayerState) doubleTapped(now uint32, dir uint8) bool {
	taps, released := 0, false

	for back := uint32(0); back < dashWindow && back <= now; back++ {
		switch d := direction(p.at(now, back), p.Facing); d {
		case dir:
			// Only a fresh press counts.
			if back == 0 || released {
				taps++
				released = false
				if taps == 2 {
					return true
				}
			}

		case DirNeutral:
			released = true

		default:
			return false
		}
	}
	return false
}

// cancelMask is the set of categories the player's current move may be
// cancelled into this frame, or zero for no cancel.
func (s *GameState) cancelMask(i int) uint16 {
	p := &s.Players[i]
	mv := p.move()
	if mv == nil {
		return 0
	}
	if p.HasHit == 0 || p.StateFrame < mv.Startup {
		return mv.CancelInto & CancelTarget
	}
	return mv.CancelInto
}

// followOpen reports that follow-up mv may come out of the player's current
// move on this frame: the current move is one of its sources, the connect rule
// is met, and the source is inside the follow-up's window.
func (p *PlayerState) followOpen(mv *Move) bool {
	src := p.move()
	if src == nil || mv.After&(1<<uint32(p.MoveIndex)) == 0 {
		return false
	}
	if mv.Whiff == 0 && p.HasHit == 0 {
		return false
	}
	start, end := mv.WindowStart, mv.WindowEnd
	if end == 0 {
		start, end = src.Startup, src.Total()
		if mv.Whiff != 0 {
			start = src.Startup + src.Active
		}
	}
	return p.StateFrame >= start && p.StateFrame < end
}

// moveFor finds the move a recent button press selects, or -1.
func (s *GameState) moveFor(i int, stance int32, now uint32, cancel uint16) int32 {
	p := &s.Players[i]
	c := CharacterAt(p.Char)

	// Candidates are ranked press frame first and move tier second — super over
	// special over normal — and the whole comparison is one integer: three
	// ranks per frame, one per tier.
	best, bestRank := int32(-1), p.Eaten*tiers+tiers-1

	for m := int32(0); m < c.NumMoves; m++ {
		mv := &c.Moves[m]
		special := mv.Motion != MotionNone

		// A cancel takes only what the move being cancelled named.
		if cancel != 0 && cancel&mv.Category() == 0 {
			continue
		}

		// A follow-up exists only inside the window of a move it was written
		// for.
		if mv.After != 0 && (cancel&CancelTarget == 0 || !p.followOpen(mv)) {
			continue
		}

		// The meter gates the super, and it gates it *here*, at selection: a
		// level 3 the player cannot afford must not be picked and then fail,
		// because a picked move has already beaten every cheaper one that
		// shared the button.
		if cost := mv.SuperCost(); cost > p.Super {
			continue
		}

		// The Drive gauge gates EX specials the same way, and Burnout refuses
		// them outright: no EX at all while the gauge is refilling is the
		// design's own rule, and it is what makes running out of Drive a state
		// worth avoiding rather than a slower meter.
		if cost := mv.DriveCost(); cost > 0 && (p.Burnout != 0 || cost > p.Drive) {
			continue
		}

		// Ground and air never mix, specials included: a fireball motion is
		// still on the stick when the character leaves the ground, and without
		// this a jump would throw one.
		if (mv.Stance == StanceAir) != (stance == StanceAir) {
			continue
		}

		// Beyond that, stance gates normals only.
		if !special && mv.Stance != stance {
			continue
		}

		f := p.pressFrame(now, mv.Button)
		if f < 0 {
			continue
		}
		if mv.Dir != 0 && direction(p.at(now, now-uint32(f)), p.Facing) != mv.Dir {
			continue
		}
		// The motion is checked at the frame of the *press*, not at the frame
		// the move finally comes out.
		if special && !s.MotionAt(i, uint32(f)).satisfies(mv.Motion) {
			continue
		}

		rank := f*tiers + tierOf(mv)
		if rank > bestRank {
			best, bestRank = m, rank
		}
	}
	return best
}

// The selection tiers.
const tiers = 5

func tierOf(m *Move) int32 {
	switch {
	case m.Super > 0:
		return 4
	case m.Drive > 0:
		return 3
	case m.Motion != MotionNone:
		return 2
	case m.IsThrow():
		return 1
	default:
		return 0
	}
}

// multiButton reports a button mask with more than one bit — a throw or an EX
// special.
func multiButton(mask uint16) bool { return mask&(mask-1) != 0 }

// pairLate reports that the player is a frame or two into a single-button
// attack and is now completing the two-button input pair, which that attack's
// button is part of.
func (s *GameState) pairLate(i int, in, pair uint16) bool {
	p := &s.Players[i]
	if !multiButton(pair) || in&pair != pair {
		return false
	}
	if p.State != StateAttack || p.HasHit != 0 || p.StateFrame >= balance.PairFrames {
		return false
	}
	mv := p.move()
	return mv != nil && !multiButton(mv.Button) && p.StateFrame < mv.Startup &&
		pair&mv.Button != 0
}

// advanceState is step 2: run the current state's clock and decide what the
// player's velocity is this frame.
func (s *GameState) advanceState(i int) {
	p := &s.Players[i]
	c := CharacterAt(p.Char)

	// Every state below sets its velocity from scratch each frame — except an
	// attack, which was handed its velocity when it started and keeps it (that
	// is what makes a launch an arc rather than one frame of movement), and
	// stun, which keeps the pushback the hit gave it.
	switch {
	case p.State == StateAttack:
		// Keeps whatever enterMove handed it.

	case stunned(p.State):
		// Pushback bleeds off on the ground and not in the air: friction is
		// what ends a slide, and a launched defender has none until they land.
		if !p.Airborne() && p.StateFrame > 0 {
			p.VX = Fix(int64(p.VX) * int64(balance.KnockbackDecay) / 100)
		}

	default:
		p.VX = 0
	}

	switch p.State {
	case StateWalkF:
		p.VX = p.forward(c.WalkForward)

	case StateWalkB:
		p.VX = -p.forward(c.WalkBack)

	case StateDash, StateBackdash:
		frames, dist := c.DashFrames, c.DashDistance
		if p.State == StateBackdash {
			frames, dist = c.BackdashFrames, -c.DashDistance
		}
		// Exit before applying velocity, not after.
		if p.StateFrame >= frames {
			p.enter(StateIdle)
			return
		}
		// Constant speed over the dash: distance divided by duration, computed
		// once per frame in fixed-point rather than stored, so a rollback into
		// the middle of a dash needs no extra state.
		p.VX = p.forward(dist.Div(FromInt(int(frames))))

	case StatePreJump:
		// Grounded, motionless, and throwable.
		if p.StateFrame >= c.PreJumpFrames {
			p.enter(StateAir)
			p.VY = c.JumpVelocity
			p.VX = p.JumpVX
			return
		}

	case StateAir:
		// No air control: the arc was committed at pre-jump.
		p.VX = p.JumpVX

	case StateAttack:
		mv := p.move()
		if mv == nil || p.StateFrame >= mv.Total() {
			// A move that runs out while the character is still off the ground
			// hands over to StateAir, not to idle: an idle player is
			// actionable, and actionable in mid-air is a different game.
			if p.Y > GroundY {
				p.JumpVX = p.VX // StateAir drives VX from this
				owed := mv.Landing
				p.enter(StateAir)
				p.Landing = owed
				return
			}
			p.enter(StateIdle)
			return
		}

	case StateHitstun, StateBlockstun, StateLanding, StateThrown, StateKnockdown:
		// Hitstun taken in the air does not run out in the air.
		if p.State == StateHitstun && p.Airborne() {
			break
		}
		if p.Stun > 0 {
			p.Stun--
		}
		if p.Stun == 0 {
			// A hit that knocks down owes the knockdown at the end of its
			// hitstun, not instead of it: the defender is struck, held for the
			// move's frames, and only then goes to the floor.
			if p.Down != 0 {
				p.knockdown()
			} else {
				p.enter(StateIdle)
			}
			return
		}
	}

	p.StateFrame++
}

// move is the player's current move, or nil.
func (p *PlayerState) move() *Move {
	if p.State != StateAttack || p.MoveIndex < 0 {
		return nil
	}
	c := CharacterAt(p.Char)
	if p.MoveIndex >= c.NumMoves {
		return nil
	}
	return &c.Moves[p.MoveIndex]
}

// Hurtboxes writes the player's active hurtboxes into out and returns how many.
func (s *GameState) Hurtboxes(i int, out *[MaxBoxes]Box) int32 {
	p := &s.Players[i]
	c := CharacterAt(p.Char)

	// A player on the floor has no hurtboxes at all.
	if p.State == StateKnockdown {
		return 0
	}

	if mv := p.move(); mv != nil {
		// An invulnerable frame has no hurtboxes at all, which is the whole
		// mechanism: this is the one funnel every attack and every projectile
		// asks, so a move that answers nothing here cannot be hit by anything.
		if mv.Invulnerable(p.StateFrame) {
			return 0
		}
		if k := mv.BoxesAt(p.StateFrame); k != nil {
			for b := int32(0); b < k.NumHurt; b++ {
				out[b] = k.Hurt[b].World(p.X, p.Y, p.Facing)
			}
			return k.NumHurt
		}
	}

	var local Box
	switch {
	case p.Airborne():
		local = c.AirHurt
	case p.State == StateCrouch:
		local = c.CrouchHurt
	default:
		local = c.StandHurt
	}
	if local.Empty() {
		return 0
	}
	out[0] = local.World(p.X, p.Y, p.Facing)
	return 1
}

// Hitboxes writes the player's active hitboxes into out and returns how many.
func (s *GameState) Hitboxes(i int, out *[MaxBoxes]Box) int32 {
	p := &s.Players[i]

	mv := p.move()
	if mv == nil {
		return 0
	}
	k := mv.BoxesAt(p.StateFrame)
	if k == nil || p.HasHit >= k.HitNumber() {
		return 0
	}
	for b := int32(0); b < k.NumHit; b++ {
		out[b] = k.Hit[b].World(p.X, p.Y, p.Facing)
	}
	return k.NumHit
}

// Pushbox is the player's world-space pushbox.
func (s *GameState) Pushbox(i int) Box {
	p := &s.Players[i]
	return CharacterAt(p.Char).Pushbox.World(p.X, p.Y, p.Facing)
}

// Blocking reports whether the player is holding away from the opponent and is
// therefore blocking, given the attack level.
func (s *GameState) blocking(i int, in uint16, level int32) bool {
	p := &s.Players[i]
	if p.Airborne() || p.State == StatePreJump {
		return false
	}
	if !Actionable(p.State) && p.State != StateBlockstun {
		return false
	}

	d := direction(in, p.Facing)
	if d != DirBack && d != DirDownBack {
		return false
	}
	crouching := d == DirDownBack

	switch level {
	case LevelHigh:
		return !crouching // standing blocks highs
	case LevelLow:
		return crouching // crouching blocks lows
	default:
		return true // mids are blocked either way
	}
}
