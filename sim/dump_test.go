package sim

import (
	"strings"
	"testing"
)

// The dump is only worth having if it names the field that differs.
func TestDumpNamesTheDifferingField(t *testing.T) {
	a := New()
	b := New()
	b.Players[1].Health -= 7

	da, db := strings.Split(a.Dump(), "\n"), strings.Split(b.Dump(), "\n")
	if len(da) != len(db) {
		t.Fatalf("dumps have different line counts: %d vs %d", len(da), len(db))
	}

	var differing []string
	for i := range da {
		if da[i] != db[i] {
			differing = append(differing, da[i]+" | "+db[i])
		}
	}
	if len(differing) != 1 {
		t.Fatalf("want exactly one differing line, got %d: %v", len(differing), differing)
	}
	if !strings.Contains(differing[0], "Players[1].Health") {
		t.Errorf("the differing line does not name the field: %s", differing[0])
	}
}

// Every field must appear, including ones added later.
func TestDumpCoversEveryField(t *testing.T) {
	start := New()
	out := start.Dump()
	for _, want := range []string{
		"Frame", "RNG", "Hitstop", "Freeze", "CamX",
		"Players[0].X", "Players[0].Y", "Players[0].VX", "Players[0].VY",
		"Players[0].Facing", "Players[0].Char", "Players[0].Health",
		"Players[0].State", "Players[0].StateFrame", "Players[0].MoveIndex",
		"Players[0].HasHit", "Players[0].Stun", "Players[0].JumpVX",
		"Players[0].Inputs", "Players[1].X",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is missing from the dump", want)
		}
	}
}
