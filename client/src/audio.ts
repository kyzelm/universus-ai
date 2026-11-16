import {options} from './options'

/**
 * Recorded sound: clips and one music loop, read from /sounds/<name>.mp3.
 * A file that is not there is silence, so the game runs before anything is recorded.
 */
export const CLIPS = [
  // The shell.
  'select', 'confirm', 'back', 'found', 'win', 'lose',
  // The match, replacing the synthesised tone of the same event (game/sound.ts).
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

/** Plays a clip once; false when there is no recording of it. */
export function play(name: Clip): boolean {
  if (!present.has(name)) return false
  const a = new Audio(url(name))
  a.volume = options.volume
  void a.play().catch(() => {})
  return true
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
