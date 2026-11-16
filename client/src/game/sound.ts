import {play, type Clip} from '../audio'
import {options} from '../options'
import {EVENT_BLOCK, EVENT_HIT, EVENT_KNOCKDOWN, EVENT_SUPER, EVENT_THROWN} from '../sim/wasm'

// A recording in /sounds wins; the synthesised tone is what plays until one exists (04 Art/Audio.md).
/** One sound per event flag: bit, recording, then the tone's start Hz, seconds and waveform. */
const TONES: readonly (readonly [number, Clip, number, number, OscillatorType])[] = [
  [EVENT_HIT, 'hit', 220, 0.08, 'square'],
  [EVENT_BLOCK, 'block', 660, 0.05, 'triangle'],
  [EVENT_THROWN, 'throw', 140, 0.15, 'sawtooth'],
  [EVENT_KNOCKDOWN, 'knockdown', 70, 0.2, 'sine'],
  [EVENT_SUPER, 'super', 880, 0.4, 'sawtooth'],
]

/** Gain at full volume. */
const MAX_GAIN = 0.3

let ctx: AudioContext | undefined

/** Plays the sound of every flag in bits; call it from the event drain only. */
export function playEvents(bits: number): void {
  if (typeof AudioContext === 'undefined') return
  ctx ??= new AudioContext()
  // The browser keeps a context suspended until the first key press.
  void ctx.resume()

  for (const [bit, clip, hz, seconds, type] of TONES) {
    if (!(bits & bit) || play(clip)) continue
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
}
