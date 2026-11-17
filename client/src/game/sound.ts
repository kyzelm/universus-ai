import {play, type Clip} from '../audio'
import {
  EVENT_ATTACK,
  EVENT_BLOCK,
  EVENT_FIGHT,
  EVENT_FIREBALL,
  EVENT_HIT,
  EVENT_JUMP,
  EVENT_KNOCKDOWN,
  EVENT_KO,
  EVENT_LAND,
  EVENT_SUPER,
  EVENT_THROWN,
} from '../sim/wasm'

/** The clip each event flag plays. */
const EVENTS: readonly (readonly [number, Clip])[] = [
  [EVENT_HIT, 'hit'],
  [EVENT_BLOCK, 'block'],
  [EVENT_THROWN, 'throw'],
  [EVENT_KNOCKDOWN, 'knockdown'],
  [EVENT_SUPER, 'super'],
  [EVENT_ATTACK, 'swing'],
  [EVENT_FIREBALL, 'fireball'],
  [EVENT_JUMP, 'jump'],
  [EVENT_LAND, 'land'],
  [EVENT_KO, 'ko'],
  [EVENT_FIGHT, 'fight'],
]

/** Plays the sound of every flag in bits; call it from the event drain only. */
export function playEvents(bits: number): void {
  for (const [bit, clip] of EVENTS) if (bits & bit) play(clip)
}
