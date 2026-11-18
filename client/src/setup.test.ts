import {expect, test} from 'vitest'
import {parseFlags} from './flags'
import {setupFromURL} from './setup'

/** A bare URL is somebody arriving and gets the menu. */
test('a bare URL is the only one that gets the menu', () => {
  expect(setupFromURL(parseFlags(''))).toBeNull()
  expect(setupFromURL(parseFlags('?sweep'))).toBeNull() // its own page, not a run

  for (const search of ['?bot', '?bench', '?room=ABC', '?training', '?ai', '?p1=1']) {
    expect(setupFromURL(parseFlags(search)), search).not.toBeNull()
  }
})

test('the URL names the same setup the menu builds', () => {
  expect(setupFromURL(parseFlags('?training&p2=1'))).toEqual({mode: 'training', ai: false, chars: [0, 1]})
  expect(setupFromURL(parseFlags('?ai&p1=1&p2=1'))).toEqual({mode: 'ai', ai: true, chars: [1, 1]})
  expect(setupFromURL(parseFlags('?room=ABC'))).toEqual({mode: 'online', ai: false, chars: [0, 0]})
  expect(setupFromURL(parseFlags('?bot'))).toEqual({mode: 'local', ai: false, chars: [0, 0]})

  // Training wins over the AI, which is the precedence the sim had before the menu existed.
  expect(setupFromURL(parseFlags('?training&ai'))?.mode).toBe('training')

  // A character index that is not one, or not there at all, is the first entry rather than a
  // fighter who cannot move.
  expect(setupFromURL(parseFlags('?p1=-3&p2=x'))?.chars).toEqual([0, 0])
})
