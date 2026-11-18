package sim

// The render snapshot: everything the view needs to draw a frame, and nothing else.
const (
	boxSize = 4 * 4
	// headerSize covers the match-level fields above.
	headerSize         = 9 * 4
	PlayerSnapshotSize = 11*4 + boxSize + 2*(4+MaxBoxes*boxSize)
	projOffset         = headerSize + 2*PlayerSnapshotSize
	SnapshotSize       = projOffset + 4 + MaxProjectiles*boxSize
)

// ponytail: hand-rolled little-endian stores.
func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func putI32(b []byte, v int32) { putU32(b, uint32(v)) }

func b2i(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

func putBox(b []byte, x Box) {
	putI32(b, int32(x.X))
	putI32(b[4:], int32(x.Y))
	putI32(b[8:], int32(x.W))
	putI32(b[12:], int32(x.H))
}

// WriteSnapshot packs the render snapshot into b, which must be at least SnapshotSize bytes.
func (s *GameState) WriteSnapshot(b []byte) {
	_ = b[SnapshotSize-1] // bounds check once, up front

	// Unused box slots are not written below, so without this they keep last frame's values.
	clear(b[:SnapshotSize])

	putU32(b, s.Frame)
	putI32(b[4:], int32(s.CamX))
	putI32(b[8:], s.Hitstop)

	// The round. The timer crosses in frames, not seconds: the view divides, because the view is
	// the only place allowed a number that is not exact.
	putI32(b[12:], s.Phase)
	putI32(b[16:], s.Timer)
	putI32(b[20:], s.Wins[0])
	putI32(b[24:], s.Wins[1])
	putI32(b[28:], s.RoundWinner)
	putI32(b[32:], s.Winner)

	var boxes [MaxBoxes]Box
	for i := range s.Players {
		p := &s.Players[i]
		o := b[headerSize+i*PlayerSnapshotSize:]

		putI32(o, int32(p.X))
		putI32(o[4:], int32(p.Y))
		putI32(o[8:], p.Facing)
		putI32(o[12:], p.MoveIndex)
		putI32(o[16:], p.State)
		putI32(o[20:], p.StateFrame)
		putI32(o[24:], p.Health)
		putI32(o[28:], p.Super)
		// The combo counter and the counter-hit class.
		putI32(o[32:], p.Combo)
		putI32(o[36:], p.Counter)
		// Whether this player may act this frame, which is the sim's own predicate and not a state
		// list the view keeps beside it.
		putI32(o[40:], b2i(Actionable(p.State)))
		putBox(o[44:], s.Pushbox(i))

		n := s.Hurtboxes(i, &boxes)
		putU32(o[60:], uint32(n))
		for k := int32(0); k < n; k++ {
			putBox(o[64+int(k)*boxSize:], boxes[k])
		}

		hitOff := 64 + MaxBoxes*boxSize
		n = s.Hitboxes(i, &boxes)
		putU32(o[hitOff:], uint32(n))
		for k := int32(0); k < n; k++ {
			putBox(o[hitOff+4+int(k)*boxSize:], boxes[k])
		}
	}

	// Projectiles are packed from the front, not by slot.
	live := 0
	for i := range s.Projectiles {
		pb := s.ProjectileBox(i)
		if pb.Empty() {
			continue
		}
		putBox(b[projOffset+4+live*boxSize:], pb)
		live++
	}
	putU32(b[projOffset:], uint32(live))
}
