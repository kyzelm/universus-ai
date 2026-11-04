/** Sprite sheets: Aseprite's export read into per-tag texture runs. */

import {Assets, Rectangle, Texture} from 'pixi.js'

import {STATE_NAMES} from '../sim/wasm'

/** Aseprite's JSON export, plus the move table the generator adds (D113). */
export interface SheetMeta {
  frames: {frame: {x: number; y: number; w: number; h: number}}[]
  meta: {
    origin: {x: number; y: number}
    frameTags: {name: string; from: number; to: number}[]
    moveTags: string[]
  }
}

export interface Sheet {
  /**
   * The sheet's origin as a Pixi anchor, so a sprite placed at the fighter's
   * simulated position lands with its feet there.
   */
  readonly anchor: {x: number; y: number}
  /** The texture a fighter in this state is on. */
  texture(state: number, stateFrame: number, moveIndex: number): Texture
}

const ATTACK = STATE_NAMES.indexOf('attack')

/** State to tag. */
const STATE_TAG: Record<string, string> = {
  idle: 'idle',
  walkF: 'walk_f',
  walkB: 'walk_b',
  crouch: 'crouch',
  dash: 'dash',
  backdash: 'dash_b',
  prejump: 'crouch',
  air: 'jump',
  attack: '', // the move names it, not the state
  hitstun: 'hitstun',
  blockstun: 'blockstun',
  landing: 'crouch',
  thrown: 'thrown',
  knockdown: 'knockdown',
}

/** Tags that cycle for as long as the state lasts. */
const LOOPS = new Set(['idle', 'walk_f', 'walk_b'])

/** Which frame of a run of `length` a fighter `stateFrame` frames in shows. */
export function frameOf(tag: string, stateFrame: number, length: number): number {
  const f = Math.max(stateFrame, 0)
  return LOOPS.has(tag) ? f % length : Math.min(f, length - 1)
}

/** The tag a fighter in this state draws from, or '' if there is none. */
export function tagFor(meta: SheetMeta, state: number, moveIndex: number): string {
  if (state === ATTACK) return meta.meta.moveTags[moveIndex] ?? ''
  return STATE_TAG[STATE_NAMES[state]] ?? ''
}

/** Every tag the sheet is asked for but does not carry, deduplicated. */
export function missingTags(meta: SheetMeta): string[] {
  const have = new Set(meta.meta.frameTags.map((t) => t.name))
  const want = new Set<string>()

  for (const name of STATE_NAMES) {
    const tag = STATE_TAG[name]
    if (tag === undefined) want.add(`<state ${name} has no tag>`)
    else if (tag) want.add(tag)
  }
  for (const tag of meta.meta.moveTags) want.add(tag)

  return [...want].filter((t) => !have.has(t)).sort()
}

/** Slices a loaded sheet into per-tag texture runs. */
export function buildSheet(meta: SheetMeta, base: Texture): Sheet {
  const missing = missingTags(meta)
  if (missing.length > 0) {
    throw new Error(`sprites: sheet is missing ${missing.length} tag(s): ${missing.join(', ')}`)
  }

  const runs = new Map<string, Texture[]>()
  for (const tag of meta.meta.frameTags) {
    const run: Texture[] = []
    for (let i = tag.from; i <= tag.to; i++) {
      const f = meta.frames[i].frame
      run.push(new Texture({source: base.source, frame: new Rectangle(f.x, f.y, f.w, f.h)}))
    }
    runs.set(tag.name, run)
  }

  const idle = runs.get('idle')!
  const cell = meta.frames[0].frame

  return {
    anchor: {x: meta.meta.origin.x / cell.w, y: meta.meta.origin.y / cell.h},
    texture(state, stateFrame, moveIndex) {
      // Startup validation means every tag resolves, so the fallbacks below are
      // for the two things it cannot cover: a move index of -1, which the sim
      // uses for "no move", and a state frame that has outrun its animation.
      const tag = tagFor(meta, state, moveIndex)
      const run = runs.get(tag) ?? idle
      return run[frameOf(runs.has(tag) ? tag : 'idle', stateFrame, run.length)]
    },
  }
}

/** Fetches and slices one character's sheet. */
export async function loadSheet(name: string): Promise<Sheet> {
  const res = await fetch(`/sprites/${name}.json`)
  if (!res.ok) throw new Error(`sprites: /sprites/${name}.json (${res.status})`)
  const meta = (await res.json()) as SheetMeta
  const base: Texture = await Assets.load(`/sprites/${name}.png`)
  // The renderer draws at display resolution, so sprites are magnified on the
  // GPU now, not by the browser's CSS scale.
  base.source.scaleMode = 'nearest'
  return buildSheet(meta, base)
}
