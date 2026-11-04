package sim

// Frame-accurate input interpretation.

// InputHistory is the ring length, in frames.
const InputHistory = 32

// InputBuffer is how long a button press stays live looking for a state that
// can act on it.
const InputBuffer = 6

// Direction in numpad notation, relative to the player's facing.
//
//	7 8 9
//	4 5 6      6 is forward, 4 is back, 5 is neutral
//	1 2 3
const (
	DirDownBack = 1 + iota
	DirDown
	DirDownFwd
	DirBack
	DirNeutral
	DirFwd
	DirUpBack
	DirUp
	DirUpFwd
)

// Motion is a recognised directional sequence.
type Motion uint8

const (
	MotionNone Motion = iota
	MotionQCF         // ↓ ↘ →
	MotionQCB         // ↓ ↙ ←
	MotionDP          // → ↓ ↘

	// The super motions: the same quarter-circle twice.
	MotionQCFx2 // ↓ ↘ → ↓ ↘ →
	MotionQCBx2 // ↓ ↙ ← ↓ ↙ ←

	MotionHCF // ← ↙ ↓ ↘ → — the grappler's command throw
)

// satisfies reports whether a recognised motion also counts as a simpler one it
// contains.
func (m Motion) satisfies(want Motion) bool {
	if m == want {
		return true
	}
	switch m {
	case MotionQCFx2:
		return want == MotionQCF
	case MotionQCBx2:
		return want == MotionQCB
	case MotionHCF:
		// A half-circle forward ends in a quarter-circle forward, and this line
		// is what keeps the table's new row from changing anything for a
		// character that has no half-circle move.
		return want == MotionQCF
	}
	return false
}

// The motion table, in priority order — the first match wins.
var motions = []struct {
	motion Motion
	seq    []uint8
	window uint32
	tight  bool
}{
	{MotionQCFx2, []uint8{DirDown, DirDownFwd, DirFwd, DirDown, DirDownFwd, DirFwd}, 26, false},
	{MotionQCBx2, []uint8{DirDown, DirDownBack, DirBack, DirDown, DirDownBack, DirBack}, 26, false},
	{MotionDP, []uint8{DirFwd, DirDown, DirDownFwd}, 13, false},
	{MotionDP, []uint8{DirFwd, DirDown, DirFwd}, 13, true}, // the shortcut: no diagonal
	{MotionHCF, []uint8{DirBack, DirDownBack, DirDown, DirDownFwd, DirFwd}, 20, false},
	{MotionQCB, []uint8{DirDown, DirDownBack, DirBack}, 13, false},
	{MotionQCF, []uint8{DirDown, DirDownFwd, DirFwd}, 13, false},
}

// direction converts a bitfield to facing-relative numpad.
func direction(bits uint16, facing int32) uint8 {
	var h, v int32
	if bits&InRight != 0 {
		h += facing // facing right, right is forward
	}
	if bits&InLeft != 0 {
		h -= facing
	}
	if bits&InUp != 0 {
		v++
	}
	if bits&InDown != 0 {
		v--
	}
	// The numpad *is* this formula: 5 is neutral, forward adds 1, up adds 3.
	return uint8(5 + h + 3*v)
}

// at returns the bitfield from frame now-back, or 0 when that frame predates
// the history.
func (p *PlayerState) at(now, back uint32) uint16 {
	if back > now || back >= InputHistory {
		return 0
	}
	return uint16(p.Inputs[(now-back)%InputHistory])
}

// Every query below comes in two forms, and the difference is not cosmetic.

// latest is the newest recorded frame, from outside Advance.
func (s *GameState) latest() uint32 {
	if s.Frame == 0 {
		return 0
	}
	return s.Frame - 1
}

// Direction is the player's current facing-relative direction.
func (s *GameState) Direction(player int) uint8 {
	return s.DirectionAt(player, s.latest())
}

func (s *GameState) DirectionAt(player int, now uint32) uint8 {
	p := &s.Players[player]
	return direction(p.at(now, 0), p.Facing)
}

// Motion returns the highest-priority motion the player has just completed, or
// MotionNone.
func (s *GameState) Motion(player int) Motion {
	return s.MotionAt(player, s.latest())
}

func (s *GameState) MotionAt(player int, now uint32) Motion {
	p := &s.Players[player]
	for _, m := range motions {
		if p.scan(now, m.seq, m.window, m.tight) {
			return m.motion
		}
	}
	return MotionNone
}

func (p *PlayerState) matches(now uint32, seq []uint8, window uint32) bool {
	return p.scan(now, seq, window, false)
}

// scan walks the history backward looking for seq's elements in reverse order.
func (p *PlayerState) scan(now uint32, seq []uint8, window uint32, tight bool) bool {
	k := len(seq) - 1
	for back := uint32(0); back < window && back <= now; back++ {
		dir := direction(p.at(now, back), p.Facing)
		if dir == seq[k] {
			k--
			if k < 0 {
				return true
			}
			continue
		}

		// k+1 == len(seq) is "nothing taken yet", so the element the row ends on
		// has to be the newest input that is not neutral: a tight shortcut reads
		// the motion the player just made, not one they made and then left.
		if tight && dir != DirNeutral && (k+1 == len(seq) || dir != seq[k+1]) {
			return false
		}
	}
	return false
}

// Pressed reports the press edge: held this frame, not held the frame before.
func (s *GameState) Pressed(player int, button uint16) bool {
	return s.PressedAt(player, button, s.latest())
}

func (s *GameState) PressedAt(player int, button uint16, now uint32) bool {
	p := &s.Players[player]
	return pressed(button, p.at(now, 0), p.at(now, 1))
}

// pressed is the edge rule, and it is written for a mask rather than a single
// bit: every bit down now, and not every bit down the frame before.
func pressed(mask, now, before uint16) bool {
	return mask != 0 && now&mask == mask && before&mask != mask
}

// pressFrame is the newest frame within the buffer window on which button was
// freshly pressed, or -1.
func (p *PlayerState) pressFrame(now uint32, button uint16) int32 {
	return p.pressWithin(now, button, InputBuffer)
}

// pressWithin is pressFrame over an arbitrary window.
func (p *PlayerState) pressWithin(now uint32, button uint16, window uint32) int32 {
	for back := uint32(0); back < window && back <= now; back++ {
		if pressed(button, p.at(now, back), p.at(now, back+1)) {
			return int32(now - back)
		}
	}
	return -1
}

// Buffered reports a press edge within the last InputBuffer frames, which is
// what makes cancels and links feel responsive instead of punishing: the press
// stays live briefly, looking for a state that can act on it.
func (s *GameState) Buffered(player int, button uint16) bool {
	return s.BufferedAt(player, button, s.latest())
}

func (s *GameState) BufferedAt(player int, button uint16, now uint32) bool {
	return s.Players[player].pressFrame(now, button) >= 0
}
