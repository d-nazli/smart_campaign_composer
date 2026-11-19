// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { DeliveryProgress as Delivery, Segment } from '../types'
import ApprovalCard from './ApprovalCard.vue'
import DeliveryProgress from './DeliveryProgress.vue'
import DateTimeField from './ui/DateTimeField.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })

describe('DeliveryProgress', () => {
  const delivery: Delivery = {
    sms: ch({ total: 30, sent: 20, failed: 2, pending: 8 }),
    email: ch({ total: 20, sent: 20 }),
    done: false,
    failures: [{ channel: 'sms', reason: 'rejected', count: 2 }],
    locales: { 'tr-TR': { total: 30, sent: 25 }, 'en-GB': { total: 12, sent: 10 }, 'ar-SA': { total: 8, sent: 5 } },
  }
  const make = (d = delivery, status: 'sending' | 'completed' = 'sending') => mount(DeliveryProgress, { props: { delivery: d, status } })

  it('says in one line how far it has come, and shows one bar', () => {
    const w = make()
    expect(w.findAll('[role="progressbar"]')).toHaveLength(1)
    expect(w.get('[data-testid="delivery-caption"]').text()).toContain('42 / 50 mesaj işlendi')
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('84')
  })

  it('shows each channel as one figure group, each figure once', () => {
    const w = make()
    const figures = (id: string) => {
      const g = w.get(`[data-testid="${id}"]`)
      return [g.get('.ui-stat__value .sr-only').text(), ...g.findAll('.leader b').map((c) => c.text())]
    }
    const sms = figures('delivery-sms')
    const email = figures('delivery-email')
    expect(sms).toEqual(['20', '2', '0', '8'])
    expect(email).toEqual(['20', '0', '0', '0'])
    // the old layout repeated these numbers as tiles above the bars; there is nothing like that now
    expect(w.find('.totals').exists()).toBe(false)
  })

  const localeRows = (w: ReturnType<typeof make>) =>
    w.findAll('[data-testid="delivery-locales"] li').map((li) => ({
      code: li.get('.loc__code').text(),
      name: li.get('.loc__name').text(),
      n: li.get('.loc__n').text(),
      width: (li.get('.loc__bar i').element as HTMLElement).style.width,
    }))
  const locales = [
    { code: 'tr-TR', name: 'Turkish (Türkiye)', native_name: 'Türkçe (Türkiye)', direction: 'ltr' as const, is_source: true },
    { code: 'en-GB', name: 'English (United Kingdom)', native_name: 'English (UK)', direction: 'ltr' as const, is_source: false },
    { code: 'ar-SA', name: 'Arabic (Saudi Arabia)', native_name: 'العربية (السعودية)', direction: 'rtl' as const, is_source: false },
  ]

  it('lists the locales largest first: code, language with a bar, sent/total while some wait', () => {
    const w = mount(DeliveryProgress, { props: { delivery, status: 'sending', locales } })
    expect(w.find('[data-testid="delivery-locales"] .ui-badge').exists()).toBe(false) // no chips
    expect(localeRows(w)).toEqual([
      { code: 'tr-TR', name: 'Türkçe (Türkiye)', n: '25/30', width: '83.3%' },
      { code: 'en-GB', name: 'English (UK)', n: '10/12', width: '33.3%' },
      { code: 'ar-SA', name: 'العربية (السعودية)', n: '5/8', width: '16.7%' },
    ])
  })

  it('once finished, sorts by what was sent; the largest locale fills its bar', () => {
    const d = { ...delivery, done: true, locales: { 'tr-TR': { total: 30, sent: 9 }, 'en-GB': { total: 12, sent: 12 }, 'ar-SA': { total: 1500, sent: 1490 } } }
    const rows = localeRows(mount(DeliveryProgress, { props: { delivery: d, status: 'completed', locales } }))
    expect(rows.map((r) => [r.code, r.n])).toEqual([
      ['ar-SA', '1.490'],
      ['en-GB', '12'],
      ['tr-TR', '9'],
    ])
    expect(rows[0].width).toBe('100%')
  })

  it('explains the failures in words', () => {
    expect(make().get('.reasons').text()).toContain('2')
  })

  const outcomeOf = (sent: number, total: number, failures: { reason: string; count: number }[]): Delivery => ({
    sms: ch({ total, sent, failed: failures.reduce((n, f) => n + f.count, 0) }),
    email: ch(),
    done: true,
    failures: failures.map((f) => ({ channel: 'sms' as const, ...f })),
  })
  const len = (el: { attributes: (n: string) => string | undefined }) => parseFloat(el.attributes('stroke-dasharray')!)
  const HALF = Math.PI * 70

  it('shows the outcome of all messages: delivered in blue, each failure reason in its own red', () => {
    const w = make(outcomeOf(7264, 7296, [{ reason: 'gave_up', count: 20 }, { reason: 'rejected', count: 12 }]), 'completed')
    expect(w.get('[data-testid="outcome"] h3').text()).toBe('Gönderim sonucu')
    expect(w.get('[data-testid="outcome-total"]').text()).toBe('7.264 / 7.296')
    expect(w.get('[data-testid="outcome-share"]').text()).toBe('başarılı · %0,44 başarısız')
    const arcs = w.findAll('path.arc')
    expect(arcs).toHaveLength(3)
    expect(arcs[0].attributes('stroke')).toBe('var(--chart-1)')
    expect(arcs[1].attributes('stroke')).toBe('var(--error)')
    expect(arcs[2].attributes('stroke')).not.toBe(arcs[1].attributes('stroke'))
    expect(arcs.slice(1).every((a) => a.attributes('stroke')!.includes('--error'))).toBe(true)
    expect(len(arcs[1])).toBeGreaterThan(0) // a tiny share is still visible
    expect(len(arcs[0]) + len(arcs[1]) + len(arcs[2])).toBeLessThanOrEqual(HALF + 0.001) // never past the half ring
    // every colour has a labelled row with its count and share
    expect(w.get('[data-testid="outcome-total"]').text()).toContain('7.264')
    expect(w.get('li[data-testid="outcome-sent"]').text()).toContain('Başarılı')
    expect(w.get('li[data-testid="outcome-sent"]').text()).toContain('%99,56 (7.264)')
    expect(w.get('li[data-testid="outcome-gave_up"]').text()).toContain('%0,27 (20)')
    expect(w.get('li[data-testid="outcome-rejected"]').text()).toContain('%0,16 (12)')
  })

  it('puts the numbers under the half ring, never over its arc', () => {
    const w = make(outcomeOf(3465, 3484, [{ reason: 'rejected', count: 19 }]), 'completed')
    const fig = w.get('[data-testid="outcome"] figure')
    expect([...fig.element.children].map((c) => c.tagName.toLowerCase())).toEqual(['svg', 'figcaption'])
    const src = Object.values(import.meta.glob('./ui/HalfDonut.vue', { query: '?raw', import: 'default', eager: true }))[0] as string
    const rule = src.slice(src.indexOf('figcaption {'), src.indexOf('}', src.indexOf('figcaption {')))
    expect(rule).not.toMatch(/position:\s*absolute/)
    expect(rule).toMatch(/margin-top/)
  })

  it('is a full blue ring when nothing failed, and is shown then too', () => {
    const w = make(outcomeOf(500, 500, []), 'completed')
    const arcs = w.findAll('path.arc')
    expect(arcs).toHaveLength(1)
    expect(len(arcs[0])).toBeCloseTo(HALF, 3)
    expect(w.get('[data-testid="outcome-share"]').text()).toBe('başarılı · %0 başarısız')
  })

  it('leaves what is still waiting empty', () => {
    const d: Delivery = { sms: ch({ total: 100, sent: 40, pending: 60 }), email: ch(), done: false }
    expect(len(make(d).get('path.arc'))).toBeCloseTo(0.4 * HALF, 1)
  })

  it('shows the speed per channel: busiest second, limit, time, retries and 429s once finished', () => {
    const d: Delivery = {
      sms: ch({ total: 2500, sent: 2470, retries: 40, rate_limited: 9, first_sent_at: '2026-11-18T12:00:00Z', last_sent_at: '2026-11-18T12:00:26Z' }),
      email: ch({ total: 100, sent: 100, first_sent_at: '2026-11-18T12:00:00Z', last_sent_at: '2026-11-18T12:01:05Z' }),
      done: true,
    }
    const w = make(d, 'completed')
    const sms = w.get('[data-testid="speed-sms"]')
    expect(sms.find('[data-testid="speed-avg"]').exists()).toBe(false) // no average: it hides the retry waits
    expect(sms.get('[data-testid="speed-limit"]').text()).toBe('100/sn')
    expect(sms.get('[data-testid="speed-time"]').text()).toBe('26 sn')
    expect(sms.text()).toContain('40')
    expect(sms.text()).toContain('9')
    expect(w.get('[data-testid="speed-email"] [data-testid="speed-time"]').text()).toBe('1 dk 5 sn')
    expect(w.find('[data-testid="speed-live"]').exists()).toBe(false) // finished: no live column
  })

  it('measures the live speed of each channel between two polls', async () => {
    vi.useFakeTimers()
    try {
      const at = (sms: number, email: number): Delivery => ({ sms: ch({ total: 1000, sent: sms, pending: 1000 - sms }), email: ch({ total: 1000, sent: email, pending: 1000 - email }), done: false })
      const w = make(at(100, 100))
      expect(w.get('[data-testid="speed-sms"] [data-testid="speed-live"]').text()).toBe('–')
      vi.advanceTimersByTime(1000)
      await w.setProps({ delivery: at(198, 195) })
      expect(w.get('[data-testid="speed-sms"] [data-testid="speed-live"]').text()).toBe('98/sn')
      expect(w.get('[data-testid="speed-email"] [data-testid="speed-live"]').text()).toBe('95/sn')
    } finally {
      vi.useRealTimers()
    }
  })

  it('has no speed row for a channel nobody receives', () => {
    const d: Delivery = { sms: ch({ total: 10, sent: 10 }), email: ch(), done: true }
    const w = make(d, 'completed')
    expect(w.find('[data-testid="speed-sms"]').exists()).toBe(true)
    expect(w.find('[data-testid="speed-email"]').exists()).toBe(false)
  })

  it('says when it is finished', () => {
    expect(make().text()).not.toContain('Gönderim tamamlandı')
    expect(make(delivery, 'completed').text()).toContain('Gönderim tamamlandı')
  })

  it('reads as complete when there is nothing to send', () => {
    const w = make({ sms: ch(), email: ch(), done: true }, 'completed')
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('100')
    expect(w.find('[data-testid="delivery-locales"]').exists()).toBe(false)
  })
})

describe('ApprovalCard', () => {
  const segment: Segment = {
    status: 'ok',
    conditions: [],
    explanation: 'İstanbul’daki müşteriler',
    audience: { matched: 300, sms_reachable: 200, email_reachable: 220, both_reachable: 150 },
    offer: { kind: 'percent', value: 30, code: '', expires_at: '2099-12-31' },
    locales: { 'tr-TR': 180, 'en-GB': 75, 'de-DE': 45 },
  }
  const props = { segment, warnings: [], confirming: false, frozen: false, error: null, mode: 'now' as const, when: '', scheduleError: null }
  const make = (extra: Record<string, unknown> = {}) => mount(ApprovalCard, { props: { ...props, ...extra }, attachTo: document.body })

  it('summarises the offer, the audience and the people it reaches', () => {
    const w = make()
    const text = w.text()
    expect(text).toContain('%30 indirim')
    expect(text).toContain('İstanbul’daki müşteriler')
    expect(text).toContain('270') // 200 + 220 - 150 people
    expect(text).toContain('SMS')
    expect(text).toContain('200')
    expect(w.get('[data-testid="approval-locales"]').text()).toContain('tr-TR')
    expect(w.get('[data-testid="approval-locales"]').text()).toContain('180')
  })

  it('chooses between now and later with a toggle, not with bare radio buttons', async () => {
    const w = make()
    expect(w.find('input[type="radio"]').exists()).toBe(false)
    const group = w.get('[role="radiogroup"]')
    expect(group.findAll('[role="radio"]').map((r) => r.text())).toEqual(['Hemen', 'Zamanla'])
    expect(w.get('[data-testid="mode-now"]').attributes('aria-checked')).toBe('true')
    await w.get('[data-testid="mode-later"]').trigger('click')
    expect(w.emitted('update:mode')).toEqual([['later']])
  })

  it('shows no picker and no estimate note when sending now', () => {
    const w = make()
    expect(w.find('[data-testid="send-at"]').exists()).toBe(false)
    expect(w.find('[data-testid="later-note"]').exists()).toBe(false)
  })

  it('shows the picker and marks the numbers as estimates when sending later (a badge, no sentence)', () => {
    const w = make({ mode: 'later', when: '2099-12-30T10:00' })
    expect(w.get('[data-testid="send-at"]').text()).toContain('30.12.2099 10:00')
    expect(w.text()).toContain('31.12.2099') // the offer's last day, in the offer line
    expect(w.get('[data-testid="later-note"]').text()).toBe('Tahmini sayılar')
    expect(w.text()).not.toContain('Kitle gönderim anında yeniden belirlenir')
    expect(w.find('input[type="datetime-local"]').exists()).toBe(false)
  })

  it('offers no day in the past and none more than 30 days ahead', async () => {
    const w = make({ mode: 'later', when: '' })
    const picker = w.getComponent(DateTimeField)
    const min = new Date(picker.props('min') as string)
    const max = new Date(picker.props('max') as string)
    const now = Date.now()
    expect(min.getTime()).toBeGreaterThan(now - 60_000)
    expect(max.getTime() - now).toBeLessThanOrEqual(30 * 24 * 60 * 60 * 1000)
    expect(max.getTime() - now).toBeGreaterThan(29 * 24 * 60 * 60 * 1000)
    await w.get('[data-testid="send-at"]').trigger('click')
    const enabled = w.findAll('.ui-dt__day:not([disabled])').map((d) => d.attributes('aria-label'))
    const yesterday = new Date(now - 24 * 60 * 60 * 1000).toLocaleDateString('tr-TR', { day: 'numeric', month: 'long', year: 'numeric', weekday: 'long' })
    expect(enabled).not.toContain(yesterday)
  })

  it('says why a time cannot be used', () => {
    const w = make({ mode: 'later', when: '', scheduleError: 'Bir gönderim zamanı seçin.' })
    expect(w.get('[data-testid="schedule-error"]').text()).toBe('Bir gönderim zamanı seçin.')
    expect(w.get('[data-testid="send-at"]').classes()).toContain('is-invalid')
  })

  it('leaves "Emin misiniz?" to the alert dialog and holds its controls while it is open', () => {
    const w = make({ confirming: true })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.text()).not.toContain('Emin misiniz?')
    expect(w.get('[role="radiogroup"]').findAll('[role="radio"]').every((r) => r.attributes('disabled') !== undefined || r.attributes('aria-disabled') === 'true')).toBe(true)
  })

  it('cannot be changed after approval', () => {
    const w = make({ frozen: true, mode: 'later' })
    expect(w.find('[role="radiogroup"]').exists()).toBe(false)
    expect(w.text()).toContain('Onaylandı')
  })

  it('lists the warnings with their locale', () => {
    const w = make({ warnings: [{ field: 'sms_body', code: 'offer_missing', message: 'Teklif geçmiyor.', locale: 'tr-TR' }] })
    const li = w.get('.warnings li')
    expect(li.text()).toContain('tr-TR')
    expect(li.text()).toContain('Teklif geçmiyor.')
  })
})
