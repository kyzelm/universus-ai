import {expect, test} from 'vitest'
import {setupFromURL} from './setup'

/** A bare URL is somebody arriving and gets the menu. */
test('a bare URL is the only one that gets the menu', () => {
  expect(setupFromURL('')).toBeNull()
  expect(setupFromURL('?sweep')).toBeNull() // its own page, not a run

  for (const search of ['?bot', '?bench', '?room=ABC', '?training', '?ai', '?p1=1']) {
    expect(setupFromURL(search), search).not.toBeNull()
  }
})

test('the URL names the same setup the menu builds', () => {
  expect(setupFromURL('?training&p2=1')).toEqual({mode: 'training', ai: false, chars: [0, 1]})
  expect(setupFromURL('?ai&p1=1&p2=1')).toEqual({mode: 'ai', ai: true, chars: [1, 1]})
  expect(setupFromURL('?room=ABC')).toEqual({mode: 'online', ai: false, chars: [0, 0]})
  expect(setupFromURL('?bot')).toEqual({mode: 'local', ai: false, chars: [0, 0]})

  // Training wins over the AI, which is the precedence the sim had
  // before the menu existed: the lab is a mode, not an opponent.
  expect(setupFromURL('?training&ai')?.mode).toBe('training')

  // A character index that is not one, or not there at all, is the first entry
  // rather than a fighter who cannot move.
  expect(setupFromURL('?p1=-3&p2=x')?.chars).toEqual([0, 0])
})
