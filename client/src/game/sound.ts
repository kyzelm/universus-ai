import {EVENT_BLOCK, EVENT_HIT, EVENT_KNOCKDOWN, EVENT_SUPER, EVENT_THROWN} from '../sim/wasm'

// ponytail: synthesised tones stand in for the recordings (04 Art/Audio.md), swap in samples when they exist.
/** One tone per event flag: bit, start Hz, seconds, waveform. */
const TONES: readonly (readonly [number, number, number, OscillatorType])[] = [
  [EVENT_HIT, 220, 0.08, 'square'],
  [EVENT_BLOCK, 660, 0.05, 'triangle'],
  [EVENT_THROWN, 140, 0.15, 'sawtooth'],
  [EVENT_KNOCKDOWN, 70, 0.2, 'sine'],
  [EVENT_SUPER, 880, 0.4, 'sawtooth'],
]

const VOLUME = 0.15

let ctx: AudioContext | undefined

/** Plays the sound of every flag in bits; call it from the event drain only. */
export function playEvents(bits: number): void {
  if (typeof AudioContext === 'undefined') return
  ctx ??= new AudioContext()
  // The browser keeps a context suspended until the first key press.
  void ctx.resume()

  for (const [bit, hz, seconds, type] of TONES) {
    if (!(bits & bit)) continue
    const end = ctx.currentTime + seconds
    const osc = new OscillatorNode(ctx, {type, frequency: hz})
    const gain = new GainNode(ctx, {gain: VOLUME})
    // Pitch and volume both fall away, which is what makes a tone read as an impact.
    osc.frequency.exponentialRampToValueAtTime(hz / 2, end)
    gain.gain.exponentialRampToValueAtTime(0.001, end)
    osc.connect(gain).connect(ctx.destination)
    osc.start()
    osc.stop(end)
  }
}
