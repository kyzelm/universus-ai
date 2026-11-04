/** Fixed-timestep accumulator. */
export interface Clock {
  /** Steps to run for an elapsed wall-clock delta. */
  tick(dtMs: number): number
}

export const STEP_MS = 1000 / 60

/**
 * @param maxCatchup steps per frame before the leftover time is dropped. A
 * frame that takes longer than the steps it schedules would otherwise ask for
 * more steps next frame, and so on, until the tab locks up.
 * @param snapMs how far a frame delta may sit from a whole number of steps and
 * still be treated as that many. See below.
 */
export function createClock(stepMs = STEP_MS, maxCatchup = 5, snapMs = 1.5): Clock {
  let acc = 0

  return {
    tick(dtMs) {
      // Frame-rate snapping.
      const whole = Math.round(dtMs / stepMs)
      if (whole >= 1 && Math.abs(dtMs - whole * stepMs) <= snapMs) {
        dtMs = whole * stepMs
      }

      acc += dtMs

      let steps = 0
      while (acc >= stepMs && steps < maxCatchup) {
        acc -= stepMs
        steps++
      }

      // Hit the ceiling: the machine is behind, so give up on the backlog
      // rather than compounding it.
      if (steps === maxCatchup) acc = 0

      return steps
    },
  }
}