import {useState} from 'react'
import {refreshVolume} from './audio'
import {ACTIONS, DEFAULTS, options, rebind, setOptions, type Action} from './options'

const LABELS: Record<Action, string> = {
  up: 'up', down: 'down', left: 'left', right: 'right',
  lp: 'light punch', mp: 'medium punch', hp: 'heavy punch',
  lk: 'light kick', mk: 'medium kick', hk: 'heavy kick', throw: 'throw',
}

/** "KeyW" to "W", "Numpad7" to "Num 7", "ArrowUp" to "Up". */
function keyName(code: string): string {
  return code.replace(/^(Key|Digit|Arrow)/, '').replace(/^Numpad/, 'Num ') || '(none)'
}

/** Volume and both seats' keys. */
export default function Options() {
  const [o, setO] = useState(options)
  // The binding waiting for a key press.
  const [waiting, setWaiting] = useState<[number, Action] | null>(null)

  const save = (next: typeof o) => {
    setOptions(next)
    refreshVolume()
    setO(next)
  }

  const listen = (seat: number, action: Action) => {
    setWaiting([seat, action])
    // Capture phase, so the key being bound does not also reach the game or close the panel.
    window.addEventListener(
      'keydown',
      (e) => {
        e.preventDefault()
        e.stopImmediatePropagation()
        if (e.code !== 'Escape') save(rebind(options, seat, action, e.code))
        setWaiting(null)
      },
      {capture: true, once: true},
    )
  }

  return (
    <details className="controls">
      <summary>options</summary>
      <dl>
        {(['volume', 'music'] as const).map((k) => (
          <div key={k} style={{display: 'contents'}}>
            <dt>{k === 'volume' ? 'effects' : 'music'}</dt>
            <dd>
              <input
                type="range" min="0" max="1" step="0.05" value={o[k]}
                onChange={(e) => save({...o, [k]: Number(e.target.value)})}
              />
            </dd>
          </div>
        ))}

        {ACTIONS.map((a) => (
          <div key={a} style={{display: 'contents'}}>
            <dt>{LABELS[a]}</dt>
            <dd>
              {[0, 1].map((seat) => (
                <button key={seat} type="button" onClick={() => listen(seat, a)}>
                  P{seat + 1}{' '}
                  <b>{waiting?.[0] === seat && waiting[1] === a ? 'press a key…' : keyName(o.keys[seat][a])}</b>
                </button>
              ))}
            </dd>
          </div>
        ))}

        <dt />
        <dd>
          <button type="button" onClick={() => save(DEFAULTS)}>
            reset to defaults
          </button>
        </dd>
      </dl>
    </details>
  )
}
