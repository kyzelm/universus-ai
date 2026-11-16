import {flags, type Flags} from './flags'

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
  /** Seat 2 is the scripted opponent: true only in mode `ai`. */
  ai: boolean
  chars: [number, number]
}

/**
 * The setup a URL describes, or null when it describes no run at all — which is
 * somebody arriving, and they get the menu.
 */
export function setupFromURL(f: Flags = flags): RunSetup | null {
  if (!f.run) return null

  const mode: Mode = f.training ? 'training' : f.ai ? 'ai' : f.room !== null ? 'online' : 'local'

  return {mode, ai: mode === 'ai', chars: f.chars}
}
