import {flags, type Flags} from './flags'

/** The modes a run can be started in (03 Game Design/Game Modes.md). */
export const MODES = ['local', 'ai', 'training', 'online'] as const
export type Mode = (typeof MODES)[number]

/** Everything a run is started from. */
export interface RunSetup {
  mode: Mode
  /** Seat 2 is the scripted opponent: true only in mode `ai`. */
  ai: boolean
  chars: [number, number]
  /** Input pairs from a saved log, when the run is a replay of one. */
  replay?: number[]
}

/** The setup a URL describes, or null when it describes no run at all. */
export function setupFromURL(f: Flags = flags): RunSetup | null {
  if (!f.run) return null

  const mode: Mode = f.training ? 'training' : f.ai ? 'ai' : f.room !== null ? 'online' : 'local'

  return {mode, ai: mode === 'ai', chars: f.chars}
}
