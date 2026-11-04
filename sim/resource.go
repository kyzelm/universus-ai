package sim

// The two resources, per 03 Game Design/Resource System.md.

// Units. One bar is 1000 units, and a resource is never a float or a fraction
// of a bar — the design note is explicit, so that a fractional cost is an exact
// integer and half of an odd number never has to be rounded.
const (
	BarUnits = 1000

	DriveBars = 6
	SuperBars = 3

	DriveMax = DriveBars * BarUnits
	SuperMax = SuperBars * BarUnits
)

// Balance is the global tunables: costs, regeneration rates and the Burnout
// penalties.
type Balance struct {
	// DriveRegen is the neutral rate, DriveRegenWalkF the faster one for
	// walking forward — moving in is rewarded, and that asymmetry is the whole
	// point of having two rates.
	DriveRegen      int32
	DriveRegenWalkF int32

	// DriveRegenBurnout is the refill rate while burnt out, which is what sets
	// how long Burnout lasts: DriveMax divided by this, in frames.
	DriveRegenBurnout int32

	// PairFrames is how late the second button of a two-button input may land
	// and still be read as the pair.
	PairFrames int32

	// DriveBlockCost is spent per blocked hit.
	DriveBlockCost int32

	// ArmorDamagePercent is how much of a move's damage an absorbed hit still
	// deals to an armoured defender (see Move.Armor).
	ArmorDamagePercent int32

	// BurnoutBlockstun is the extra blockstun a burnt-out defender takes, and
	// BurnoutChipPercent the chip damage a blocked *special* deals to them —
	// the only situation in the game where blocking deals damage (D32).
	BurnoutBlockstun   int32
	BurnoutChipPercent int32

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
	AITiers             [aiTierCount]AITier
	AIDecisionFrames    int32
	AICloseRange        Fix
	AIMidRange          Fix
	AIAntiAirRange      Fix
	AIThrowPercent      int32
	AIProjectilePercent int32
	AIReversalPercent   int32

	// Super is built by dealing damage, by taking it, and by landing a special.
	SuperDealtPercent int32
	SuperTakenPercent int32
	SuperOnSpecial    int32
}

// AITier is one difficulty: how long the AI takes to notice something, how
// often it blocks what it noticed, and how often it throws the rule list away
// and does something else.
type AITier struct {
	Reaction      int32
	BlockPercent  int32
	RandomPercent int32
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
			p.Drive, p.Super = DriveMax, SuperMax
			p.Burnout = 0
		}
		return
	}

	for i := range s.Players {
		p := &s.Players[i]

		p.Drive += p.driveRegen()
		if p.Drive >= DriveMax {
			p.Drive = DriveMax
			// Burnout ends when the gauge is *full*, not when it is above zero.
			p.Burnout = 0
		}
	}
}

// anyoneStunned reports whether either fighter is mid-consequence: hitstun,
// blockstun, a knockdown or a throw.
func (s *GameState) anyoneStunned() bool {
	return stunned(s.Players[0].State) || stunned(s.Players[1].State) ||
		s.Players[0].Combo > 0 || s.Players[1].Combo > 0
}

// driveRegen is this frame's regeneration rate.
func (p *PlayerState) driveRegen() int32 {
	switch {
	case p.Burnout != 0:
		return balance.DriveRegenBurnout
	case p.State == StateBlockstun:
		// Paused while blocking.
		return 0
	case p.State == StateWalkF:
		return balance.DriveRegenWalkF
	default:
		return balance.DriveRegen
	}
}

// spendDrive takes n units and enters Burnout if that empties the gauge.
func (p *PlayerState) spendDrive(n int32) {
	p.Drive -= n
	if p.Drive <= 0 {
		p.Drive = 0
		p.Burnout = 1
	}
}

// gainSuper adds n units, capped.
func (p *PlayerState) gainSuper(n int32) {
	p.Super += n
	if p.Super > SuperMax {
		p.Super = SuperMax
	}
}
