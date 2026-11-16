package sim

// The super meter, per 03 Game Design/Resource System.md.

// BarUnits is one bar, so a fractional cost is an exact integer.
const BarUnits = 1000

// Balance is the global tunables.
type Balance struct {
	// PairFrames is how late the second button of a two-button input may land
	// and still be read as the pair.
	PairFrames int32

	// ArmorDamagePercent is how much of a move's damage an absorbed hit still
	// deals to an armoured defender (see Move.Armor).
	ArmorDamagePercent int32

	// ChipPercent is how much of its damage a blocked special, EX or super
	// still deals.
	ChipPercent int32

	// The damage pipeline (see damage.go).
	ComboScale       [ComboScaleSteps]int32
	StarterLight     int32
	StarterMedium    int32
	StarterHeavy     int32
	MinDamagePercent int32

	// A counter hit pays more damage and more hitstun; a punish counter — a hit
	// landed in the opponent's recovery — pays the same damage and more frames
	// still.
	CounterHitPercent int32
	CounterHitstun    int32
	PunishHitstun     int32

	// Round flow (round.go).
	RoundFrames  int32
	RoundsToWin  int32
	MaxRounds    int32
	KOFreeze     int32
	RoundEndHold int32
	IntroFrames  int32

	// Throws (03 Game Design/Movement and Defense.md).
	ThrowTechFrames   int32
	ThrowTechRecovery int32
	ThrowTechPush     int32

	// Knockback (knockback.go).
	KnockbackHit   Fix
	KnockbackBlock Fix
	KnockbackDecay int32

	// Juggles. JuggleLimit is how many hits an airborne defender takes before
	// moves stop connecting, standing in for every move that does not name its
	// own; JuggleGravityPercent is how much heavier each juggle hit makes them
	// fall, which is what makes an air combo self-terminate rather than merely
	// hit a cap.
	JuggleLimit          int32
	JuggleGravityPercent int32

	// KnockdownFrames is how long a knocked-down player is on the ground, and
	// it is deliberately one number: the design note says wakeup timing stays
	// identical for every knockdown type, because varying it is a balance
	// rabbit hole that adds nothing the thesis needs.
	KnockdownFrames int32

	// Half the stage and half the screen, in units.
	StageHalfWidth  Fix
	CameraHalfWidth Fix
	CameraMargin    Fix

	// The scripted opponent (ai.go).
	// AIReaction is how long the AI takes to notice something, AIBlockPercent
	// how often it blocks what it noticed, and AIRandomPercent how often it
	// throws the rule list away.
	AIReaction          int32
	AIBlockPercent      int32
	AIRandomPercent     int32
	AIDecisionFrames    int32
	AICloseRange        Fix
	AIMidRange          Fix
	AIAntiAirRange      Fix
	AIThrowPercent      int32
	AIProjectilePercent int32
	AIReversalPercent   int32

	// SuperMax is the full gauge; Super is built by dealing damage, by taking
	// it, and by landing a special.
	SuperMax          int32
	SuperDealtPercent int32
	SuperTakenPercent int32
	SuperOnSpecial    int32
	// SuperFreeze is how many frames a super stops the match for.
	SuperFreeze int32
}

// The loaded balance.
var balance Balance

// LoadBalance installs the tunables.
func LoadBalance(b Balance) { balance = b }

// BalanceOf returns the loaded tunables.
func BalanceOf() Balance { return balance }

// updateResources is the resource half of step 8.
func (s *GameState) updateResources() {
	// Training mode keeps both fighters topped up — infinite resources, per the
	// mode's own list.
	if s.Training != 0 && !s.anyoneStunned() {
		for i := range s.Players {
			p := &s.Players[i]
			p.Health = CharacterAt(p.Char).Health
			p.Super = balance.SuperMax
		}
	}
}

// anyoneStunned reports whether either fighter is mid-consequence: hitstun,
// blockstun, a knockdown or a throw.
func (s *GameState) anyoneStunned() bool {
	return stunned(s.Players[0].State) || stunned(s.Players[1].State) ||
		s.Players[0].Combo > 0 || s.Players[1].Combo > 0
}

// gainSuper adds n units, capped.
func (p *PlayerState) gainSuper(n int32) {
	p.Super = min(p.Super+n, balance.SuperMax)
}
