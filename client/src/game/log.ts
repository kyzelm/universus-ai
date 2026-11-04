import type {Setup} from '../sim/wasm'

/**
 * The replay log format, written here and read by tools/replay (see its package
 * comment for the table this must match).
 */
export const LOG_HEADER = 16
const MAGIC = 'UNIV'
const VERSION = 1

/** Packs a session into the log format. */
export function encodeLog(
  setup: Setup,
  dataVersion: number,
  inputs: readonly number[],
): ArrayBuffer {
  const buf = new ArrayBuffer(LOG_HEADER + inputs.length * 2)
  const v = new DataView(buf)

  for (let i = 0; i < MAGIC.length; i++) v.setUint8(i, MAGIC.charCodeAt(i))
  v.setUint8(4, VERSION)
  v.setUint8(5, setup.training ? 1 : 0)
  v.setUint8(6, setup.ai)
  v.setUint8(7, setup.chars[0])
  v.setUint8(8, setup.chars[1])
  // Bytes 9-11 stay zero: reserved, and the reader checks the header's size.

  // Recorded, and compared on the way back in rather than enforced: a log taken
  // against older frame data still replays, it just replays a match that is no
  // longer the same one.
  v.setUint32(12, dataVersion, true)

  for (let i = 0; i < inputs.length; i++) v.setUint16(LOG_HEADER + i * 2, inputs[i], true)
  return buf
}
