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

	// EventAttack is a move starting.
	EventAttack int32 = 1 << 5

	// EventFireball is a projectile spawning, on its owner.
	EventFireball int32 = 1 << 6

	// EventJump is a jump starting.
	EventJump int32 = 1 << 7

	// EventLand is touching down from the air.
	EventLand int32 = 1 << 8

	// EventKO is health reaching zero in a fight.
	EventKO int32 = 1 << 9

	// EventFight is a round starting; match-wide, so it rides on player 0 only.
	EventFight int32 = 1 << 10
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
