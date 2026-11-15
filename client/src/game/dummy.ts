import type {Snapshot} from '../sim/wasm'
import {IN_DOWN, IN_LEFT, IN_LP, IN_RIGHT, IN_UP} from './input'

/**
 * The training dummy: the second half of the lab overlay (03 Game Design/Game
 * Modes.md), and what makes the frame advantage readout readable by one person.
 */

export const DUMMY_MODES = ['manual', 'stand', 'crouch', 'block', 'jump', 'reversal'] as const

export type DummyMode = (typeof DUMMY_MODES)[number]

export interface Dummy {
  mode(): DummyMode
  /** Number keys pick the behaviour: 0 hands the seat back to the keyboard. */
  setMode(mode: DummyMode): void
  /** One frame's bitfield, from the state as of the frame just simulated. */
  poll(snap: Snapshot, manual: number): number
}

export function createDummy(): Dummy {
  let mode: DummyMode = 'manual'

  // Where the reversal is in its motion.
  let beat = 0

  return {
    mode: () => mode,
    setMode(next) {
      mode = next
      beat = 0
    },

    poll(snap, manual) {
      const me = snap.players[1]
      const them = snap.players[0]
      // Facing is +1 for a player looking right, so away is the other way.
      const fwd = me.facing > 0 ? IN_RIGHT : IN_LEFT
      const back = me.facing > 0 ? IN_LEFT : IN_RIGHT

      // Holding away only while the attacker is committed to something.
      // ponytail: blocks anything the attacker starts, high or low, because
      // guarding the wrong height is a mechanic the sim does not have yet.
      const guard = () => (them.moveIndex >= 0 ? back : 0)

      switch (mode) {
        case 'manual':
          return manual
        case 'stand':
          return 0
        case 'crouch':
          return IN_DOWN
        case 'block':
          return guard()
        case 'jump':
          // Pressed while it can act, released while it cannot — which in the
          // air is the whole jump.
          return me.actionable === 1 ? IN_UP : 0
        case 'reversal': {
          if (me.actionable === 1) {
            beat = 0
            return guard()
          }
          // Not actionable: knocked down, in hitstun, in blockstun.
          const step = beat++ % 3
          if (step === 0) return fwd
          if (step === 1) return IN_DOWN
          // The button lands on the frame that completes the motion: the
          // motion is judged at the frame of the press, not at the frame the
          // move comes out.
          return fwd | IN_LP
        }
      }
    },
  }
}
