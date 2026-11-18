import {useEffect, useRef, useState} from 'react'
import {play} from './audio'
import {decodeLog} from './game/log'
import {MODES, type Mode, type RunSetup} from './setup'

const MODE_LABELS: Record<Mode, string> = {
  local: 'local versus',
  ai: 'versus AI',
  training: 'training',
  online: 'online',
}

const MODE_HINTS: Record<Mode, string> = {
  local: 'two players, one machine',
  ai: 'a scripted opponent, no network',
  training: 'boxes, frame data, a dummy with five behaviours',
  online: 'rollback netcode, ranked, casual or a room code',
}

const TITLE = 'UNIVERSUS'

/** How much the sheet's pixels are magnified in the select screen. */
const PORTRAIT_SCALE = 1.5

/** The part of a sheet's JSON the portrait reads. */
interface PortraitMeta {
  frames: {frame: {x: number; y: number; w: number; h: number}}[]
  meta: {frameTags: {name: string; from: number; to: number}[]}
}

/** A fighter's idle loop, stepped through the sprite sheet one cell per sim frame. */
export function Portrait({name, flip = false}: {name: string; flip?: boolean}) {
  const el = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let raf = 0
    let cancelled = false
    void fetch(`/sprites/${name}.json`)
      .then((r) => r.json() as Promise<PortraitMeta>)
      .then((sheet) => {
        const idle = sheet.meta.frameTags.find((t) => t.name === 'idle')
        if (cancelled || !idle || !el.current) return
        const div = el.current
        const {w, h} = sheet.frames[idle.from].frame
        div.style.width = `${w}px`
        div.style.height = `${h}px`
        div.style.backgroundImage = `url(/sprites/${name}.png)`
        const start = performance.now()
        const tick = (now: number) => {
          const n = idle.to - idle.from + 1
          const f = sheet.frames[idle.from + (Math.floor(((now - start) * 60) / 1000) % n)].frame
          div.style.backgroundPosition = `${-f.x}px ${-f.y}px`
          raf = requestAnimationFrame(tick)
        }
        raf = requestAnimationFrame(tick)
      })
      .catch(() => {})
    return () => {
      cancelled = true
      cancelAnimationFrame(raf)
    }
  }, [name])

  return (
    <div className="portrait" style={{zoom: PORTRAIT_SCALE}}>
      <div ref={el} className={flip ? 'flip' : ''} />
    </div>
  )
}

export interface MenuProps {
  roster: {index: number; name: string}[]
  onStart(setup: RunSetup): void
}

/** Title, then a character select. */
export default function Menu({roster, onStart}: MenuProps) {
  const [mode, setMode] = useState<Mode | null>(null)
  const [chars, setChars] = useState<[number, number]>([0, 0])
  const [error, setError] = useState('')
  const file = useRef<HTMLInputElement>(null)

  /** A saved input log is a whole run: its header carries the mode and the fighters. */
  async function watch(f: File | undefined) {
    if (!f) return
    try {
      const {setup, inputs} = decodeLog(await f.arrayBuffer())
      const mode: Mode = setup.training ? 'training' : setup.ai ? 'ai' : 'local'
      onStart({mode, ai: setup.ai, chars: setup.chars, replay: inputs})
    } catch (e) {
      setError((e as Error).message)
    }
  }

  if (mode === null) {
    return (
      <div className="menu" key="title">
        <h1 className="logo" aria-label={TITLE}>
          {[...TITLE].map((c, i) => (
            <span key={i} style={{'--i': i} as React.CSSProperties}>
              {c}
            </span>
          ))}
        </h1>
        <p className="tagline">a fighting game that rolls back</p>

        <div className="modes">
          {MODES.map((m, i) => (
            <button
              key={m}
              type="button"
              style={{'--i': i} as React.CSSProperties}
              onClick={() => {
                play('select')
                setMode(m)
              }}
            >
              <b>{MODE_LABELS[m]}</b>
              <span>{MODE_HINTS[m]}</span>
            </button>
          ))}
          <button
            type="button"
            style={{'--i': MODES.length} as React.CSSProperties}
            onClick={() => file.current?.click()}
          >
            <b>replay</b>
            <span>{error || 'play back a saved input log, frame for frame'}</span>
          </button>
          <input ref={file} type="file" hidden onChange={(e) => void watch(e.target.files?.[0])} />
        </div>
      </div>
    )
  }

  // Who the second column belongs to is the mode's business.
  const seat2 = mode === 'training' ? 'dummy' : mode === 'ai' ? 'opponent' : 'P2'

  return (
    <div className="menu" key="select">
      <h1 className="heading">{MODE_LABELS[mode]}</h1>

      <div className="select">
        {(['P1', seat2] as const).map((label, seat) => (
          <div key={label} className={`column seat${seat}`}>
            <h2>{label}</h2>
            {/* Keyed by the pick, so a new fighter walks in rather than swapping in place. */}
            <Portrait key={chars[seat]} name={roster[chars[seat]].name.toLowerCase()} flip={seat === 1} />
            <div className="picks">
              {roster.map((c) => (
                <button
                  key={c.index}
                  type="button"
                  className={chars[seat] === c.index ? 'on' : ''}
                  onClick={() => {
                    play('select')
                    setChars((prev) => {
                      const next: [number, number] = [prev[0], prev[1]]
                      next[seat] = c.index
                      return next
                    })
                  }}
                >
                  {c.name}
                </button>
              ))}
            </div>
          </div>
        ))}
        <span className="vs">VS</span>
      </div>

      <div className="row">
        <button
          type="button"
          onClick={() => {
            play('back')
            setMode(null)
          }}
        >
          back
        </button>
        <button
          type="button"
          className="fight"
          onClick={() => {
            play('confirm')
            onStart({mode, ai: mode === 'ai', chars})
          }}
        >
          fight
        </button>
      </div>
    </div>
  )
}
