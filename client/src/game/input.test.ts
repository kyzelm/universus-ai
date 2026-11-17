import {afterEach, expect, test} from 'vitest'
import {
  createInput,
  IN_DOWN,
  IN_HP,
  IN_LEFT,
  IN_LK,
  IN_LP,
  IN_RIGHT,
  IN_UP,
  packBits,
  resolveSOCD,
} from './input'

/** A Gamepad with everything released, for tests to poke holes in. */
function fakePad(over: {buttons?: number[]; axes?: [number, number]} = {}): Gamepad {
  const buttons = Array.from({length: 16}, (_, i) => ({
    pressed: over.buttons?.includes(i) ?? false,
  }))
  return {buttons, axes: over.axes ?? [0, 0]} as unknown as Gamepad
}

function connectPads(...pads: (Gamepad | null)[]) {
  navigator.getGamepads = () => pads
}

afterEach(() => {
  // jsdom has no Gamepad API; createInput's optional call handles that, and
  // leaving a stub behind would leak into the tests that rely on its absence.
  delete (navigator as {getGamepads?: unknown}).getGamepads
})

test('SOCD: left and right resolve to neutral', () => {
  expect(resolveSOCD(IN_LEFT | IN_RIGHT)).toBe(0)
  expect(resolveSOCD(IN_LEFT | IN_RIGHT | IN_UP)).toBe(IN_UP)
})

test('SOCD: up beats down', () => {
  expect(resolveSOCD(IN_UP | IN_DOWN)).toBe(IN_UP)
})

test('SOCD: every combination of the four directions is stable', () => {
  for (let bits = 0; bits < 16; bits++) {
    const once = resolveSOCD(bits)
    expect(resolveSOCD(once)).toBe(once)
    // A resolved bitfield never holds an opposing pair.
    expect(once & IN_LEFT && once & IN_RIGHT).toBeFalsy()
    expect(once & IN_UP && once & IN_DOWN).toBeFalsy()
  }
})

test('SOCD: uncontested directions pass through untouched', () => {
  for (const bit of [IN_UP, IN_DOWN, IN_LEFT, IN_RIGHT]) {
    expect(resolveSOCD(bit)).toBe(bit)
  }
})

test('packBits reads only the keys it was given', () => {
  const held = new Set(['KeyA', 'ArrowRight'])
  expect(packBits(held, {KeyA: IN_LEFT})).toBe(IN_LEFT)
  expect(packBits(held, {ArrowRight: IN_RIGHT})).toBe(IN_RIGHT)
  expect(packBits(held, {KeyW: IN_UP})).toBe(0)
})

function press(t: EventTarget, code: string) {
  t.dispatchEvent(new KeyboardEvent('keydown', {code, cancelable: true}))
}

function release(t: EventTarget, code: string) {
  t.dispatchEvent(new KeyboardEvent('keyup', {code}))
}

test('poll reports held keys per player', () => {
  const target = new EventTarget()
  const input = createInput(target)

  expect(input.poll()).toEqual([0, 0])

  press(target, 'KeyD')
  press(target, 'ArrowLeft')
  expect(input.poll()).toEqual([IN_RIGHT, IN_LEFT])

  // Held state persists across polls — the loop reads it every fixed step.
  expect(input.poll()).toEqual([IN_RIGHT, IN_LEFT])

  release(target, 'KeyD')
  expect(input.poll()).toEqual([0, IN_LEFT])

  input.dispose()
})

test('a tap that starts and ends between two polls still registers', () => {
  const target = new EventTarget()
  const input = createInput(target)

  press(target, 'KeyD')
  release(target, 'KeyD')

  // The key is already up by the time the loop looks, but the player pressed
  // it, so it lands on this frame.
  expect(input.poll()).toEqual([IN_RIGHT, 0])
  // ...and on exactly one frame, not forever.
  expect(input.poll()).toEqual([0, 0])

  input.dispose()
})

test('a stuck key is released when focus is lost', () => {
  const target = new EventTarget()
  const input = createInput(target)

  press(target, 'KeyA')
  expect(input.poll()).toEqual([IN_LEFT, 0])

  target.dispatchEvent(new Event('blur'))
  expect(input.poll()).toEqual([0, 0])

  input.dispose()
})

test('dispose stops tracking', () => {
  const target = new EventTarget()
  const input = createInput(target)
  input.dispose()

  press(target, 'KeyA')
  expect(input.poll()).toEqual([0, 0])
})

// Each device is SOCD-clean on its own; the opposing pair only exists after the
// merge.
test('SOCD resolves across devices, not just within one', () => {
  const target = new EventTarget()
  connectPads(fakePad({buttons: [15]})) // pad holds right
  const input = createInput(target)

  target.dispatchEvent(new KeyboardEvent('keydown', {code: 'KeyA', cancelable: true}))
  expect(input.poll()).toEqual([0, 0]) // keyboard left + pad right -> neutral
  input.dispose()
})

test('attack keys are bound for both seats', () => {
  const target = new EventTarget()
  const input = createInput(target)

  target.dispatchEvent(new KeyboardEvent('keydown', {code: 'KeyU', cancelable: true}))
  target.dispatchEvent(new KeyboardEvent('keydown', {code: 'Numpad9', cancelable: true}))
  expect(input.poll()).toEqual([IN_LP, IN_HP])

  input.dispose()
})

// The throw macro.
test('the throw key presses both buttons on the same frame', () => {
  const target = new EventTarget()
  const input = createInput(target)

  press(target, 'KeyH')
  expect(input.poll()).toEqual([IN_LP | IN_LK, 0])

  release(target, 'KeyH')
  press(target, 'Numpad0')
  expect(input.poll()).toEqual([0, IN_LP | IN_LK])

  input.dispose()
})
