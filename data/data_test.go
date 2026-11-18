package data

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"universus/sim"
)

// The embedded roster must actually load.
func TestEmbeddedRosterLoads(t *testing.T) {
	cs, err := Load()
	if err != nil {
		t.Fatalf("the embedded roster does not load: %v", err)
	}
	if len(cs) == 0 {
		t.Fatal("no characters loaded")
	}
	if !sim.LoadCharacters(cs) {
		t.Fatal("sim refused the loaded roster")
	}

	for i, c := range cs {
		if c.Health <= 0 || c.WalkForward <= 0 || c.NumMoves == 0 {
			t.Errorf("character %d looks empty: %+v", i, c)
		}
		for m := int32(0); m < c.NumMoves; m++ {
			if c.Moves[m].Total() <= 0 {
				t.Errorf("character %d move %d has no duration", i, m)
			}
		}
	}

	// The full grounded normal set: every button, standing and crouching.
	for _, stance := range []int32{sim.StanceStand, sim.StanceCrouch} {
		for _, b := range []uint16{sim.InLP, sim.InMP, sim.InHP, sim.InLK, sim.InMK, sim.InHK} {
			found := false
			for m := int32(0); m < cs[0].NumMoves; m++ {
				mv := &cs[0].Moves[m]
				found = found || (mv.Motion == sim.MotionNone && mv.Stance == stance && mv.Button == b)
			}
			if !found {
				t.Errorf("no normal for stance %d button %#x", stance, b)
			}
		}
	}

	// Frame advantage is computed from the frame data, never authored, so this
	// is a check on the numbers rather than on a field.
	for m := int32(0); m < cs[0].NumMoves; m++ {
		mv := &cs[0].Moves[m]
		if adv := mv.OnBlock(); adv > 3 {
			t.Errorf("move %d is %+d on block", m, adv)
		}
	}

	// The strength variants are the data-driven claim in miniature: three moves
	// that differ only in numbers, sharing one motion and one code path.
	strengths := map[uint16]bool{}
	for m := int32(0); m < cs[0].NumMoves; m++ {
		if mv := &cs[0].Moves[m]; mv.Motion == sim.MotionQCF && mv.Meter == 0 {
			strengths[mv.Button] = true
		}
	}
	if len(strengths) != 3 {
		t.Errorf("the fireball has %d strength variants, want 3", len(strengths))
	}
}

// The handshake compares this.
func TestVersionIsStableAndCoversTheData(t *testing.T) {
	a, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("Version is not stable: %d then %d", a, b)
	}
	if a == 0 {
		t.Error("Version is 0, which is what an empty roster would produce")
	}
}

// D69: a pair that passes every per-field check but gives a 533-frame jump.
func TestCoupledJumpConstantsAreChecked(t *testing.T) {
	var j character
	if err := decode("characters/00-shoto.json", &j); err != nil {
		t.Fatal(err)
	}
	if _, err := j.convert(); err != nil {
		t.Fatalf("the shipped character does not load: %v", err)
	}
	j.JumpVelocity, j.Gravity = sim.FromInt(8), -1966 // 8.0 against -0.03
	if _, err := j.convert(); err == nil {
		t.Error("a 533-frame jump loaded")
	}
}

// A mistyped key fails the load instead of leaving a field at zero.
func TestUnknownKeyIsRefused(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(`{"healht": 10000}`))
	dec.DisallowUnknownFields()
	var j character
	if err := dec.Decode(&j); err == nil {
		t.Error("an unknown key decoded")
	}
}

// Every tunable is set: a key missing from balance.json would run at zero.
func TestEmbeddedBalanceLoads(t *testing.T) {
	b, err := LoadBalance()
	if err != nil {
		t.Fatalf("the embedded balance does not load: %v", err)
	}
	v := reflect.ValueOf(b)
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Errorf("%s is zero", v.Type().Field(i).Name)
		}
	}
	if b.RoundFrames != 99*60 {
		t.Errorf("RoundFrames is %d, want 99 s", b.RoundFrames)
	}
}

func TestVersionCoversTheBalanceFile(t *testing.T) {
	raw, err := files.ReadFile(balanceFile)
	if err != nil {
		t.Fatal(err)
	}

	v, err := Version()
	if err != nil {
		t.Fatal(err)
	}

	h := uint32(2166136261)
	for _, b := range raw {
		h = (h ^ uint32(b)) * 16777619
	}
	if v == h {
		t.Fatal("setup: the version is the balance hash alone")
	}

	// Hashing the roster without it must give a different answer, or the file
	// is embedded and unhashed.
	names, err := filenames()
	if err != nil {
		t.Fatal(err)
	}
	without := uint32(2166136261)
	for _, name := range names {
		r, err := files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range append([]byte(name), r...) {
			without = (without ^ uint32(b)) * 16777619
		}
	}
	if v == without {
		t.Error("the version does not cover balance.json")
	}
}

// loadRoster installs the shipped roster and balance in the sim, for the tests
// that play moves out rather than read them.
func loadRoster(t *testing.T) []sim.Character {
	t.Helper()
	cs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadBalance()
	if err != nil {
		t.Fatal(err)
	}
	if !sim.LoadCharacters(cs) {
		t.Fatal("sim refused the roster")
	}
	sim.LoadBalance(b)
	return cs
}

// Every EX reads any two buttons of one kind (D122).
func TestEXMovesTakeAnyTwoOfAKind(t *testing.T) {
	cs := loadRoster(t)
	punches, kicks := uint16(sim.InLP|sim.InMP|sim.InHP), uint16(sim.InLK|sim.InMK|sim.InHK)
	n := 0
	for i := range cs {
		for m := int32(0); m < cs[i].NumMoves; m++ {
			mv := &cs[i].Moves[m]
			if mv.Meter == 0 || mv.Super > 0 {
				continue
			}
			n++
			if mv.Need != 2 || (mv.Button != punches && mv.Button != kicks) {
				t.Errorf("character %d move %d: buttons %#x need %d, want any two of a kind", i, m, mv.Button, mv.Need)
			}
		}
	}
	if n == 0 {
		t.Error("no EX moves in the roster")
	}
}
