import {playEvents} from './sound'
import {Application} from 'pixi.js'
import {CLEAN, createLink, type Impairment, type Link} from '../net/impair'
import {createNetplay, depthP99, type Netplay} from '../net/netplay'
import type {Peer} from '../net/peer'
import {
  advance,
  checksum,
  dataVersion,
  eventsAt,
  loadSim,
  NOBODY,
  numCharacters,
  PHASE_MATCH_END,
  readSnapshot,
  reset,
  rewind,
  roster,
  type Setup,
} from '../sim/wasm'
import type {Bot} from './bot'
import {encodeLog} from './log'
import {uploadResult} from '../net/report'
import {createClock, STEP_MS} from './clock'
import {createScene, VIEW_H, VIEW_W} from './draw'
import {createEvents} from './events'
import {createDummy, type DummyMode} from './dummy'
import {announcement, averageDepth, costByDepth, costHud, localHud, netHud} from './hud'
import {createInput} from './input'
import {loadSheet} from './sprites'
import {advantageText, createLab} from './lab'
import {createSamples} from './stats'

/** The loop: input, the fixed step, netplay, and what gets reported. */

/**
 * Displayed frames between HUD refreshes, counted off the display and not off
 * the sim frame.
 */
const HUD_EVERY = 30

/** Frames between pings. */
const PING_EVERY = 30

/**
 * Silence that counts as a disconnect (02 Architecture/Transport and
 * Connectivity.md).
 */
const DISCONNECT_MS = 3000

export interface Game {
  /**
   * The session so far in the replay log format: a 16-byte header carrying the
   * setup, then little-endian uint16 pairs, player 1 then player 2, one pair
   * per frame.
   */
  inputLog(): Blob
  /** Switch from local play to netplay over a connected peer. */
  connect(peer: Peer, seat: 0 | 1, session?: MatchSession): void
  /** Feed a packet in. */
  receive(data: ArrayBuffer): void
  /** True in the lab. */
  readonly training: boolean
  /** The characters this run was started with. */
  readonly chars: [number, number]
  /** Starts the match over, in the same mode. */
  restart(): void
  /** Picks what the dummy does with seat 2. */
  setDummy(mode: DummyMode): void
  /**
   * The figures the HUD draws, as plain numbers, for the measurement harness
   * (01 Thesis/Measurement Methodology.md).
   */
  measure(): Record<string, unknown>
  /** Sets the artificial network conditions applied to arriving packets. */
  impair(cfg: Impairment): void
  dispose(): void
}

/** What the room agreed on, as the game needs it. */
export interface MatchSession {
  chars?: [number, number]
  matchID?: number
  transport?: 'p2p' | 'relay'
}

/** Starts the sim, the render loop and the input layer. */
export async function startGame(
  parent: HTMLElement,
  bot?: Bot,
  training = false,
  ai = false,
  chars: [number, number] = [0, 0],
  showBoxes = false,
): Promise<Game> {
  await loadSim()
  // The characters arrive from the URL, so they are user input and clamped here
  // — the first point at which the roster is loaded and its size is known.
  const count = numCharacters()
  chars = [chars[0] % count, chars[1] % count]
  reset(training, ai, chars)

  // A sheet per seat, named after the character.
  const names = roster()
  const sheets = await Promise.all(chars.map((c) => loadSheet(names[c].name.toLowerCase())))

  const app = new Application()
  await app.init({width: VIEW_W, height: VIEW_H, background: 0x14161a, antialias: false})
  parent.appendChild(app.canvas)

  // CSS stretches the canvas to the window (index.css).
  const sizer = new ResizeObserver(() =>
    app.renderer.resize(VIEW_W, VIEW_H, (app.canvas.clientWidth * devicePixelRatio) / VIEW_W),
  )
  sizer.observe(app.canvas)

  const scene = createScene(app, sheets, showBoxes, training)
  const lab = training ? createLab() : null
  const dummy = training ? createDummy() : null

  const input = createInput()
  const clock = createClock()
  let stepCost = createSamples()

  // Wall-clock time between displayed frames.
  let frameCost = createSamples()
  let longFrames = 0

  let net: Netplay | null = null
  let link: Link | null = null
  // The match this session was recorded as, and whether its result has gone up.
  let matchID = 0
  let reported = false
  let transport: 'p2p' | 'relay' | undefined
  /** Which seat this end is driving, once connected. */
  let mySeat: 0 | 1 = 0
  /** The peer stopped sending. */
  let opponentLeft = false
  /** What the server made of the upload, for the HUD. */
  let uploadStatus = ''
  let ticks = 0
  let hudTicks = 0

  // Every playtest is a free regression log, and the corpus is worth more than
  // any single test in here — but only if it is actually recorded.
  const recorded: number[] = []

  /** The session so far in the replay log format. */
  const logBytes = (): ArrayBuffer => {
    const src = net ? net.log : recorded
    const setup: Setup = net ? {training: false, ai: false, chars} : {training, ai, chars}
    return encodeLog(setup, dataVersion(), src)
  }

  // Effects fire from state, on confirmed frames, and never from inside the sim.
  const pump = createEvents(eventsAt)

  app.ticker.add((ticker) => {
    frameCost.push(ticker.deltaMS)
    // A frame that took longer than one and a half steps is one the display did
    // not get.
    if (ticker.deltaMS > STEP_MS * 1.5) longFrames++

    // Once the opponent has gone the match is over: no reconnection window
    // (D51), so there is nothing left to simulate and the last confirmed frame
    // is the one the result is reported from.
    for (let i = opponentLeft ? 0 : clock.tick(ticker.deltaMS); i > 0; i--) {
      const [keys, p2] = input.poll()
      // The bot is a device: it stands in for the local player's keyboard and
      // is read once per fixed step, exactly where the keyboard is read.
      const p1 = bot ? bot.poll() : keys
      // So is the dummy, in the other seat.
      const p2in = dummy ? dummy.poll(readSnapshot(), p2) : p2

      const t0 = performance.now()
      if (net) {
        // WASD is always the local player, whichever seat they are in.
        net.step(p1)
        if (++ticks % PING_EVERY === 0) net.ping()
      } else {
        advance(p1, p2in)
        // What the sim was fed, dummy included — a log that recorded the
        // keyboard instead would replay a lab session nobody played.
        recorded.push(p1, p2in)
      }
      stepCost.push(performance.now() - t0)

      // Per simulated frame, not per displayed one: a display frame that runs
      // two sim steps would otherwise lose a row of the input display and
      // could miss the frame a recovery ended on.
      lab?.step(readSnapshot(), [p1, p2in])
    }

    const snap = readSnapshot()

    // Under netplay the confirmed line lags the simulated frame by the rollback
    // window; locally every frame that has run is settled the moment it has.
    pump.drain(net ? net.confirmed : snap.frame - 1, ({seat, bits}) => {
      scene.spark(snap, seat, bits)
      playEvents(bits)
    })

    // Three seconds of silence is a disconnect and the match ends there (02
    // Architecture/Transport and Connectivity.md).
    if (net && !opponentLeft && net.silentMs() > DISCONNECT_MS) {
      opponentLeft = true
    }

    // The result goes up once the match is over and the frame is settled.
    const finished = snap.phase === PHASE_MATCH_END && snap.winner !== NOBODY &&
      net !== null && net.confirmed >= snap.frame
    if (net && matchID && !reported && (finished || opponentLeft)) {
      reported = true
      const s = net.stats
      void uploadResult(matchID, {
        // The sim counts seats from 0 and the server from 1.
        winner: opponentLeft ? mySeat + 1 : snap.winner + 1,
        // The frames the log actually contains, which is what the server
        // replays and counts — not the frame this end had simulated to, which
        // on a disconnect runs past the last confirmed input.
        endFrame: net.log.length / 2,
        chars,
        disconnect: opponentLeft,
        transport,
        rttMs: s.rtt.count ? Math.round(s.rtt.percentile(50)) : 0,
        rollbackAvg: averageDepth(s),
        inputLog: new Uint8Array(logBytes()),
        checksums: net.checksums(),
      }).then((status) => {
        uploadStatus = status
      })
    }

    scene.draw(snap, {
      net,
      // The disconnect takes the screen over: immediate match end, an explicit
      // message and a win (02 Architecture/Disconnect and Match Integrity.md).
      banner: opponentLeft ? 'OPPONENT DISCONNECTED\nYOU WIN' : announcement(snap),
      lab: lab && dummy && {
        inputs: [0, 1].map((seat) => lab.history(seat).join('\n')),
        advantage: advantageText(lab.advantage()),
        dummy: `DUMMY ${dummy.mode().toUpperCase()}`,
      },
    })

    // ponytail: no interpolation.
    if (hudTicks++ % HUD_EVERY === 0) {
      const top = net ? netHud(net, link, uploadStatus) : localHud(snap)
      scene.setHud(`${top}\n${costHud(stepCost, frameCost, longFrames)}`)
    }
  })

  return {
    training,

    // A getter, not the value: connect() replaces the pair with the room's, and
    // a field captured here would still be reporting what this end picked.
    get chars() {
      return chars
    },

    restart() {
      reset(training, ai, chars)
      pump.reset()
      lab?.reset()
      scene.reset()
      recorded.length = 0
    },

    setDummy(mode) {
      dummy?.setMode(mode)
    },

    inputLog() {
      return new Blob([logBytes()], {type: 'application/octet-stream'})
    },

    connect(peer, seat, session) {
      // Never into a training match: the mode is in the state, so two ends that
      // disagreed about it would desync on frame 0.
      chars = session?.chars ?? chars
      // Zero for a private match by code, which is not recorded: a ladder made
      // of matches two people arranged between themselves is not a ladder.
      matchID = session?.matchID ?? 0
      transport = session?.transport
      mySeat = seat
      reported = false
      opponentLeft = false
      reset(false, false, chars)
      // The match restarts at frame 0, so what has been fired restarts with it.
      pump.reset()
      scene.reset()
      ticks = 0
      longFrames = 0
      // Both ends of a measured run open the same URL, so the seat is the only
      // thing that can make the two input streams differ.
      bot?.reseed(seat + 1)
      net = createNetplay({advance, rewind, checksum, dataVersion}, (data) => peer.send(data), seat)
      // Every arriving packet goes through the impairment layer, whatever it
      // is set to — the clean setting passes straight through, so a real match
      // runs the same path a measured one does.
      link = createLink((data) => net?.receive(data), CLEAN)
      net.hello()
    },

    receive(data) {
      if (link) link.receive(data)
      else net?.receive(data)
    },

    measure: () => ({
      connected: net !== null,
      frame: net ? net.frame : readSnapshot().frame,
      advantage: net?.advantage ?? 0,
      rttP50: net?.stats.rtt.percentile(50) ?? NaN,
      rttP99: net?.stats.rtt.percentile(99) ?? NaN,
      rollbacks: net?.stats.rollbacks ?? 0,
      mispredicted: net?.stats.mispredicted ?? 0,
      depthP99: net ? depthP99(net.stats.depths) : 0,
      frames: net?.stats.frames ?? 0,
      stalls: net?.stats.stalls ?? 0,
      skipped: net?.stats.skipped ?? 0,
      verified: net?.stats.verified ?? 0,
      desyncs: net?.stats.desyncs ?? 0,
      desyncFrame: net?.stats.desyncFrame ?? -1,
      dropped: net?.stats.dropped ?? 0,
      simP50: stepCost.percentile(50),
      simP99: stepCost.percentile(99),
      byDepth: costByDepth(stepCost, net),
      displayP50: frameCost.percentile(50),
      displayP99: frameCost.percentile(99),
      longFrames,
      impaired: link ? link.cfg : null,
      lost: link?.stats.dropped ?? 0,
    }),

    impair(cfg) {
      link?.set(cfg)
      // The cost numbers belong to the condition that produced them.
      stepCost = createSamples()
      frameCost = createSamples()
      longFrames = 0
      net?.resetStats()
    },

    dispose() {
      input.dispose()
      link?.dispose()
      // The Go runtime keeps running: quit() cannot be undone and loadSim is
      // cached, so a remount would have no sim.
      sizer.disconnect()
      app.destroy(true, {children: true})
    },
  }
}
