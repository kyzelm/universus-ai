package sim

import "testing"

// The property the architecture needs.
func TestSameSeedSameSequence(t *testing.T) {
	a, b := New(), New()
	for i := range 1000 {
		x, y := a.Rand(), b.Rand()
		if x != y {
			t.Fatalf("draw %d diverged: %d != %d", i, x, y)
		}
	}
	if a.RNG != b.RNG {
		t.Errorf("generator state diverged: %d != %d", a.RNG, b.RNG)
	}
}

// The reason the generator lives in GameState and not beside it.
func TestRandRollsBackWithTheState(t *testing.T) {
	s := New()
	for range 20 {
		s.Rand()
	}

	saved := s // the rollback: one struct assignment, exactly as Session does it

	var want [10]uint32
	for i := range want {
		want[i] = s.Rand()
	}

	s = saved
	for i := range want {
		if got := s.Rand(); got != want[i] {
			t.Fatalf("draw %d after rewind = %d, want %d", i, got, want[i])
		}
	}
	if s != saved && s.RNG == saved.RNG {
		t.Error("replay left the generator where it started")
	}
}

func TestRandNStaysInRange(t *testing.T) {
	s := New()
	for _, n := range []uint32{1, 2, 3, 6, 7, 100, 1 << 16} {
		for range 5000 {
			if got := s.RandN(n); got >= n {
				t.Fatalf("RandN(%d) = %d, out of range", n, got)
			}
		}
	}
	if got := s.RandN(0); got != 0 {
		t.Errorf("RandN(0) = %d, want 0 (and no division by zero)", got)
	}
}
