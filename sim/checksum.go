package sim

import "unsafe"

// Save and load need no code: GameState is flat and comparable, so `saved := s`
// and `s = saved` already are the memcpy.

const (
	fnvOffset = 2166136261
	fnvPrime  = 16777619
)

func fnv1a(b []byte) uint32 {
	h := uint32(fnvOffset)
	for _, c := range b {
		h ^= uint32(c)
		h *= fnvPrime
	}
	return h
}

// Checksum is FNV-1a over the raw bytes of the state.
// ponytail: no separate packed representation.
func (s *GameState) Checksum() uint32 {
	return fnv1a(unsafe.Slice((*byte)(unsafe.Pointer(s)), unsafe.Sizeof(*s)))
}
