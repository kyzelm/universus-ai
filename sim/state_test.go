package sim

import (
	"reflect"
	"testing"
	"unsafe"
)

// The state must stay flat and fixed-size.
func TestGameStateIsFlatAndFixedSize(t *testing.T) {
	// Comparable at all: a slice/map/func field would make this line not compile.
	if New() == (GameState{}) {
		t.Error("New() returned the zero value; players should start apart")
	}
	if got := unsafe.Sizeof(GameState{}); got > 2048 {
		t.Errorf("sizeof(GameState) = %d, over the 2 KB target", got)
	}
	if got := unsafe.Alignof(GameState{}); got != 4 {
		t.Errorf("alignof(GameState) = %d, want 4 — an 8-byte field forces padding", got)
	}
}

// Checksum hashes the struct's own memory, so a single byte of implicit padding is a byte of
// uninitialised memory in the hash.
func TestGameStateHasNoImplicitPadding(t *testing.T) {
	assertPacked(t, reflect.TypeOf(GameState{}), "GameState")
}

func assertPacked(t *testing.T, typ reflect.Type, path string) {
	t.Helper()

	switch typ.Kind() {
	case reflect.Struct:
		var end uintptr
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Offset != end {
				t.Errorf("%s.%s is at offset %d, want %d: %d padding byte(s) before it",
					path, f.Name, f.Offset, end, f.Offset-end)
			}
			assertPacked(t, f.Type, path+"."+f.Name)
			end += f.Type.Size()
		}
		if end != typ.Size() {
			t.Errorf("%s is %d bytes but its fields total %d: %d trailing padding byte(s)",
				path, typ.Size(), end, typ.Size()-end)
		}

	// Elements of a padding-free type are contiguous, so the element type is the whole question for
	// an array.
	case reflect.Array:
		assertPacked(t, typ.Elem(), path+"[]")

	case reflect.Int32, reflect.Uint32:
		// The width the whole scheme depends on.

	case reflect.Float32, reflect.Float64:
		// CI greps for the type names.
		t.Errorf("%s is %s — the sim is fixed-point only", path, typ.Kind())

	default:
		t.Errorf("%s is %s: every field must be a 4-byte integer type, "+
			"or the struct picks up padding and Checksum stops being sound", path, typ.Kind())
	}
}

func run(s *GameState, frames int, in [2]uint16) {
	for range frames {
		s.Advance(in)
	}
}

// The whole architecture rests on this: same start, same inputs, same bytes.
func TestSameInputsProduceIdenticalState(t *testing.T) {
	inputs := inputSeq(2000)

	a, b := New(), New()
	for _, in := range inputs {
		a.Advance(in)
	}
	for _, in := range inputs {
		b.Advance(in)
	}
	if a != b {
		t.Fatalf("identical inputs diverged:\n a = %+v\n b = %+v", a, b)
	}
}

// hold advances n frames with the same input for player 0 and neutral for 1.
func hold(s *GameState, n int, in uint16) {
	for range n {
		s.Advance([2]uint16{in, 0})
	}
}

func TestJumpRisesAndLands(t *testing.T) {
	s := New()
	hold(&s, int(char().PreJumpFrames), InUp)

	airborne, peak := 0, Fix(0)
	for range 200 {
		s.Advance([2]uint16{}) // released: the arc is committed
		if !s.Players[0].Airborne() {
			break
		}
		airborne++
		if s.Players[0].Y > peak {
			peak = s.Players[0].Y
		}
	}

	// Derived from the constants rather than pinned.
	want := 2 * int(char().JumpVelocity/(-char().Gravity))
	if airborne < want-3 || airborne > want+3 {
		t.Errorf("jump lasted %d frames, want ~%d", airborne, want)
	}
	if peak.ToInt() < 60 {
		t.Errorf("jump peaked at %d units, want >= 60", peak.ToInt())
	}
	if s.Players[0].Y != GroundY || s.Players[0].VY != 0 {
		t.Errorf("landed at Y=%d VY=%d, want 0 and 0", s.Players[0].Y, s.Players[0].VY)
	}
	if s.Players[0].State != StateIdle {
		t.Errorf("state after landing = %d, want idle", s.Players[0].State)
	}
}

func TestWallsClamp(t *testing.T) {
	s := New()
	// Both walking left: the screen follows them, so they reach the stage end.
	run(&s, 1000, [2]uint16{InLeft, InLeft})

	for i := range s.Players {
		box := s.Pushbox(i)
		if box.X < -balance.StageHalfWidth || box.X+box.W > balance.StageHalfWidth {
			t.Errorf("player %d pushbox %+v is outside the stage", i, box)
		}
	}
	// And they actually reached the wall rather than stopping early.
	if (s.Pushbox(0).X + balance.StageHalfWidth).Abs() > One {
		t.Errorf("player 0 did not reach the left wall: %+v", s.Pushbox(0))
	}
}

func TestPlayersDoNotMerge(t *testing.T) {
	s := New()
	for f := range 500 {
		s.Advance([2]uint16{InRight, InLeft}) // walk into each other and keep pushing
		a, b := s.Pushbox(0), s.Pushbox(1)
		if a.Overlaps(b) {
			t.Fatalf("frame %d: pushboxes overlap: %+v and %+v", f, a, b)
		}
	}
}

// The camera is gameplay: a desynced camera is a desynced corner.
func TestCameraFollowsAndStopsAtTheWalls(t *testing.T) {
	s := New()
	if s.CamX != 0 {
		t.Errorf("camera starts at %d, want centred", s.CamX)
	}

	s.Players[0].X = balance.StageHalfWidth
	s.Players[1].X = balance.StageHalfWidth
	s.updateCamera()
	if want := balance.StageHalfWidth - balance.CameraHalfWidth; s.CamX != want && balance.CameraHalfWidth < balance.StageHalfWidth {
		t.Errorf("camera at the right wall = %d, want %d", s.CamX, want)
	}
	if s.CamX+balance.CameraHalfWidth > balance.StageHalfWidth {
		t.Errorf("camera shows past the right wall: %d", s.CamX)
	}
}

// Dash recognition, including the sequences that must NOT dash.
func TestDashRecognition(t *testing.T) {
	// Numpad, from player 0's point of view (it starts facing right).
	const N, F, B, D, U = 5, 6, 4, 2, 8

	for _, tc := range []struct {
		name string
		dirs []uint8
		want int32
	}{
		{"forward, gap, forward", []uint8{F, N, F}, StateDash},
		{"back, gap, back", []uint8{B, N, B}, StateBackdash},
		// Only a clean tap-neutral-tap dashes.
		{"forward, down, forward", []uint8{F, D, F}, StateWalkF},
		{"back, down, back", []uint8{B, D, B}, StateWalkB},
		{"forward, down-forward, forward", []uint8{F, 3, F}, StateWalkF},
		{"down-forward twice is not a dash", []uint8{3, N, 3}, StateCrouch},

		// Up in the middle is a jump, not a gap.
		{"back, up, back", []uint8{B, U, B}, StatePreJump},

		{"back, forward, back", []uint8{B, F, B}, StateWalkB},
		{"forward, back, forward", []uint8{F, B, F}, StateWalkF},
		{"back, forward, gap, back", []uint8{B, F, N, B}, StateWalkB},

		{"held forward is a walk", []uint8{F, F, F, F}, StateWalkF},
		{"held back is a walk", []uint8{B, B, B, B}, StateWalkB},
		{"one tap is a walk", []uint8{N, F}, StateWalkF},

		// Outside the window the first tap no longer counts.
		{"two taps too far apart", []uint8{F, N, N, N, N, N, N, N, N, N, N, N, F}, StateWalkF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			feed(&s, tc.dirs...)
			if got := s.Players[0].State; got != tc.want {
				t.Errorf("%v gave state %d, want %d", tc.dirs, got, tc.want)
			}
		})
	}
}

// One press, one move.
func TestOnePressFiresOneMove(t *testing.T) {
	s := New()
	s.Advance([2]uint16{InLP, 0})
	if s.Players[0].State != StateAttack {
		t.Fatal("the press did not start a move")
	}

	// Cut the move short so the player is actionable while its own press is still inside the buffer
	// window.
	s.Players[0].enter(StateIdle)
	s.Advance([2]uint16{0, 0})

	if s.Players[0].State == StateAttack {
		t.Error("a single press started a second move")
	}

	// And it is Eaten doing the work, not the state machine being lucky.
	s.Players[0].Eaten = -1
	s.Advance([2]uint16{0, 0})
	if s.Players[0].State != StateAttack {
		t.Error("with the press un-eaten the move does not come out: the test proves nothing")
	}
}

// The buffer is state, so it rolls back with everything else.
func TestEatenRollsBack(t *testing.T) {
	sess := NewSession()

	sess.Advance([2]uint16{0, 0})
	sess.Advance([2]uint16{InLP, 0})
	if sess.State().Players[0].Eaten < 0 {
		t.Fatal("starting a move did not spend the press")
	}

	if !sess.Rewind(1) {
		t.Fatal("rewind refused")
	}
	if got := sess.State().Players[0].Eaten; got != -1 {
		t.Errorf("Eaten = %d after a rewind, want -1: the buffer did not roll back", got)
	}
}

// A special and the normal on the same button are told apart by the motion and nothing else.
func TestMotionSelectsTheSpecial(t *testing.T) {
	const jab, special = int32(0), int32(2)

	bare := New()
	bare.Advance([2]uint16{InLP, 0})
	if got := bare.Players[0].MoveIndex; got != jab {
		t.Errorf("LP with no motion gave move %d, want the jab %d", got, jab)
	}

	qcf := New()
	feed(&qcf, 2, 3)
	qcf.Advance([2]uint16{pad[6] | InLP, 0})
	if got := qcf.Players[0].MoveIndex; got != special {
		t.Errorf("QCF+LP gave move %d, want the special %d", got, special)
	}
}

// cancelWindow runs the jab into the opponent and returns the state on the first frame after
// hitstop, with the cancel window open and the jab still running.
func cancelWindow(t *testing.T, gap int) GameState {
	t.Helper()
	s := facing(gap)

	s.Advance([2]uint16{InLP, 0})
	if s.Players[0].MoveIndex != 0 {
		t.Fatalf("the jab did not come out: move %d", s.Players[0].MoveIndex)
	}
	for range char().Moves[0].Startup + char().Moves[0].Active {
		s.Advance([2]uint16{0, 0})
	}
	for s.Hitstop > 0 {
		s.Advance([2]uint16{0, 0})
	}
	return s
}

// A cancelable normal that connected is not a commitment any more.
func TestACancelableNormalCancelsIntoASpecial(t *testing.T) {
	s := cancelWindow(t, 30)
	jab := &char().Moves[0]

	feed(&s, 2, 3) // the motion, inside the window
	at := s.Players[0].StateFrame
	s.Advance([2]uint16{pad[6] | InLP, 0})

	p := &s.Players[0]
	if p.MoveIndex != 2 {
		t.Fatalf("move %d after a cancel out of the jab, want the special 2", p.MoveIndex)
	}
	if at >= jab.Total() {
		t.Errorf("the jab was over (frame %d of %d): that is a fresh press, not a cancel", at, jab.Total())
	}
}

// withTargetCombo adds a follow-up to the jab on the jab's own button.
func withTargetCombo(t *testing.T) int32 {
	t.Helper()
	c := char()
	saved := *c
	t.Cleanup(func() { *c = saved })

	f := c.NumMoves
	c.Moves[f] = c.Moves[0]
	c.Moves[f].After = 1 << 0 // follows move 0
	c.Moves[f].CancelInto = 0
	c.Moves[0].CancelInto |= CancelTarget
	c.NumMoves++
	return f
}

// The follow-up comes out of the jab's window, on the same button the jab itself is on, and it does
// not loop.
func TestATargetComboComesOutOfItsSource(t *testing.T) {
	f := withTargetCombo(t)
	s := cancelWindow(t, 30)

	s.Advance([2]uint16{InLP, 0})
	if got := s.Players[0].MoveIndex; got != f {
		t.Fatalf("move %d out of the jab's window, want the follow-up %d", got, f)
	}

	// Connect again and press again: the follow-up has no follow-up.
	for s.Players[0].HasHit == 0 && s.Players[0].StateFrame < char().Moves[f].Total() {
		s.Advance([2]uint16{0, 0})
	}
	for s.Hitstop > 0 {
		s.Advance([2]uint16{0, 0})
	}
	s.Advance([2]uint16{InLP, 0})
	if got := s.Players[0].MoveIndex; got != f || s.Players[0].StateFrame == 1 {
		t.Errorf("move %d on frame %d: the follow-up restarted itself", got, s.Players[0].StateFrame)
	}
}

// withSecondStage gives the fireball (move 2) a second stage on 6+LP, the fireball again, the way a
// rekka is written.
func withSecondStage(t *testing.T) int32 {
	t.Helper()
	c := char()
	saved := *c
	t.Cleanup(func() { *c = saved })

	f := c.NumMoves
	c.Moves[f] = c.Moves[2]
	c.Moves[f].Motion = MotionNone
	c.Moves[f].After = 1 << 2
	c.Moves[f].Whiff = 1
	c.Moves[f].Dir = DirFwd
	c.Moves[2].CancelInto |= CancelTarget
	c.NumMoves++
	return f
}

// fireballRecovery throws the fireball and runs it to the first frame after its active frames,
// which is where a whiffable second stage's default window opens.
func fireballRecovery(t *testing.T) GameState {
	t.Helper()
	s := New()
	feed(&s, 2, 3)
	s.Advance([2]uint16{pad[6] | InLP, 0})
	if got := s.Players[0].MoveIndex; got != 2 {
		t.Fatalf("the fireball did not come out: move %d", got)
	}
	fb := &char().Moves[2]
	for s.Players[0].StateFrame < fb.Startup+fb.Active {
		s.Advance([2]uint16{0, 0})
	}
	return s
}

func activeProjectiles(s *GameState) (n int) {
	for i := range s.Projectiles {
		if s.Projectiles[i].Active != 0 {
			n++
		}
	}
	return n
}

// The second stage comes out of a fireball that hit nothing, on the direction it was written for
// and on no other, and throws a second fireball.
func TestASecondStageComesOutOfAWhiff(t *testing.T) {
	f := withSecondStage(t)
	s := fireballRecovery(t)
	if s.Players[0].HasHit != 0 {
		t.Fatal("the fireball's move connected, so this proves nothing about a whiff")
	}

	s.Advance([2]uint16{InLP, 0}) // no direction: not the second stage
	if got := s.Players[0].MoveIndex; got != 2 {
		t.Fatalf("plain LP gave move %d, want the fireball still running", got)
	}
	s.Advance([2]uint16{0, 0})
	s.Advance([2]uint16{pad[6] | InLP, 0})
	if got := s.Players[0].MoveIndex; got != f {
		t.Fatalf("6+LP gave move %d, want the second stage %d", got, f)
	}

	for s.Players[0].StateFrame <= char().Moves[f].Startup {
		s.Advance([2]uint16{0, 0})
	}
	if n := activeProjectiles(&s); n != 2 {
		t.Errorf("%d fireballs in flight, want both stages'", n)
	}
}

// A multi-hit move connects once per hit number.
func TestAMultiHitMoveHitsOncePerNumber(t *testing.T) {
	c := char()
	saved := *c
	t.Cleanup(func() { *c = saved })

	jab := &c.Moves[0]
	// The jab's three active frames as two hits: frame 4 and frame 5.
	jab.Keys[3] = jab.Keys[2]
	jab.Keys[2] = jab.Keys[1]
	jab.Keys[2].Frame = 5
	jab.NumKeys = 4

	for _, tc := range []struct {
		num  int32
		want int32
	}{{0, 1}, {2, 2}} {
		jab.Keys[2].HitNum = tc.num
		s := facing(30)
		s.Advance([2]uint16{InLP, 0})
		for s.Players[0].State == StateAttack {
			s.Advance([2]uint16{0, 0})
		}
		if got := s.Players[1].Combo; got != tc.want {
			t.Errorf("second keyframe hitNum %d: combo %d, want %d", tc.num, got, tc.want)
		}
	}
}
