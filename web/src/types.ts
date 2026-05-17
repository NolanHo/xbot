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

/** Reply reference information for quoted messages */
export interface ReplyInfo {
  id: string
  /** Truncated preview of the original message content */
  content: string
  type: 'user' | 'assistant'
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
  /** Reply reference — links to the message being replied to */
  replyTo?: ReplyInfo
}

/** Turn-based message grouping (Codex style) */
export type Turn =
  | { type: 'user'; message: Message }
  | { type: 'assistant'; messages: Message[] }
