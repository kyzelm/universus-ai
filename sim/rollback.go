package sim

// Rollback parameters, GGPO-standard: 2 frames of input delay (the net layer's
// concern), 8 frames of maximum rollback.
const (
	MaxRollback = 8
	ringSize    = 12

	// The event ring is longer than the state ring on purpose.
	eventRing = 64
)

// Session drives the sim for rollback.
type Session struct {
	state  GameState
	ring   [ringSize]GameState // ring[f%ringSize] is the state at the start of frame f
	inputs [ringSize][2]uint16 // inputs[f%ringSize] is what advanced frame f

	// events[f%eventRing] is what happened on frame f, packed (see events.go).
	events [eventRing]struct {
		frame uint32
		bits  uint32
	}
}

func NewSession() Session { return Session{state: New()} }

// NewSessionOf starts a session from a setup — the lab, the scripted opponent
// in seat 2, the characters (see Setup).
func NewSessionOf(u Setup) Session { return Session{state: NewStateOf(u)} }

func (s *Session) State() *GameState { return &s.state }
func (s *Session) Frame() uint32     { return s.state.Frame }
func (s *Session) Checksum() uint32  { return s.state.Checksum() }

// Advance runs one frame, recording what it took to get there.
func (s *Session) Advance(in [2]uint16) {
	i := s.state.Frame % ringSize
	s.ring[i] = s.state
	s.inputs[i] = in

	f := s.state.Frame
	s.state.Advance(in)

	// Recorded after the frame ran and keyed by the frame it belongs to, so a
	// replay of frame f writes the same slot again rather than a new one.
	e := &s.events[f%eventRing]
	e.frame, e.bits = f, s.state.packEvents()
}

// EventsAt is what happened on a frame, packed — player 0 in the low 16 bits.
func (s *Session) EventsAt(frame uint32) uint32 {
	e := &s.events[frame%eventRing]
	if e.frame != frame {
		return 0
	}
	return e.bits
}

// Rewind restores the state at the start of frame.
func (s *Session) Rewind(frame uint32) bool {
	now := s.state.Frame
	if frame >= now || now-frame > MaxRollback {
		return false
	}
	s.state = s.ring[frame%ringSize] // the rewind is one struct assignment
	return true
}

// Adjust is the single-frame case: the real input for frame turned out to be
// in, so rewind and replay from the inputs already recorded.
func (s *Session) Adjust(frame uint32, in [2]uint16) bool {
	now := s.state.Frame
	if !s.Rewind(frame) {
		return false
	}
	s.inputs[frame%ringSize] = in

	for s.state.Frame < now {
		s.Advance(s.inputs[s.state.Frame%ringSize])
	}
	return true
}
