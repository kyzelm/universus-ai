/** Per-user settings, kept in localStorage and never seen by the sim (D127). */
export const ACTIONS = ['up', 'down', 'left', 'right', 'lp', 'mp', 'hp', 'lk', 'mk', 'hk', 'throw'] as const
export type Action = (typeof ACTIONS)[number]

/** One seat's keyboard: action to KeyboardEvent.code. */
export type Bindings = Record<Action, string>

export interface Options {
  /** Effects, 0 to 1. */
  volume: number
  /** Music, 0 to 1. */
  music: number
  keys: [Bindings, Bindings]
}

export const DEFAULTS: Options = {
  volume: 0.5,
  music: 0.3,
  keys: [
    {
      up: 'KeyW', down: 'KeyS', left: 'KeyA', right: 'KeyD',
      lp: 'KeyU', mp: 'KeyI', hp: 'KeyO', lk: 'KeyJ', mk: 'KeyK', hk: 'KeyL', throw: 'KeyH',
    },
    {
      up: 'ArrowUp', down: 'ArrowDown', left: 'ArrowLeft', right: 'ArrowRight',
      lp: 'Numpad7', mp: 'Numpad8', hp: 'Numpad9', lk: 'Numpad4', mk: 'Numpad5', hk: 'Numpad6',
      throw: 'Numpad0',
    },
  ],
}

const STORAGE_KEY = 'universus.options'

/** Stored JSON to options; anything missing or malformed falls back to the default. */
export function parseOptions(json: string | null): Options {
  let raw: Partial<Options> = {}
  try {
    raw = (JSON.parse(json ?? '{}') ?? {}) as Partial<Options>
  } catch {
    // Unreadable storage is the same as none.
  }
  const level = (v: unknown, d: number) => (typeof v === 'number' && v >= 0 && v <= 1 ? v : d)
  const seat = (i: number): Bindings => {
    const out = {...DEFAULTS.keys[i]}
    const stored = (raw.keys?.[i] ?? {}) as Partial<Bindings>
    for (const a of ACTIONS) if (typeof stored[a] === 'string') out[a] = stored[a]
    return out
  }
  return {
    volume: level(raw.volume, DEFAULTS.volume),
    music: level(raw.music, DEFAULTS.music),
    keys: [seat(0), seat(1)],
  }
}

/** The live settings; read it at use, never copy it. */
// Optional: the test environment has no storage.
export let options = parseOptions(globalThis.localStorage?.getItem(STORAGE_KEY) ?? null)

export function setOptions(next: Options): void {
  options = next
  globalThis.localStorage?.setItem(STORAGE_KEY, JSON.stringify(next))
}

/** Binds code to one action, taking it away from whatever held it before. */
export function rebind(o: Options, seat: number, action: Action, code: string): Options {
  const keys = o.keys.map((b) => ({...b})) as [Bindings, Bindings]
  for (const b of keys) for (const a of ACTIONS) if (b[a] === code) b[a] = ''
  keys[seat][action] = code
  return {...o, keys}
}
