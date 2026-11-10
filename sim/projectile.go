package sim

// Projectiles.

// MaxProjectiles is the pool for the whole match, both players.
const MaxProjectiles = 4

// Projectile is one thing in flight.
type Projectile struct {
	// Active is 0 for a free slot.
	Active int32

	Owner int32
	// Move indexes the owner's move list.
	Move int32

	X, Y Fix
	VX   Fix
	Life int32
}

// spec is the projectile data of the move that fired it.
func (s *GameState) spec(pr *Projectile) *ProjectileSpec {
	c := CharacterAt(s.Players[pr.Owner].Char)
	if pr.Move < 0 || pr.Move >= c.NumMoves {
		return nil
	}
	return &c.Moves[pr.Move].Proj
}

// ProjectileBox is the world-space hitbox of slot i, or an empty box if the
// slot is free.
func (s *GameState) ProjectileBox(i int) Box {
	pr := &s.Projectiles[i]
	if pr.Active == 0 {
		return Box{}
	}
	sp := s.spec(pr)
	if sp == nil {
		return Box{}
	}
	facing := int32(1)
	if pr.VX < 0 {
		facing = -1
	}
	return sp.Box.World(pr.X, pr.Y, facing)
}

// advanceProjectiles is step 5: move what is in flight, expire it, then spawn
// what this frame's moves produce — in that order, so a fireball does not
// travel on the frame it appears.
func (s *GameState) advanceProjectiles() {
	for i := range s.Projectiles {
		pr := &s.Projectiles[i]
		if pr.Active == 0 {
			continue
		}

		pr.X += pr.VX
		pr.Life--

		// Off the screen or out of life.
		if pr.Life <= 0 || pr.X < s.CamX-balance.CameraHalfWidth || pr.X > s.CamX+balance.CameraHalfWidth {
			pr.Active = 0
		}
	}

	for i := range s.Players {
		p := &s.Players[i]
		mv := p.move()
		// The spawn frame is the move's first active frame, which is data the
		// move already carries.
		if mv == nil || !mv.Proj.Exists() || p.StateFrame != mv.Startup {
			continue
		}
		s.spawn(int32(i), p.MoveIndex, mv)
	}
}

// spawn puts a projectile in the first free slot.
func (s *GameState) spawn(owner, move int32, mv *Move) {
	p := &s.Players[owner]

	for i := range s.Projectiles {
		pr := &s.Projectiles[i]
		if pr.Active != 0 {
			continue
		}
		*pr = Projectile{
			Active: 1,
			Owner:  owner,
			Move:   move,
			X:      p.X + p.forward(mv.Proj.SpawnX),
			Y:      p.Y + mv.Proj.SpawnY,
			VX:     p.forward(mv.Proj.Speed),
			Life:   mv.Proj.Life,
		}
		return
	}
}

// resolveProjectiles is the projectile half of steps 6 and 7.
func (s *GameState) resolveProjectiles(in [2]uint16) {
	var hurts [MaxBoxes]Box

	for i := range s.Projectiles {
		pr := &s.Projectiles[i]
		if pr.Active == 0 {
			continue
		}

		defender := int(1 - pr.Owner)
		box := s.ProjectileBox(i)
		if box.Empty() {
			continue
		}

		n := s.Hurtboxes(defender, &hurts)
		for d := int32(0); d < n; d++ {
			if !box.Overlaps(hurts[d]) {
				continue
			}
			c := CharacterAt(s.Players[pr.Owner].Char)
			mv := &c.Moves[pr.Move]
			// A fireball obeys the juggle limit like anything else, and obeys
			// it *before* it is spent: a projectile that has run out of juggles
			// passes through and stays in flight, the same whiff the move
			// itself would have.
			if s.juggled(defender, mv) {
				break
			}
			s.applyHit(int(pr.Owner), defender, mv, in[defender])
			pr.Active = 0
			break
		}
	}
}
