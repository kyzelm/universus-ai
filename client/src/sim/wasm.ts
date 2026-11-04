declare class Go {
  importObject: WebAssembly.Imports

  run(instance: WebAssembly.Instance): Promise<void>
}

declare global {
  let sim: {
    advance(p1: number, p2: number): void
    rewind(frame: number): boolean
    eventsAt(frame: number): number
    reset(training?: boolean, ai?: number, c0?: number, c1?: number): void
    checksum(): number
    dataVersion(): number
    numCharacters(): number
    characterName(i: number): string
    stageHalfWidth(): number
    cameraHalfWidth(): number
    snapshotPtr(): number
    snapshotLen(): number
    noop(): void
    quit(): void
  }
}

/** 16.16 fixed-point scale. */
const ONE = 65536

/**
 * Snapshot layout — the contract with sim/snapshot.go, which cannot check it
 * from its side.
 */
const MAX_BOXES = 4
const MAX_PROJECTILES = 4
const BOX = 4 * 4
const HIT_OFF = 72 + MAX_BOXES * BOX
const PLAYER_SIZE = 13 * 4 + BOX + 2 * (4 + MAX_BOXES * BOX)
const HEADER = 9 * 4
const PROJ_OFF = HEADER + 2 * PLAYER_SIZE

/** Mirrors the state constants in sim/fighter.go, for the debug readout. */
export const STATE_NAMES = [
  'idle',
  'walkF',
  'walkB',
  'crouch',
  'dash',
  'backdash',
  'prejump',
  'air',
  'attack',
  'hitstun',
  'blockstun',
  'landing',
  'thrown',
  'knockdown',
]

/** Round phases, mirroring sim/round.go. */
export const PHASE_FIGHT = 0
export const PHASE_KO = 1
export const PHASE_ROUND_END = 2
export const PHASE_INTRO = 3
export const PHASE_MATCH_END = 4

/** No winner: a draw round, or a match that is still being played. */
export const NOBODY = -1

/**
 * Resource maxima, mirroring sim/resource.go: a bar is a thousand units, and
 * the gauges are 6 and 3 bars.
 */
export const DRIVE_BARS = 6
export const SUPER_BARS = 3
export const BAR_UNITS = 1000

/** Counter-hit classes, mirroring sim/damage.go. */
export const COUNTER_NAMES = ['', 'COUNTER', 'PUNISH COUNTER']

/** Effect flags, mirroring sim/events.go. */
export const EVENT_HIT = 1 << 0
export const EVENT_BLOCK = 1 << 1
export const EVENT_THROWN = 1 << 2
export const EVENT_KNOCKDOWN = 1 << 3
export const EVENT_SUPER = 1 << 4

/** How many frames of events the sim keeps, mirroring sim/rollback.go. */
export const EVENT_RING = 64

export interface Box {
  x: number
  y: number
  w: number
  h: number
}

export interface PlayerSnapshot {
  x: number
  y: number
  facing: number
  moveIndex: number
  state: number
  stateFrame: number
  health: number
  drive: number
  super: number
  /** 1 while the Drive gauge is refilling from empty. */
  burnout: number
  /** Hits taken without recovering; 0 when not in a combo. */
  combo: number
  /** Class of the most recent hit taken — index into COUNTER_NAMES. */
  counter: number
  /** 1 while this player may act. */
  actionable: number
  pushbox: Box
  hurtboxes: Box[]
  hitboxes: Box[]
}

export interface Snapshot {
  frame: number
  camX: number
  hitstop: number
  /** Round phase — one of the PHASE_* constants. */
  phase: number
  /** The round clock, in frames. */
  timer: number
  /** Rounds won, per seat. */
  wins: number[]
  /** Who took the round just decided, or NOBODY for a draw. */
  roundWinner: number
  /** Match winner once the phase is PHASE_MATCH_END, NOBODY until then. */
  winner: number
  players: PlayerSnapshot[]
  /**
   * Live projectiles, packed from the front — the sim's slot numbers do not
   * cross, because nothing over here needs to name one.
   */
  projectiles: Box[]
}

let mem: WebAssembly.Memory
let view: DataView
let loading: Promise<void> | undefined

/** Fetches and starts the sim. */
export function loadSim(): Promise<void> {
  return (loading ??= fetchAndInit())
}

async function fetchAndInit(): Promise<void> {
  const res = await fetch('/main.wasm')
  const type = res.headers.get('content-type')
  if (!res.ok || type !== 'application/wasm') {
    throw new Error(`sim: /main.wasm not served as wasm (${res.status}, ${type})`)
  }
  await initSim(res)
}

/** Starts the sim from an already-obtained module. */
export async function initSim(src: Response | BufferSource): Promise<void> {
  const go = new Go()
  const {instance} =
    src instanceof Response
      ? await WebAssembly.instantiateStreaming(src, go.importObject)
      : await WebAssembly.instantiate(src, go.importObject)

  // go.run resolves only when the sim calls quit, so it is deliberately not awaited.
  void go.run(instance)

  mem = instance.exports.mem as WebAssembly.Memory
  view = newView()
}

function newView(): DataView {
  return new DataView(mem.buffer, sim.snapshotPtr(), sim.snapshotLen())
}

/** The snapshot buffer. */
function snapshot(): DataView {
  if (view.buffer !== mem.buffer) view = newView()
  return view
}

export function advance(p1: number, p2: number): void {
  sim.advance(p1, p2)
}

/**
 * Restores the state at the start of `frame`; the caller replays with advance.
 */
export function rewind(frame: number): boolean {
  return sim.rewind(frame)
}

/** What happened on a frame, per seat. */
export function eventsAt(frame: number): [number, number] {
  const packed = sim.eventsAt(frame)
  return [packed & 0xffff, (packed >>> 16) & 0xffff]
}

/** Difficulty tiers for the scripted opponent, mirroring sim/ai.go. */
export const AI_TIERS = ['off', 'easy', 'normal', 'hard'] as const

export type AITier = (typeof AI_TIERS)[number]

/**
 * Everything about a match that is chosen before frame 0 and cannot be read
 * back out of its input log: which characters, which mode, whether seat 2 is
 * the scripted opponent.
 */
export interface Setup {
  training: boolean
  ai: number
  chars: [number, number]
}

/** Starts a fresh match. */
export function reset(training = false, ai = 0, chars: [number, number] = [0, 0]): void {
  sim.reset(training, ai, chars[0], chars[1])
}

/**
 * The roster, as the character select needs it: index and display name, in the
 * order the sim holds them.
 */
export function roster(): {index: number; name: string}[] {
  return Array.from({length: numCharacters()}, (_, index) => ({
    index,
    name: sim.characterName(index),
  }))
}

/** Half the stage and half the screen, in units — balance data, read once. */
export function stageGeometry(): {stageHalf: number; camHalf: number} {
  return {stageHalf: sim.stageHalfWidth(), camHalf: sim.cameraHalfWidth()}
}

/** How many entries the loaded roster has. */
export function numCharacters(): number {
  return sim.numCharacters()
}

/** FNV-1a over the packed state. */
export function checksum(): number {
  return sim.checksum()
}

/** Hash of the embedded character data. */
export function dataVersion(): number {
  return sim.dataVersion()
}

function readBox(v: DataView, o: number): Box {
  return {
    x: v.getInt32(o, true) / ONE,
    y: v.getInt32(o + 4, true) / ONE,
    w: v.getInt32(o + 8, true) / ONE,
    h: v.getInt32(o + 12, true) / ONE,
  }
}

function readBoxList(v: DataView, countOffset: number, cap = MAX_BOXES): Box[] {
  const n = v.getUint32(countOffset, true)
  const out: Box[] = []
  for (let i = 0; i < n && i < cap; i++) {
    out.push(readBox(v, countOffset + 4 + i * BOX))
  }
  return out
}

// ponytail: allocates a small object graph per frame.
export function readSnapshot(): Snapshot {
  const v = snapshot()
  return {
    frame: v.getUint32(0, true),
    camX: v.getInt32(4, true) / ONE,
    hitstop: v.getInt32(8, true),
    phase: v.getInt32(12, true),
    timer: v.getInt32(16, true),
    wins: [v.getInt32(20, true), v.getInt32(24, true)],
    roundWinner: v.getInt32(28, true),
    winner: v.getInt32(32, true),
    players: [0, 1].map((i) => {
      const o = HEADER + i * PLAYER_SIZE
      return {
        x: v.getInt32(o, true) / ONE,
        y: v.getInt32(o + 4, true) / ONE,
        facing: v.getInt32(o + 8, true),
        moveIndex: v.getInt32(o + 12, true),
        state: v.getInt32(o + 16, true),
        stateFrame: v.getInt32(o + 20, true),
        health: v.getInt32(o + 24, true),
        drive: v.getInt32(o + 28, true),
        super: v.getInt32(o + 32, true),
        burnout: v.getInt32(o + 36, true),
        combo: v.getInt32(o + 40, true),
        counter: v.getInt32(o + 44, true),
        actionable: v.getInt32(o + 48, true),
        pushbox: readBox(v, o + 52),
        hurtboxes: readBoxList(v, o + 68),
        hitboxes: readBoxList(v, o + HIT_OFF),
      }
    }),
    projectiles: readBoxList(v, PROJ_OFF, MAX_PROJECTILES),
  }
}
