import { describe, expect, it } from 'vitest'
// @ts-expect-error the project has no Node typings; the test runner runs on Node
import { readFileSync } from 'node:fs'
import stepBar from '../components/StepBar.vue?raw'
import { scheduleCaption } from './dashboard'
import { FALLBACK_LANGUAGES, countsLine, sentLine } from './languages'
import { currentStep } from './steps'
import type { Campaign, CampaignStatus } from '../types'

// Plain .css files are not loaded by the test runner (they come back empty), so the file is read directly.
const styleCss: string = readFileSync(new URL('../style.css', import.meta.url), 'utf-8')

// ---- per-language numbers -----------------------------------------------------------

describe('countsLine', () => {
  it('writes "TR 171 · EN 72 · DE 42" in language order', () => {
    expect(countsLine({ de: 42, tr: 171, en: 72 }, FALLBACK_LANGUAGES)).toBe('TR 171 · EN 72 · DE 42')
  })
  it('leaves out languages nobody speaks, and says nothing without data', () => {
    expect(countsLine({ tr: 5, en: 0 }, FALLBACK_LANGUAGES)).toBe('TR 5')
    expect(countsLine(undefined, FALLBACK_LANGUAGES)).toBe('')
    expect(countsLine({}, FALLBACK_LANGUAGES)).toBe('')
  })
})

describe('sentLine', () => {
  const langs = { tr: { total: 171, sent: 171 }, en: { total: 72, sent: 60 }, de: { total: 42, sent: 0 } }
  it('shows what was sent once everything is handled', () => {
    expect(sentLine(langs, FALLBACK_LANGUAGES, true)).toBe('TR 171 · EN 60 · DE 0')
  })
  it('shows sent/total while messages still wait', () => {
    expect(sentLine(langs, FALLBACK_LANGUAGES, false)).toBe('TR 171/171 · EN 60/72 · DE 0/42')
  })
  it('skips a language without messages and says nothing without data', () => {
    expect(sentLine({ tr: { total: 3, sent: 3 }, en: { total: 0, sent: 0 } }, FALLBACK_LANGUAGES, true)).toBe('TR 3')
    expect(sentLine(undefined, FALLBACK_LANGUAGES, true)).toBe('')
  })
})

// ---- the planned time on a card -----------------------------------------------------

describe('scheduleCaption', () => {
  const at = '2026-12-01T10:00:00Z'
  it('is absent for a campaign that was never scheduled', () => {
    expect(scheduleCaption({ status: 'completed' })).toBeNull()
    expect(scheduleCaption({ status: 'content_ready', scheduled_at: undefined })).toBeNull()
  })
  it('names the time by what became of the campaign', () => {
    expect(scheduleCaption({ status: 'scheduled', scheduled_at: at })).toBe('Zamanlandı')
    expect(scheduleCaption({ status: 'cancelled', scheduled_at: at })).toBe('İptal edilen zaman')
    expect(scheduleCaption({ status: 'failed', scheduled_at: at })).toBe('Planlanan zaman')
  })
  it('keeps the time on a campaign that has started', () => {
    for (const status of ['approved', 'sending', 'completed'] as const) {
      expect(scheduleCaption({ status, scheduled_at: at }), status).toBe('Zamanlanan gönderim')
    }
  })
})

// ---- where a campaign opens ---------------------------------------------------------

// A dashboard card opens the wizard at the step the campaign stands at.
describe('the step a campaign opens at', () => {
  const camp = (status: CampaignStatus, segmentStatus: 'ok' | 'needs_clarification' | 'unsupported' = 'ok'): Campaign =>
    ({ id: 1, brief: 'b', status, created_at: '', segment: { status: segmentStatus, conditions: [], explanation: '' } }) as Campaign
  const at = (c: Campaign, hasContent = false) => currentStep({ campaign: c, streaming: false, hasContent, unsaved: false }).current

  it('is the audience step for a draft waiting on an answer or a reason', () => {
    expect(at(camp('draft', 'needs_clarification'))).toBe('audience')
    expect(at(camp('draft', 'unsupported'))).toBe('audience')
  })
  it('is the audience step when the messages are still to be written', () => {
    expect(at(camp('segment_ready'))).toBe('audience')
  })
  it('is the approval step when the messages are ready', () => {
    expect(at(camp('content_ready'), true)).toBe('approve')
  })
  it('is the last step once approved, scheduled, cancelled or failed', () => {
    for (const s of ['scheduled', 'approved', 'sending', 'completed', 'cancelled', 'failed'] as const) {
      expect(at(camp(s)), s).toBe('send')
    }
  })
})

// ---- looks (the rules that carry the requirement) ----------------------------------------

describe('read-only fields look read-only', () => {
  const rule = styleCss.match(/textarea\[readonly\][^{]*\{([^}]*)\}/)?.[1] ?? ''
  it('are grey, cannot be resized and show no text cursor', () => {
    expect(rule).toMatch(/background:\s*var\(--surface-2\)/)
    expect(rule).toMatch(/resize:\s*none/)
    expect(rule).toMatch(/cursor:\s*default/)
  })
})

describe('the active step stands out', () => {
  const button = stepBar.match(/\.current button\s*\{([^}]*)\}/)?.[1] ?? ''
  it('is blue, semibold and underlined in the flow bar', () => {
    expect(button).toMatch(/color:\s*var\(--accent\)/)
    expect(button).toMatch(/font-weight:\s*600/)
    expect(button).toMatch(/box-shadow:\s*inset 0 -2px 0 var\(--accent\)/)
  })
})
