// @vitest-environment jsdom
// The whole wizard, mounted for real, with the API and the event stream mocked:
// brief → audience card → the message stream flows into the fields → edit with the
// pencil and save → "Onayla" → approval → the sending step.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { AppInfo } from '../api/info'
import type { SSEEvent } from '../api/sse'
import type { Campaign, StoredContent } from '../types'

const api = {
  createCampaign: vi.fn(),
  clarifyCampaign: vi.fn(),
  updateBrief: vi.fn(),
  getCampaign: vi.fn(),
  saveContent: vi.fn(),
  translateCampaign: vi.fn(),
  approveCampaign: vi.fn(),
}
vi.mock('../api/campaigns', () => ({
  createCampaign: (...a: unknown[]) => api.createCampaign(...a),
  clarifyCampaign: (...a: unknown[]) => api.clarifyCampaign(...a),
  updateBrief: (...a: unknown[]) => api.updateBrief(...a),
  getCampaign: (...a: unknown[]) => api.getCampaign(...a),
  saveContent: (...a: unknown[]) => api.saveContent(...a),
  translateCampaign: (...a: unknown[]) => api.translateCampaign(...a),
  approveCampaign: (...a: unknown[]) => api.approveCampaign(...a),
  cancelCampaign: vi.fn(),
  deleteVariant: vi.fn(),
}))

// The stream: the test pushes events and decides when the server closes it.
let emit: (e: SSEEvent) => void = () => {}
let close: () => void = () => {}
const streamPost = vi.fn()
vi.mock('../api/sse', () => ({ streamPost: (...a: unknown[]) => streamPost(...a) }))

import ComposeView from './ComposeView.vue'

const SMS = 'Merhaba {{first_name}}, sepetin seni bekliyor: %15 indirim!'
const stored = (sms: string, o: Partial<StoredContent> = {}): StoredContent => ({
  sms_body: sms,
  email_subject: 'Sepetin seni bekliyor: %15 indirim',
  email_body: 'Merhaba {{first_name}},\n\n%15 indirim seni bekliyor.',
  locale: 'tr-TR',
  source: 'llm',
  version: 1,
  stale: false,
  report: { ok: true, issues: [], sms_length: sms.length, subject_length: 34 },
  updated_at: '',
  ...o,
})
const audience = { matched: 1200, sms_reachable: 900, email_reachable: 1000, both_reachable: 800 }
const segmentReady: Campaign = {
  id: 7,
  brief: 'Son 30 günde sepete ürün ekleyip almayanlara %15 indirim',
  status: 'segment_ready',
  source_locale: 'tr-TR',
  locales: ['tr-TR', 'en-GB'],
  segment: {
    status: 'ok',
    match: 'all',
    conditions: [{ field: 'days_since_last_cart_add', op: 'lte', value: 30 }, { field: 'purchased_since_last_cart_add', op: 'eq', value: false }],
    explanation: 'son 30 günde sepete ekleyip almayanlar',
    audience,
    offer: { kind: 'percent', value: 15, code: '' },
    locales: { 'tr-TR': 1000, 'en-GB': 200 },
  },
  created_at: '',
}
const english = (sms: string, version = 1) =>
  stored(sms, { locale: 'en-GB', source: 'translated', source_version: version, email_subject: '15% off', email_body: 'Hi {{first_name}}, 15% off.' })
const generated: Campaign = {
  ...segmentReady,
  status: 'content_ready',
  content: stored(SMS),
  translations: { 'en-GB': english('Hi {{first_name}}, 15% off is waiting!') },
  pending_locales: [],
  mismatches: [],
  variants: ['A'],
}
const EDITED = 'Merhaba {{first_name}}, %15 indirim seni bekliyor!'
const saved: Campaign = {
  ...generated,
  content: stored(EDITED, { version: 2, source: 'edited' }),
  translations: { 'en-GB': { ...english('Hi {{first_name}}, 15% off is waiting!'), stale: true } },
  pending_locales: ['en-GB'],
}
const translated: Campaign = { ...saved, translations: { 'en-GB': english('Hi {{first_name}}, 15% off for you!', 2) }, pending_locales: [] }
const ch = (o = {}) => ({ retries: 0, rate_limited: 0, total: 900, pending: 900, sent: 0, failed: 0, skipped: 0, ...o })
const sending: Campaign = { ...translated, status: 'sending', delivery: { sms: ch(), email: ch({ total: 1000, pending: 1000 }), done: false } }

const info: AppInfo = {
  llm_mode: 'mock',
  limits: { sms_max: 160, subject_max: 80, email_body_max: 2000 },
  variables: [{ name: 'first_name', budget: 20, fallback: 'Değerli müşterimiz' }],
  source_locale: 'tr-TR',
  locales: [
    { code: 'tr-TR', name: 'Turkish', native_name: 'Türkçe', direction: 'ltr', is_source: true },
    { code: 'en-GB', name: 'English', native_name: 'English', direction: 'ltr', is_source: false },
  ],
}

let w: VueWrapper | null = null
const q = (id: string) => w!.find(`[data-testid="${id}"]`)
const primary = () => w!.get('[data-action="primary"]')
const step = () => w!.get('[aria-current="step"]').text()
const sms = () => w!.get('#sms').element as HTMLTextAreaElement

beforeEach(() => {
  for (const f of Object.values(api)) f.mockReset()
  streamPost.mockReset()
  streamPost.mockImplementation(
    (_url: string, _signal: AbortSignal, onEvent: (e: SSEEvent) => void) =>
      new Promise<void>((resolve) => {
        emit = onEvent
        close = resolve
      }),
  )
  window.history.replaceState(null, '', '/')
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
  window.history.replaceState(null, '', '/')
})

describe('the campaign wizard, from the brief to sending', () => {
  it('brief → audience → streamed messages → edit and save → approve → sending', async () => {
    w = mount(ComposeView, { props: { info }, attachTo: document.body })
    await flushPromises()

    // 1 · Brief
    expect(step()).toBe('Brief')
    api.createCampaign.mockResolvedValue(segmentReady)
    await w.get('#brief').setValue(segmentReady.brief)
    expect(primary().text()).toBe('Kitleyi çıkar')
    await primary().trigger('click')
    await flushPromises()
    expect(api.createCampaign).toHaveBeenCalledWith(segmentReady.brief)
    expect(window.location.search).toBe('?c=7')

    // 2 · Audience: the segment card
    expect(step()).toBe('Kitle')
    expect(w.get('#step-audience').text()).toContain('Son 30 günde sepete ekleyip almayanlar')
    expect(primary().text()).toBe('Mesajları üret')

    // 3 · Messages: the stream flows into the fields while it is written
    await primary().trigger('click')
    await flushPromises()
    expect(streamPost.mock.calls[0][0]).toBe('/api/campaigns/7/messages/stream')
    expect(step()).toBe('Mesajlar')
    emit({ event: 'attempt', data: { locale: 'tr-TR', n: 1, max: 3 } })
    emit({ event: 'delta', data: { locale: 'tr-TR', field: 'sms_body', text: 'Merhaba {{first_name}}, ' } })
    emit({ event: 'delta', data: { locale: 'tr-TR', field: 'sms_body', text: 'sepetin' } })
    await flushPromises()
    expect(sms().value).toBe('Merhaba {{first_name}}, sepetin')
    expect(sms().readOnly).toBe(true) // the model is writing
    expect(primary().attributes('disabled')).toBeDefined()

    api.getCampaign.mockResolvedValue(generated)
    emit({ event: 'validated', data: { locale: 'tr-TR', content: generated.content, report: generated.content!.report, source: 'llm' } })
    emit({ event: 'done', data: { status: 'content_ready', variant: 'A' } })
    close()
    await flushPromises()
    expect(sms().value).toBe(SMS)
    expect(q('source').text()).toBe('Model metni')

    // Edit with the pencil and save: the translations are written again
    await q('edit').trigger('click')
    await w.get('#sms').setValue(EDITED)
    expect(primary().attributes('disabled')).toBeDefined()
    api.saveContent.mockResolvedValue(saved)
    api.translateCampaign.mockResolvedValue(translated)
    await q('edit-save').trigger('click')
    await flushPromises()
    expect(api.saveContent).toHaveBeenCalledWith(7, { 'tr-TR': { sms_body: EDITED, ...pick(generated.content!) } }, 'A')
    expect(api.translateCampaign).toHaveBeenCalledWith(7, ['en-GB'], 'A')
    expect(sms().value).toBe(EDITED)
    expect(q('source').text()).toBe('Düzenlendi')

    // "Onayla": nothing waits, so the approval step opens
    expect(primary().text()).toBe('Onayla')
    expect(primary().attributes('disabled')).toBeUndefined()
    await primary().trigger('click')
    await flushPromises()
    expect(step()).toBe('Onay')

    // 4 · Approval: send now, confirmed in the dialog
    expect(primary().text()).toBe('Gönder')
    await primary().trigger('click')
    await flushPromises()
    api.approveCampaign.mockResolvedValue({ status: 'sending' })
    api.getCampaign.mockResolvedValue(sending)
    ;(document.body.querySelector('[data-testid="confirm-approve"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(api.approveCampaign).toHaveBeenCalledWith(7, undefined)

    // 5 · Sending
    expect(step()).toBe('Gönderim')
    expect(w.text()).toContain('SMS · gönderilen')
  })

  it('a question is answered on the same campaign: no new campaign, the history is shown', async () => {
    const QUESTION = 'Bu kampanya kime gönderilsin? Tüm müşterilere mi?'
    const asking: Campaign = {
      id: 9,
      brief: 'Kış ürünlerinde 200 TL indirim',
      name: 'Kış ürünleri · 200 TL',
      status: 'draft',
      source_locale: 'tr-TR',
      locales: ['tr-TR'],
      segment: { status: 'needs_clarification', conditions: [], explanation: '', question: QUESTION, suggestion: 'all_customers' },
      created_at: '',
    }
    const qa = (answer: string) => [{ question: QUESTION, answer, answered_at: '2026-11-19T10:00:00Z' }]
    const askedAgain: Campaign = { ...asking, segment: { ...asking.segment!, question: 'Hangi şehirdeki müşteriler?', suggestion: undefined, clarifications: qa('bilmiyorum') } }
    const answered: Campaign = {
      ...segmentReady,
      id: 9,
      brief: asking.brief,
      segment: {
        ...segmentReady.segment!,
        explanation: "Ankara'daki müşteriler",
        clarifications: [...qa('bilmiyorum'), { question: 'Hangi şehirdeki müşteriler?', answer: "Ankara'dakilere", answered_at: '2026-11-19T10:01:00Z' }],
      },
    }

    w = mount(ComposeView, { props: { info }, attachTo: document.body })
    await flushPromises()
    api.createCampaign.mockResolvedValue(asking)
    await w.get('#brief').setValue(asking.brief)
    await primary().trigger('click')
    await flushPromises()
    expect(step()).toBe('Kitle')
    expect(q('question').text()).toBe(QUESTION)
    expect(w.text()).not.toContain('Kitle çıkarılıyor')

    // A typed answer that is not enough: the same campaign asks again, with the history.
    api.clarifyCampaign.mockResolvedValue(askedAgain)
    await w.get('[data-testid="answer"]').setValue('bilmiyorum')
    await w.get('[data-testid="send-answer"]').trigger('click')
    await flushPromises()
    expect(api.clarifyCampaign).toHaveBeenLastCalledWith(9, 'bilmiyorum')
    expect(q('question').text()).toBe('Hangi şehirdeki müşteriler?')
    expect(q('clarifications').findAll('p').map((p) => p.text())).toEqual([`Soru: ${QUESTION}`, 'Cevap: bilmiyorum'])
    expect((w.get('[data-testid="answer"]').element as HTMLTextAreaElement).value).toBe('') // a fresh box for the new question

    // The second answer resolves the audience on campaign 9.
    api.clarifyCampaign.mockResolvedValue(answered)
    await w.get('[data-testid="answer"]').setValue("Ankara'dakilere")
    await w.get('[data-testid="send-answer"]').trigger('click')
    await flushPromises()
    expect(api.clarifyCampaign).toHaveBeenLastCalledWith(9, "Ankara'dakilere")
    expect(api.createCampaign).toHaveBeenCalledTimes(1) // the bug: an answer used to create a second campaign
    expect(window.location.search).toBe('?c=9')
    expect(w.get('#step-audience').text()).toContain("Ankara'daki müşteriler")
    expect(q('clarifications').text()).toContain("Cevap: Ankara'dakilere")
    expect(primary().text()).toBe('Mesajları üret')
  })

  it('"Tüm müşterilere gönder" answers the same campaign; a 409 shows where it stands', async () => {
    const asking: Campaign = {
      id: 10,
      brief: '%10 indirim gönder',
      status: 'draft',
      source_locale: 'tr-TR',
      locales: ['tr-TR'],
      segment: { status: 'needs_clarification', conditions: [], explanation: '', question: 'Bu kampanya kime gönderilsin? Tüm müşterilere mi?', suggestion: 'all_customers' },
      created_at: '',
    }
    w = mount(ComposeView, { props: { info }, attachTo: document.body })
    await flushPromises()
    api.createCampaign.mockResolvedValue(asking)
    await w.get('#brief').setValue(asking.brief)
    await primary().trigger('click')
    await flushPromises()

    const { ApiError } = await import('../api/http')
    api.clarifyCampaign.mockRejectedValue(new ApiError(409, 'invalid_state', 'yalnızca soru bekleyen kampanyaya cevap verilebilir'))
    api.getCampaign.mockResolvedValue({ ...segmentReady, id: 10 })
    await w.get('[data-testid="send-to-all"]').trigger('click')
    await flushPromises()
    expect(api.clarifyCampaign).toHaveBeenCalledWith(10, 'Tüm müşterilere gönder.')
    expect(api.createCampaign).toHaveBeenCalledTimes(1)
    expect(q('answer-error').text()).toContain('yalnızca soru bekleyen')
    expect(api.getCampaign).toHaveBeenCalledWith(10)
    expect(w.get('#step-audience').text()).toContain('Son 30 günde sepete ekleyip almayanlar')
  })

  it('"Desteklenmiyor" → "Brief\'i düzenle" → the edited brief goes to the same campaign', async () => {
    const unsupported: Campaign = {
      id: 12,
      brief: '35 yaş üstü müşterilere Kasım indirimi %15',
      name: 'Kasım indirimi · %15',
      status: 'draft',
      source_locale: 'tr-TR',
      locales: ['tr-TR'],
      segment: { status: 'unsupported', conditions: [], explanation: 'Yaş verisi yok.' },
      created_at: '',
    }
    const EDITED_BRIEF = "İstanbul'daki müşterilere Kasım indirimi %15"
    const resolved: Campaign = { ...segmentReady, id: 12, brief: EDITED_BRIEF, segment: { ...segmentReady.segment!, explanation: "İstanbul'daki müşteriler" } }

    w = mount(ComposeView, { props: { info }, attachTo: document.body })
    await flushPromises()
    api.createCampaign.mockResolvedValue(unsupported)
    await w.get('#brief').setValue(unsupported.brief)
    await primary().trigger('click')
    await flushPromises()
    expect(w.get('#step-audience').text()).toContain('Bu brief desteklenmiyor')
    expect(w.text()).not.toContain('Kitle çıkarılıyor') // only the segment card, no empty placeholder card under it

    await q('edit-brief').trigger('click')
    await flushPromises()
    expect(step()).toMatch(/^Brief/)
    expect(document.activeElement?.id).toBe('brief')
    expect(primary().text()).toBe('İleri') // unchanged brief: nothing to rebuild
    await w.get('#brief').setValue(EDITED_BRIEF)
    expect(primary().text()).toBe('Kitleyi çıkar')

    api.updateBrief.mockResolvedValue(resolved)
    await primary().trigger('click')
    await flushPromises()
    expect(api.updateBrief).toHaveBeenCalledWith(12, EDITED_BRIEF)
    expect(api.createCampaign).toHaveBeenCalledTimes(1) // the bug: every try opened a new campaign
    expect(window.location.search).toBe('?c=12')
    expect(step()).toBe('Kitle')
    expect(w.get('#step-audience').text()).toContain("İstanbul'daki müşteriler")
  })

  it('an edited brief refused with 409 (approved meanwhile) shows the campaign as it is', async () => {
    const asking: Campaign = {
      id: 13,
      brief: '%10 indirim gönder',
      status: 'draft',
      source_locale: 'tr-TR',
      locales: ['tr-TR'],
      segment: { status: 'needs_clarification', conditions: [], explanation: '', question: 'Kime?' },
      created_at: '',
    }
    w = mount(ComposeView, { props: { info }, attachTo: document.body })
    await flushPromises()
    api.createCampaign.mockResolvedValue(asking)
    await w.get('#brief').setValue(asking.brief)
    await primary().trigger('click')
    await flushPromises()
    await q('edit-brief').trigger('click')
    await w.get('#brief').setValue("Ankara'daki müşterilere %10 indirim gönder")

    const { ApiError } = await import('../api/http')
    api.updateBrief.mockRejectedValue(new ApiError(409, 'invalid_state', 'yalnızca onaylanmamış kampanyanın brief\'i değiştirilebilir'))
    api.getCampaign.mockResolvedValue({ ...sending, id: 13, brief: asking.brief })
    await primary().trigger('click')
    await flushPromises()
    expect(api.createCampaign).toHaveBeenCalledTimes(1)
    expect(api.getCampaign).toHaveBeenCalledWith(13)
    expect(w.text()).toContain('yalnızca onaylanmamış kampanyanın brief')
  })
})

// The three fields of a saved text as the editor sends them.
function pick(c: { sms_body: string; email_subject: string; email_body: string }) {
  return { email_subject: c.email_subject, email_body: c.email_body }
}
