import {token} from './account'

/**
 * Uploading a finished match (02 Architecture/Anti-Cheat and Verification.md).
 */
export interface MatchReport {
  /** The winning seat, 1 or 2 — the server knows who sat where. */
  winner: number
  endFrame: number
  chars: [number, number]
  /** The match ended because the other side stopped sending. */
  disconnect?: boolean
  /** Which transport it was played on, and how it went. */
  transport?: 'p2p' | 'relay'
  rttMs?: number
  rollbackAvg?: number
  /** The replay log, header and all: the bytes tools/replay reads. */
  inputLog: Uint8Array
  /** State hashes every 30 frames, packed little-endian. */
  checksums: Uint8Array
}

function base64(bytes: Uint8Array): string {
  // Chunked: String.fromCharCode(...bytes) on a 20 KB log is an argument list
  // long enough to blow the call stack on some engines.
  let out = ''
  for (let i = 0; i < bytes.length; i += 0x8000) {
    out += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  }
  return btoa(out)
}

/** Sends the report. */
export async function uploadResult(matchID: number, r: MatchReport): Promise<string> {
  try {
    const res = await fetch(`/api/match/${matchID}/result`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json', Authorization: `Bearer ${token()}`},
      body: JSON.stringify({
        winner: r.winner,
        endFrame: r.endFrame,
        chars: r.chars,
        disconnect: r.disconnect ?? false,
        transport: r.transport ?? '',
        rttMs: r.rttMs ?? 0,
        rollbackAvg: r.rollbackAvg ?? 0,
        inputLog: base64(r.inputLog),
        checksums: base64(r.checksums),
      }),
    })
    const body = (await res.json().catch(() => ({}))) as {status?: string; error?: string}
    return body.status ?? body.error ?? `upload failed (${res.status})`
  } catch {
    return 'could not reach the server'
  }
}
