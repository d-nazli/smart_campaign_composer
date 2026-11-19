// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { AppInfo } from '../api/info'
import { ApiError } from '../api/http'
import type { Campaign, StoredContent } from '../types'

const get = vi.fn()
const save = vi.fn()
const translate = vi.fn()
vi.mock('../api/campaigns', () => ({
  getCampaign: (...a: unknown[]) => get(...a),
  saveContent: (...a: unknown[]) => save(...a),
  translateCampaign: (...a: unknown[]) => translate(...a),
  approveCampaign: vi.fn(),
  cancelCampaign: vi.fn(),
  createCampaign: vi.fn(),
  deleteVariant: vi.fn(),
}))

import ComposeView from './ComposeView.vue'
import { LOCKED_MESSAGE } from '../utils/editing'

const stored = (sms: string, o: Partial<StoredContent> = {}): StoredContent => ({
  sms_body: sms,
  email_subject: 'Konu',
  email_body: 'Gövde',
  locale: 'tr-TR',
  source: 'llm',
  version: 1,
  stale: false,
  report: { ok: true, issues: [], sms_length: sms.length, subject_length: 4 },
  updated_at: '',
  ...o,
})

const ready: Campaign = {
  id: 1,
  brief: 'brief',
  status: 'content_ready',
  source_locale: 'tr-TR',
  locales: ['tr-TR', 'en-GB'],
  segment: { status: 'ok', conditions: [], explanation: 'x', locales: { 'tr-TR': 10, 'en-GB': 5 } },
  content: stored('Merhaba'),
  translations: { 'en-GB': stored('Hello', { locale: 'en-GB', source: 'translated', source_version: 1 }) },
  pending_locales: [],
  mismatches: [],
  variants: ['A'],
  created_at: '',
}
// After saving "Merhaba yeni": the source is version 2, its translation stale until translated again.
const saved: Campaign = {
  ...ready,
  content: stored('Merhaba yeni', { version: 2, source: 'edited' }),
  translations: { 'en-GB': stored('Hello', { locale: 'en-GB', source: 'translated', source_version: 1, stale: true }) },
  pending_locales: ['en-GB'],
}
const translated: Campaign = {
  ...saved,
  translations: { 'en-GB': stored('Hello new', { locale: 'en-GB', source: 'translated', source_version: 2 }) },
  pending_locales: [],
}

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
const sms = () => w!.get('#sms').element as HTMLTextAreaElement
const approveButton = () => w!.get('[data-action="primary"]')

// Opens the campaign and goes back from the approval step (where content_ready opens) to the messages.
async function openMessages() {
  window.history.replaceState(null, '', '/?c=1')
  w = mount(ComposeView, { props: { info }, attachTo: document.body })
  await flushPromises()
  await w.get('[data-testid="back"]').trigger('click')
  await flushPromises()
}

async function editTo(text: string) {
  await q('edit').trigger('click')
  await w!.get('#sms').setValue(text)
}

beforeEach(() => {
  get.mockReset()
  save.mockReset()
  translate.mockReset()
  get.mockResolvedValue(ready)
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
  window.history.replaceState(null, '', '/')
})

describe('editing the Turkish text', () => {
  it('is read-only until the pencil is pressed', async () => {
    await openMessages()
    expect(sms().readOnly).toBe(true)
    expect(q('edit').attributes('aria-label')).toBe('Türkçe metni düzenle')
    expect(q('edit').text()).toBe('Düzenle')
    expect(q('locale-head').find('[data-testid="edit"]').exists()).toBe(false) // not next to the recipient count
    expect(w!.text()).not.toContain('Kişiselleştirme')
    expect(approveButton().text()).toBe('Onayla')
    expect(approveButton().attributes('disabled')).toBeUndefined()

    await q('edit').trigger('click')
    expect(sms().readOnly).toBe(false)
    expect(q('edit').exists()).toBe(false)
    expect(q('edit-save').exists()).toBe(true)
    expect(q('edit-cancel').exists()).toBe(true)
    expect(approveButton().attributes('disabled')).toBeDefined() // an open edit holds "Onayla"
  })

  it('Kaydet stores the text, then updates the translations; "Onayla" waits for both', async () => {
    await openMessages()
    await editTo('Merhaba yeni')
    expect(q('unsaved').exists()).toBe(true)
    expect(approveButton().attributes('disabled')).toBeDefined()

    let finish: (c: Campaign) => void = () => {}
    save.mockResolvedValue(saved)
    translate.mockReturnValue(new Promise<Campaign>((r) => (finish = r)))
    await q('edit-save').trigger('click')
    await flushPromises()

    expect(save).toHaveBeenCalledWith(1, { 'tr-TR': { sms_body: 'Merhaba yeni', email_subject: 'Konu', email_body: 'Gövde' } }, 'A')
    expect(translate).toHaveBeenCalledWith(1, ['en-GB'], 'A')
    expect(q('translating').text()).toBe('Çeviriler güncelleniyor…')
    expect(q('translating').attributes('role')).toBe('status')
    expect(approveButton().attributes('disabled')).toBeDefined()
    expect(sms().readOnly).toBe(true)

    finish(translated)
    await flushPromises()
    expect(q('translating').exists()).toBe(false)
    expect(sms().value).toBe('Merhaba yeni')
    expect(approveButton().attributes('disabled')).toBeUndefined()
  })

  it('the SMS counter still reserves each variable its budget (the list of variables is not shown)', async () => {
    await openMessages()
    await editTo('Merhaba {{first_name}}')
    // "Merhaba " is 8 characters, {{first_name}} counts as its budget of 20.
    expect(w!.find('.counter.as-ring').attributes('aria-label')).toBe('28 / 160 karakter')
  })

  it('the source head has no version badge; an edited text says "Düzenlendi"', async () => {
    get.mockResolvedValue({ ...ready, content: stored('Merhaba', { version: 3, source: 'edited' }) })
    await openMessages()
    const head = q('locale-head')
    expect(head.text()).not.toMatch(/\bv\d+\b/)
    expect(q('source').text()).toBe('Düzenlendi')
  })

  it('a model text keeps "Model metni"', async () => {
    await openMessages()
    expect(q('source').text()).toBe('Model metni')
  })

  it('tells App when there is unsaved work (an open edit), and when there is none', async () => {
    await openMessages()
    const last = () => w!.emitted<[boolean]>('dirty')!.at(-1)![0]
    expect(last()).toBe(false)
    await q('edit').trigger('click')
    expect(last()).toBe(true)
    await w!.get('#sms').setValue('Merhaba yeni')
    expect(last()).toBe(true)
    await q('edit-cancel').trigger('click')
    expect(last()).toBe(false)
  })

  it('Vazgeç puts the stored text back and saves nothing', async () => {
    await openMessages()
    await editTo('başka bir şey')
    await q('edit-cancel').trigger('click')
    expect(sms().value).toBe('Merhaba')
    expect(sms().readOnly).toBe(true)
    expect(q('unsaved').exists()).toBe(false)
    expect(save).not.toHaveBeenCalled()
    expect(approveButton().attributes('disabled')).toBeUndefined()
  })

  it('a 409 (approved meanwhile) says so, takes the server state and locks the pencil', async () => {
    await openMessages()
    await editTo('Merhaba yeni')
    save.mockRejectedValue(new ApiError(409, 'invalid_state', 'kampanya "approved" durumunda; içerik düzenlenemez'))
    get.mockResolvedValue({ ...ready, status: 'approved' })
    await q('edit-save').trigger('click')
    await flushPromises()

    expect(get).toHaveBeenCalledTimes(2) // opening the campaign, then again after the refusal
    expect(q('save-error').text()).toBe(LOCKED_MESSAGE)
    expect(translate).not.toHaveBeenCalled()
    expect(sms().value).toBe('Merhaba')
    expect(sms().readOnly).toBe(true)
    expect(q('edit-save').exists()).toBe(false)
    const pencil = q('edit')
    expect(pencil.attributes('aria-disabled')).toBe('true')
    expect(pencil.attributes('title')).toBe('Onaylandı')
  })

  it('a rejected text (422) stays open for fixing', async () => {
    await openMessages()
    await editTo('Merhaba yeni')
    save.mockRejectedValue(new ApiError(422, 'invalid_content', 'içerik karakter sınırlarına veya kurallara uymuyor', { report: { ok: false, issues: [], sms_length: 0, subject_length: 0 } }))
    await q('edit-save').trigger('click')
    await flushPromises()
    expect(sms().readOnly).toBe(false)
    expect(q('edit-save').exists()).toBe(true)
    expect(translate).not.toHaveBeenCalled()
  })
})
