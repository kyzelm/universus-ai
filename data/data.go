// Package data owns the character and balance JSON and converts it into the
// sim's fixed-point structs.
package data

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"universus/sim"
)

//go:embed characters/*.json balance.json
var files embed.FS

// balanceFile is the global tunables.
const balanceFile = "balance.json"

// framesPerSecond is the fixed timestep, and it lives here rather than in the
// sim on purpose: the simulation has no clock but the frame number.
const framesPerSecond = 60

// Load reads, validates and converts the embedded roster.
func Load() ([]sim.Character, error) {
	names, err := filenames()
	if err != nil {
		return nil, err
	}

	out := make([]sim.Character, 0, len(names))
	for _, name := range names {
		raw, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}

		var jc jsonCharacter
		dec := json.NewDecoder(bytes.NewReader(raw))
		// Keeps decimals as their literal text so they can be converted with
		// integer arithmetic.
		dec.UseNumber()
		if err := dec.Decode(&jc); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		c, err := jc.convert()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out = append(out, c)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no character files embedded")
	}
	return out, nil
}

// LoadBalance reads, validates and converts the embedded tunables.
func LoadBalance() (sim.Balance, error) {
	var b sim.Balance

	raw, err := files.ReadFile(balanceFile)
	if err != nil {
		return b, err
	}

	var jb jsonBalance
	if err := json.Unmarshal(raw, &jb); err != nil {
		return b, fmt.Errorf("%s: %w", balanceFile, err)
	}

	b, err = jb.convert()
	if err != nil {
		return b, fmt.Errorf("%s: %w", balanceFile, err)
	}
	return b, nil
}

// Version is a hash of the raw embedded bytes — the value the match handshake
// compares.
func Version() (uint32, error) {
	names, err := filenames()
	if err != nil {
		return 0, err
	}

	h := uint32(2166136261)
	// The balance file is hashed with the roster: a client that tuned the block
	// cost is as desynced as one that tuned a hitbox.
	for _, name := range append(names, balanceFile) {
		raw, err := files.ReadFile(name)
		if err != nil {
			return 0, err
		}
		// The name is hashed too, so renaming a file is a version change.
		for _, b := range append([]byte(name), raw...) {
			h = (h ^ uint32(b)) * 16777619
		}
	}
	return h, nil
}

// filenames returns the embedded files in a fixed order.
func filenames() ([]string, error) {
	entries, err := files.ReadDir("characters")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, "characters/"+e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ---- JSON shapes ----------------------------------------------------------

type jsonCharacter struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	Health int `json:"health"`

	WalkForward json.Number `json:"walkForward"`
	WalkBack    json.Number `json:"walkBack"`

	DashDistance   json.Number `json:"dashDistance"`
	DashFrames     int         `json:"dashFrames"`
	BackdashFrames int         `json:"backdashFrames"`

	JumpVelocity  json.Number `json:"jumpVelocity"`
	JumpForwardVX json.Number `json:"jumpForwardVX"`
	Gravity       json.Number `json:"gravity"`
	PreJumpFrames int         `json:"preJumpFrames"`

	Pushbox    []json.Number `json:"pushbox"`
	StandHurt  []json.Number `json:"standHurt"`
	CrouchHurt []json.Number `json:"crouchHurt"`
	AirHurt    []json.Number `json:"airHurt"`

	Moves []jsonMove `json:"moves"`
}

// jsonBalance groups by resource, because that is how the design note reads and
// how the values are tuned.
type jsonBalance struct {
	Drive struct {
		Regen            int `json:"regen"`
		RegenWalkForward int `json:"regenWalkForward"`
		RegenBurnout     int `json:"regenBurnout"`
		BlockCost        int `json:"blockCost"`
		ArmorDamage      int `json:"armorDamagePercent"`
		BurnoutBlockstun int `json:"burnoutBlockstun"`
		BurnoutChip      int `json:"burnoutChipPercent"`
	} `json:"drive"`

	// Input. The pair window is how late the second button of a two-button
	// input may land and still be read as the pair rather than as the single
	// button that arrived first.
	Input struct {
		PairFrames int `json:"pairFrames"`
	} `json:"input"`

	// Round is authored in seconds, because 99 is the number the design note
	// and every other fighting game state, and 5940 is not.
	Round struct {
		Seconds      int `json:"seconds"`
		RoundsToWin  int `json:"roundsToWin"`
		MaxRounds    int `json:"maxRounds"`
		KOFreeze     int `json:"koFreeze"`
		RoundEndHold int `json:"roundEndHold"`
		Intro        int `json:"intro"`
	} `json:"round"`

	// Throws. The tech window is the design's ~5 frames; the recovery and the
	// push are what both players get out of one, and they are shared because a
	// tech that favoured either side would make the throw free or unusable.
	Throw struct {
		TechFrames   int `json:"techFrames"`
		TechRecovery int `json:"techRecovery"`
		TechPush     int `json:"techPush"`
	} `json:"throw"`

	// Knockdown is one number on purpose — see sim.Balance.
	Knockdown struct {
		Frames int `json:"frames"`
	} `json:"knockdown"`

	// Knockback and juggles (sim/knockback.go).
	Knockback struct {
		Hit          json.Number `json:"hit"`
		Block        json.Number `json:"block"`
		DecayPercent int         `json:"decayPercent"`
	} `json:"knockback"`

	Juggle struct {
		Limit          int `json:"limit"`
		GravityPercent int `json:"gravityPercent"`
	} `json:"juggle"`

	// Stage geometry in whole units (sim.Balance.StageHalfWidth).
	Stage struct {
		HalfWidth       int `json:"halfWidth"`
		CameraHalfWidth int `json:"cameraHalfWidth"`
		CameraMargin    int `json:"cameraMargin"`
	} `json:"stage"`

	Damage struct {
		ComboScale        []int `json:"comboScale"`
		StarterLight      int   `json:"starterLight"`
		StarterMedium     int   `json:"starterMedium"`
		StarterHeavy      int   `json:"starterHeavy"`
		MinDamagePercent  int   `json:"minDamagePercent"`
		CounterHitPercent int   `json:"counterHitPercent"`
		CounterHitstun    int   `json:"counterHitstun"`
		PunishHitstun     int   `json:"punishHitstun"`
	} `json:"damage"`

	Super struct {
		DealtPercent int `json:"dealtPercent"`
		TakenPercent int `json:"takenPercent"`
		OnSpecial    int `json:"onSpecial"`
	} `json:"super"`

	// The scripted opponent (03 Game Design/AI Opponent.md).
	AI struct {
		DecisionFrames    int `json:"decisionFrames"`
		CloseRange        int `json:"closeRange"`
		MidRange          int `json:"midRange"`
		AntiAirRange      int `json:"antiAirRange"`
		ThrowPercent      int `json:"throwPercent"`
		ProjectilePercent int `json:"projectilePercent"`
		ReversalPercent   int `json:"reversalPercent"`
		Tiers             []struct {
			Name          string `json:"name"`
			Reaction      int    `json:"reaction"`
			BlockPercent  int    `json:"blockPercent"`
			RandomPercent int    `json:"randomPercent"`
		} `json:"tiers"`
	} `json:"ai"`
}

// convertAI fills in the scripted opponent's numbers.
func (jb *jsonBalance) convertAI(b *sim.Balance) error {
	// A decision every frame is not a harder opponent, it is a different one:
	// the design note's whole point is that a human cannot re-decide sixty
	// times a second.
	if jb.AI.DecisionFrames < 2 || jb.AI.DecisionFrames > 60 {
		return fmt.Errorf("ai.decisionFrames is %d, want 2..60", jb.AI.DecisionFrames)
	}

	// The ranges are one ordering, like starter scaling: close inside mid, and
	// the anti-air window somewhere inside mid too.
	if jb.AI.CloseRange <= 0 || jb.AI.CloseRange >= jb.AI.MidRange {
		return fmt.Errorf("ai.closeRange %d must be positive and below ai.midRange %d",
			jb.AI.CloseRange, jb.AI.MidRange)
	}
	if jb.AI.AntiAirRange <= 0 || jb.AI.AntiAirRange > jb.AI.MidRange {
		return fmt.Errorf("ai.antiAirRange %d must be positive and at most ai.midRange %d",
			jb.AI.AntiAirRange, jb.AI.MidRange)
	}
	b.AICloseRange = sim.FromInt(jb.AI.CloseRange)
	b.AIMidRange = sim.FromInt(jb.AI.MidRange)
	b.AIAntiAirRange = sim.FromInt(jb.AI.AntiAirRange)

	if len(jb.AI.Tiers) != len(b.AITiers) {
		return fmt.Errorf("ai.tiers has %d entries, want %d", len(jb.AI.Tiers), len(b.AITiers))
	}
	for i, t := range jb.AI.Tiers {
		// Zero reaction is an opponent that blocks everything on the frame it
		// starts, which is the AI nobody enjoys and the one this field exists
		// to prevent.
		if t.Reaction < 1 || t.Reaction > 30 {
			return fmt.Errorf("ai.tiers[%d].reaction is %d, want 1..30", i, t.Reaction)
		}
		if t.BlockPercent < 0 || t.BlockPercent > 100 {
			return fmt.Errorf("ai.tiers[%d].blockPercent is %d, want 0..100", i, t.BlockPercent)
		}
		if t.RandomPercent < 0 || t.RandomPercent > 100 {
			return fmt.Errorf("ai.tiers[%d].randomPercent is %d, want 0..100", i, t.RandomPercent)
		}
		// The tiers are an ordering and not three independent rows: harder
		// means noticing sooner, blocking more and improvising less.
		if i > 0 {
			prev := jb.AI.Tiers[i-1]
			if t.Reaction > prev.Reaction || t.BlockPercent < prev.BlockPercent ||
				t.RandomPercent > prev.RandomPercent {
				return fmt.Errorf("ai.tiers[%d] (%s) is not harder than ai.tiers[%d] (%s)",
					i, t.Name, i-1, prev.Name)
			}
		}
		b.AITiers[i] = sim.AITier{
			Reaction:      int32(t.Reaction),
			BlockPercent:  int32(t.BlockPercent),
			RandomPercent: int32(t.RandomPercent),
		}
	}
	return nil
}

func (jb *jsonBalance) convert() (sim.Balance, error) {
	var b sim.Balance

	for _, f := range []struct {
		name string
		n    int
		dst  *int32
	}{
		{"drive.regen", jb.Drive.Regen, &b.DriveRegen},
		{"drive.regenWalkForward", jb.Drive.RegenWalkForward, &b.DriveRegenWalkF},
		{"drive.regenBurnout", jb.Drive.RegenBurnout, &b.DriveRegenBurnout},
		{"drive.blockCost", jb.Drive.BlockCost, &b.DriveBlockCost},
		{"drive.armorDamagePercent", jb.Drive.ArmorDamage, &b.ArmorDamagePercent},
		{"drive.burnoutBlockstun", jb.Drive.BurnoutBlockstun, &b.BurnoutBlockstun},
		{"drive.burnoutChipPercent", jb.Drive.BurnoutChip, &b.BurnoutChipPercent},
		{"input.pairFrames", jb.Input.PairFrames, &b.PairFrames},
		{"round.seconds", jb.Round.Seconds * framesPerSecond, &b.RoundFrames},
		{"round.roundsToWin", jb.Round.RoundsToWin, &b.RoundsToWin},
		{"round.maxRounds", jb.Round.MaxRounds, &b.MaxRounds},
		{"round.koFreeze", jb.Round.KOFreeze, &b.KOFreeze},
		{"round.roundEndHold", jb.Round.RoundEndHold, &b.RoundEndHold},
		{"round.intro", jb.Round.Intro, &b.IntroFrames},
		{"throw.techFrames", jb.Throw.TechFrames, &b.ThrowTechFrames},
		{"throw.techRecovery", jb.Throw.TechRecovery, &b.ThrowTechRecovery},
		{"throw.techPush", jb.Throw.TechPush, &b.ThrowTechPush},
		{"knockdown.frames", jb.Knockdown.Frames, &b.KnockdownFrames},
		{"knockback.decayPercent", jb.Knockback.DecayPercent, &b.KnockbackDecay},
		{"juggle.limit", jb.Juggle.Limit, &b.JuggleLimit},
		{"juggle.gravityPercent", jb.Juggle.GravityPercent, &b.JuggleGravityPercent},
		{"damage.starterLight", jb.Damage.StarterLight, &b.StarterLight},
		{"damage.starterMedium", jb.Damage.StarterMedium, &b.StarterMedium},
		{"damage.starterHeavy", jb.Damage.StarterHeavy, &b.StarterHeavy},
		{"damage.minDamagePercent", jb.Damage.MinDamagePercent, &b.MinDamagePercent},
		{"damage.counterHitPercent", jb.Damage.CounterHitPercent, &b.CounterHitPercent},
		{"damage.counterHitstun", jb.Damage.CounterHitstun, &b.CounterHitstun},
		{"damage.punishHitstun", jb.Damage.PunishHitstun, &b.PunishHitstun},
		{"super.dealtPercent", jb.Super.DealtPercent, &b.SuperDealtPercent},
		{"super.takenPercent", jb.Super.TakenPercent, &b.SuperTakenPercent},
		{"super.onSpecial", jb.Super.OnSpecial, &b.SuperOnSpecial},
		{"ai.decisionFrames", jb.AI.DecisionFrames, &b.AIDecisionFrames},
		{"ai.throwPercent", jb.AI.ThrowPercent, &b.AIThrowPercent},
		{"ai.projectilePercent", jb.AI.ProjectilePercent, &b.AIProjectilePercent},
		{"ai.reversalPercent", jb.AI.ReversalPercent, &b.AIReversalPercent},
	} {
		if f.n < 0 {
			return b, fmt.Errorf("%s must not be negative, got %d", f.name, f.n)
		}
		*f.dst = int32(f.n)
	}

	var err error

	// The round clock.
	if b.RoundFrames <= 0 {
		return b, fmt.Errorf("round.seconds must be positive, got %d", jb.Round.Seconds)
	}
	if b.RoundsToWin <= 0 {
		return b, fmt.Errorf("round.roundsToWin must be positive, or no round ever wins a match")
	}
	// Coupled, like jumpVelocity against gravity: the cap has to allow the
	// rounds the match needs before it can be decided by damage instead, and
	// neither field says so alone.
	if min := b.RoundsToWin*2 - 1; b.MaxRounds < min {
		return b, fmt.Errorf("round.maxRounds is %d, below the %d rounds a first-to-%d needs",
			b.MaxRounds, min, b.RoundsToWin)
	}

	// A zero window is the behaviour this value exists to replace: the two
	// buttons of a throw or an EX special never land on the same frame on a
	// keyboard, so the first one out selects its own normal and the pair never
	// happens.
	if b.PairFrames <= 0 || b.PairFrames > 8 {
		return b, fmt.Errorf("input.pairFrames must be 1-8, got %d", b.PairFrames)
	}

	// A tech window of zero is a throw nobody can escape, which is the mechanic
	// without the half that makes it fair.
	if b.ThrowTechFrames <= 0 || b.ThrowTechFrames > 20 {
		return b, fmt.Errorf("throw.techFrames is %d, want 1..20", b.ThrowTechFrames)
	}

	// A knockdown of zero frames is a sweep that leaves the defender standing,
	// which reads as a bug in the state machine rather than as a balance value
	// somebody chose.
	if b.KnockdownFrames <= 0 {
		return b, fmt.Errorf("knockdown.frames must be positive, or nothing is ever knocked down")
	}

	// A screen wider than the stage would show past the walls, and a stage
	// with no width puts both players on one pixel.
	if jb.Stage.CameraHalfWidth <= 0 || jb.Stage.HalfWidth < jb.Stage.CameraHalfWidth {
		return b, fmt.Errorf("stage.cameraHalfWidth is %d, want 1..stage.halfWidth (%d)",
			jb.Stage.CameraHalfWidth, jb.Stage.HalfWidth)
	}
	b.StageHalfWidth = sim.FromInt(jb.Stage.HalfWidth)
	b.CameraHalfWidth = sim.FromInt(jb.Stage.CameraHalfWidth)
	// Below a pushbox's half-width the camera never scrolls (see
	// sim.Balance.CameraMargin); past half the screen there is no dead zone
	// left and the camera is the midpoint again.
	if jb.Stage.CameraMargin <= 0 || 2*jb.Stage.CameraMargin >= jb.Stage.CameraHalfWidth {
		return b, fmt.Errorf("stage.cameraMargin is %d, want 1..%d",
			jb.Stage.CameraMargin, jb.Stage.CameraHalfWidth/2-1)
	}
	b.CameraMargin = sim.FromInt(jb.Stage.CameraMargin)

	if b.KnockbackHit, err = parseFix(jb.Knockback.Hit); err != nil {
		return b, fmt.Errorf("knockback.hit: %w", err)
	}
	if b.KnockbackBlock, err = parseFix(jb.Knockback.Block); err != nil {
		return b, fmt.Errorf("knockback.block: %w", err)
	}
	// Zero is a hit that moves nobody, which is the state of the game before
	// pushback existed: a blockstring that never spaces itself out and a corner
	// that does nothing.
	if b.KnockbackHit <= 0 || b.KnockbackBlock <= 0 {
		return b, fmt.Errorf("knockback.hit and knockback.block must be positive")
	}
	// The push is multiplied by this every grounded frame.
	if b.KnockbackDecay >= 100 {
		return b, fmt.Errorf("knockback.decayPercent is %d, want 0..99", b.KnockbackDecay)
	}
	// A limit of zero refuses every hit against an airborne opponent, since the
	// counter starts there — anti-airs would stop working and the cause would
	// be one field in the balance file.
	if b.JuggleLimit <= 0 {
		return b, fmt.Errorf("juggle.limit must be positive, or nothing may be hit in the air")
	}

	// Chip is a fraction of the move's damage, so a value over 100 makes
	// blocking a special worse than eating it.
	if b.BurnoutChipPercent > 100 {
		return b, fmt.Errorf("drive.burnoutChipPercent is %d, want 0..100", b.BurnoutChipPercent)
	}
	if b.DriveBlockCost <= 0 {
		return b, fmt.Errorf("drive.blockCost must be positive, or blocking is free")
	}
	// Walking in is meant to be the faster rate.
	if b.DriveRegenWalkF < b.DriveRegen {
		return b, fmt.Errorf("drive.regenWalkForward %d is below drive.regen %d",
			b.DriveRegenWalkF, b.DriveRegen)
	}

	// Coupled, like jumpVelocity against gravity: the refill rate is what sets
	// how long Burnout lasts, and neither field says so on its own.
	if b.DriveRegenBurnout <= 0 {
		return b, fmt.Errorf("drive.regenBurnout must be positive, or Burnout never ends")
	}
	if f := sim.DriveMax / b.DriveRegenBurnout; f < 60 || f > 1800 {
		return b, fmt.Errorf("drive.regenBurnout %d gives a %d-frame Burnout, want 60..1800",
			b.DriveRegenBurnout, f)
	}

	// The scaling table.
	if len(jb.Damage.ComboScale) != sim.ComboScaleSteps {
		return b, fmt.Errorf("damage.comboScale has %d entries, want %d",
			len(jb.Damage.ComboScale), sim.ComboScaleSteps)
	}
	prev := 101
	for i, v := range jb.Damage.ComboScale {
		if v <= 0 || v > 100 {
			return b, fmt.Errorf("damage.comboScale[%d] is %d, want 1..100", i, v)
		}
		// Scaling that goes back up is a transposed pair, and the symptom is a
		// combo that deals more on its sixth hit than its fifth.
		if v > prev {
			return b, fmt.Errorf("damage.comboScale[%d] is %d, above the %d before it", i, v, prev)
		}
		prev = v
		b.ComboScale[i] = int32(v)
	}

	for _, f := range []struct {
		name string
		v    int32
		lo   int32
		hi   int32
	}{
		{"damage.starterLight", b.StarterLight, 1, 100},
		{"damage.starterMedium", b.StarterMedium, 1, 100},
		{"damage.starterHeavy", b.StarterHeavy, 1, 100},
		{"damage.minDamagePercent", b.MinDamagePercent, 1, 100},
		// A counter hit that pays less than a normal one is the sign flipped.
		{"damage.counterHitPercent", b.CounterHitPercent, 100, 300},
	} {
		if f.v < f.lo || f.v > f.hi {
			return b, fmt.Errorf("%s is %d, want %d..%d", f.name, f.v, f.lo, f.hi)
		}
	}

	// The starters are an ordering, not three independent numbers: a light that
	// scales harder than a heavy inverts what starter scaling is for.
	if b.StarterLight > b.StarterMedium || b.StarterMedium > b.StarterHeavy {
		return b, fmt.Errorf("starter scaling is not ordered light <= medium <= heavy: %d, %d, %d",
			b.StarterLight, b.StarterMedium, b.StarterHeavy)
	}
	// A punish counter is the bigger reward of the two, by definition.
	if b.PunishHitstun < b.CounterHitstun {
		return b, fmt.Errorf("damage.punishHitstun %d is below damage.counterHitstun %d",
			b.PunishHitstun, b.CounterHitstun)
	}

	if err := jb.convertAI(&b); err != nil {
		return b, err
	}

	return b, nil
}

type jsonMove struct {
	ID    string `json:"id"`
	Input struct {
		Stance string `json:"stance"`
		Button string `json:"button"`
		Motion string `json:"motion"`
	} `json:"input"`

	Startup  int `json:"startup"`
	Active   int `json:"active"`
	Recovery int `json:"recovery"`

	Damage    int `json:"damage"`
	Hitstun   int `json:"hitstun"`
	Blockstun int `json:"blockstun"`
	Hitstop   int `json:"hitstop"`

	AttackLevel string `json:"attackLevel"`

	// Recovery owed on touchdown by a move that was still in the air when it
	// ended.
	Landing int `json:"landing"`

	// Invulnerable window, [start, end) in the move's own frames.
	Invuln []int `json:"invuln"`

	// Categories this move may be cancelled into once it has connected.
	Cancel []string `json:"cancel"`

	// After makes the move a follow-up: the ids of the moves it comes out of,
	// inside their cancel window.
	After []string `json:"after"`

	// NeedsHit false lets the follow-up come out of a source that whiffed.
	NeedsHit *bool `json:"needsHit"`

	// Window is [start, end) in the source's frames.
	Window []int `json:"window"`

	// Dir is the numpad direction the button must be pressed with, "1".."9",
	// facing-relative.
	Dir string `json:"dir"`

	// Super level, 1 to 3.
	Super int `json:"super"`

	// Drive bars the move costs: 2 for an EX special.
	Drive int `json:"drive"`

	// Hits the move absorbs while it comes out.
	Armor int `json:"armor"`

	// Knockdown puts the defender on the floor once the hitstun ends.
	Knockdown bool `json:"knockdown"`

	// Throw marks the move unblockable, refused against anyone airborne or in
	// stun, and escapable by a tech.
	Throw bool `json:"throw"`

	// Velocity the move gives the character on its first frame, [vx, vy],
	// forward-relative.
	Launch []json.Number `json:"launch"`

	// Velocity the move gives the *defender* on a clean hit, [vx, vy], away
	// from the attacker.
	Knockback []json.Number `json:"knockback"`

	// The juggle count at which this move stops connecting with an airborne
	// defender — a low one designates a combo ender.
	JuggleLimit int `json:"juggleLimit"`

	Boxes []jsonKeyframe `json:"boxes"`

	// Absent for the moves that do not fire one, which is most of them.
	Projectile *jsonProjectile `json:"projectile"`
}

type jsonProjectile struct {
	Speed json.Number   `json:"speed"`
	Life  int           `json:"life"`
	Spawn []json.Number `json:"spawn"`
	Box   []json.Number `json:"box"`
}

type jsonKeyframe struct {
	Frame int `json:"frame"`
	// HitNum is which hit of a multi-hit move the hitboxes are.
	HitNum int             `json:"hitNum"`
	Hurt   [][]json.Number `json:"hurt"`
	Hit    [][]json.Number `json:"hit"`
}

var buttons = map[string]uint16{
	"LP": sim.InLP, "MP": sim.InMP, "HP": sim.InHP,
	"LK": sim.InLK, "MK": sim.InMK, "HK": sim.InHK,
}

// parseButtons reads "LP" or a combination like "LP+LK".
func parseButtons(spec string) (uint16, error) {
	var mask uint16
	for _, name := range strings.Split(spec, "+") {
		bit, ok := buttons[strings.TrimSpace(name)]
		if !ok {
			return 0, fmt.Errorf("unknown button %q", name)
		}
		mask |= bit
	}
	if mask == 0 {
		return 0, fmt.Errorf("move has no button")
	}
	return mask, nil
}

var levels = map[string]int32{
	"mid": sim.LevelMid, "high": sim.LevelHigh, "low": sim.LevelLow,
	// An overhead is a high that must be blocked standing; the distinction is
	// startup, which is already in the frame data.
	"overhead": sim.LevelHigh,
}

var cancelCategories = map[string]uint16{
	"chain": sim.CancelChain, "special": sim.CancelSpecial,
	"super1": sim.CancelSuper1, "super2": sim.CancelSuper2, "super3": sim.CancelSuper3,
}

var stances = map[string]int32{
	"stand": sim.StanceStand, "crouch": sim.StanceCrouch, "air": sim.StanceAir,
}

// Motions a move can require.
var inputMotions = map[string]sim.Motion{
	"": sim.MotionNone, "qcf": sim.MotionQCF, "qcb": sim.MotionQCB, "dp": sim.MotionDP,
	"qcfx2": sim.MotionQCFx2, "qcbx2": sim.MotionQCBx2, "hcf": sim.MotionHCF,
}

// ---- conversion and validation --------------------------------------------

// Validation is not optional.
func (jc *jsonCharacter) convert() (sim.Character, error) {
	var c sim.Character
	var err error

	// The upper bound is not a design opinion, it is arithmetic: a timeout is
	// decided by cross-multiplying the two players' health against each other's
	// maximum, and two values above 46 340 overflow the int32 that holds the
	// product.
	if jc.Name == "" {
		return c, fmt.Errorf("no name")
	}
	c.Name = jc.Name

	if jc.Health <= 0 || jc.Health > 30000 {
		return c, fmt.Errorf("health is %d, want 1..30000", jc.Health)
	}
	c.Health = int32(jc.Health)

	for _, f := range []struct {
		name string
		src  json.Number
		dst  *sim.Fix
	}{
		{"walkForward", jc.WalkForward, &c.WalkForward},
		{"walkBack", jc.WalkBack, &c.WalkBack},
		{"dashDistance", jc.DashDistance, &c.DashDistance},
		{"jumpVelocity", jc.JumpVelocity, &c.JumpVelocity},
		{"jumpForwardVX", jc.JumpForwardVX, &c.JumpForwardVX},
		{"gravity", jc.Gravity, &c.Gravity},
	} {
		if *f.dst, err = parseFix(f.src); err != nil {
			return c, fmt.Errorf("%s: %w", f.name, err)
		}
	}

	if c.Gravity >= 0 {
		return c, fmt.Errorf("gravity must be negative (down), got %d", c.Gravity)
	}
	if c.JumpVelocity <= 0 {
		return c, fmt.Errorf("jumpVelocity must be positive, got %d", c.JumpVelocity)
	}

	// Coupled constants, checked against each other rather than individually.
	if air := 2 * int64(c.JumpVelocity) / int64(-c.Gravity); air < 20 || air > 90 {
		return c, fmt.Errorf("jumpVelocity %d against gravity %d gives a %d-frame jump, want 20..90",
			c.JumpVelocity, c.Gravity, air)
	}

	for _, f := range []struct {
		name string
		n    int
		dst  *int32
	}{
		{"dashFrames", jc.DashFrames, &c.DashFrames},
		{"backdashFrames", jc.BackdashFrames, &c.BackdashFrames},
		{"preJumpFrames", jc.PreJumpFrames, &c.PreJumpFrames},
	} {
		if f.n <= 0 {
			return c, fmt.Errorf("%s must be positive, got %d", f.name, f.n)
		}
		*f.dst = int32(f.n)
	}

	for _, f := range []struct {
		name string
		src  []json.Number
		dst  *sim.Box
	}{
		{"pushbox", jc.Pushbox, &c.Pushbox},
		{"standHurt", jc.StandHurt, &c.StandHurt},
		{"crouchHurt", jc.CrouchHurt, &c.CrouchHurt},
		{"airHurt", jc.AirHurt, &c.AirHurt},
	} {
		if *f.dst, err = parseBox(f.src); err != nil {
			return c, fmt.Errorf("%s: %w", f.name, err)
		}
	}

	if len(jc.Moves) == 0 {
		return c, fmt.Errorf("character has no moves")
	}
	if len(jc.Moves) > sim.MaxMoves {
		return c, fmt.Errorf("%d moves, max %d", len(jc.Moves), sim.MaxMoves)
	}
	// Throws with a motion are command throws and cannot be teched; the one
	// without a motion is the ordinary throw, and it is also the input the sim
	// reads to decide whether *this* character can tech at all.
	plainThrows := 0
	for i := range jc.Moves {
		m, err := jc.Moves[i].convert()
		if err != nil {
			return c, fmt.Errorf("move %q: %w", jc.Moves[i].ID, err)
		}
		if m.IsThrow() && m.Motion == sim.MotionNone {
			plainThrows++
		}
		c.Moves[i] = m
	}
	if plainThrows > 1 {
		return c, fmt.Errorf("%d throws with no motion: only one can be the tech input", plainThrows)
	}
	c.NumMoves = int32(len(jc.Moves))

	// Target combos are resolved here, once every id has an index.
	index := make(map[string]int32, len(jc.Moves))
	for i := range jc.Moves {
		if _, dup := index[jc.Moves[i].ID]; dup {
			return c, fmt.Errorf("move id %q appears twice", jc.Moves[i].ID)
		}
		index[jc.Moves[i].ID] = int32(i)
	}
	for i := range jc.Moves {
		jm, m := &jc.Moves[i], &c.Moves[i]
		if len(jm.After) == 0 {
			if jm.NeedsHit != nil || jm.Window != nil || jm.Dir != "" {
				return c, fmt.Errorf("move %q: needsHit, window and dir are for follow-ups (after)", jm.ID)
			}
			continue
		}
		if m.Motion != sim.MotionNone || m.Super > 0 || m.IsThrow() {
			// A follow-up is a normal; a special or super already has its own
			// cancel categories, and being both would make Category lie.
			return c, fmt.Errorf("move %q: only a plain normal can be a follow-up", jm.ID)
		}
		if jm.NeedsHit != nil && !*jm.NeedsHit {
			m.Whiff = 1
		}
		if jm.Dir != "" {
			d := jm.Dir
			if len(d) != 1 || d[0] < '1' || d[0] > '9' || d == "5" {
				return c, fmt.Errorf("move %q: dir %q, want a numpad direction 1..9 other than 5", jm.ID, d)
			}
			m.Dir = d[0] - '0'
		}
		for _, after := range jm.After {
			src, ok := index[after]
			switch {
			case !ok:
				return c, fmt.Errorf("move %q: after %q, which is not a move", jm.ID, after)
			case src == int32(i):
				// The one cycle that is reachable: anything longer has no way in.
				return c, fmt.Errorf("move %q follows itself", jm.ID)
			}
			m.After |= 1 << uint32(src)
			c.Moves[src].CancelInto |= sim.CancelTarget

			if len(jm.Window) != 0 {
				if len(jm.Window) != 2 || jm.Window[0] < 0 || jm.Window[1] <= jm.Window[0] {
					return c, fmt.Errorf("move %q: window wants a non-empty [start, end)", jm.ID)
				}
				// Checked against every source: a window past the end of one of
				// them is a follow-up that never comes out of it.
				if total := c.Moves[src].Total(); int32(jm.Window[1]) > total {
					return c, fmt.Errorf("move %q: window ends at %d, but %q is %d frames", jm.ID, jm.Window[1], after, total)
				}
				m.WindowStart, m.WindowEnd = int32(jm.Window[0]), int32(jm.Window[1])
			}
		}
	}

	return c, nil
}

func (jm *jsonMove) convert() (sim.Move, error) {
	var m sim.Move

	if jm.Startup <= 0 {
		return m, fmt.Errorf("startup must be positive, got %d", jm.Startup)
	}
	if jm.Active <= 0 {
		return m, fmt.Errorf("active must be positive, got %d", jm.Active)
	}
	if jm.Recovery < 0 {
		return m, fmt.Errorf("recovery must not be negative, got %d", jm.Recovery)
	}
	if jm.Damage < 0 {
		return m, fmt.Errorf("damage must not be negative, got %d", jm.Damage)
	}
	if jm.Hitstun < 0 || jm.Blockstun < 0 || jm.Hitstop < 0 {
		return m, fmt.Errorf("hitstun/blockstun/hitstop must not be negative")
	}

	m.Startup = int32(jm.Startup)
	m.Active = int32(jm.Active)
	m.Recovery = int32(jm.Recovery)
	m.Damage = int32(jm.Damage)
	m.Hitstun = int32(jm.Hitstun)
	m.Blockstun = int32(jm.Blockstun)
	m.Hitstop = int32(jm.Hitstop)

	button, err := parseButtons(jm.Input.Button)
	if err != nil {
		return m, err
	}
	m.Button = button

	var ok bool
	if m.Stance, ok = stances[jm.Input.Stance]; !ok {
		return m, fmt.Errorf("unknown stance %q", jm.Input.Stance)
	}
	if m.Level, ok = levels[jm.AttackLevel]; !ok {
		return m, fmt.Errorf("unknown attackLevel %q", jm.AttackLevel)
	}
	if m.Motion, ok = inputMotions[jm.Input.Motion]; !ok {
		return m, fmt.Errorf("unknown motion %q", jm.Input.Motion)
	}

	if jm.Landing < 0 {
		return m, fmt.Errorf("landing must not be negative, got %d", jm.Landing)
	}
	m.Landing = int32(jm.Landing)

	if len(jm.Invuln) != 0 {
		if len(jm.Invuln) != 2 {
			return m, fmt.Errorf("invuln wants [start, end), got %d values", len(jm.Invuln))
		}
		start, end := jm.Invuln[0], jm.Invuln[1]
		// Half-open and non-empty: an authored window that contains no frames
		// is a move someone believes is invincible and is not.
		if start < 0 || end <= start {
			return m, fmt.Errorf("invuln [%d, %d) is empty or negative", start, end)
		}
		if int32(end) > m.Total() {
			return m, fmt.Errorf("invuln ends at frame %d, but the move is %d frames", end, m.Total())
		}
		m.InvulnStart, m.InvulnEnd = int32(start), int32(end)
	}

	if jm.Super < 0 || jm.Super > 3 {
		return m, fmt.Errorf("super must be 1, 2 or 3, got %d", jm.Super)
	}
	m.Super = int32(jm.Super)

	// A super with no motion is a super on a bare button press, which would
	// beat the normal on that button for the rest of the match.
	if m.Super > 0 && m.Motion == sim.MotionNone {
		return m, fmt.Errorf("super %d has no motion", m.Super)
	}

	// The gauge has six bars, so a move that costs more than six can never come
	// out — and it would fail silently, by never being selected.
	if jm.Drive < 0 || jm.Drive > sim.DriveBars {
		return m, fmt.Errorf("drive must be 0..%d bars, got %d", sim.DriveBars, jm.Drive)
	}
	m.Drive = int32(jm.Drive)

	if jm.Armor < 0 {
		return m, fmt.Errorf("armor must not be negative, got %d", jm.Armor)
	}
	m.Armor = int32(jm.Armor)

	// Armour must be paid for, and there are three currencies.
	if m.Armored() && m.Drive == 0 && m.Super == 0 && m.Motion == sim.MotionNone {
		return m, fmt.Errorf("armor %d on a plain normal: it costs no drive, no super level and no motion", m.Armor)
	}

	if jm.Knockdown {
		m.Knockdown = 1
	}

	if jm.Throw {
		m.Throw = 1
		// A throw is decided entirely by its hitstun and its reach: it cannot
		// be blocked, so blockstun is a value nobody will ever read, and a
		// throw that let go instantly would be a hit with no consequence.
		if m.Hitstun <= 0 {
			return m, fmt.Errorf("throw has no hitstun, so it releases immediately")
		}
	}

	for _, name := range jm.Cancel {
		bit, ok := cancelCategories[name]
		if !ok {
			return m, fmt.Errorf("unknown cancel category %q", name)
		}
		m.CancelInto |= bit
	}

	if len(jm.Boxes) == 0 {
		return m, fmt.Errorf("no box keyframes")
	}
	if len(jm.Boxes) > sim.MaxKeyframes {
		return m, fmt.Errorf("%d box keyframes, max %d", len(jm.Boxes), sim.MaxKeyframes)
	}

	prev, lastHit := int32(-1), int32(1)
	for i := range jm.Boxes {
		k, err := jm.Boxes[i].convert()
		if err != nil {
			return m, fmt.Errorf("box frame %d: %w", jm.Boxes[i].Frame, err)
		}
		// BoxesAt scans forward and stops at the first entry past the frame it
		// wants, so out-of-order keyframes would silently never apply.
		if k.Frame <= prev {
			return m, fmt.Errorf("box keyframes must be in ascending frame order, %d after %d", k.Frame, prev)
		}
		if k.Frame >= m.Total() {
			return m, fmt.Errorf("box keyframe at frame %d, but the move is %d frames", k.Frame, m.Total())
		}
		prev = k.Frame
		// Hits are numbered in the order they come out.
		if k.NumHit > 0 {
			if k.HitNumber() < lastHit {
				return m, fmt.Errorf("box frame %d: hitNum %d after %d", k.Frame, k.HitNumber(), lastHit)
			}
			lastHit = k.HitNumber()
		}
		m.Keys[i] = k
	}
	m.NumKeys = int32(len(jm.Boxes))

	if len(jm.Launch) != 0 {
		if len(jm.Launch) != 2 {
			return m, fmt.Errorf("launch wants [vx, vy], got %d values", len(jm.Launch))
		}
		vx, err := parseFix(jm.Launch[0])
		if err != nil {
			return m, fmt.Errorf("launch vx: %w", err)
		}
		vy, err := parseFix(jm.Launch[1])
		if err != nil {
			return m, fmt.Errorf("launch vy: %w", err)
		}
		m.LaunchVX, m.LaunchVY = vx, vy
	}

	if len(jm.Knockback) != 0 {
		if len(jm.Knockback) != 2 {
			return m, fmt.Errorf("knockback wants [vx, vy], got %d values", len(jm.Knockback))
		}
		vx, err := parseFix(jm.Knockback[0])
		if err != nil {
			return m, fmt.Errorf("knockback vx: %w", err)
		}
		vy, err := parseFix(jm.Knockback[1])
		if err != nil {
			return m, fmt.Errorf("knockback vy: %w", err)
		}
		// An authored [0, 0] is the balance default spelled in a way that reads
		// as "this move pushes nobody", and the two are not the same claim.
		if vx == 0 && vy == 0 {
			return m, fmt.Errorf("knockback [0, 0] is the balance default; omit it instead")
		}
		m.KnockbackVX, m.KnockbackVY = vx, vy
	}

	if jm.JuggleLimit < 0 {
		return m, fmt.Errorf("juggleLimit must not be negative, got %d", jm.JuggleLimit)
	}
	m.JuggleLimit = int32(jm.JuggleLimit)

	if jm.Projectile != nil {
		p, err := jm.Projectile.convert()
		if err != nil {
			return m, fmt.Errorf("projectile: %w", err)
		}
		m.Proj = p
	}

	// A hitbox outside the active window can never connect, which means the
	// frame data and the boxes disagree and one of them is a typo.
	for i := int32(0); i < m.NumKeys; i++ {
		k := &m.Keys[i]
		if k.NumHit == 0 {
			continue
		}
		if k.Frame < m.Startup || k.Frame >= m.Startup+m.Active {
			return m, fmt.Errorf("hitbox at frame %d is outside the active window %d..%d",
				k.Frame, m.Startup, m.Startup+m.Active-1)
		}
	}

	return m, nil
}

func (jp *jsonProjectile) convert() (sim.ProjectileSpec, error) {
	var p sim.ProjectileSpec
	var err error

	if p.Speed, err = parseFix(jp.Speed); err != nil {
		return p, fmt.Errorf("speed: %w", err)
	}
	if p.Speed <= 0 {
		return p, fmt.Errorf("speed must be positive, got %s", jp.Speed)
	}
	// Life is what marks a move as firing a projectile at all, so zero here is
	// not "a very short fireball", it is a move that silently does nothing.
	if jp.Life <= 0 {
		return p, fmt.Errorf("life must be positive, got %d", jp.Life)
	}
	p.Life = int32(jp.Life)

	if len(jp.Spawn) != 2 {
		return p, fmt.Errorf("spawn wants [x, y], got %d values", len(jp.Spawn))
	}
	if p.SpawnX, err = parseFix(jp.Spawn[0]); err != nil {
		return p, fmt.Errorf("spawn x: %w", err)
	}
	if p.SpawnY, err = parseFix(jp.Spawn[1]); err != nil {
		return p, fmt.Errorf("spawn y: %w", err)
	}

	if p.Box, err = parseBox(jp.Box); err != nil {
		return p, fmt.Errorf("box: %w", err)
	}
	return p, nil
}

func (jk *jsonKeyframe) convert() (sim.Keyframe, error) {
	var k sim.Keyframe

	if jk.Frame < 0 {
		return k, fmt.Errorf("negative frame")
	}
	k.Frame = int32(jk.Frame)

	if len(jk.Hurt) > sim.MaxBoxes || len(jk.Hit) > sim.MaxBoxes {
		return k, fmt.Errorf("more than %d boxes", sim.MaxBoxes)
	}

	for i, raw := range jk.Hurt {
		b, err := parseBox(raw)
		if err != nil {
			return k, fmt.Errorf("hurt[%d]: %w", i, err)
		}
		k.Hurt[i] = b
	}
	k.NumHurt = int32(len(jk.Hurt))

	for i, raw := range jk.Hit {
		b, err := parseBox(raw)
		if err != nil {
			return k, fmt.Errorf("hit[%d]: %w", i, err)
		}
		k.Hit[i] = b
	}
	k.NumHit = int32(len(jk.Hit))

	if jk.HitNum < 0 || (jk.HitNum > 0 && k.NumHit == 0) {
		return k, fmt.Errorf("hitNum %d needs hitboxes and a positive number", jk.HitNum)
	}
	k.HitNum = int32(jk.HitNum)

	return k, nil
}

// parseBox reads [x, y, width, height].
func parseBox(raw []json.Number) (sim.Box, error) {
	var b sim.Box
	if len(raw) != 4 {
		return b, fmt.Errorf("want [x, y, w, h], got %d values", len(raw))
	}

	var err error
	dst := [...]*sim.Fix{&b.X, &b.Y, &b.W, &b.H}
	for i, n := range raw {
		if *dst[i], err = parseFix(n); err != nil {
			return b, err
		}
	}

	// A zero-size box silently never collides, which reads as a move that
	// mysteriously does not work rather than as the typo it is.
	if b.W <= 0 || b.H <= 0 {
		return b, fmt.Errorf("width and height must be positive, got %d x %d", b.W, b.H)
	}
	return b, nil
}

// parseFix converts a decimal literal to 16.16 using integer arithmetic only.
func parseFix(n json.Number) (sim.Fix, error) {
	s := n.String()
	if s == "" {
		return 0, fmt.Errorf("missing value")
	}

	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}

	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}

	// strconv.ParseInt accepts its own sign, so "--1" would strip one here and
	// the other there and come out positive.
	if !digitsOnly(whole) || (hasFrac && !digitsOnly(frac)) {
		return 0, fmt.Errorf("bad number %q", n.String())
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad number %q", n.String())
	}
	// Bounded before the shift below, which would otherwise overflow int64 and
	// land on a plausible-looking value rather than failing.
	if units > 1<<20 || units < -(1<<20) {
		return 0, fmt.Errorf("%q is out of range for 16.16", n.String())
	}

	var scaled int64
	if hasFrac {
		if frac == "" {
			return 0, fmt.Errorf("bad number %q", n.String())
		}
		// More digits than this cannot survive 1/65536 of a unit anyway, and
		// the intermediate below has to stay inside int64.
		if len(frac) > 9 {
			frac = frac[:9]
		}
		digits, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("bad number %q", n.String())
		}
		den := int64(1)
		for range len(frac) {
			den *= 10
		}
		// Round half away from zero, in integers: (2*num/den + 1) / 2.
		scaled = ((digits<<(sim.FracBits+1))/den + 1) / 2
	}

	v := units<<sim.FracBits + scaled
	if neg {
		v = -v
	}
	// 16.16 holds ±32768.
	if v > (1<<31)-1 || v < -(1<<31) {
		return 0, fmt.Errorf("%q is out of range for 16.16", n.String())
	}
	return sim.Fix(v), nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
