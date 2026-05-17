import type { WsProgressPayload, IterationSnapshot } from './components/ProgressPanel'

/** Preset command stored in user_settings (key: preset_commands) */
export interface PresetCommand {
  id: string
  label: string
  icon: string
  content: string
  fill?: boolean  // true = fill editor instead of direct send
  sort: number
}

/** Unified Message type used across ChatPage and AssistantTurn */
export interface Message {
  id: string
  type: 'user' | 'assistant' | 'system'
  content: string
  ts?: number
  // Saved progress snapshot when this message was finalized (for showing intermediate process)
  savedProgress?: WsProgressPayload | null
  // Full iteration history (persisted across refreshes)
  iterationHistory?: IterationSnapshot[] | null
}

/** Turn-based message grouping (Codex style) */
export type Turn =
  | { type: 'user'; message: Message }
  | { type: 'assistant'; messages: Message[] }
