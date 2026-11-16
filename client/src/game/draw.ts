import {type Application, Container, Graphics, Sprite, Text} from 'pixi.js'
import {MAX_ROLLBACK, TIMELINE, type Netplay} from '../net/netplay'
import {
  BAR_UNITS,
  type Box,
  COUNTER_NAMES,
  EVENT_BLOCK,
  EVENT_HIT,
  EVENT_KNOCKDOWN,
  EVENT_SUPER,
  EVENT_THROWN,
  type PlayerSnapshot,
  type Snapshot,
  stageGeometry,
  SUPER_BARS,
} from '../sim/wasm'
import {seconds} from './hud'
import type {Sheet} from './sprites'

/** Everything Pixi draws: the scene, built once, and one draw per displayed frame. */

export const VIEW_W = 800
export const VIEW_H = 450

/** Pixels per sim unit at full zoom-out: the 400-unit screen fills the view. */
const SCALE = 2
const GROUND_PX = 380

/**
 * Zoom is framing, not gameplay: the sim's camera (and the screen-edge walls
 * that cap separation) always use the full screen width.
 */
const ZOOM_CLOSE_HALF = 160
const ZOOM_START = 240
const ZOOM_FULL = 80
/** Fraction of the gap to the target half-width closed per displayed frame. */
const ZOOM_SMOOTH = 0.08
/** How close to the side of a zoomed frame a player gets before it shifts. */
const ZOOM_EDGE = 40

/**
 * Debug box colours, the convention every fighting game debug view uses: blue
 * pushbox, green hurtbox, red hitbox.
 */
const PUSH_COLOR = 0x4a8ce0
const HURT_COLOR = 0x4ae08c
const HIT_COLOR = 0xe0574a

/** Effect colours, one per event flag. */
const EVENT_COLORS: readonly (readonly [number, number])[] = [
  [EVENT_HIT, 0xffe08a],
  [EVENT_BLOCK, 0x8ac8ff],
  [EVENT_THROWN, 0xffa84a],
  [EVENT_KNOCKDOWN, 0xff7a4a],
  [EVENT_SUPER, 0xd08aff],
]

/** How long a spark lives, in display frames. */
const SPARK_FRAMES = 10
const SPARK_RADIUS = 14

/** How long the super flash fades for, in display frames. */
const FLASH_FRAMES = 20

/** The rollback visualiser (01 Thesis/Supervisor Deliverables.md). */
const STRIP_COL = 2
const STRIP_H = 12
const STRIP_DEPTH_H = 22
const STRIP_X = 10
const STRIP_Y = VIEW_H - 32

/** Clean, predicted, corrected — indexed by the FRAME_* constants. */
const STRIP_COLORS = [0x3a7a4a, 0xd8a53a, 0xe0574a]

/** What the loop hands the scene besides the snapshot. */
export interface Overlay {
  net: Netplay | null
  /** Replaces the round announcement when set. */
  banner: string
  /** The lab's readouts, or null outside training. */
  lab: {inputs: string[]; advantage: string; dummy: string} | null
}

export interface Scene {
  /** Starts a spark for every event flag in bits, at that seat's fighter. */
  spark(snap: Snapshot, seat: number, bits: number): void
  draw(snap: Snapshot, o: Overlay): void
  setHud(text: string): void
  /** Drops what belonged to the match that just ended. */
  reset(): void
}

/** Builds the stage, back to front. */
export function createScene(app: Application, sheets: Sheet[], showBoxes: boolean, training: boolean): Scene {
  // The world scrolls under a fixed camera; the stage layer does not.
  const world = new Container()
  // Balance data, read once from the sim so the view frames what it collides.
  const {stageHalf, camHalf: screenHalf} = stageGeometry()
  let camHalf = screenHalf
  app.stage.addChild(world)

  world.addChild(new Graphics().rect(-2000, GROUND_PX, 4000, VIEW_H - GROUND_PX).fill(0x2a2f38))

  // Wall markers, so the corner is visible as a place rather than a surprise.
  for (const side of [-1, 1]) {
    world.addChild(
      new Graphics().rect(side * stageHalf * SCALE - 2, 0, 4, GROUND_PX).fill(0x3a4150),
    )
  }

  // Fighters go under the overlay: boxes are a thing drawn *on* a character.
  const fighters = sheets.map((sheet) => {
    const s = new Sprite()
    s.anchor.set(sheet.anchor.x, sheet.anchor.y)
    world.addChild(s)
    return s
  })

  // Projectiles have no sprite yet, so they are drawn as the hitbox they are —
  // on their own layer, because that hitbox is the only thing on screen saying
  // a fireball exists.
  const projectiles = new Graphics()
  world.addChild(projectiles)

  const boxes = new Graphics()
  boxes.visible = showBoxes
  world.addChild(boxes)

  // Sparks live in the world, not on the stage: an effect at a fighter's feet
  // has to scroll with the fighter.
  const sparkLayer = new Graphics()
  world.addChild(sparkLayer)

  // The super flash sits over the world and under the bars.
  const flash = new Graphics().rect(0, 0, VIEW_W, VIEW_H).fill(0xffffff)
  flash.alpha = 0
  app.stage.addChild(flash)
  let flashLife = 0

  const bars = [new Graphics(), new Graphics()]
  bars.forEach((b) => app.stage.addChild(b))

  const addText = (x: number, y: number, ax: number, ay: number, fontSize: number,
    fill = 0x8a94a6, fontWeight: 'bold' | 'normal' = 'normal'): Text => {
    const t = new Text({text: '', style: {fill, fontFamily: 'monospace', fontSize, fontWeight}})
    t.position.set(x, y)
    t.anchor.set(ax, ay)
    app.stage.addChild(t)
    return t
  }

  // The combo readout, one per seat.
  const combos = [0, 1].map((seat) =>
    addText(seat ? VIEW_W - 12 : 12, 48, seat, 0, 14, 0xe0c04a, 'bold'))

  // The round clock and the announcement.
  const timer = addText(VIEW_W / 2, 6, 0.5, 0, 26, 0xe8ecf2, 'bold')
  const announce = addText(VIEW_W / 2, 150, 0.5, 0.5, 30, 0xe8ecf2, 'bold')

  const strip = new Graphics()
  app.stage.addChild(strip)
  const stripKey = addText(STRIP_X, STRIP_Y + STRIP_H + 2, 0, 0, 10)
  const hud = addText(8, 66, 0, 0, 12)

  // The lab overlay.
  // One column per seat, at the edges the fighters spend least time against.
  const inputCols = training ? [addText(8, 190, 0, 0, 11), addText(VIEW_W - 8, 190, 1, 0, 11)] : null
  // Over the column for the seat it drives, so what the dummy is doing and what
  // it is pressing read as one thing.
  const dummyText = training ? addText(VIEW_W - 8, 176, 1, 0, 11) : null
  // Named for the frame advantage on screen, not the netplay one: net.advantage
  // is how far ahead of the other machine this one is running.
  const advText = training ? addText(VIEW_W / 2, 38, 0.5, 0, 16) : null

  const sparks: {x: number; y: number; life: number; color: number}[] = []

  return {
    spark(snap, seat, bits) {
      const p = snap.players[seat]
      for (const [bit, color] of EVENT_COLORS) {
        // ponytail: drawn where the fighter is now, not where they were on the
        // event's own frame.
        if (bits & bit) sparks.push({x: p.x, y: p.y, life: SPARK_FRAMES, color})
      }
      if (bits & EVENT_SUPER) flashLife = FLASH_FRAMES
    },

    setHud(text) {
      hud.text = text
    },

    reset() {
      sparks.length = 0
      flashLife = 0
    },

    draw(snap, {net, banner, lab}) {
      // The camera is sim state, not a view decision — corner position is
      // gameplay, so both machines must agree on where the corner is.
      const [a, b] = snap.players
      const gap = Math.abs(a.x - b.x)
      const t = Math.min(1, Math.max(0, (ZOOM_START - gap) / (ZOOM_START - ZOOM_FULL)))
      const target = screenHalf + (ZOOM_CLOSE_HALF - screenHalf) * t * t * (3 - 2 * t)
      camHalf += (target - camHalf) * ZOOM_SMOOTH
      const zoom = screenHalf / camHalf
      const centre = frame(snap.camX, Math.min(a.x, b.x), Math.max(a.x, b.x), camHalf, stageHalf)
      world.scale.set(zoom)
      // Pinned at the ground line, so zooming grows the fighters upward.
      world.position.set(VIEW_W / 2 - centre * SCALE * zoom, GROUND_PX * (1 - zoom))

      flash.alpha = (0.6 * flashLife) / FLASH_FRAMES
      if (flashLife > 0) flashLife--

      sparkLayer.clear()
      for (let i = sparks.length - 1; i >= 0; i--) {
        const s = sparks[i]
        const t = s.life / SPARK_FRAMES
        sparkLayer
          .circle(s.x * SCALE, GROUND_PX - (s.y + 24) * SCALE, SPARK_RADIUS * (1.4 - t))
          .stroke({color: s.color, width: 2, alpha: t})
        if (--s.life <= 0) sparks.splice(i, 1)
      }

      // The animation is a pure function of the snapshot — state, how many frames
      // it has been in it, and which move — so a frame resimulated eight times
      // during a rollback picks the same texture eight times.
      for (let i = 0; i < fighters.length; i++) {
        const p = snap.players[i]
        const f = fighters[i]
        f.texture = sheets[i].texture(p.state, p.stateFrame, p.moveIndex)
        f.position.set(p.x * SCALE, GROUND_PX - p.y * SCALE)
        // Facing is the sim's, and it flips the sprite rather than the data:
        // boxes are authored facing right and mirrored when placed, and the art
        // is drawn once for the same reason.
        f.scale.set(p.facing, 1)
      }

      projectiles.clear()
      for (const b of snap.projectiles) drawHitbox(projectiles, b)

      boxes.clear()
      if (boxes.visible) for (const p of snap.players) drawBoxes(boxes, p)
      setText(timer, `${seconds(snap.timer)}`)
      setText(announce, banner)

      for (let i = 0; i < bars.length; i++) {
        drawBars(bars[i], snap.players[i], i, snap.wins[i])
        // The combo belongs to the player taking it; it is shown on the side of
        // the player landing it, which is where every game in the genre puts it.
        setCombo(combos[1 - i], snap.players[i])
      }

      // Rollback made visible.
      if (net) {
        drawTimeline(strip, net)
        setText(stripKey, 'clean · predicted · corrected   ▏= rollback landed, height = depth')
      } else {
        strip.clear()
        setText(stripKey, '')
      }

      // Every frame, unlike the HUD: an input display that refreshed twice a
      // second would not show the input it exists to show.
      if (lab && inputCols && advText && dummyText) {
        for (let seat = 0; seat < inputCols.length; seat++) setText(inputCols[seat], lab.inputs[seat])
        setText(advText, lab.advantage)
        setText(dummyText, lab.dummy)
      }
    },
  }
}

/**
 * Three seconds of frames, oldest on the left, coloured by what each was
 * simulated from — and a white tick wherever a correction landed, as tall as
 * the rollback was deep.
 */
function drawTimeline(g: Graphics, net: Netplay): void {
  g.clear()
  const end = net.frame - 1
  const first = end - TIMELINE + 1

  g.rect(STRIP_X, STRIP_Y - STRIP_DEPTH_H, TIMELINE * STRIP_COL, STRIP_DEPTH_H + STRIP_H).fill({
    color: 0x0e1014,
    alpha: 0.55,
  })

  let runStart = 0
  let runKind = -1

  const flush = (until: number) => {
    if (runKind < 0) return
    const x = STRIP_X + runStart * STRIP_COL
    g.rect(x, STRIP_Y, (until - runStart) * STRIP_COL, STRIP_H).fill(STRIP_COLORS[runKind])
  }

  for (let i = 0; i < TIMELINE; i++) {
    const packed = net.sample(first + i)
    const kind = packed < 0 ? -1 : packed & 3
    if (kind !== runKind) {
      flush(i)
      runStart = i
      runKind = kind
    }

    // Ticks are drawn per frame rather than per run: two corrections a frame
    // apart are two events, and merging them would report one.
    const depth = packed < 0 ? 0 : packed >> 2
    if (depth > 0) {
      const h = (depth / MAX_ROLLBACK) * STRIP_DEPTH_H
      g.rect(STRIP_X + i * STRIP_COL, STRIP_Y - h, STRIP_COL, h).fill(0xe8ecf2)
    }
  }
  flush(TIMELINE)

  // The confirmed line: everything to its right is a prediction that can still
  // be taken back, which is also the line effects are fired up to.
  const c = net.confirmed - first
  if (c >= 0 && c < TIMELINE) {
    g.rect(STRIP_X + c * STRIP_COL, STRIP_Y - 3, 1, STRIP_H + 6).fill(0xe8ecf2)
  }
}

/** Sim coordinates are feet-origin, y up; the canvas is y down. */
function toScreen(b: Box): [number, number, number, number] {
  return [b.x * SCALE, GROUND_PX - (b.y + b.h) * SCALE, b.w * SCALE, b.h * SCALE]
}

function drawBoxes(g: Graphics, p: PlayerSnapshot): void {
  g.rect(...toScreen(p.pushbox)).stroke({color: PUSH_COLOR, width: 1})
  for (const b of p.hurtboxes) {
    g.rect(...toScreen(b)).fill({color: HURT_COLOR, alpha: 0.2}).stroke({color: HURT_COLOR, width: 1})
  }
  for (const b of p.hitboxes) drawHitbox(g, b)
}

function drawHitbox(g: Graphics, b: Box): void {
  g.rect(...toScreen(b)).fill({color: HIT_COLOR, alpha: 0.3}).stroke({color: HIT_COLOR, width: 1})
}

const BAR_W = 340

const MAX_HEALTH = 10000

/** Pips drawn per seat. */
const ROUNDS_TO_WIN = 2

const EMPTY_COLOR = 0x2a2f38

const WIN_COLOR = 0xe8ecf2

const HEALTH_COLOR = 0xd8c15a

const SUPER_COLOR = 0xc86ee0

/** Health, then the super meter. */
function drawBars(g: Graphics, p: PlayerSnapshot, seat: number, wins: number): void {
  const x = seat === 0 ? 10 : VIEW_W - 10 - BAR_W

  g.clear()
  g.rect(x, 8, BAR_W, 14).fill(EMPTY_COLOR)
  // Drains from the centre outward, so both bars empty toward the middle.
  const w = BAR_W * clamp01(p.health / MAX_HEALTH)
  g.rect(seat === 0 ? x + BAR_W - w : x, 8, w, 14).fill(HEALTH_COLOR)

  drawGauge(g, x, 25, 9, seat, p.super, SUPER_BARS, SUPER_COLOR)

  // Round wins, as pips beside the health bar.
  for (let i = 0; i < ROUNDS_TO_WIN; i++) {
    const px = seat === 0 ? x + BAR_W + 6 + i * 12 : x - 12 - i * 12
    g.circle(px, 15, 4).fill(i < wins ? WIN_COLOR : EMPTY_COLOR)
  }
}

/** One segmented gauge. */
function drawGauge(
  g: Graphics,
  x: number,
  y: number,
  h: number,
  seat: number,
  value: number,
  bars: number,
  color: number,
): void {
  const gap = 2
  const segW = (BAR_W - gap * (bars - 1)) / bars

  for (let i = 0; i < bars; i++) {
    // Fills away from the outside edge of the screen, like health.
    const index = seat === 0 ? bars - 1 - i : i
    const sx = x + index * (segW + gap)
    const filled = clamp01(value / BAR_UNITS - i) * segW

    g.rect(sx, y, segW, h).fill(EMPTY_COLOR)
    g.rect(seat === 0 ? sx + segW - filled : sx, y, filled, h).fill(color)
  }
}

/** Assigning re-lays out the text, so only do it when it actually changed. */
function setText(t: Text, text: string): void {
  if (t.text !== text) t.text = text
}

function clamp01(v: number): number {
  return Math.max(0, Math.min(1, v))
}

/** Reads as "COUNTER · 3 hits", or nothing at all outside a combo. */
function setCombo(t: Text, defender: PlayerSnapshot): void {
  const parts = []
  if (defender.combo > 0) {
    if (COUNTER_NAMES[defender.counter]) parts.push(COUNTER_NAMES[defender.counter])
    parts.push(`${defender.combo} hit${defender.combo === 1 ? '' : 's'}`)
  }
  setText(t, parts.join(' · '))
}

/**
 * Centre of a frame `half` wide that starts at the sim's camera and moves only
 * to keep both players `ZOOM_EDGE` inside it — the same dead zone the sim's
 * camera uses, so a zoomed frame does not start sliding a standing player
 * either.
 */
function frame(camX: number, lo: number, hi: number, half: number, stageHalf: number): number {
  const reach = half - ZOOM_EDGE
  let c = camX
  if (hi - lo <= 2 * reach) {
    if (hi > c + reach) c = hi - reach
    if (lo < c - reach) c = lo + reach
  }
  return Math.min(Math.max(c, -stageHalf + half), stageHalf - half)
}
