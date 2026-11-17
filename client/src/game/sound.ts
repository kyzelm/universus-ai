import {play, type Clip} from '../audio'
import {EVENT_BLOCK, EVENT_HIT, EVENT_KNOCKDOWN, EVENT_SUPER, EVENT_THROWN} from '../sim/wasm'

/** The clip each event flag plays. */
const EVENTS: readonly (readonly [number, Clip])[] = [
  [EVENT_HIT, 'hit'],
  [EVENT_BLOCK, 'block'],
  [EVENT_THROWN, 'throw'],
  [EVENT_KNOCKDOWN, 'knockdown'],
  [EVENT_SUPER, 'super'],
]

/** Plays the sound of every flag in bits; call it from the event drain only. */
export function playEvents(bits: number): void {
  for (const [bit, clip] of EVENTS) if (bits & bit) play(clip)
}
