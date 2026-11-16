import type { ContentField, ContentSource, LanguageCode, MessageContent, Report } from '../types'
import type { SSEEvent } from '../api/sse'
import { DEFAULT_LANGUAGE } from '../utils/languages'

export type Phase = 'idle' | 'streaming' | 'done' | 'error'

export interface StreamResult {
  content: MessageContent
  report: Report
  source: ContentSource
}

/** What one language's run looks like. The languages are written in parallel. */
export interface LangStream {
  attempt: { n: number; max: number } | null
  /** Live text, only a preview until `result` arrives. */
  draft: MessageContent
  /** Why the previous attempt was rejected (shown while the next one runs). */
  retryProblems: string[]
  result: StreamResult | null
}

export interface StreamState {
  phase: Phase
  /** Per language code, created when its first event arrives. */
  langs: Record<LanguageCode, LangStream>
  fallbackReason: string | null
  error: { code: string; message: string } | null
}

export const emptyContent = (): MessageContent => ({ sms_body: '', email_subject: '', email_body: '' })

export const emptyLang = (): LangStream => ({ attempt: null, draft: emptyContent(), retryProblems: [], result: null })

export function initialState(): StreamState {
  return { phase: 'idle', langs: {}, fallbackReason: null, error: null }
}

export function startedState(): StreamState {
  return { ...initialState(), phase: 'streaming' }
}

const FIELDS: ContentField[] = ['sms_body', 'email_subject', 'email_body']

function withLang(s: StreamState, code: LanguageCode, change: (l: LangStream) => LangStream): StreamState {
  return { ...s, langs: { ...s.langs, [code]: change(s.langs[code] ?? emptyLang()) } }
}

/** The language an event is about (an event without one is the reference language's). */
const langOf = (d: Record<string, unknown>): LanguageCode => (typeof d.language === 'string' && d.language ? d.language : DEFAULT_LANGUAGE)

/** Pure reducer: one server-sent event in, next state out. */
export function reduce(s: StreamState, ev: SSEEvent): StreamState {
  const d = (ev.data ?? {}) as Record<string, unknown>
  const code = langOf(d)
  switch (ev.event) {
    case 'attempt':
      // A new attempt starts from a blank draft; the rejected text is discarded.
      return withLang({ ...s, phase: 'streaming' }, code, (l) => ({ ...l, attempt: { n: Number(d.n), max: Number(d.max) }, draft: emptyContent() }))
    case 'delta': {
      const field = d.field as ContentField
      if (!FIELDS.includes(field)) return s
      return withLang(s, code, (l) => ({ ...l, draft: { ...l.draft, [field]: l.draft[field] + String(d.text ?? '') } }))
    }
    case 'retry':
      return withLang(s, code, (l) => ({ ...l, retryProblems: (d.problems as string[]) ?? [] }))
    case 'validated':
      return withLang(s, code, (l) => ({
        ...l,
        retryProblems: [],
        draft: d.content as MessageContent,
        result: { content: d.content as MessageContent, report: d.report as Report, source: 'llm' },
      }))
    case 'fallback':
      return withLang({ ...s, fallbackReason: String(d.reason) }, code, (l) => ({
        ...l,
        retryProblems: [],
        draft: d.content as MessageContent,
        result: { content: d.content as MessageContent, report: d.report as Report, source: 'fallback' },
      }))
    case 'done':
      return { ...s, phase: 'done' }
    case 'error':
      return { ...s, phase: 'error', error: { code: String(d.code), message: String(d.message) } }
    default:
      return s
  }
}

/** The run is complete: every language has its validated (or fallback) text. */
export function allResults(s: StreamState, codes: LanguageCode[]): boolean {
  return codes.length > 0 && codes.every((c) => !!s.langs[c]?.result)
}

/** The languages the model is still fixing, with the reasons its last attempt was rejected. */
export function retryNotes(s: StreamState): { language: LanguageCode; problems: string[] }[] {
  return Object.entries(s.langs)
    .filter(([, l]) => l.retryProblems.length > 0)
    .map(([language, l]) => ({ language, problems: l.retryProblems }))
}
