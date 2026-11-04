import {readFileSync} from 'node:fs'
import {afterEach, expect, test, vi} from 'vitest'
import '../../public/wasm_exec.js'
import {createLink} from './impair'
import {CHECKSUM_EVERY, createNetplay, MAX_ROLLBACK, type SimBridge} from './netplay'

/** The bot-vs-bot harness (D49, 05 Tooling/Testing and CI.md). */

/** 100 ms round trip, 15 ms of jitter, 5% loss — a cell of the M-B matrix. */
const CONDITIONS = {delayMs: 50, jitterMs: 15, lossPercent: 5}

const STEP_MS = 1000 / 60

/**
 * Long enough to cross a round boundary, which is 99 seconds of play plus its
 * transitions.
 */
const FRAMES = Number(process.env.BOT_FRAMES) || 7200

/** How many seed pairs to play. */
const MATCHES = Number(process.env.BOT_SEEDS) || 1

/**
 * The only test here whose runtime is a variable, so it is the only one that
 * cannot live with vitest's fixed 5 s default.
 */
const TIMEOUT_MS = Math.max(30_000, MATCHES * 10_000)

declare class Go {
  importObject: WebAssembly.Imports
  run(instance: WebAssembly.Instance): Promise<void>
}

interface End extends SimBridge {
  /** The scripted opponent, pressing for this end's own seat. */
  press(seat: 0 | 1): number
  frame(): number
  /** Every state field as text — used here to read the health bars. */
  dump(): string
}

/**
 * One headless client: its own instance of the WASM module, and therefore its
 * own sim, its own memory and its own devices.
 */
async function client(seed: number, corruptAt = -1): Promise<End> {
  const go = new Go()
  const {instance} = await WebAssembly.instantiate(
    readFileSync('public/main.wasm'),
    go.importObject,
  )
  // Resolves only when the sim quits, so deliberately not awaited; the API is
  // published by the time run returns.
  void go.run(instance)

  const api = (globalThis as {sim?: Record<string, (...a: number[]) => number>}).sim
  if (!api) throw new Error('the WASM module published no API')

  return {
    // corruptAt flips one bit of one frame's remote input on this end only,
    // which is what a desync looks like from the inside: one bad frame, and
    // every state after it diverges.
    advance: (p1, p2) => void api.advance(p1, api.frame() === corruptAt ? p2 ^ 1 : p2),
    rewind: (f) => Boolean(api.rewind(f)),
    checksum: () => api.checksum(),
    dataVersion: () => api.dataVersion(),
    press: (seat) => api.aiPress(seat, 3, seed),
    frame: () => api.frame(),
    dump: () => api.dump() as unknown as string,
  }
}

/** Damage dealt across the whole match, per seat, off the state dump. */
function round(end: End): number {
  return Number(/^Round = (\d+)$/m.exec(end.dump())?.[1] ?? 0)
}

function dealt(end: End): number[] {
  return [...end.dump().matchAll(/Dealt\[\d] = (-?\d+)/g)].map((m) => Number(m[1]))
}

afterEach(() => vi.useRealTimers())

/**
 * Plays the two ends against each other and hands back everything worth
 * asserting on.
 */
async function botMatch(corruptAt = -1, pair = 0) {
  const A = await client(11 + pair * 2)
  const B = await client(22 + pair * 2, corruptAt)

  // The trick this harness stands on: two instances of the module, each with
  // its own session.
  A.advance(0, 0)
  expect([A.frame(), B.frame()]).toEqual([1, 0])
  // Then undone.
  expect(A.rewind(0)).toBe(true)
  expect([A.frame(), A.checksum()]).toEqual([0, B.checksum()])

  vi.useFakeTimers()

  let netA!: ReturnType<typeof createNetplay>
  let netB!: ReturnType<typeof createNetplay>
  // Each end impairs what arrives at it, which is where a real link sits.
  const toB = createLink((d) => netB.receive(d), CONDITIONS, 101)
  const toA = createLink((d) => netA.receive(d), CONDITIONS, 202)

  netA = createNetplay(A, (d) => toB.receive(d), 0)
  netB = createNetplay(B, (d) => toA.receive(d), 1)


  for (let f = 0; f < FRAMES; f++) {
    // Each end presses for its own seat, from its own state, and the other
    // end learns about it over the wire — which is the point.
    netA.step(A.press(0))
    netB.step(B.press(1))
    if (f % 30 === 0) {
      netA.ping()
      netB.ping()
    }
    vi.advanceTimersByTime(STEP_MS)
  }

  // Let everything in flight land and the last corrections settle.
  vi.advanceTimersByTime(2000)

  return {A, B, netA, netB}
}

test('two bots play a match over a bad connection without desyncing', async () => {
  for (let pair = 0; pair < MATCHES; pair++) await oneMatch(pair)
}, TIMEOUT_MS)

async function oneMatch(pair: number) {
  const {A, B, netA, netB} = await botMatch(-1, pair)

  // The assertion the harness exists for.
  expect(netA.stats.desyncs).toBe(0)
  expect(netB.stats.desyncs).toBe(0)

  // The netcode was actually exercised: a run with no rollbacks proves the
  // conditions never bit, not that the sim agrees under them.
  expect(netA.stats.rollbacks).toBeGreaterThan(0)
  expect(netB.stats.rollbacks).toBeGreaterThan(0)
  // A correction older than the window cannot be applied, and one arriving is
  // the first sign the rollback parameters do not fit these conditions.
  expect(netA.stats.dropped).toBe(0)
  expect(netB.stats.dropped).toBe(0)

  // Stalling sometimes is the design working; stalling constantly is not.
  expect(netA.stats.frames).toBeGreaterThan(FRAMES * 0.8)

  // Both ends run to the same frame, then hash the whole state.
  while (A.frame() !== B.frame()) {
    const behind = A.frame() < B.frame() ? netA : netB
    const before = behind === netA ? A.frame() : B.frame()
    behind.step(0)
    if ((behind === netA ? A.frame() : B.frame()) === before) break // stalled: stop
    vi.advanceTimersByTime(STEP_MS)
  }
  expect(A.frame()).toBe(B.frame())
  expect(A.checksum()).toBe(B.checksum())

  // The bots played rather than stood still.
  expect(A.frame()).toBeGreaterThan(MAX_ROLLBACK)
  expect(netA.log.some((v) => v !== 0)).toBe(true)
  expect(dealt(A).some((d) => d > 0)).toBe(true)
  // The run covered a round transition rather than merely being long enough to
  // have covered one — the comment on FRAMES is a claim, and this is the check.
  expect(round(A)).toBeGreaterThan(1)
}

/** The detector has to be shown to fire. */
test('a desync on one end is caught', async () => {
  const {netA, netB} = await botMatch(300)

  expect(netA.stats.desyncs + netB.stats.desyncs).toBeGreaterThan(0)
})

/**
 * The coupling the test above uncovered, asserted so it cannot be tuned away by
 * accident: the checksum cadence must be shorter than the input history
 * (`InputHistory`, sim/input.go, 32 frames).
 */
test('the checksum cadence samples faster than the input ring forgets', () => {
  expect(CHECKSUM_EVERY).toBeLessThanOrEqual(32)
})
