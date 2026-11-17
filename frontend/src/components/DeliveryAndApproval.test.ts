// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
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
    languages: { tr: { total: 30, sent: 25 }, en: { total: 12, sent: 10 }, de: { total: 8, sent: 5 } },
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

  it('shows sent per language, sent/total while some wait', () => {
    const t = make().get('[data-testid="delivery-languages"]').text()
    expect(t).toContain('TR 25/30')
    expect(t).toContain('EN 10/12')
    expect(t).toContain('DE 5/8')
    expect(make({ ...delivery, done: true }, 'completed').get('[data-testid="delivery-languages"]').text()).toContain('TR 25')
  })

  it('explains the failures in words', () => {
    expect(make().get('.reasons').text()).toContain('2')
  })

  it('says when it is finished', () => {
    expect(make().text()).not.toContain('Gönderim tamamlandı')
    expect(make(delivery, 'completed').text()).toContain('Gönderim tamamlandı')
  })

  it('reads as complete when there is nothing to send', () => {
    const w = make({ sms: ch(), email: ch(), done: true }, 'completed')
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('100')
    expect(w.find('[data-testid="delivery-languages"]').exists()).toBe(false)
  })
})

describe('ApprovalCard', () => {
  const segment: Segment = {
    status: 'ok',
    conditions: [],
    explanation: 'İstanbul’daki müşteriler',
    audience: { matched: 300, sms_reachable: 200, email_reachable: 220, both_reachable: 150 },
    offer: { kind: 'percent', value: 30, code: '', expires_at: '2099-12-31' },
    languages: { tr: 180, en: 75, de: 45 },
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
    expect(w.get('[data-testid="approval-languages"]').text()).toContain('Türkçe')
    expect(w.get('[data-testid="approval-languages"]').text()).toContain('180')
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

  it('asks "Emin misiniz?" in a sentence, briefly, differently for now and later', () => {
    const now = make({ confirming: true }).get('[role="alertdialog"]').text()
    expect(now).toContain('Emin misiniz?')
    expect(now.length).toBeLessThan(110)
    const later = make({ confirming: true, mode: 'later', when: '2099-12-30T10:00' }).get('[role="alertdialog"]').text()
    expect(later).toContain('iptal edebilirsiniz')
    expect(later.length).toBeLessThan(140)
  })

  it('cannot be changed after approval', () => {
    const w = make({ frozen: true, mode: 'later' })
    expect(w.find('[role="radiogroup"]').exists()).toBe(false)
    expect(w.text()).toContain('Onaylandı')
  })

  it('lists the warnings with their language', () => {
    const w = make({ warnings: [{ field: 'sms_body', code: 'offer_missing', message: 'Teklif geçmiyor.', language: 'en' }] })
    const li = w.get('.warnings li')
    expect(li.text()).toContain('EN')
    expect(li.text()).toContain('Teklif geçmiyor.')
  })
})
