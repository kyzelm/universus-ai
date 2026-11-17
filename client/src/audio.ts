import {options} from './options'

/**
 * Recorded sound: clips and one music loop, read from /sounds/<name>.mp3.
 * A clip with no file plays a synthesised tone instead; a track with no file is silence.
 */
export const CLIPS = [
  // The shell.
  'select', 'confirm', 'back', 'found', 'win', 'lose',
  // The match, fired from the event drain (game/sound.ts).
  'hit', 'block', 'throw', 'knockdown', 'super',
] as const
export type Clip = (typeof CLIPS)[number]
export type Track = 'menu' | 'match'

const url = (name: string) => `/sounds/${name}.mp3`

/** The files that exist, found by asking once at startup. */
const present = new Set<string>()
// The dev server answers a missing file with the page itself, so the type is checked too.
const probe = (name: string) =>
  fetch(url(name), {method: 'HEAD'})
    .then((r) => {
      if (r.ok && r.headers.get('content-type')?.startsWith('audio')) present.add(name)
    })
    .catch(() => {})
// Optional: the test environment has no fetch target.
if (typeof Audio !== 'undefined') for (const n of [...CLIPS, 'menu', 'match']) void probe(n)

/** The tone a clip plays until it is recorded: start Hz, seconds, waveform. */
const TONES: Record<Clip, readonly [number, number, OscillatorType]> = {
  select: [520, 0.04, 'triangle'],
  confirm: [780, 0.12, 'triangle'],
  back: [330, 0.08, 'triangle'],
  found: [440, 0.5, 'square'],
  win: [660, 0.6, 'triangle'],
  lose: [180, 0.6, 'sine'],
  hit: [220, 0.08, 'square'],
  block: [660, 0.05, 'triangle'],
  throw: [140, 0.15, 'sawtooth'],
  knockdown: [70, 0.2, 'sine'],
  super: [880, 0.4, 'sawtooth'],
}

/** Tone gain at full volume. */
const MAX_GAIN = 0.3

let ctx: AudioContext | undefined

/** Plays a clip once, its recording if there is one, else its tone. */
export function play(name: Clip): void {
  if (present.has(name)) {
    const a = new Audio(url(name))
    a.volume = options.volume
    void a.play().catch(() => {})
    return
  }
  if (typeof AudioContext === 'undefined') return
  ctx ??= new AudioContext()
  // The browser keeps a context suspended until the first click or key press.
  void ctx.resume()
  const [hz, seconds, type] = TONES[name]
  const end = ctx.currentTime + seconds
  const osc = new OscillatorNode(ctx, {type, frequency: hz})
  const gain = new GainNode(ctx, {gain: Math.max(options.volume * MAX_GAIN, 0.001)})
  // Pitch and volume both fall away, which is what makes a tone read as an impact.
  osc.frequency.exponentialRampToValueAtTime(hz / 2, end)
  gain.gain.exponentialRampToValueAtTime(0.001, end)
  osc.connect(gain).connect(ctx.destination)
  osc.start()
  osc.stop(end)
}

let player: HTMLAudioElement | undefined
let wanted: Track | null = null

/** Loops a track, or stops the music with null. */
export function music(track: Track | null): void {
  wanted = track
  if (typeof Audio === 'undefined') return
  player ??= Object.assign(new Audio(), {loop: true})
  if (!track || !present.has(track)) return player.pause()
  if (!player.src.endsWith(url(track))) player.src = url(track)
  player.volume = options.music
  // Refused until the first click or key press; the listener below asks again then.
  if (player.paused) void player.play().catch(() => {})
}

/** Applies a changed music volume to the track already playing. */
export function refreshVolume(): void {
  if (player) player.volume = options.music
}

if (typeof window !== 'undefined') {
  // The probes are still in flight at the first call, and the browser blocks audio before a gesture.
  const retry = () => music(wanted)
  window.addEventListener('pointerdown', retry)
  window.addEventListener('keydown', retry)
}
