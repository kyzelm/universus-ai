/** Devices to input bitfield. */
export const IN_UP = 1 << 0
export const IN_DOWN = 1 << 1
export const IN_LEFT = 1 << 2
export const IN_RIGHT = 1 << 3
export const IN_LP = 1 << 4
export const IN_MP = 1 << 5
export const IN_HP = 1 << 6
export const IN_LK = 1 << 8
export const IN_MK = 1 << 9
export const IN_HK = 1 << 10

/** Two buttons on one key, which is how throws are played everywhere. */
const THROW = IN_LP | IN_LK

/** KeyboardEvent.code to bit — or to a mask, for the macro. */
const P1_KEYS: Record<string, number> = {
  KeyW: IN_UP,
  KeyS: IN_DOWN,
  KeyA: IN_LEFT,
  KeyD: IN_RIGHT,
  KeyU: IN_LP,
  KeyI: IN_MP,
  KeyO: IN_HP,
  KeyJ: IN_LK,
  KeyK: IN_MK,
  KeyL: IN_HK,
  KeyH: THROW,
}

const P2_KEYS: Record<string, number> = {
  ArrowUp: IN_UP,
  ArrowDown: IN_DOWN,
  ArrowLeft: IN_LEFT,
  ArrowRight: IN_RIGHT,
  Numpad7: IN_LP,
  Numpad8: IN_MP,
  Numpad9: IN_HP,
  Numpad4: IN_LK,
  Numpad5: IN_MK,
  Numpad6: IN_HK,
  Numpad0: THROW,
}

/** Standard Gamepad mapping, button index to bit. */
const PAD_BUTTONS: Record<number, number> = {
  12: IN_UP,
  13: IN_DOWN,
  14: IN_LEFT,
  15: IN_RIGHT,
  2: IN_LP, // square / X
  3: IN_MP, // triangle / Y
  5: IN_HP, // R1 / RB
  6: IN_HP, // L2 / LT
  0: IN_LK, // cross / A
  1: IN_MK, // circle / B
  7: IN_HK, // R2 / RT
  4: THROW,  // L1 / LB — where every fighting game puts throw
}

/** Stick deflection past which an axis counts as held. */
const DEADZONE = 0.5

/** Reads one gamepad into a bitfield. */
export function packPad(pad: Gamepad): number {
  let bits = 0
  for (const index in PAD_BUTTONS) {
    if (pad.buttons[index]?.pressed) bits |= PAD_BUTTONS[index]
  }

  const [x, y] = pad.axes
  if (x <= -DEADZONE) bits |= IN_LEFT
  if (x >= DEADZONE) bits |= IN_RIGHT
  if (y <= -DEADZONE) bits |= IN_UP // browser Y is negative up
  if (y >= DEADZONE) bits |= IN_DOWN

  return bits
}

/** Simultaneous Opposite Cardinal Directions. */
export function resolveSOCD(bits: number): number {
  if (bits & IN_LEFT && bits & IN_RIGHT) bits &= ~(IN_LEFT | IN_RIGHT)
  if (bits & IN_UP && bits & IN_DOWN) bits &= ~IN_DOWN
  return bits
}

export function packBits(held: ReadonlySet<string>, keys: Record<string, number>): number {
  let bits = 0
  for (const code in keys) {
    if (held.has(code)) bits |= keys[code]
  }
  return resolveSOCD(bits)
}

export interface Input {
  /** Both players' bitfields for the current frame. */
  poll(): [number, number]

  dispose(): void
}

/** Tracks held keys and packs them on demand. */
export function createInput(target: EventTarget = window): Input {
  const held = new Set<string>()

  // Keys pressed since the last poll, whether or not they are still down.
  const tapped = new Set<string>()

  const down = (e: Event) => {
    const {code} = e as KeyboardEvent
    if (code in P1_KEYS || code in P2_KEYS) {
      held.add(code)
      tapped.add(code)
      e.preventDefault() // arrows scroll the page otherwise
    }
  }
  const up = (e: Event) => held.delete((e as KeyboardEvent).code)

  // Losing focus mid-hold leaves the key stuck down forever; the keyup lands
  // on whatever the user tabbed to.
  const clear = () => held.clear()

  target.addEventListener('keydown', down)
  target.addEventListener('keyup', up)
  target.addEventListener('blur', clear)

  // The Gamepad API is a polling API by design, which is exactly the shape
  // this loop wants: gamepads are read in poll(), never from an event.
  const pads = (): (Gamepad | null)[] => navigator.getGamepads?.() ?? []

  return {
    poll() {
      const pressed = new Set([...held, ...tapped])
      tapped.clear()

      // First two connected pads take seat 0 and seat 1.
      const connected = pads().filter((p): p is Gamepad => p !== null)

      // Resolve SOCD once more after merging: each device is clean on its own,
      // but keyboard-left plus pad-right is an opposing pair that only exists
      // after the merge, and an unresolved pair must never reach the sim.
      return [
        resolveSOCD(packBits(pressed, P1_KEYS) | (connected[0] ? packPad(connected[0]) : 0)),
        resolveSOCD(packBits(pressed, P2_KEYS) | (connected[1] ? packPad(connected[1]) : 0)),
      ]
    },
    dispose() {
      target.removeEventListener('keydown', down)
      target.removeEventListener('keyup', up)
      target.removeEventListener('blur', clear)
      held.clear()
      tapped.clear()
    },
  }
}