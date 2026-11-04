import {EVENT_RING} from '../sim/wasm'

/** Firing effects without firing them eight times. */
export interface Fired {
  frame: number
  /**
   * 0 or 1 — the player the effect happened *to*, which is where to draw it.
   */
  seat: number
  /** The EVENT_* bits set on that seat for that frame. */
  bits: number
}

export interface Events {
  /** Fires everything settled since the last call, oldest frame first. */
  drain(confirmed: number, fire: (f: Fired) => void): void
  /** Forgets what has been fired. */
  reset(): void
}

/**
 * @param read the sim's per-frame event words, one per seat. Called only for
 * frames inside the sim's own ring; older ones read as zero there anyway,
 * which is why that ring is where the catch-up below stops.
 */
export function createEvents(read: (frame: number) => readonly [number, number]): Events {
  let firedThrough = -1

  return {
    drain(confirmed, fire) {
      // A backgrounded tab can confirm hundreds of frames at once.
      const from = Math.max(firedThrough + 1, confirmed - EVENT_RING + 1)

      for (let f = from; f <= confirmed; f++) {
        const bits = read(f)
        for (let seat = 0; seat < bits.length; seat++) {
          if (bits[seat] !== 0) fire({frame: f, seat, bits: bits[seat]})
        }
      }
      if (confirmed > firedThrough) firedThrough = confirmed
    },

    reset() {
      firedThrough = -1
    },
  }
}
