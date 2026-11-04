package sim

// Damage scaling.
//
//	final = base
//	final = final * starterScale    / 100
//	final = final * comboScale[n]   / 100
//	final = max(final, minDamage)
//	final = final * counterHitBonus / 100

// ComboScaleSteps is the length of the scaling table: one entry per hit, and
// the last entry is the floor every hit past it uses.
const ComboScaleSteps = 10

// Counter hit classes, stored on the defender and shown in the HUD — a counter
// hit that is not visible is one nobody learns from.
const (
	CounterNone int32 = iota
	// CounterHit is a hit landed during the opponent's startup or active
	// frames: they were attacking and lost the exchange.
	CounterHit
	// CounterPunish is a hit landed during their recovery — the punish they
	// left open.
	CounterPunish
)

// counterClass reports what kind of hit is about to land on the defender.
func (s *GameState) counterClass(defender int) int32 {
	dp := &s.Players[defender]
	mv := dp.move()
	if mv == nil {
		return CounterNone
	}
	if dp.StateFrame < mv.Startup+mv.Active {
		return CounterHit
	}
	return CounterPunish
}

// starterScale is the multiplier the move that *begins* a combo applies to the
// whole of it.
func starterScale(m *Move) int32 {
	if m.Super > 0 || m.Motion != MotionNone {
		return balance.StarterHeavy
	}
	switch m.Button {
	case InLP, InLK:
		return balance.StarterLight
	case InMP, InMK:
		return balance.StarterMedium
	default:
		return balance.StarterHeavy
	}
}

// comboScale is the multiplier for the nth hit of a combo, 1-based.
func comboScale(n int32) int32 {
	if n < 1 {
		n = 1
	}
	if n > ComboScaleSteps {
		n = ComboScaleSteps
	}
	return balance.ComboScale[n-1]
}

// scaledDamage runs the formula above for a hit landing on the defender.
func scaledDamage(m *Move, starter, hit, counter int32) int32 {
	d := m.Damage
	d = d * starter / 100
	d = d * comboScale(hit) / 100

	// The floor is per-move and proportional: a scaled super doing forty damage
	// reads as a bug to the player holding the controller.
	// ponytail: one global percentage rather than a minDamage field on all
	// twenty-four moves.
	if lo := m.Damage * balance.MinDamagePercent / 100; d < lo {
		d = lo
	}

	if counter != CounterNone {
		d = d * balance.CounterHitPercent / 100
	}
	return d
}

// counterHitstun is the extra hitstun a counter carries.
func counterHitstun(counter int32) int32 {
	switch counter {
	case CounterHit:
		return balance.CounterHitstun
	case CounterPunish:
		return balance.PunishHitstun
	}
	return 0
}

// chip is damage that cannot kill: health floors at 1 rather than 0.
func (p *PlayerState) chip(n int32) {
	p.Health -= n
	if p.Health < 1 {
		p.Health = 1
	}
}
