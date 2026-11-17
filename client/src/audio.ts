import {options} from './options'

/** Sound files, /sounds/<name>.mp3, made by tools/sounds.py or recorded (art/sounds/). */
export const CLIPS = [
  // The shell.
  'select', 'confirm', 'back', 'found', 'win', 'lose',
  // The match, fired from the event drain (game/sound.ts).
  'hit', 'block', 'throw', 'knockdown', 'super', 'swing', 'fireball', 'jump', 'land', 'ko', 'fight',
] as const
export type Clip = (typeof CLIPS)[number]
export type Track = 'menu' | 'match'

// Optional: the test environment has no Web Audio.
const ctx = typeof AudioContext === 'undefined' ? undefined : new AudioContext()
const buffers = new Map<string, AudioBuffer>()

let playing: {track: Track; src: AudioBufferSourceNode; gain: GainNode} | undefined
let wanted: Track | null = null

// A missing file fails to decode (the dev server answers it with the page) and is silence.
if (ctx) {
  for (const name of [...CLIPS, 'menu', 'match']) {
    fetch(`/sounds/${name}.mp3`)
      .then((r) => r.arrayBuffer())
      .then((b) => ctx.decodeAudioData(b))
      .then((buffer) => {
        buffers.set(name, buffer)
        if (name === wanted) music(wanted)
      })
      .catch(() => {})
  }
  // The browser keeps the context suspended until the first click or key press.
  const resume = () => void ctx.resume()
  window.addEventListener('pointerdown', resume)
  window.addEventListener('keydown', resume)
}

function start(name: string, volume: number, loop: boolean) {
  const buffer = buffers.get(name)
  if (!ctx || !buffer) return undefined
  const gain = new GainNode(ctx, {gain: volume})
  const src = new AudioBufferSourceNode(ctx, {buffer, loop})
  src.connect(gain).connect(ctx.destination)
  src.start()
  return {src, gain}
}

/** Plays a clip once. */
export function play(name: Clip): void {
  // Clips fired before the first gesture would all play at once when it comes.
  if (ctx?.state === 'running') start(name, options.volume, false)
}

/** Loops a track, or stops the music with null. */
export function music(track: Track | null): void {
  wanted = track
  if (playing?.track === track) return
  playing?.src.stop()
  playing = undefined
  const p = track && start(track, options.music, true)
  if (p) playing = {track: track!, ...p}
}

/** Applies a changed music volume to the track already playing. */
export function refreshVolume(): void {
  if (playing) playing.gain.gain.value = options.music
}
