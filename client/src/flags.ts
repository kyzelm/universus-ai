/** Every URL flag the client reads, in one place. */
export interface Flags {
  training: boolean
  ai: boolean
  /** ?bot hands the local player to a seeded input device. */
  bot: boolean
  /** ?bench leaves the sim untouched for a measurement harness. */
  bench: boolean
  /** ?boxes shows the box overlay outside training. */
  boxes: boolean
  /** ?sweep replaces the game with the M-A harness. */
  sweep: boolean
  /** ?room=CODE joins on load. */
  room: string | null
  /** ?relay=ws://host overrides the socket origin. */
  relay: string | null
  /** ?ice=off makes ICE unable to connect, forcing the relay (D15, D88). */
  iceOff: boolean
  /** ?p1=0&p2=1 picks the roster entries, defaulting to the first for both. */
  chars: [number, number]
  /** ?rtt=200&loss=5&jitter=15, null when absent. */
  rtt: number | null
  loss: number | null
  jitter: number | null
  /** True when the URL names part of a run, false for somebody arriving. */
  run: boolean
}

const RUN_PARAMS = ['training', 'ai', 'room', 'bot', 'bench', 'p1', 'p2']

export function parseFlags(search: string): Flags {
  const p = new URLSearchParams(search)
  const pick = (key: string) => {
    const n = Number(p.get(key))
    return Number.isInteger(n) && n >= 0 ? n : 0
  }
  const amount = (key: string) => (p.has(key) ? Math.max(0, Number(p.get(key)) || 0) : null)
  return {
    training: p.has('training'),
    ai: p.has('ai'),
    bot: p.has('bot'),
    bench: p.has('bench'),
    boxes: p.has('boxes'),
    sweep: p.has('sweep'),
    room: p.get('room'),
    relay: p.get('relay'),
    iceOff: p.get('ice') === 'off',
    chars: [pick('p1'), pick('p2')],
    rtt: amount('rtt'),
    loss: amount('loss'),
    jitter: amount('jitter'),
    run: RUN_PARAMS.some((k) => p.has(k)),
  }
}

/** The page's own flags, read once. */
export const flags = parseFlags(location.search)
