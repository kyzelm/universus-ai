//go:build js && wasm

// Command wasm is the sim's WASM entrypoint.
package main

import (
	"syscall/js"
	"unsafe"

	"universus/data"
	"universus/sim"
)

var (
	session = sim.NewSession()
	snap    [sim.SnapshotSize]byte

	// The scripted opponent played as a device, per seat, for the bot-vs-bot
	// harness.
	devices [2]*sim.AIDevice
)

func main() {
	// The roster loads before the first state exists.
	cs, err := data.Load()
	if err != nil {
		panic("character data failed to load: " + err.Error())
	}
	if !sim.LoadCharacters(cs) {
		panic("sim refused the embedded roster")
	}
	// The balance data is loaded for the same reason and with the same
	// consequence: an unloaded balance is a match with no Drive costs.
	b, err := data.LoadBalance()
	if err != nil {
		panic("balance data failed to load: " + err.Error())
	}
	sim.LoadBalance(b)
	session = sim.NewSession()

	done := make(chan struct{})

	api := js.Global().Get("Object").New()

	// advance(p1, p2) runs one frame from two input bitfields and refreshes the
	// snapshot.
	api.Set("advance", js.FuncOf(func(_ js.Value, args []js.Value) any {
		session.Advance([2]uint16{uint16(args[0].Int()), uint16(args[1].Int())})
		session.State().WriteSnapshot(snap[:])
		return nil
	}))

	// reset(training, ai, c0, c1) starts a fresh match from a setup.
	api.Set("reset", js.FuncOf(func(_ js.Value, args []js.Value) any {
		arg := func(i int) int {
			if len(args) > i {
				return args[i].Int()
			}
			return 0
		}
		session = sim.NewSessionOf(sim.Setup{
			Training: len(args) > 0 && args[0].Truthy(),
			AI:       int32(arg(1)),
			Chars:    [2]int32{int32(arg(2)), int32(arg(3))},
		})
		session.State().WriteSnapshot(snap[:])
		return nil
	}))

	// aiPress(seat, tier, seed) is the scripted opponent played as a *device*:
	// it reads the state and returns a bitfield, and the caller feeds that in
	// wherever a keyboard's would go.
	api.Set("aiPress", js.FuncOf(func(_ js.Value, args []js.Value) any {
		seat := args[0].Int()
		if seat < 0 || seat > 1 {
			return 0
		}
		if devices[seat] == nil {
			devices[seat] = sim.NewAIDevice(int32(args[1].Int()), uint32(args[2].Int()))
		}
		return int(devices[seat].Press(session.State(), seat))
	}))

	// rewind(frame) restores the state at the start of frame; the caller then
	// replays with advance().
	api.Set("rewind", js.FuncOf(func(_ js.Value, args []js.Value) any {
		ok := session.Rewind(uint32(args[0].Int()))
		session.State().WriteSnapshot(snap[:])
		return ok
	}))

	// eventsAt(frame) is what happened on a frame — hit, block, throw,
	// knockdown, super — packed with player 0 in the low 16 bits.
	api.Set("eventsAt", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return int(session.EventsAt(uint32(args[0].Int())))
	}))

	// numCharacters() is how many entries the roster actually has.
	api.Set("numCharacters", js.FuncOf(func(js.Value, []js.Value) any {
		return int(sim.NumCharacters())
	}))

	// stageHalfWidth() and cameraHalfWidth() are the walls and the screen, in
	// whole units, so the view frames what the sim collides against rather
	// than a copy of it.
	api.Set("stageHalfWidth", js.FuncOf(func(js.Value, []js.Value) any {
		return sim.BalanceOf().StageHalfWidth.ToInt()
	}))
	api.Set("cameraHalfWidth", js.FuncOf(func(js.Value, []js.Value) any {
		return sim.BalanceOf().CameraHalfWidth.ToInt()
	}))

	// characterName(i) is the roster entry's display name, for the character
	// select.
	api.Set("characterName", js.FuncOf(func(_ js.Value, args []js.Value) any {
		i := int32(args[0].Int())
		if i < 0 || i >= sim.NumCharacters() {
			return ""
		}
		return sim.CharacterAt(i).Name
	}))

	// dataVersion() is the hash of the embedded character files.
	api.Set("dataVersion", js.FuncOf(func(js.Value, []js.Value) any {
		v, err := data.Version()
		if err != nil {
			return 0
		}
		return v
	}))

	// dump() is what it compares when the checksums already disagree: the whole
	// state, one field per line, so the gate can report *which* field diverged.
	api.Set("dump", js.FuncOf(func(js.Value, []js.Value) any {
		return session.State().Dump()
	}))

	// frame() is the frame the sim is on.
	api.Set("frame", js.FuncOf(func(js.Value, []js.Value) any {
		return int(session.Frame())
	}))

	// checksum() is what the native-vs-WASM differential compares.
	api.Set("checksum", js.FuncOf(func(js.Value, []js.Value) any {
		return session.Checksum()
	}))

	api.Set("snapshotPtr", js.FuncOf(func(js.Value, []js.Value) any {
		return int(uintptr(unsafe.Pointer(&snap[0])))
	}))
	api.Set("snapshotLen", js.FuncOf(func(js.Value, []js.Value) any {
		return len(snap)
	}))
	// noop() exists only for the benchmark: it isolates the cost of crossing
	// the JS/Go boundary from the cost of the sim, which turn out to differ by
	// almost three orders of magnitude.
	api.Set("noop", js.FuncOf(func(js.Value, []js.Value) any {
		return nil
	}))
	api.Set("quit", js.FuncOf(func(js.Value, []js.Value) any {
		close(done)
		return nil
	}))

	js.Global().Set("sim", api)

	// Frame 0 is drawable before the first advance.
	session.State().WriteSnapshot(snap[:])

	<-done
}
