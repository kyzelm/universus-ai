package sim

// The scripted opponent (03 Game Design/AI Opponent.md).

// Difficulty tiers.
const (
	AIOff int32 = iota
	AIEasy
	AINormal
	AIHard
	aiTierCount = 3
)

// The plans. A decision picks one and it runs until the next decision, which is
// what keeps the AI from re-deciding sixty times a second — a human cannot, and
// an opponent that does reads as a machine.
const (
	aiNeutral int32 = iota
	aiApproach
	aiRetreat
	aiBlock
	aiJab
	aiPunish
	aiSweep
	aiDP
	aiFireball
	aiThrow
	aiJump

	// The plan for a character that does not own the Shoto's answers.
	aiAntiAir // the anti-air normal, for a character with no invincible reversal

	aiPlanCount
)

// aiRandom is where a decision's rolls come from.
type aiRandom interface {
	RandN(n uint32) uint32
}

// AIDevice plays a seat from outside the simulation: the same rule list, its
// own generator and its own plan, and nothing written to GameState.
type AIDevice struct {
	tier int32
	plan int32
	rng  uint32
}

// NewAIDevice returns a device at the given tier.
func NewAIDevice(tier int32, seed uint32) *AIDevice {
	if seed == 0 {
		seed = 1
	}
	return &AIDevice{tier: tier, rng: seed}
}

// RandN is the device's own generator — the same xorshift the sim uses, kept
// separate so a device draw never perturbs the match's sequence.
func (d *AIDevice) RandN(n uint32) uint32 {
	x := d.rng
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	d.rng = x
	if n == 0 {
		return 0
	}
	return uint32((uint64(x) * uint64(n)) >> 32)
}

// Press is one frame of input for a seat, decided from the state as it stands.
func (d *AIDevice) Press(s *GameState, seat int) uint16 {
	every := balance.AIDecisionFrames
	if every <= 0 || seat < 0 || seat > 1 {
		return 0
	}
	elapsed := int32(s.Frame % uint32(every))
	if elapsed == 0 {
		d.plan = aiDecide(s, seat, d.tier, d)
	}
	return aiPress(d.plan, elapsed, s.Players[seat].Facing)
}

// aiInput is the bitfield this seat presses this frame.
func (s *GameState) aiInput(i int) uint16 {
	p := &s.Players[i]
	if p.AI == AIOff {
		return 0
	}

	// Decisions are on a fixed cadence off the frame number rather than a
	// countdown in state: one fewer field, and it rolls back for free because
	// the frame number does.
	every := balance.AIDecisionFrames
	if every <= 0 {
		return 0 // an unloaded balance is a still opponent, not a random one
	}
	elapsed := int32(s.Frame % uint32(every))
	if elapsed == 0 {
		p.AIPlan = aiDecide(s, i, p.AI, s)
	}
	return aiPress(p.AIPlan, elapsed, p.Facing)
}

// aiPress turns a plan and its age into buttons.
func aiPress(plan, age int32, facing int32) uint16 {
	fwd, back := InRight, InLeft
	if facing < 0 {
		fwd, back = InLeft, InRight
	}

	switch plan {
	case aiApproach:
		return fwd
	case aiRetreat, aiBlock:
		// Blocking *is* holding away, so the plan that blocks and the plan that
		// backs off are the same input.
		return back
	case aiJab:
		return press(age, InLP)
	case aiPunish:
		return press(age, InHP)
	case aiSweep:
		return press(age, InDown|InHK)
	case aiThrow:
		return press(age, InLP|InLK)
	case aiAntiAir:
		// Crouching heavy punch: a hitbox rather than an invincible move, which
		// is the trade a character without a reversal makes everywhere else too.
		return press(age, InDown|InHP)
	case aiJump:
		// Pressed, not held: holding up would jump again on every landing,
		// which is a plan nobody chose.
		return press(age, InUp)
	case aiDP:
		// The shortcut motion, → ↓ →, and deliberately not the real → ↓ ↘.
		switch age {
		case 0:
			return fwd
		case 1:
			return InDown
		case 2:
			return fwd | InLP
		}
	case aiFireball:
		switch age {
		case 0:
			return InDown
		case 1:
			return InDown | fwd
		case 2:
			return fwd | InLP
		}
	}
	return 0
}

// press is one frame of a button and then nothing: a held button is one press
// to the sim, and a plan that kept holding would eat its own next decision.
func press(age int32, bits uint16) uint16 {
	if age == 0 {
		return bits
	}
	return 0
}

// aiDecide is the rule list, in priority order, first match wins.
func aiDecide(s *GameState, i int, tier int32, r aiRandom) int32 {
	p, o := &s.Players[i], &s.Players[1-i]
	t := &balance.AITiers[aiTier(tier)]
	dist := s.aiDistance(i)

	// Deliberate imperfection, first and on purpose.
	if aiRoll(r, t.RandomPercent) {
		return int32(r.RandN(uint32(aiPlanCount)))
	}

	switch {
	// Anti-air. The window is generous on purpose: a dragon punch that comes
	// out late loses to the jump-in it was meant to beat, and that is the
	// mistake a human makes too.
	case o.Airborne() && dist <= balance.AIAntiAirRange && aiSeen(o, t.Reaction):
		if aiHasMotion(p.Char, MotionDP) {
			return aiDP
		}
		return aiAntiAir

	// Punish. A move in its recovery cannot block, and the AI's heaviest button
	// is the answer.
	// ponytail: one button, not a combo.
	case aiRecovering(o) && dist <= balance.AICloseRange && aiSeen(o, t.Reaction):
		return aiPunish

	// Block what is coming, as often as the tier says.
	case aiIncoming(o) && dist <= balance.AIMidRange &&
		aiSeen(o, t.Reaction) && aiRoll(r, t.BlockPercent):
		return aiBlock

	// Reversal out of pressure.
	case p.State == StateBlockstun && aiHasMotion(p.Char, MotionDP) &&
		aiRoll(r, balance.AIReversalPercent):
		return aiDP

	// Fireball at range, and only with the screen clear of its own.
	case dist > balance.AIMidRange && aiHasProjectile(p.Char) && !s.aiHasProjectile(i) &&
		aiRoll(r, balance.AIProjectilePercent):
		return aiFireball

	// Mid range: close the gap.
	case dist > balance.AICloseRange:
		return aiApproach

	// Close, and they are holding a block.
	case o.State == StateBlockstun && aiRoll(r, balance.AIThrowPercent):
		return aiThrow

	// Close and nothing else applies: press something small, back off, or
	// stand.
	default:
		switch r.RandN(4) {
		case 0:
			return aiJab
		case 1:
			return aiSweep
		case 2:
			return aiRetreat
		}
		return aiNeutral
	}
}

// aiHasMotion reports whether the character owns a move on this motion, and
// aiHasProjectile whether any of its moves spawns one.
func aiHasMotion(char int32, m Motion) bool {
	c := CharacterAt(char)
	for i := int32(0); i < c.NumMoves; i++ {
		if c.Moves[i].Motion == m {
			return true
		}
	}
	return false
}

func aiHasProjectile(char int32) bool {
	c := CharacterAt(char)
	for i := int32(0); i < c.NumMoves; i++ {
		if c.Moves[i].Proj.Exists() {
			return true
		}
	}
	return false
}

// aiTier clamps a difficulty to a tier index, so a hand-built state with a
// nonsense value plays like Normal rather than reading past the array.
func aiTier(ai int32) int32 {
	if ai < AIEasy || ai > AIHard {
		return AINormal - 1
	}
	return ai - 1
}

// aiRoll is a percentage chance.
func aiRoll(r aiRandom, percent int32) bool {
	if percent <= 0 {
		return false
	}
	return int32(r.RandN(100)) < percent
}

// aiSeen models perception latency, which is the honest difficulty lever: the
// AI may not react to something until it has been true for `reaction` frames.
func aiSeen(o *PlayerState, reaction int32) bool { return o.StateFrame >= reaction }

// aiIncoming is an attack that has not yet connected — something still worth
// blocking.
func aiIncoming(o *PlayerState) bool { return o.State == StateAttack && o.HasHit == 0 }

// aiRecovering is an attack past its active frames: the punish window, and the
// only time an attacker cannot block.
func aiRecovering(o *PlayerState) bool {
	if o.State != StateAttack {
		return false
	}
	mv := o.move()
	return mv != nil && o.StateFrame >= mv.Startup+mv.Active
}

// aiDistance is the gap between the two fighters, always positive.
func (s *GameState) aiDistance(i int) Fix {
	d := s.Players[1-i].X - s.Players[i].X
	if d < 0 {
		return -d
	}
	return d
}

// aiHasProjectile reports whether this player already has one on screen.
func (s *GameState) aiHasProjectile(i int) bool {
	for k := range s.Projectiles {
		if s.Projectiles[k].Active != 0 && s.Projectiles[k].Owner == int32(i) {
			return true
		}
	}
	return false
}
