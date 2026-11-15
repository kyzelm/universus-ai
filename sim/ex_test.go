package sim

import "testing"

// EX specials and armour.

const (
	moveArmored = 13
	moveEX      = 14
)

// qcf inputs a quarter-circle and presses the buttons on the final frame, the
// way every other motion test in this package does.
func qcf(s *GameState, button uint16) {
	feed(s, 2, 3)
	s.Advance([2]uint16{pad[6] | button, 0})
}

// The price is checked at selection: the EX wins the press it shares with the
// plain special, on any two punches, and the meter is gone afterwards.
func TestEXTakesAnyTwoPunchesAndCostsMeter(t *testing.T) {
	for _, pair := range []uint16{InLP | InMP, InLP | InHP, InMP | InHP} {
		s := New()
		s.Players[0].Super = 400
		qcf(&s, pair)

		if got := s.Players[0].MoveIndex; got != moveEX {
			t.Errorf("buttons %#x: move = %d, want the EX special (%d)", pair, got, moveEX)
		}
		if got := s.Players[0].Super; got != 0 {
			t.Errorf("buttons %#x: meter = %d after the EX, want 0", pair, got)
		}
	}
}

// One punch and one kick is the throw's press, never an EX.
func TestLPLKIsNotAnEX(t *testing.T) {
	s := New()
	s.Players[0].Super = SuperMax
	qcf(&s, InLP|InLK)
	if got := s.Players[0].MoveIndex; got != 2 {
		t.Errorf("move = %d, want the plain special (2)", got)
	}

	s = New()
	s.Players[0].Super = SuperMax
	s.Advance([2]uint16{InLP | InLK, 0})
	if got := s.Players[0].MoveIndex; got != 10 {
		t.Errorf("move = %d, want the throw (10)", got)
	}
}

// The second punch may land a frame late and still make the EX.
func TestEXPairMayLandAFrameApart(t *testing.T) {
	s := New()
	s.Players[0].Super = 400
	qcf(&s, InLP)
	s.Advance([2]uint16{pad[6] | InLP | InHP, 0})

	if got := s.Players[0].MoveIndex; got != moveEX {
		t.Errorf("move = %d, want the EX special (%d)", got, moveEX)
	}
}

// A meter that cannot pay leaves the plain special to win the press.
func TestAnUnaffordableEXComesOutAsThePlainSpecial(t *testing.T) {
	s := New()
	s.Players[0].Super = 399
	qcf(&s, InLP|InMP)

	if got := s.Players[0].MoveIndex; got != 2 {
		t.Fatalf("move = %d, want the plain special (2)", got)
	}
	if got := s.Players[0].Super; got != 399 {
		t.Errorf("meter = %d: an unaffordable move was charged for", got)
	}
}

// armored starts the armoured move on player 1 and returns the state on the
// frame after it came out, with the two players close enough for move 0 to
// reach.
func armored(t *testing.T, gap int) GameState {
	t.Helper()
	s := facing(gap)
	s.Players[1].Super = 400
	s.Advance([2]uint16{0, InHP | InHK})

	if got := s.Players[1].MoveIndex; got != moveArmored {
		t.Fatalf("move = %d, want the armoured move (%d)", got, moveArmored)
	}
	return s
}

// The mechanism the bar buys: the hit lands, the move keeps going.
func TestArmorAbsorbsAHitWithoutStoppingTheMove(t *testing.T) {
	s := armored(t, 40)
	health := s.Players[1].Health

	// Player 0 jabs into it.
	for f := 0; f < 8 && s.Players[1].State == StateAttack; f++ {
		s.Advance([2]uint16{InLP, 0})
	}

	if s.Players[1].State != StateAttack {
		t.Fatalf("the armoured player is in state %d — the hit stopped the move", s.Players[1].State)
	}
	if s.Players[1].Stun != 0 {
		t.Errorf("stun = %d, want none: an absorbed hit costs no hitstun", s.Players[1].Stun)
	}
	if s.Players[1].Armor != 1 {
		t.Fatalf("armor = %d, want 1 hit absorbed", s.Players[1].Armor)
	}

	// Damage is reduced, not nothing: armouring through a heavy has a price.
	if lost := health - s.Players[1].Health; lost != 50 {
		t.Errorf("lost %d health, want the fixture's halved jab (50)", lost)
	}
}

// One hit of armour is one hit.
func TestTheSecondHitIsNotAbsorbed(t *testing.T) {
	s := armored(t, 40)
	s.Players[1].Armor = 1 // the first hit is already spent

	for f := 0; f < 8 && s.Players[1].State == StateAttack; f++ {
		s.Advance([2]uint16{InLP, 0})
	}

	if s.Players[1].State != StateHitstun {
		t.Errorf("state = %d, want hitstun: armour was already spent", s.Players[1].State)
	}
}

// Armour's reduced damage can kill.
func TestAnAbsorbedHitCanKill(t *testing.T) {
	s := armored(t, 40)
	s.Players[1].Health = 10

	for f := 0; f < 8 && s.Players[1].Armor == 0; f++ {
		s.Advance([2]uint16{InLP, 0})
	}

	if s.Players[1].Health != 0 {
		t.Errorf("health = %d, want 0", s.Players[1].Health)
	}
}

// The answer to armour.
func TestAThrowBeatsArmor(t *testing.T) {
	s := armored(t, 40)

	for f := 0; f < 10 && s.Players[1].State == StateAttack; f++ {
		s.Advance([2]uint16{InLP | InLK, 0})
	}

	if s.Players[1].State != StateThrown {
		t.Errorf("state = %d, want thrown: a throw goes through armour", s.Players[1].State)
	}
	if s.Players[1].Armor != 0 {
		t.Errorf("armor = %d: the throw was absorbed instead of landing", s.Players[1].Armor)
	}
}

// Armour covers the frames that carry the move, not the recovery it is exposed
// on afterwards.
func TestArmorDoesNotCoverTheRecovery(t *testing.T) {
	// Out of range to begin with: the armoured move's own hitbox would
	// otherwise knock the attacker down during its active frames, and the
	// recovery would be tested against somebody lying on the floor.
	s := armored(t, 200)
	mv := &char().Moves[moveArmored]

	// Sit through the active window without pressing anything, so the whiffed
	// move reaches its recovery with its armour unspent.
	for s.Players[1].StateFrame < mv.Startup+mv.Active {
		s.Advance([2]uint16{0, 0})
	}

	// Now close the distance, so what lands during the recovery is a jab and
	// not the exchange that never happened.
	s.Players[0].X, s.Players[1].X = FromInt(-20), FromInt(20)
	if s.Players[1].Armor != 0 {
		t.Fatalf("armor = %d: something hit it during the active frames", s.Players[1].Armor)
	}

	for f := 0; f < 8 && s.Players[1].State == StateAttack; f++ {
		s.Advance([2]uint16{InLP, 0})
	}

	if s.Players[1].State != StateHitstun {
		t.Errorf("state = %d, want hitstun: the recovery is not armoured", s.Players[1].State)
	}
}
