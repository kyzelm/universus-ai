package sim

// Deterministic randomness.
// ponytail: not math/rand.

// seed is any nonzero constant — the golden-ratio one, chosen only because it
// has a well-mixed bit pattern.
const seed uint32 = 0x9E3779B9

// Rand returns the next value in the sequence and advances the state.
func (s *GameState) Rand() uint32 {
	x := s.RNG
	// Zero is xorshift's fixed point: it would return zero forever.
	if x == 0 {
		x = seed
	}
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	s.RNG = x
	return x
}

// RandN returns a value in [0, n).
func (s *GameState) RandN(n uint32) uint32 {
	if n == 0 {
		return 0 // rather than dividing by zero, which is what % would do
	}
	return uint32((uint64(s.Rand()) * uint64(n)) >> 32)
}
