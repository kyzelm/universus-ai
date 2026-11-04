import {
  IN_DOWN,
  IN_HK,
  IN_HP,
  IN_LEFT,
  IN_LK,
  IN_LP,
  IN_MK,
  IN_MP,
  IN_RIGHT,
  IN_UP,
} from './input'

/** A seeded input source, for measurement runs. */
export interface Bot {
  /** One frame's bitfield. */
  poll(): number
  /**
   * Both ends open the same URL, so the seat is what makes the streams differ.
   */
  reseed(seed: number): void
}

/**
 * Single directions and the two down-diagonals — enough to walk, crouch, jump
 * and occasionally complete a motion, which is what makes the stream worth
 * predicting wrongly.
 */
const DIRECTIONS = [0, IN_LEFT, IN_RIGHT, IN_DOWN, IN_UP, IN_DOWN | IN_RIGHT, IN_DOWN | IN_LEFT]
const ATTACKS = [IN_LP, IN_MP, IN_HP, IN_LK, IN_MK, IN_HK]

export function createBot(seed = 1): Bot {
  let s = seed >>> 0 || 1

  // xorshift32, the same generator the sim uses.
  const rand = () => {
    s ^= s << 13
    s ^= s >>> 17
    s ^= s << 5
    s >>>= 0
    return s
  }

  let dir = 0
  let dirFrames = 0
  let atk = 0
  let atkFrames = 0

  return {
    reseed(next) {
      s = next >>> 0 || 1
      dirFrames = atkFrames = 0
    },

    poll() {
      if (--dirFrames <= 0) {
        dir = DIRECTIONS[rand() % DIRECTIONS.length]
        dirFrames = 6 + (rand() % 15)
      }

      // A two-frame press and then a gap.
      if (--atkFrames <= 0) {
        atk = atk ? 0 : ATTACKS[rand() % ATTACKS.length]
        atkFrames = atk ? 2 : 10 + (rand() % 30)
      }

      return dir | atk
    },
  }
}
