# Universus — agent brief (code repo)

Browser 2D fighting game, BSc engineering thesis. **This repo is the reference implementation.** The
defended artifact is a second repo, `~/Dev/universus`, which the user retypes from this one by hand:
never edit it, and remember that every line kept here is a line typed there. Prefer the simplest
version that works. The design vault is the source of truth for *what* to build and *why*:

```
/home/kyzelm/Obsidian/Universus/
  Universus.md                  home map, locked stack
  00 Meta/Decision Log.md       D1–D127, every locked decision + reason
  01 Thesis/Implementation Log.md  dated problems and fixes — chapter 5 is written from it
  06 Roadmap/Refactor Plan.md   the plan since 2026-11-13: cut, simplify, add, then the hand-port
  06 Roadmap/Task Board.md      what was built (M0–M3) and the checklist of what is left
  06 Roadmap/Build Roadmap.md   milestones, M0 pass criteria
  02 Architecture/*.md          sim, fixed-point, WASM boundary, rollback, transport, backend
  03 Game Design/*.md           combat, input, character data, resources, rounds, AI
  05 Tooling/Testing and CI.md  the determinism gate
```

**Before proposing an alternative, read the Decision Log.** Most alternatives (TS sim, 32.32
fixed-point, JSON over the WASM boundary, TURN, Redis, microservices, skeletal animation, story
mode, ML AI) were already evaluated and rejected with a reason. Reversing one means updating the
vault note that links to it.

**Standing bias: the thesis outweighs the artifact.** When trading scope, cut content and roster;
never cut instrumentation, testing, or measurement — those are the thesis.

## Locked stack

Deterministic fixed-point sim in **Go → WASM** (same source compiled native for the server) ·
**PixiJS + TypeScript** view layer (read-only) · **rollback netcode**, GGPO params · **WebRTC
DataChannel** P2P with **WebSocket relay** fallback · **Go** backend · **Postgres**.

Go 1.26, module `universus`. Client: Vite + TypeScript + React (shell/menus only) + pnpm + oxlint +
PixiJS 8. Four Go dependencies and no framework on either side (D93).

## Non-negotiable invariants

- **No floats in the sim.** Fixed-point 16.16 in `int32` only. CI greps `./sim/` for
  `float32|float64|"math"` and fails.
- **No gameplay state outside `GameState`.** If it is not in the struct, it does not roll back, and
  it is a bug. Includes: camera, RNG, input history, AI state, timers, round transitions.
- **The view never writes sim state.** It reads a `RenderSnapshot` and draws. It may interpolate
  visually; it may never feed anything back.
- **No hardcoded gameplay values.** Frame data, damage, boxes, speeds, costs → character/balance
  JSON, `go:embed`ed so the binary hash covers the balance data.
- **The sim is single-threaded, has no clock, and does no I/O.** Frame number is the only clock.
- **No map iteration, no pointer comparison, no unseeded randomness, no goroutines** in sim code.
- **Any per-user setting (negative edge, keybinds) resolves client-side before the input bitfield is
  built.** Nothing the sim reads may depend on a local setting.

## Repo layout

```
sim/        deterministic core, zero dependencies. sim/wasm/ = WASM entrypoint
server/     Go backend: auth, matchmaking, signaling, relay, verification. imports sim natively
client/     TypeScript, Vite, PixiJS view + net layer
tools/      tools/replay = headless replay/checksum harness; frame data editor later
data/       character + balance JSON
art/        hand-drawn Aseprite exports per character, sources for tools/sprites.py
testdata/   input log corpus — every desync bug ever found lands here as a regression log
```

Monorepo, one Go module, one version. All four trees are real: about 13 600 lines of Go across sim,
server, data and tools, tests included, and 48 TypeScript files in the client. Assume code exists and read it before
proposing to write it.

## Core formats — do not drift from these

```go
type Fix int32                                            // 16.16
const FracBits = 16; const One = Fix(1 << FracBits)
func (a Fix) Mul(b Fix) Fix { return Fix((int64(a) * int64(b)) >> FracBits) }
func (a Fix) Div(b Fix) Fix { return Fix((int64(a) << FracBits) / int64(b)) }
```

`GameState`: one flat, fixed-size struct. No pointers, no slices, no maps. Explicit zeroed padding.
Target < 2 KB, so save/load are memcpys and checksum is a hash over a byte range.

Checksum: FNV-1a over the packed state, every frame.

Input: one `uint16` per player per frame.
`bit0 up, 1 down, 2 left, 3 right, 4 LP, 5 MP, 6 HP, 8 LK, 9 MK, 10 HK` (7, 11 reserved).
Same bytes go to the sim, the network, the replay, and the server.

Net packet: `[uint8 type][uint32 startFrame][uint8 count][uint16 inputs[count]]`, ~20 bytes.
Types: 1 inputs, 2 checksum, 3 ping, 4 control. **Every packet carries the last ~8 frames of
inputs** — that redundancy replaces retransmission entirely. Unreliable, unordered channel
(`ordered: false, maxRetransmits: 0`).

WASM boundary: packed binary `RenderSnapshot` in linear memory, one `DataView` created once and
reused. `GameState` never crosses. Re-create the view if `wasmMemory.buffer.byteLength` changes —
Go's memory growth detaches every `ArrayBuffer`.

Rollback: 2 frames input delay, max 8 frames, prediction = repeat last input, checksum exchange
every 30 frames, ring buffer of 12 snapshots allocated once.

## Update order is part of the spec

Fixed, documented, never casually reordered:

1. resolve inputs (SOCD → buffer → motion recognition)
2. advance state machines
3. movement and gravity
4. pushboxes and stage bounds
5. projectiles
6. hit detection — **P1's hitboxes vs P2's hurtboxes first, then the reverse** (trades must resolve
   identically on both machines)
7. hit resolution: damage, scaling, hitstun, meter
8. timers, resources, round state
9. increment frame, compute checksum

Damage order, equally fixed, all integer, all divisions after multiplications:
`base → ×starterScale/100 → ×comboScale[n]/100 → max(minDamage) → ×counterHit/100`.

## Rollback traps

- Sounds, particles, screen shake, hit flashes: the sim sets **event flags in state**; the view fires
  effects only for flags newly present on *confirmed* frames. The classic bug is one sound playing
  eight times during a rollback. Build the mechanism in M2 even with no sounds attached.
- A frame with an 8-frame rollback runs the sim 9 times and must still fit 16.6 ms.
- Motion recognition scans **backward**; **DP takes priority over QCF** or every DP is a fireball.
  Test this case explicitly.
- SOCD resolves to neutral (left+right), up wins (up+down), **before** packing.

## Testing — the determinism gate is the most important test in the project

```
load input log → run native → run WASM (Node) → compare per-frame checksums
              → FAIL on first divergence, report frame number + state diff
```

CI pipeline, one GitHub Actions workflow, minutes not hours:
`go vet` + float-ban grep → `go test ./...` → build native + WASM → determinism differential over
the whole corpus → rollback correctness test → performance regression → client bundle.

Rollback correctness test: run 100 frames, save checksum; rerun the same inputs forcing rollbacks of
depth 1–8 at 20 seeded frames; final checksum must be identical. Fails immediately if any state
lives outside `GameState`.

Unit tests where correctness is a formula: fixed-point ops, damage scaling order, every motion, every
SOCD combination, box overlap edges, LP tier boundaries.

Instrument from day one: rollback frequency, depth distribution, frame time by depth, misprediction
rate, stall frequency. Report distributions and 99th percentiles, never bare means. These are the
results chapter and they are painful to retrofit.

## Where the project actually is

**M0 through M3 are code-complete, and the refactor to a reference is done through phase 4**
(2026-11-16, see the vault's *Refactor Plan*). Sim, rollback, netcode, training mode, one scripted
AI, bot-vs-bot harness, and the whole backend spine: auth, Postgres with embedded migrations, ranked
and casual queues, match results, the ladder, disconnect handling, verification by re-simulation,
all three anti-cheat checks, menus, character select, options and a result screen. Decision Log runs
to **D127**.

What the refactor changed, so old notes and old habits do not bring it back:

- **One gauge.** Drive and Burnout are gone (D122). EX specials cost 400 of the 2000-unit super
  meter and take the motion plus any two punches or any two kicks; a move's press is a button set
  plus how many of it must be down. Chip damage and armour damage can kill.
- **One super per character**, on any button of its kind (`236236P`, `236236K`), no levels (D123).
  It freezes the match for `super.freeze` frames, counted in `GameState.Freeze` (D126).
- **One AI**, no tiers; `?ai` takes no value (D124). **No dummy recording** (D125).
- **Options are client-side**: volume and keybinds in `localStorage`, read when the bitfield is
  packed (D127). URL flags are parsed once, in `client/src/flags.ts`.
- `sim.Advance` is the nine documented steps, one function each. `client/src/game/` is `game.ts`
  (loop), `draw.ts` (Pixi scene), `hud.ts` (HUD text), `sound.ts` (match event sounds).
- **The shell is React with CSS animations; Pixi draws only the match.** Recorded sound is
  `client/public/sounds/<name>.mp3`, played by `client/src/audio.ts`; a missing clip plays a
  synthesised tone, a missing music track is silence. Names are listed in that folder's README.

M0's bars were measured, not estimated, and four cleared with margin (see the vault's *M0 Results*):
sim step **p99 0.023 ms** against 0.5 · 8-frame rollback **p99 0.540 ms** against 4 · **10 001
identical** checksums native vs WASM · sustained 60 fps under continuous rollback. No escape hatch
was needed; max rollback stays at 8 and TinyGo stays unopened.

**One bar is still open and it is the same one blocking two milestones**: RTT across a real network
between two machines. It needs a partner and a tunnel, not code — the server already serves the
built client at its own origin behind one port (D93), and a run is set up entirely from its URL.

**What is left is the user's**: the hand-port, the measurements (M-A, M-B, M-C and the two-machine
session, all run on the hand-written repo), the drawing, and the thesis text. Code is due in the
first days of January 2027 and the text at the end of January. Work here is limited to what the
port turns up. **Measurements starting late is the failure mode**: cut backend features and art,
never the measurement stage.

Art pipeline, unchanged by the refactor:

- Tag naming is **D113**: an attack's tag is the move's `id`, everything else is the sim's state
  name. There is no `animation` field in character JSON and there must not be — `Version()` hashes
  the raw embedded bytes, so a cosmetic rename would refuse a handshake against identical frame data.
- `tools/sprites.py` generates placeholder sheets, the move-index-to-tag table and the stage
  background. CI reruns it and fails on a JSON diff, so the table cannot drift from the roster.
- Hand-drawn Aseprite exports go in `art/<name>/` (any number per character, each its own canvas),
  never over `client/public/sprites/*`. The generator composites them over the placeholders tag by
  tag, so a half-drawn sheet still loads. Frame durations become sim frames. Canvas size comes from
  the export and the feet from a slice named `origin` (D117). Every character in `data/characters/`
  gets a sheet (D118). The stage is `art/stage.png`, 1280×450, floor line at y=380. Rerun the
  generator after every export.

Placeholder art throughout; art never blocks engineering. The box overlay is off outside training
now that fighters are sprites; `?boxes` brings it back anywhere.

## When working here

- A task is done when it is **verified**, not when it is written.
- Record an input log for every playtest and every fixed desync; commit it to `testdata/`. A change
  to anything under `data/` moves `Version()`: re-stamp with `replay gen 10000 testdata/m0-10k.inputs`
  and `replay bot testdata/botmatch.inputs`.
- Check anything visual in a real browser. Headless Chrome renders the Pixi canvas black.
- New locked decision → append to the vault's Decision Log with the next D-number **and the reason**.
- Migrations are numbered `.sql` files applied by the Go binary at startup. No framework.
- Deliberately absent, do not add: Redis, message broker, microservices, k8s, migration framework,
  physics engine, ORM, state-management library. Expected concurrent users is single digits.
