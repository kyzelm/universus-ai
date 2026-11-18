import type {Link} from '../net/impair'
import {depthP99, MAX_ROLLBACK, type Netplay} from '../net/netplay'
import {
  BAR_UNITS,
  NOBODY,
  PHASE_FIGHT,
  PHASE_INTRO,
  PHASE_KO,
  PHASE_MATCH_END,
  type Snapshot,
  STATE_NAMES,
} from '../sim/wasm'
import {STEP_MS} from './clock'
import type {Samples} from './stats'

/** The HUD's text: every string on screen that is built from numbers. */

/** The round clock in whole seconds, rounded up. */
export function seconds(frames: number): number {
  return Math.ceil((frames * STEP_MS) / 1000)
}

/** What is written across the middle of the screen. */
export function announcement(snap: Snapshot): string {
  const who = (seat: number) => `PLAYER ${seat + 1}`

  switch (snap.phase) {
    case PHASE_FIGHT:
      return ''
    case PHASE_KO:
      // A drawn round ended either on a double KO or on the clock.
      if (snap.roundWinner === NOBODY) return 'DRAW'
      return snap.timer === 0 ? 'TIME UP' : 'K.O.'
    case PHASE_INTRO:
      return 'FIGHT'
    case PHASE_MATCH_END:
      return `${who(snap.winner)} WINS THE MATCH`
    default:
      return snap.roundWinner === NOBODY ? 'DRAW' : `${who(snap.roundWinner)} WINS THE ROUND`
  }
}

/** One line per player. */
export function localHud(snap: Snapshot): string {
  const who = (i: number) => {
    const p = snap.players[i]
    const meter = (p.super / BAR_UNITS).toFixed(1)
    return (
      `p${i} ${STATE_NAMES[p.state] ?? p.state}:${p.stateFrame} hp ${p.health} ` +
      `super ${meter}` +
      (p.combo ? ` combo ${p.combo}` : '')
    )
  }
  return [
    `frame ${snap.frame}${snap.hitstop ? `  HITSTOP ${snap.hitstop}` : ''}`,
    who(0),
    who(1),
  ].join('\n')
}

/** M-A as the methodology asks for it. */
export function costByDepth(step: Samples, net: Netplay | null) {
  // percentile(100) is the maximum.
  const row = (depth: number, s: Samples) => ({
    depth,
    count: s.count,
    p50: s.percentile(50),
    p99: s.percentile(99),
    max: s.percentile(100),
  })

  const rows = [row(0, step)]
  if (net) {
    for (let d = 1; d <= MAX_ROLLBACK; d++) rows.push(row(d, net.stats.replayByDepth[d]))
  }
  return rows
}

/** The cost line, shown in both modes. */
export function costHud(stepCost: Samples, frameCost: Samples, longFrames: number): string {
  const sim = (p: number) => stepCost.percentile(p).toFixed(3)
  const frame = (p: number) => frameCost.percentile(p).toFixed(1)

  return (
    `sim p50 ${sim(50)}ms p99 ${sim(99)}ms  ` +
    `frame p50 ${frame(50)}ms p99 ${frame(99)}ms  long ${longFrames}`
  )
}

/** Mean rollback depth over the session. */
export function averageDepth(s: Netplay['stats']): number {
  let frames = 0
  let total = 0
  for (let d = 1; d < s.depths.length; d++) {
    frames += s.depths[d]
    total += s.depths[d] * d
  }
  return frames ? total / frames : 0
}

export function netHud(net: Netplay, link: Link | null, upload: string): string {
  const s = net.stats
  if (s.dataMismatch) {
    return 'REFUSED: the peer has different character data.\nRebuild both ends from the same commit.'
  }
  const pct = (n: number) => ((n / Math.max(1, s.frames)) * 100).toFixed(1)
  const rtt = (p: number) => (s.rtt.count ? s.rtt.percentile(p).toFixed(1) : '—')

  return [
    // Ahead and behind read differently and both matter.
    `frame ${net.frame}  ahead ${net.advantage > 0 ? '+' : ''}${net.advantage}  ` +
      `rtt p50 ${rtt(50)}ms p99 ${rtt(99)}ms`,
    `rollback ${pct(s.rollbacks)}% depth p99 ${depthP99(s.depths)}  ` +
      `mispredict ${pct(s.mispredicted)}%`,
    `stalls ${s.stalls}  skips ${s.skipped}  checked ${s.verified}  ` +
      (s.desyncs ? `DESYNC at frame ${s.desyncFrame}` : 'desync 0'),
    conditions(link),
    // What the server made of the result, once there is one.
    upload && `result: ${upload}`,
  ]
    .filter(Boolean)
    .join('\n')
}

/** The artificial conditions in force, and what they actually did. */
export function conditions(link: Link | null): string {
  if (!link) return ''
  const {delayMs, jitterMs, lossPercent} = link.cfg
  if (delayMs <= 0 && jitterMs <= 0 && lossPercent <= 0) return ''

  const {arrived, dropped} = link.stats
  const rate = ((dropped / Math.max(1, arrived + dropped)) * 100).toFixed(1)
  return `SIM-NET +${delayMs}±${jitterMs}ms ${lossPercent}% loss (dropped ${dropped}, ${rate}%)`
}
