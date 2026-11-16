import {expect, test} from 'vitest'
import {packBits, keyMasks, IN_LP, IN_UP} from './game/input'
import {DEFAULTS, parseOptions, rebind} from './options'

test('stored options fall back to the defaults field by field', () => {
  expect(parseOptions(null)).toEqual(DEFAULTS)
  expect(parseOptions('not json')).toEqual(DEFAULTS)
  const o = parseOptions(JSON.stringify({volume: 7, keys: [{up: 'KeyT', lp: 3}]}))
  expect(o.volume).toBe(DEFAULTS.volume)
  expect(o.keys[0].up).toBe('KeyT')
  expect(o.keys[0].lp).toBe(DEFAULTS.keys[0].lp)
  expect(o.keys[1]).toEqual(DEFAULTS.keys[1])
})

test('a rebound key presses its new action and nothing else', () => {
  // KeyW was up; binding it to light punch must take it away from up.
  const o = rebind(DEFAULTS, 0, 'lp', 'KeyW')
  expect(o.keys[0].up).toBe('')
  expect(packBits(new Set(['KeyW']), keyMasks(o.keys[0]))).toBe(IN_LP)
  expect(packBits(new Set(['KeyW']), keyMasks(DEFAULTS.keys[0]))).toBe(IN_UP)
})
