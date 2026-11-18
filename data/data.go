// Package data owns the character and balance JSON and decodes it into the
// sim's fixed-point structs.
package data

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"universus/sim"
)

//go:embed characters/*.json balance.json
var files embed.FS

const balanceFile = "balance.json"

// framesPerSecond lives here, not in the sim: the sim's only clock is the frame number.
const framesPerSecond = 60

// The JSON keys are the sim's field names; these wrappers hold only the fields that differ.
type character struct {
	sim.Character
	ID      int
	Comment string `json:"_comment"`
	Moves   []move
}

type move struct {
	sim.Move
	ID                      string
	Comment                 string `json:"_comment"`
	Input                   struct{ Stance, Button, Motion string }
	AttackLevel             string
	Super, Knockdown, Throw bool
	NeedsHit                *bool
	Dir                     string
	Invuln, Window          [2]int32
	Launch, Knockback       [2]sim.Fix
	Cancel, After           []string
	Boxes                   []keyframe
	Projectile              *struct {
		sim.ProjectileSpec
		Spawn [2]sim.Fix
	}
}

type keyframe struct {
	Frame, HitNum int32
	Hurt, Hit     []sim.Box
}

// Load decodes the embedded roster.
func Load() ([]sim.Character, error) {
	names, err := filenames()
	if err != nil {
		return nil, err
	}
	var out []sim.Character
	for _, name := range names {
		var j character
		if err := decode(name, &j); err != nil {
			return nil, err
		}
		c, err := j.convert()
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

// LoadBalance decodes the embedded tunables.
func LoadBalance() (sim.Balance, error) {
	var j struct {
		sim.Balance
		Comment      string `json:"_comment"`
		RoundSeconds int32
	}
	if err := decode(balanceFile, &j); err != nil {
		return sim.Balance{}, err
	}
	j.RoundFrames = j.RoundSeconds * framesPerSecond
	return j.Balance, nil
}

// Install loads the embedded roster and balance into the sim.
func Install() error {
	cs, err := Load()
	if err != nil {
		return err
	}
	if !sim.LoadCharacters(cs) {
		return fmt.Errorf("sim refused the roster (%d characters)", len(cs))
	}
	b, err := LoadBalance()
	if err != nil {
		return err
	}
	sim.LoadBalance(b)
	return nil
}

// Version is an FNV-1a hash of the embedded file names and bytes, compared at the handshake.
func Version() (uint32, error) {
	names, err := filenames()
	if err != nil {
		return 0, err
	}
	h := uint32(2166136261)
	for _, name := range append(names, balanceFile) {
		raw, err := files.ReadFile(name)
		if err != nil {
			return 0, err
		}
		for _, b := range append([]byte(name), raw...) {
			h = (h ^ uint32(b)) * 16777619
		}
	}
	return h, nil
}

// filenames returns the embedded character files in a fixed order.
func filenames() ([]string, error) {
	entries, err := files.ReadDir("characters")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, "characters/"+e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// decode refuses unknown keys, so a typo fails here instead of loading as zero.
func decode(name string, into any) error {
	raw, err := files.ReadFile(name)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (j *character) convert() (sim.Character, error) {
	c := j.Character
	// D69: coupled constants are checked against each other, not one by one.
	if c.Gravity >= 0 {
		return c, fmt.Errorf("gravity %d must be negative", c.Gravity)
	}
	if air := 2 * int64(c.JumpVelocity) / int64(-c.Gravity); air < 20 || air > 90 {
		return c, fmt.Errorf("jumpVelocity %d against gravity %d gives a %d-frame jump, want 20..90",
			c.JumpVelocity, c.Gravity, air)
	}
	if len(j.Moves) > sim.MaxMoves {
		return c, fmt.Errorf("%d moves, max %d", len(j.Moves), sim.MaxMoves)
	}

	index := map[string]int32{}
	for i := range j.Moves {
		m, err := j.Moves[i].convert()
		if err != nil {
			return c, fmt.Errorf("move %q: %w", j.Moves[i].ID, err)
		}
		c.Moves[i] = m
		index[j.Moves[i].ID] = int32(i)
	}
	c.NumMoves = int32(len(j.Moves))

	// Follow-ups name their sources by id, resolved once every move has an index.
	for i := range j.Moves {
		for _, after := range j.Moves[i].After {
			src, ok := index[after]
			if !ok {
				return c, fmt.Errorf("move %q: after %q is not a move", j.Moves[i].ID, after)
			}
			c.Moves[i].After |= 1 << uint32(src)
			c.Moves[src].CancelInto |= sim.CancelTarget
		}
	}
	return c, nil
}

func (j *move) convert() (sim.Move, error) {
	m := j.Move
	var err error
	if m.Button, m.Need, err = parseButtons(j.Input.Button); err != nil {
		return m, err
	}
	var okS, okL, okM bool
	m.Stance, okS = stances[j.Input.Stance]
	m.Level, okL = levels[j.AttackLevel]
	m.Motion, okM = motions[j.Input.Motion]
	if !okS || !okL || !okM {
		return m, fmt.Errorf("unknown stance %q, attackLevel %q or motion %q", j.Input.Stance, j.AttackLevel, j.Input.Motion)
	}
	for _, name := range j.Cancel {
		bit, ok := cancels[name]
		if !ok {
			return m, fmt.Errorf("unknown cancel %q", name)
		}
		m.CancelInto |= bit
	}

	m.Super, m.Knockdown, m.Throw = flag(j.Super), flag(j.Knockdown), flag(j.Throw)
	if j.NeedsHit != nil && !*j.NeedsHit {
		m.Whiff = 1
	}
	if j.Dir != "" {
		m.Dir = j.Dir[0] - '0'
	}
	m.InvulnStart, m.InvulnEnd = j.Invuln[0], j.Invuln[1]
	m.WindowStart, m.WindowEnd = j.Window[0], j.Window[1]
	m.LaunchVX, m.LaunchVY = j.Launch[0], j.Launch[1]
	m.KnockbackVX, m.KnockbackVY = j.Knockback[0], j.Knockback[1]
	if p := j.Projectile; p != nil {
		m.Proj = p.ProjectileSpec
		m.Proj.SpawnX, m.Proj.SpawnY = p.Spawn[0], p.Spawn[1]
	}

	if len(j.Boxes) > sim.MaxKeyframes {
		return m, fmt.Errorf("%d box keyframes, max %d", len(j.Boxes), sim.MaxKeyframes)
	}
	for i, k := range j.Boxes {
		if len(k.Hurt) > sim.MaxBoxes || len(k.Hit) > sim.MaxBoxes {
			return m, fmt.Errorf("frame %d: more than %d boxes", k.Frame, sim.MaxBoxes)
		}
		key := sim.Keyframe{Frame: k.Frame, HitNum: k.HitNum, NumHurt: int32(len(k.Hurt)), NumHit: int32(len(k.Hit))}
		copy(key.Hurt[:], k.Hurt)
		copy(key.Hit[:], k.Hit)
		m.Keys[i] = key
	}
	m.NumKeys = int32(len(j.Boxes))
	return m, nil
}

func flag(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

var buttons = map[string]uint16{
	"LP": sim.InLP, "MP": sim.InMP, "HP": sim.InHP,
	"LK": sim.InLK, "MK": sim.InMK, "HK": sim.InHK,
}

// A kind of button, pressed n at a time: the super's any one, the EX's any two.
var kinds = map[string]struct {
	mask uint16
	need int32
}{
	"P": {sim.InLP | sim.InMP | sim.InHP, 1}, "PP": {sim.InLP | sim.InMP | sim.InHP, 2},
	"K": {sim.InLK | sim.InMK | sim.InHK, 1}, "KK": {sim.InLK | sim.InMK | sim.InHK, 2},
}

// parseButtons reads "LP", "LP+LK", or a kind; need 0 means every named button.
func parseButtons(spec string) (mask uint16, need int32, err error) {
	if k, ok := kinds[spec]; ok {
		return k.mask, k.need, nil
	}
	for _, name := range strings.Split(spec, "+") {
		bit, ok := buttons[strings.TrimSpace(name)]
		if !ok {
			return 0, 0, fmt.Errorf("unknown button %q", name)
		}
		mask |= bit
	}
	return mask, 0, nil
}

// An overhead is a high; the difference is startup, already in the frame data.
var levels = map[string]int32{
	"mid": sim.LevelMid, "high": sim.LevelHigh, "low": sim.LevelLow, "overhead": sim.LevelHigh,
}

var cancels = map[string]uint16{
	"chain": sim.CancelChain, "special": sim.CancelSpecial, "super": sim.CancelSuper,
}

var stances = map[string]int32{
	"stand": sim.StanceStand, "crouch": sim.StanceCrouch, "air": sim.StanceAir,
}

var motions = map[string]sim.Motion{
	"": sim.MotionNone, "qcf": sim.MotionQCF, "qcb": sim.MotionQCB, "dp": sim.MotionDP,
	"qcfx2": sim.MotionQCFx2, "hcf": sim.MotionHCF,
}
