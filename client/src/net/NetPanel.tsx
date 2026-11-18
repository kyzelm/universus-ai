import {useEffect, useRef, useState} from 'react'
import type {Game} from '../game/game'
import {CLEAN, type Impairment} from './impair'
import {joinQueue, joinRoom, type Session} from './room'
import SignIn from './SignIn'
import {account, type Account} from './account'
import {roster} from '../sim/wasm'
import {flags} from '../flags'

/**
 * The measurement matrix, as buttons (01 Thesis/Measurement Methodology.md, M-B): 0/50/100/150/200
 * ms against 0/2/5% loss.
 */
const RTT_PRESETS = [0, 50, 100, 150, 200]
const LOSS_PRESETS = [0, 2, 5]

/** A whole run out of the URL: `?room=CODE&bot&rtt=200&loss=5&jitter=15`. */
function urlImpairment(): Impairment | null {
  const {rtt, loss, jitter} = flags
  if (rtt === null && loss === null && jitter === null) return null

  // The presets are round-trip figures and the layer delays arrivals in one direction, so it is
  // given half.
  return {delayMs: (rtt ?? 0) / 2, jitterMs: jitter ?? 0, lossPercent: loss ?? 0}
}

/** "Shoto vs Grappler". */
function matchup(chars: [number, number]): string {
  const names = roster()
  return `${names[chars[0]]?.name ?? '?'} vs ${names[chars[1]]?.name ?? '?'}`
}

/** The online lobby: sign in, find a match, and say what it found. */
export default function NetPanel({game, onFound}: {game: Game | null; onFound(chars: [number, number]): void}) {
  // What is being waited for, or why the last attempt failed.
  const [status, setStatus] = useState('')
  const [searching, setSearching] = useState(false)
  const [session, setSession] = useState<Session | null>(null)
  const [code, setCode] = useState('')
  const [cfg, setCfg] = useState<Impairment>(CLEAN)
  // Who is signed in, if anybody.
  const [who, setWho] = useState<Account | null>(account)

  const room = useRef<Session | null>(null)
  const autoJoined = useRef(false)
  useEffect(() => () => room.current?.close(), [])

  /** A queue and a room code differ only in who decided the two of you should meet. */
  async function enter(waiting: string, join: (g: Game) => Promise<Session>) {
    if (!game) return
    try {
      setSearching(true)
      setStatus(waiting)
      const s = await join(game)
      room.current = s
      game.connect(s.peer, s.seat, {chars: s.chars, matchID: s.matchID, transport: s.kind})
      setSession(s)
      onFound(s.chars)

      // Applied after connecting, not before.
      const condition = urlImpairment()
      if (condition) impair(condition)
    } catch (e) {
      setStatus(`failed: ${(e as Error).message}`)
    } finally {
      setSearching(false)
    }
  }

  const queueUp = (mode: 'ranked' | 'casual') =>
    enter(`searching for a ${mode} match`, (g) => joinQueue(mode, (d) => g.receive(d), g.chars))
  const connectRoom = (joining = code) =>
    enter(`waiting in room ${joining}`, (g) => joinRoom(joining, (d) => g.receive(d), g.chars))

  // ?room=CODE joins on load.
  useEffect(() => {
    const joining = flags.room
    if (!game || !joining || autoJoined.current) return

    autoJoined.current = true
    setCode(joining)
    void connectRoom(joining)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [game])

  /** Conditions are pushed into the game rather than held there. */
  function impair(next: Impairment) {
    setCfg(next)
    game?.impair(next)
  }

  if (searching) {
    return (
      <section className="net">
        <div className="spinner" />
        <p className="searching">{status}</p>
        {/* ponytail: leaving the queue is a reload, which closes the socket; an abort signal if a reload ever costs something. */}
        <button type="button" onClick={() => location.assign(location.pathname)}>
          cancel
        </button>
      </section>
    )
  }

  if (!session) {
    return (
      <section className="net">
        <SignIn onChange={setWho} />

        {/* Ranked and casual are the same socket and the same negotiation; what differs is how wide the skill bands open while you wait (D43, D44). */}
        {who && (
          <div className="row">
            <button className="big" disabled={!game} onClick={() => void queueUp('ranked')}>
              ranked
            </button>
            <button className="big" disabled={!game} onClick={() => void queueUp('casual')}>
              casual
            </button>
          </div>
        )}

        <div className="row">
          <input
            placeholder="room code"
            value={code}
            // The server takes [A-Za-z0-9-]{1,32} and refuses the rest, so a typed space becomes no
            // character rather than a failed connection.
            onChange={(e) => setCode(e.target.value.replace(/[^A-Za-z0-9-]/g, '').slice(0, 32))}
          />
          <button disabled={!code || !game} onClick={() => void connectRoom()}>
            private match
          </button>
        </div>

        <p className="keys">
          {status || 'sign in for the ranked and casual queues, or share a room code with a friend'}
        </p>
      </section>
    )
  }

  return (
    <section className="net">
      <p className="keys">
        you are <b>P{session.seat + 1}</b> · {matchup(session.chars)} · {session.kind}
      </p>

      {flags.debug && (
        <div className="conditions">
          <p className="keys">
            artificial conditions — set the same pair on <b>both</b> ends, then read the
            measured RTT off the HUD
          </p>

          <div className="row">
            {RTT_PRESETS.map((rtt) => (
              <button
                key={rtt}
                type="button"
                className={cfg.delayMs * 2 === rtt ? 'on' : ''}
                onClick={() => impair({...cfg, delayMs: rtt / 2})}
              >
                {rtt} ms
              </button>
            ))}
          </div>

          <div className="row">
            {LOSS_PRESETS.map((loss) => (
              <button
                key={loss}
                type="button"
                className={cfg.lossPercent === loss ? 'on' : ''}
                onClick={() => impair({...cfg, lossPercent: loss})}
              >
                {loss}% loss
              </button>
            ))}
            <label>
              jitter
              <input
                type="number"
                min={0}
                max={200}
                value={cfg.jitterMs}
                onChange={(e) => impair({...cfg, jitterMs: Math.max(0, +e.target.value)})}
              />
              ms
            </label>
          </div>
        </div>
      )}
    </section>
  )
}
