import {STEP_MS} from '../game/clock'
import {createSamples, type Samples} from '../game/stats'
import {
  decodeChecksum,
  decodeInputs,
  decodePing,
  encodeChecksum,
  encodeInputs,
  encodePing,
  packetType,
  decodeControl,
  encodeControl,
  PacketType,
  REDUNDANCY,
  stampNow,
} from './packet'

/** Rollback netcode driver. */
export const INPUT_DELAY = 2
export const MAX_ROLLBACK = 8

/** Time synchronisation (02 Architecture/Rollback Netcode.md). */
export const AHEAD_LIMIT = 2
export const SKIP_EVERY = 6

/** Frames between checksum exchanges. */
export const CHECKSUM_EVERY = 30

/**
 * The rollback visualiser's backing data (01 Thesis/Supervisor
 * Deliverables.md).
 */
export const TIMELINE = 180

/** A frame simulated from the remote input that had already arrived. */
export const FRAME_CLEAN = 0
/** …from a guess, which has not been contradicted. */
export const FRAME_PREDICTED = 1
/** …from a guess that turned out wrong, and has since been replayed. */
export const FRAME_CORRECTED = 2

/**
 * The 99th-percentile rollback depth from the histogram — the number worth
 * reporting, since the mean depth of a healthy connection is near zero and
 * hides the frames that actually cost something.
 */
export function depthP99(depths: readonly number[]): number {
  const total = depths.reduce((a, b) => a + b, 0)
  if (total === 0) return 0

  let seen = 0
  for (let d = depths.length - 1; d >= 0; d--) {
    seen += depths[d]
    if (seen >= total * 0.01) return d
  }
  return 0
}

/** Every counter in NetplayStats at zero. */
function freshStats(): Omit<NetplayStats, 'dataMismatch'> {
  return {
    frames: 0,
    rollbacks: 0,
    depths: Array(MAX_ROLLBACK + 1).fill(0),
    predicted: 0,
    mispredicted: 0,
    stalls: 0,
    skipped: 0,
    dropped: 0,
    malformed: 0,
    verified: 0,
    desyncs: 0,
    desyncFrame: -1,
    rtt: createSamples(),
    replayByDepth: Array.from({length: MAX_ROLLBACK + 1}, () => createSamples()),
  }
}

/** The sim, from the driver's side. */
export interface SimBridge {
  advance(p1: number, p2: number): void
  rewind(frame: number): boolean
  /** Hash of the current state. */
  checksum(): number
  /** Hash of the character data. */
  dataVersion(): number
}

export interface NetplayStats {
  /** Frames simulated, not counting replays. */
  frames: number
  rollbacks: number
  /** Count by depth, index 1..MAX_ROLLBACK. */
  depths: number[]
  /** Frames advanced on a guess, and how many of those guesses were wrong. */
  predicted: number
  mispredicted: number
  /** Frames we refused to simulate because the remote was too far behind. */
  stalls: number
  /** Frames deliberately dropped to let the other end catch up. */
  skipped: number
  /**
   * Corrections that arrived too late to apply — a desync, if it ever fires.
   */
  dropped: number
  malformed: number
  /** Set when the peer's character data does not match ours. */
  dataMismatch: boolean

  /** Confirmed frames where both ends hashed the same state. */
  verified: number
  /** …and where they did not. */
  desyncs: number
  desyncFrame: number
  rtt: Samples
  /**
   * What a rollback's replay cost, bucketed by the depth that produced it —
   * index 1..MAX_ROLLBACK, and index 0 is never pushed to.
   */
  replayByDepth: Samples[]
}

export interface Netplay {
  /** Simulates one frame. */
  step(localBits: number): boolean
  /** Zeroes the counters without touching the match. */
  resetStats(): void
  /** Announces our character data hash. */
  hello(): void
  receive(data: ArrayBuffer): void
  ping(): void
  readonly frame: number
  /**
   * The newest frame that is both simulated and settled — every input in it is
   * known, so it cannot be simulated again.
   */
  readonly confirmed: number
  /**
   * How many frames ahead of the other end we are, with the connection's own
   * latency taken out.
   */
  readonly advantage: number
  /**
   * The confirmed inputs so far, in the replay log's order: p1 then p2, one
   * pair per frame, seat-independent.
   */
  readonly log: readonly number[]
  /** Milliseconds since anything at all arrived from the peer. */
  silentMs(): number
  /**
   * The state hashes taken every CHECKSUM_EVERY frames, in frame order, packed
   * as little-endian uint32s.
   */
  checksums(): Uint8Array
  /**
   * What happened on a frame, for the visualiser: `kind | depth << 2`, where
   * kind is one of the FRAME_* constants and depth is the depth of a rollback
   * that *landed* on this frame, or zero.
   */
  sample(frame: number): number
  readonly stats: NetplayStats
}

/**
 * @param seat which player this end drives — 0 is P1, 1 is P2. The sim always
 * takes (p1, p2) in that order on both machines, because the update order is
 * part of the spec and swapping it per-machine is a desync.
 */
export function createNetplay(
  sim: SimBridge,
  send: (data: ArrayBuffer) => void,
  seat: 0 | 1,
): Netplay {
  const local: number[] = []
  const remote: number[] = []
  const known: boolean[] = []

  /** What we actually fed the sim for the remote player at each frame. */
  const used: number[] = []

  /** Our state hash at the start of each checkpoint frame. */
  const sums = new Map<number, number>()
  /** Theirs, held until the frame is settled on our side too. */
  const theirSums = new Map<number, number>()
  let sentThrough = 0

  let frame = 0
  /** Highest frame with remote input known contiguously from the start. */
  let confirmed = -1

  /** The frame the other end was on when it sent the newest inputs we have. */
  let theirFrame = -1
  let lastSkip = -SKIP_EVERY

  // Wall clock, not the frame counter: the question is whether the other
  // machine is still there, and a stalled sim stops counting frames at exactly
  // the moment that question starts mattering.
  let lastHeard = Date.now()

  /**
   * The newest frame we have put on the wire, for the resend on a held tick.
   */
  let sentInputsThrough = -1

  /** Frames written to the log so far, so settle only ever appends. */
  let logged = -1
  const log: number[] = []

  // The visualiser's ring.
  const slotFrame = new Int32Array(TIMELINE).fill(-1)
  const slotKind = new Int8Array(TIMELINE)
  const slotDepth = new Int8Array(TIMELINE)

  /** Records what a frame was simulated from. */
  function mark(f: number, k: number): void {
    const i = f % TIMELINE
    if (slotFrame[i] === f) {
      if (slotKind[i] !== FRAME_CORRECTED) slotKind[i] = k
      return
    }
    slotFrame[i] = f
    slotKind[i] = k
    slotDepth[i] = 0
  }

  const stats: NetplayStats = {dataMismatch: false, ...freshStats()}

  /** Prediction: repeat the last input we actually saw. */
  function remoteAt(f: number): number {
    if (known[f]) return remote[f]
    for (let k = f - 1; k >= 0 && f - k <= REDUNDANCY * 4; k--) {
      if (known[k]) return remote[k]
    }
    return 0
  }

  /**
   * Frames we are ahead, corrected for the flight time of what we are reading.
   */
  function advantage(): number {
    if (theirFrame < 0 || stats.rtt.count === 0) return 0
    const flight = Math.round(stats.rtt.percentile(50) / 2 / STEP_MS)
    return frame - theirFrame - flight
  }

  /** The newest frame that is settled *and* simulated here. */
  function settled(): number {
    return Math.min(confirmed, frame - 1)
  }

  /** Appends every frame that is both simulated and confirmed. */
  function record(): void {
    const upTo = settled()
    for (let f = logged + 1; f <= upTo; f++) {
      const mine = local[f] ?? 0
      log.push(seat === 0 ? mine : remote[f], seat === 0 ? remote[f] : mine)
      logged = f
    }
  }

  function simulate(f: number, remoteBits: number): void {
    mark(f, known[f] ? FRAME_CLEAN : FRAME_PREDICTED)
    used[f] = remoteBits
    const mine = local[f] ?? 0
    if (seat === 0) sim.advance(mine, remoteBits)
    else sim.advance(remoteBits, mine)

    // Recorded on the way past, because the hash of frame N is unreachable once
    // the sim has moved on.
    const next = f + 1
    if (next % CHECKSUM_EVERY === 0) sums.set(next, sim.checksum() >>> 0)
  }

  /** Sends and compares checksums for frames that can no longer change. */
  function settle(): void {
    record()
    const upTo = confirmed + 1

    for (let f = sentThrough + CHECKSUM_EVERY; f <= upTo; f += CHECKSUM_EVERY) {
      const sum = sums.get(f)
      if (sum === undefined) break // not simulated yet; it will go out later
      send(encodeChecksum(f, sum))
      sentThrough = f
    }

    for (const [f, theirs] of theirSums) {
      if (f > upTo) continue // still predicted here; judging it now is unfair

      const ours = sums.get(f)
      if (ours === undefined) {
        // Settled but not simulated here yet — inputs can arrive faster than we
        // step.
        if (f < upTo - CHECKSUM_EVERY * 4) theirSums.delete(f)
        continue
      }

      theirSums.delete(f)
      if (ours === theirs) {
        stats.verified++
      } else {
        stats.desyncs++
        if (stats.desyncFrame < 0) stats.desyncFrame = f
      }
    }
  }

  /** The window we last sent, sent again. */
  function resendInputs(): void {
    if (sentInputsThrough >= 0) sendInputs(sentInputsThrough)
  }

  function sendInputs(through: number): void {
    sentInputsThrough = through
    const start = Math.max(0, through - REDUNDANCY + 1)
    const window = new Uint16Array(through - start + 1)
    for (let f = start; f <= through; f++) window[f - start] = local[f] ?? 0
    send(encodeInputs(start, window))
  }

  /** Rewind to `to` and re-simulate everything since with what we now know. */
  function rollback(to: number): void {
    const depth = frame - to
    // Deeper than the window is still a rollback of window depth as far as the
    // cost goes: it replayed that many frames and no more.
    const bucket = Math.min(depth, MAX_ROLLBACK)
    if (!sim.rewind(to)) {
      // Older than the rollback window.
      stats.dropped++
      return
    }

    const t0 = performance.now()
    const end = frame
    for (let f = to; f < end; f++) simulate(f, remoteAt(f))
    stats.replayByDepth[bucket].push(performance.now() - t0)

    stats.rollbacks++
    stats.depths[bucket]++

    // Recorded on the frame the rollback *landed* on, which is where the strip
    // draws the tick: that is the frame the correction was about, and its
    // height is how much had to be thrown away.
    mark(to, FRAME_CORRECTED)
    slotDepth[to % TIMELINE] = bucket
  }

  return {
    get frame() {
      return frame
    },
    get advantage() {
      return advantage()
    },
    get confirmed() {
      return settled()
    },

    sample(f) {
      // The strip asks about the last three seconds, which at the start of a
      // match runs off the front of it.
      if (f < 0) return -1
      const i = f % TIMELINE
      if (slotFrame[i] !== f) return -1
      return slotKind[i] | (slotDepth[i] << 2)
    },
    get log() {
      return log
    },
    get stats() {
      return stats
    },

    step(localBits) {
      // Nothing is recorded or sent before the guards below.

      // Never outrun the window: a misprediction older than MAX_ROLLBACK cannot
      // be corrected, so wait rather than desync.
      if (frame - confirmed > MAX_ROLLBACK) {
        stats.stalls++
        resendInputs()
        return false
      }

      // Time synchronisation: drop this frame if we are the end that is ahead.
      if (advantage() > AHEAD_LIMIT && frame - lastSkip >= SKIP_EVERY) {
        lastSkip = frame
        stats.skipped++
        resendInputs()
        return false
      }

      // Nothing is worth simulating against a peer with different frame data:
      // every frame would diverge and the checksums would report it without
      // ever saying why.
      if (stats.dataMismatch) return false

      // Our own input lands INPUT_DELAY frames from now, so it has time to
      // reach the other end before the frame it belongs to is simulated.
      const at = frame + INPUT_DELAY
      local[at] = localBits
      sendInputs(at)

      if (!known[frame]) stats.predicted++
      simulate(frame, remoteAt(frame))
      frame++
      stats.frames++
      settle()
      return true
    },

    receive(data) {
      // Before the parse and outside the try: a packet that arrived and failed
      // to decode is still evidence the peer is alive, and a malformed one is
      // the peer's bug rather than its funeral.
      lastHeard = Date.now()

      let earliest = -1
      try {
        switch (packetType(data)) {
          case PacketType.ping: {
            const {stamp, reply} = decodePing(data)
            if (reply) stats.rtt.push((stampNow() - stamp) / 10)
            else send(encodePing(stamp, true))
            return
          }
          case PacketType.control:
            if (decodeControl(data).dataVersion !== sim.dataVersion()) stats.dataMismatch = true
            return
          case PacketType.checksum: {
            const {frame: f, sum} = decodeChecksum(data)
            theirSums.set(f, sum)
            settle()
            return
          }
        }

        const {startFrame, inputs} = decodeInputs(data)

        // The newest input in the packet is the sender's own frame plus the
        // input delay, which is what they were on when they sent it.
        const theirs = startFrame + inputs.length - 1 - INPUT_DELAY
        if (theirs > theirFrame) theirFrame = theirs

        for (let i = 0; i < inputs.length; i++) {
          const f = startFrame + i
          if (known[f]) continue // redundancy: already had it, or it was resent
          known[f] = true
          remote[f] = inputs[i]

          // Already simulated this frame on a guess, and the guess was wrong.
          if (f < frame && used[f] !== inputs[i]) {
            stats.mispredicted++
            mark(f, FRAME_CORRECTED)
            if (earliest < 0 || f < earliest) earliest = f
          }
        }
      } catch {
        stats.malformed++
        return
      }

      while (known[confirmed + 1]) confirmed++
      if (earliest >= 0) rollback(earliest)
      settle()
    },

    resetStats() {
      // dataMismatch is a fact about the peer, not a measurement, so it stays.
      Object.assign(stats, freshStats())
    },

    silentMs() {
      return Date.now() - lastHeard
    },

    checksums() {
      // Sorted by frame, because frame order is what the server replays in and
      // nothing here guarantees it otherwise.
      const frames = [...sums.keys()].sort((a, b) => a - b)
      const out = new Uint8Array(frames.length * 4)
      const v = new DataView(out.buffer)
      frames.forEach((f, i) => v.setUint32(i * 4, sums.get(f)! >>> 0, true))
      return out
    },

    ping() {
      send(encodePing(stampNow(), false))
    },

    hello() {
      send(encodeControl(sim.dataVersion()))
    },
  }
}
