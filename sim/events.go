package sim

// Rollback-safe events (02 Architecture/Rollback Netcode.md, 04 Art/Audio.md).
const (
	// EventHit is a clean hit taken — the strike connected and was not blocked.
	EventHit int32 = 1 << 0

	// EventBlock is a hit blocked.
	EventBlock int32 = 1 << 1

	// EventThrown is a throw connecting, on the player being thrown.
	EventThrown int32 = 1 << 2

	// EventKnockdown is hitting the floor.
	EventKnockdown int32 = 1 << 3

	// EventSuper is a super activating, on the player who spent the bars.
	EventSuper int32 = 1 << 4
)

// clearEvents wipes both players' flags.
func (s *GameState) clearEvents() {
	s.Players[0].Events = 0
	s.Players[1].Events = 0
}

// packEvents is both players' flags in one word, player 0 low.
func (s *GameState) packEvents() uint32 {
	return uint32(uint16(s.Players[0].Events)) | uint32(uint16(s.Players[1].Events))<<16
}
