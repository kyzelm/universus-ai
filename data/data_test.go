package data

import (
	"bytes"
	"encoding/json"
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

// The exact values the design notes quote, so a change to the conversion is
// caught here rather than as a subtly different jump arc.
func TestParseFixMatchesTheDocumentedConstants(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want sim.Fix
	}{
		{"1.5", 98304},     // WalkSpeed
		{"8.0", 524288},    // JumpVelocity
		{"-0.375", -24576}, // Gravity
		{"0", 0},
		{"1", 65536},
		{"-1", -65536},
		{"0.5", 32768},
		{"-0.5", -32768},
		{"40", 2621440},
		{"0.0000152587890625", 1}, // exactly 1/65536
	} {
		got, err := parseFix(json.Number(tc.in))
		if err != nil {
			t.Errorf("parseFix(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseFix(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseFixRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "abc", "1.2.3", "1.", "--1", "999999999", "-999999999"} {
		if got, err := parseFix(json.Number(in)); err == nil {
			t.Errorf("parseFix(%q) = %d, want an error", in, got)
		}
	}
}

// Validation exists to catch authoring mistakes loudly.
func TestValidationRejectsBadData(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*jsonCharacter)
	}{
		{"zero health", func(c *jsonCharacter) { c.Health = 0 }},
		{"positive gravity", func(c *jsonCharacter) { c.Gravity = "0.375" }},
		{"no moves", func(c *jsonCharacter) { c.Moves = nil }},
		{"zero startup", func(c *jsonCharacter) { c.Moves[0].Startup = 0 }},
		{"zero active", func(c *jsonCharacter) { c.Moves[0].Active = 0 }},
		{"negative recovery", func(c *jsonCharacter) { c.Moves[0].Recovery = -1 }},
		{"unknown button", func(c *jsonCharacter) { c.Moves[0].Input.Button = "LOL" }},
		{"unknown stance", func(c *jsonCharacter) { c.Moves[0].Input.Stance = "hover" }},
		{"unknown motion", func(c *jsonCharacter) { c.Moves[0].Input.Motion = "360" }},
		{"launch with one value", func(c *jsonCharacter) { c.Moves[0].Launch = []json.Number{"2"} }},
		{"launch that is not a number", func(c *jsonCharacter) {
			c.Moves[0].Launch = []json.Number{"2", "up"}
		}},
		{"unknown level", func(c *jsonCharacter) { c.Moves[0].AttackLevel = "sideways" }},
		{"zero-size box", func(c *jsonCharacter) { c.Pushbox = []json.Number{"0", "0", "0", "48"} }},
		{"box with too few values", func(c *jsonCharacter) { c.StandHurt = []json.Number{"0", "0"} }},
		{"no keyframes", func(c *jsonCharacter) { c.Moves[0].Boxes = nil }},
		{"keyframes out of order", func(c *jsonCharacter) {
			c.Moves[0].Boxes[0].Frame, c.Moves[0].Boxes[1].Frame = 4, 0
		}},
		{"keyframe past the end of the move", func(c *jsonCharacter) {
			c.Moves[0].Boxes[2].Frame = 999
		}},
		{"hitbox outside the active window", func(c *jsonCharacter) {
			c.Moves[0].Boxes[1].Hit = nil
			c.Moves[0].Boxes[0].Hit = [][]json.Number{{"12", "30", "22", "12"}}
		}},

		{"empty invuln window", func(c *jsonCharacter) { c.Moves[0].Invuln = []int{4, 4} }},
		{"invuln with one value", func(c *jsonCharacter) { c.Moves[0].Invuln = []int{0} }},
		{"invuln past the end of the move", func(c *jsonCharacter) { c.Moves[0].Invuln = []int{0, 999} }},
		{"negative landing recovery", func(c *jsonCharacter) { c.Moves[0].Landing = -1 }},
		{"knockback with one value", func(c *jsonCharacter) {
			c.Moves[0].Knockback = []json.Number{"2"}
		}},
		{"knockback that is not a number", func(c *jsonCharacter) {
			c.Moves[0].Knockback = []json.Number{"2", "away"}
		}},
		// [0, 0] is the balance default written in a way that reads as "this
		// move pushes nobody", and the two are not the same claim.
		{"a knockback of nothing", func(c *jsonCharacter) {
			c.Moves[0].Knockback = []json.Number{"0", "0"}
		}},
		{"a negative juggle limit", func(c *jsonCharacter) { c.Moves[0].JuggleLimit = -1 }},
		{"unknown cancel category", func(c *jsonCharacter) { c.Moves[0].Cancel = []string{"super4"} }},

		{"a free super", func(c *jsonCharacter) { c.Moves[0].Meter = 0 }},
		{"a super with no motion", func(c *jsonCharacter) { c.Moves[0].Input.Motion = "" }},

		{"an unknown button inside a combination", func(c *jsonCharacter) {
			c.Moves[0].Input.Button = "LP+LOL"
		}},
		{"a throw that releases immediately", func(c *jsonCharacter) {
			c.Moves[0].Throw, c.Moves[0].Hitstun = true, 0
		}},
		// A throw super is legal,
		// but two throws with no motion are not: the motion-less one is the
		// tech input, and a second would make the lookup pick by file order.
		{"two throws with no motion", func(c *jsonCharacter) {
			for i := range c.Moves {
				c.Moves[i].Throw, c.Moves[i].Hitstun = true, 20
				c.Moves[i].Super, c.Moves[i].Input.Motion = false, ""
			}
		}},

		// Target combos: after names a move by id, so ids must be unique and
		// must exist, a move cannot follow itself (HP>HP>HP...
		{"two moves with one id", func(c *jsonCharacter) { c.Moves[1].ID = c.Moves[0].ID }},
		{"a follow-up of no move", func(c *jsonCharacter) { c.Moves[len(c.Moves)-1].After = []string{"nope"} }},
		{"a move that follows itself", func(c *jsonCharacter) {
			last := &c.Moves[len(c.Moves)-1]
			last.After = []string{last.ID}
		}},
		{"a super as a follow-up", func(c *jsonCharacter) { c.Moves[0].After = []string{"5LP"} }},
		// The second-stage knobs: follow-ups only, and in range.
		{"needsHit on a move that follows nothing", func(c *jsonCharacter) {
			no := false
			c.Moves[0].NeedsHit = &no
		}},
		{"a dir on a move that follows nothing", func(c *jsonCharacter) { c.Moves[0].Dir = "6" }},
		{"dir 5", func(c *jsonCharacter) { c.Moves[len(c.Moves)-1].Dir = "5" }},
		{"a dir that is not numpad", func(c *jsonCharacter) { c.Moves[len(c.Moves)-1].Dir = "forward" }},
		{"an empty window", func(c *jsonCharacter) { c.Moves[len(c.Moves)-1].Window = []int{10, 10} }},
		{"a window past the source's end", func(c *jsonCharacter) { c.Moves[len(c.Moves)-1].Window = []int{0, 999} }},
		// Multi-hit: numbers only where there are hitboxes, never going back.
		{"a hitNum with no hitboxes", func(c *jsonCharacter) {
			for i := range c.Moves {
				if c.Moves[i].ID == "5LP" {
					c.Moves[i].Boxes[0].HitNum = 2
				}
			}
		}},
		{"a hitNum that goes back down", func(c *jsonCharacter) {
			for i := range c.Moves {
				if c.Moves[i].ID == "5LP" {
					b := &c.Moves[i].Boxes
					*b = append(*b, (*b)[1])
					(*b)[1].HitNum = 2
					(*b)[3].Frame, (*b)[3].HitNum = 5, 1
					(*b)[2], (*b)[3] = (*b)[3], (*b)[2]
				}
			}
		}},

		// The one the design note itself got wrong: 8.0 and -0.03 pass every
		// individual check and give a nine-second jump.
		{"jump constants that do not go together", func(c *jsonCharacter) { c.Gravity = "-0.03" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jc := validCharacter(t)
			tc.edit(&jc)
			if _, err := jc.convert(); err == nil {
				t.Error("accepted data that should have been rejected")
			}
		})
	}
}

// validCharacter is the embedded file, re-parsed, so the test data cannot drift
// away from what actually ships.
func validCharacter(t *testing.T) jsonCharacter {
	t.Helper()

	raw, err := files.ReadFile("characters/00-shoto.json")
	if err != nil {
		t.Fatal(err)
	}
	var jc jsonCharacter
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&jc); err != nil {
		t.Fatal(err)
	}
	if _, err := jc.convert(); err != nil {
		t.Fatalf("the shipped character does not validate: %v", err)
	}
	return jc
}

// The balance file is loaded on the same terms as the roster: it fails here
// rather than at the start of a match, and every field it sets is one the sim
// would otherwise run at zero.
func TestEmbeddedBalanceLoads(t *testing.T) {
	b, err := LoadBalance()
	if err != nil {
		t.Fatalf("the embedded balance does not load: %v", err)
	}

	// A zero here is a renamed or mistyped key, which parses without complaint
	// and shows up in play as a gauge that never moves.
	for _, f := range []struct {
		name string
		v    int32
	}{
		{"damage.chipPercent", b.ChipPercent},
		{"damage.armorDamagePercent", b.ArmorDamagePercent},
		{"damage.starterLight", b.StarterLight},
		{"damage.starterMedium", b.StarterMedium},
		{"damage.starterHeavy", b.StarterHeavy},
		{"damage.minDamagePercent", b.MinDamagePercent},
		{"damage.counterHitPercent", b.CounterHitPercent},
		{"damage.counterHitstun", b.CounterHitstun},
		{"damage.punishHitstun", b.PunishHitstun},
		{"super.max", b.SuperMax},
		{"super.dealtPercent", b.SuperDealtPercent},
		{"super.takenPercent", b.SuperTakenPercent},
		{"super.onSpecial", b.SuperOnSpecial},
	} {
		if f.v <= 0 {
			t.Errorf("%s is %d", f.name, f.v)
		}
	}
}

// The balance file is part of the simulation's identity: a client that tuned
// the block cost is as desynced as one that tuned a hitbox, and the handshake
// compares one number for both.
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
