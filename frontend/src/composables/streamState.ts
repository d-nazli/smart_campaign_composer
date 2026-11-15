import type { ContentField, ContentSource, MessageContent, Report } from '../types'
import type { SSEEvent } from '../api/sse'

export type Phase = 'idle' | 'streaming' | 'done' | 'error'

export interface StreamResult {
  content: MessageContent
  report: Report
  source: ContentSource
}

export interface StreamState {
  phase: Phase
  attempt: { n: number; max: number } | null
  /** Live text, only a preview until `result` arrives. */
  draft: MessageContent
  /** Why the previous attempt was rejected (shown while the next one runs). */
  retryProblems: string[]
  fallbackReason: string | null
  error: { code: string; message: string } | null
  result: StreamResult | null
}

export const emptyContent = (): MessageContent => ({ sms_body: '', email_subject: '', email_body: '' })

export function initialState(): StreamState {
  return { phase: 'idle', attempt: null, draft: emptyContent(), retryProblems: [], fallbackReason: null, error: null, result: null }
}

export function startedState(): StreamState {
  return { ...initialState(), phase: 'streaming' }
}

const FIELDS: ContentField[] = ['sms_body', 'email_subject', 'email_body']

/** Pure reducer: one server-sent event in, next state out. */
export function reduce(s: StreamState, ev: SSEEvent): StreamState {
  const d = (ev.data ?? {}) as Record<string, unknown>
  switch (ev.event) {
    case 'attempt':
      // A new attempt starts from a blank draft; the rejected text is discarded.
      return { ...s, phase: 'streaming', attempt: { n: Number(d.n), max: Number(d.max) }, draft: emptyContent() }
    case 'delta': {
      const field = d.field as ContentField
      if (!FIELDS.includes(field)) return s
      return { ...s, draft: { ...s.draft, [field]: s.draft[field] + String(d.text ?? '') } }
    }
    case 'retry':
      return { ...s, retryProblems: (d.problems as string[]) ?? [] }
    case 'validated':
      return {
        ...s,
        retryProblems: [],
        draft: d.content as MessageContent,
        result: { content: d.content as MessageContent, report: d.report as Report, source: 'llm' },
      }
    case 'fallback':
      return {
        ...s,
        fallbackReason: String(d.reason),
        draft: d.content as MessageContent,
        result: { content: d.content as MessageContent, report: d.report as Report, source: 'fallback' },
      }
    case 'done':
      return { ...s, phase: 'done' }
    case 'error':
      return { ...s, phase: 'error', error: { code: String(d.code), message: String(d.message) } }
    default:
      return s
  }
}
