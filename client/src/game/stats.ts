/** A rolling window of timing samples. */
export interface Samples {
  push(v: number): void

  /** Nearest-rank percentile, 0-100. */
  percentile(p: number): number

  readonly count: number
}

export function createSamples(capacity = 600): Samples {
  const buf = new Float64Array(capacity)
  let n = 0
  let next = 0

  return {
    push(v) {
      buf[next] = v
      next = (next + 1) % capacity
      if (n < capacity) n++
    },
    percentile(p) {
      if (n === 0) return NaN
      const sorted = buf.slice(0, n).sort()
      const rank = Math.ceil((p / 100) * n)
      return sorted[Math.min(Math.max(rank, 1), n) - 1]
    },
    get count() {
      return n
    },
  }
}