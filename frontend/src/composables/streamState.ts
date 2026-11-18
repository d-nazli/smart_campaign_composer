import type { ContentField, ContentSource, LocaleCode, MessageContent, Report } from '../types'
import type { SSEEvent } from '../api/sse'
import { SOURCE_LOCALE } from '../utils/locales'

export type Phase = 'idle' | 'streaming' | 'done' | 'error'

export interface StreamResult {
  content: MessageContent
  report: Report
  source: ContentSource
}

/** What one locale's run looks like: the source first, then the translations in parallel. */
export interface LocaleStream {
  attempt: { n: number; max: number } | null
  /** Live text, only a preview until `result` arrives. */
  draft: MessageContent
  /** Why the previous attempt was rejected (shown while the next one runs). */
  retryProblems: string[]
  result: StreamResult | null
  /** Why the translation could not be made (the locale keeps what it had). */
  failed: string | null
}

export interface StreamState {
  phase: Phase
  /** Per locale code, created when its first event arrives. */
  locales: Record<LocaleCode, LocaleStream>
  fallbackReason: string | null
  error: { code: string; message: string } | null
}

export const emptyContent = (): MessageContent => ({ sms_body: '', email_subject: '', email_body: '' })

export const emptyLocale = (): LocaleStream => ({ attempt: null, draft: emptyContent(), retryProblems: [], result: null, failed: null })

export function initialState(): StreamState {
  return { phase: 'idle', locales: {}, fallbackReason: null, error: null }
}

export function startedState(): StreamState {
  return { ...initialState(), phase: 'streaming' }
}

const FIELDS: ContentField[] = ['sms_body', 'email_subject', 'email_body']

function withLocale(s: StreamState, code: LocaleCode, change: (l: LocaleStream) => LocaleStream): StreamState {
  return { ...s, locales: { ...s.locales, [code]: change(s.locales[code] ?? emptyLocale()) } }
}

/** The locale an event is about (an event without one is the source's). */
const localeOf = (d: Record<string, unknown>): LocaleCode => (typeof d.locale === 'string' && d.locale ? d.locale : SOURCE_LOCALE)

/** Pure reducer: one server-sent event in, next state out. */
export function reduce(s: StreamState, ev: SSEEvent): StreamState {
  const d = (ev.data ?? {}) as Record<string, unknown>
  const code = localeOf(d)
  switch (ev.event) {
    case 'attempt':
      // A new attempt starts from a blank draft; the rejected text is discarded.
      return withLocale({ ...s, phase: 'streaming' }, code, (l) => ({ ...l, attempt: { n: Number(d.n), max: Number(d.max) }, draft: emptyContent() }))
    case 'delta': {
      const field = d.field as ContentField
      if (!FIELDS.includes(field)) return s
      return withLocale(s, code, (l) => ({ ...l, draft: { ...l.draft, [field]: l.draft[field] + String(d.text ?? '') } }))
    }
    case 'retry':
      return withLocale(s, code, (l) => ({ ...l, retryProblems: (d.problems as string[]) ?? [] }))
    case 'validated':
      return withLocale(s, code, (l) => ({
        ...l,
        retryProblems: [],
        draft: d.content as MessageContent,
        failed: null,
        result: { content: d.content as MessageContent, report: d.report as Report, source: (d.source as ContentSource) ?? 'llm' },
      }))
    case 'fallback':
      return withLocale({ ...s, fallbackReason: String(d.reason) }, code, (l) => ({
        ...l,
        retryProblems: [],
        draft: d.content as MessageContent,
        result: { content: d.content as MessageContent, report: d.report as Report, source: 'fallback' },
      }))
    case 'failed':
      return withLocale(s, code, (l) => ({ ...l, retryProblems: [], failed: String(d.reason ?? 'invalid_output') }))
    case 'done':
      return { ...s, phase: 'done' }
    case 'error':
      return { ...s, phase: 'error', error: { code: String(d.code), message: String(d.message) } }
    default:
      return s
  }
}

/** Every given locale has finished: a validated (or fallback) text, or a failed translation. */
export function allResults(s: StreamState, codes: LocaleCode[]): boolean {
  return codes.length > 0 && codes.every((c) => !!s.locales[c]?.result || !!s.locales[c]?.failed)
}

/** The locales the model is still fixing, with the reasons its last attempt was rejected. */
export function retryNotes(s: StreamState): { locale: LocaleCode; problems: string[] }[] {
  return Object.entries(s.locales)
    .filter(([, l]) => l.retryProblems.length > 0)
    .map(([locale, l]) => ({ locale, problems: l.retryProblems }))
}

/** The translations that could not be made in this run. */
export function failedLocales(s: StreamState): LocaleCode[] {
  return Object.entries(s.locales)
    .filter(([, l]) => !!l.failed)
    .map(([code]) => code)
}
