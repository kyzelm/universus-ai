import {AI_TIERS, type AITier} from './sim/wasm'

/** The modes a run can be started in (03 Game Design/Game Modes.md). */
export const MODES = ['local', 'ai', 'training', 'online'] as const
export type Mode = (typeof MODES)[number]

/**
 * Everything a run is started from — the same three things a replay log's
 * header carries, because they are the same three things the input stream
 * cannot recover (tools/replay/main.go).
 */
export interface RunSetup {
  mode: Mode
  /** Difficulty tier, 1..3, and 0 in every mode but `ai`. */
  ai: number
  chars: [number, number]
}

/** ?ai, or ?ai=easy|normal|hard. */
export function aiFromURL(params: URLSearchParams): number {
  if (!params.has('ai')) return 0
  const tier = AI_TIERS.indexOf((params.get('ai') || 'normal') as AITier)
  return tier > 0 ? tier : AI_TIERS.indexOf('normal')
}

/** ?p1=0&p2=1 picks the roster entries, defaulting to the first for both. */
export function charsFromURL(params: URLSearchParams): [number, number] {
  const pick = (key: string) => {
    const n = Number(params.get(key))
    return Number.isInteger(n) && n >= 0 ? n : 0
  }
  return [pick('p1'), pick('p2')]
}

/** The parameters that name part of a run. */
const RUN_PARAMS = ['training', 'ai', 'room', 'bot', 'bench', 'p1', 'p2']

/**
 * The setup a URL describes, or null when it describes no run at all — which is
 * somebody arriving, and they get the menu.
 */
export function setupFromURL(search = location.search): RunSetup | null {
  const params = new URLSearchParams(search)
  if (!RUN_PARAMS.some((k) => params.has(k))) return null

  const ai = aiFromURL(params)
  const mode: Mode = params.has('training')
    ? 'training'
    : ai > 0
      ? 'ai'
      : params.has('room')
        ? 'online'
        : 'local'

  return {mode, ai, chars: charsFromURL(params)}
}
