package sim

// Knockback: what a hit does to the defender's *position*, and the juggle
// system that grows out of it (03 Game Design/Movement and Defense.md, 03 Game
// Design/Damage and Combo Scaling.md).

// Knockback is the velocity the move gives the defender, forward-relative and
// unsigned — the caller turns it away from the attacker.
func (m *Move) Knockback(blocked bool) (Fix, Fix) {
	if blocked {
		return balance.KnockbackBlock, 0
	}
	if m.KnockbackVX == 0 && m.KnockbackVY == 0 {
		return balance.KnockbackHit, 0
	}
	return m.KnockbackVX, m.KnockbackVY
}

// Launcher reports a move that puts the defender in the air.
func (m *Move) Launcher() bool { _, vy := m.Knockback(false); return vy > 0 }

// JuggleLimitOf is the juggle count at which the move stops connecting, the
// balance default standing in for the moves that do not name one.
func (m *Move) JuggleLimitOf() int32 {
	if m.JuggleLimit != 0 {
		return m.JuggleLimit
	}
	return balance.JuggleLimit
}

// applyKnockback pushes the defender away from the attacker.
// ponytail: the attacker is not pushed back except by the corner rule.
func (s *GameState) applyKnockback(attacker, defender int, mv *Move, blocked bool) {
	ap, dp := &s.Players[attacker], &s.Players[defender]

	vx, vy := mv.Knockback(blocked)
	if dp.X < ap.X || (dp.X == ap.X && ap.Facing < 0) {
		vx = -vx
	}
	dp.VX, dp.VY = vx, vy
}

// pushedIntoWall reports a defender whose own pushback drove them into the wall
// that correction d is bringing them back out of.
func pushedIntoWall(p *PlayerState, d Fix) bool {
	if !stunned(p.State) {
		return false
	}
	return (p.VX > 0 && d < 0) || (p.VX < 0 && d > 0)
}

// juggled reports a move that must refuse to connect because the defender has
// taken all the juggle hits it allows.
func (s *GameState) juggled(defender int, mv *Move) bool {
	if mv == nil {
		return false
	}
	p := &s.Players[defender]
	return p.Airborne() && p.Juggle >= mv.JuggleLimitOf()
}

// gravity is the fall this player takes this frame: the character's own,
// increased with every juggle hit so an air combo self-terminates.
func (p *PlayerState) gravity() Fix {
	g := CharacterAt(p.Char).Gravity
	if p.Juggle == 0 {
		return g
	}
	return g + Fix(int64(g)*int64(p.Juggle*balance.JuggleGravityPercent)/100)
}
