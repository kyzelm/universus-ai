import {createSamples, type Samples} from '../game/stats'

/** The artificial network condition layer. */
export interface Impairment {
  /** Delay added to every arriving packet, in milliseconds, one way. */
  delayMs: number
  /** Uniform spread either side of that delay. */
  jitterMs: number
  /** Chance a packet is dropped outright, 0-100. */
  lossPercent: number
}

/** No impairment: the packet goes straight through, with no timer in the way. */
export const CLEAN: Impairment = {delayMs: 0, jitterMs: 0, lossPercent: 0}

export interface LinkStats {
  arrived: number
  dropped: number
  /** The delay actually applied, so the HUD reports what happened. */
  delay: Samples
}

export interface Link {
  /** Feed an arriving packet in. */
  receive(data: ArrayBuffer): void
  set(cfg: Impairment): void
  readonly cfg: Impairment
  readonly stats: LinkStats
  /** Drops every packet still in flight. */
  dispose(): void
}

/** @param deliver where a packet goes once the link is done with it. */
export function createLink(
  deliver: (data: ArrayBuffer) => void,
  cfg: Impairment = CLEAN,
  seed = 1,
): Link {
  const rand = createRng(seed)
  const stats: LinkStats = {arrived: 0, dropped: 0, delay: createSamples()}
  const pending = new Set<ReturnType<typeof setTimeout>>()
  let current = {...cfg}

  return {
    get cfg() {
      return current
    },
    get stats() {
      return stats
    },

    set(next) {
      current = {...next}
      // A new condition is a new measurement.
      stats.arrived = 0
      stats.dropped = 0
      stats.delay = createSamples()
    },

    receive(data) {
      // The clean path is synchronous and allocates nothing.
      if (current.delayMs <= 0 && current.jitterMs <= 0 && current.lossPercent <= 0) {
        stats.arrived++
        deliver(data)
        return
      }

      if (rand() * 100 < current.lossPercent) {
        stats.dropped++
        return
      }

      // Every packet already carries the last few frames of inputs, so a dropped one costs nothing
      // as long as its successor arrives.
      const ms = Math.max(0, current.delayMs + (rand() * 2 - 1) * current.jitterMs)
      stats.delay.push(ms)

      const id = setTimeout(() => {
        pending.delete(id)
        stats.arrived++
        deliver(data)
      }, ms)
      pending.add(id)
    },

    dispose() {
      for (const id of pending) clearTimeout(id)
      pending.clear()
    },
  }
}

/** xorshift32, the same generator the sim uses, for the same reason. */
function createRng(seed: number): () => number {
  let x = seed >>> 0 || 1
  return () => {
    x ^= x << 13
    x >>>= 0
    x ^= x >>> 17
    x ^= x << 5
    x >>>= 0
    return x / 0x100000000
  }
}
