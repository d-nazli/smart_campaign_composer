// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { CampaignSummary } from '../types'

const list = vi.fn()
const cancel = vi.fn()
vi.mock('../api/campaigns', () => ({ listCampaigns: (...a: unknown[]) => list(...a), cancelCampaign: (...a: unknown[]) => cancel(...a) }))
vi.mock('../api/system', () => ({ fetchSystem: () => Promise.resolve({ api: 'ok', database: 'ok', worker: 'ok' }) }))

import DashboardView from './DashboardView.vue'

const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })
const scheduled: CampaignSummary = { id: 7, brief: 'anneler gününe özel indirim', status: 'scheduled', segment_status: 'ok', matched: 40, created_at: '2026-11-16T10:00:00Z', scheduled_at: '2026-12-01T09:00:00Z' }
const sending: CampaignSummary = {
  id: 8,
  brief: 'tüm müşteriler',
  status: 'sending',
  segment_status: 'ok',
  matched: 100,
  created_at: '2026-11-16T10:00:00Z',
  delivery: { sms: ch({ total: 10, sent: 4, failed: 1, pending: 5 }), email: ch({ total: 10, sent: 6 }), done: false },
}

let w: VueWrapper | null = null
const dialog = () => document.body.querySelector('[role="dialog"]') as HTMLElement | null
const byTestId = (id: string) => document.body.querySelector(`[data-testid="${id}"]`) as HTMLElement | null

beforeEach(() => {
  list.mockReset()
  cancel.mockReset()
  list.mockResolvedValue([scheduled, sending])
  cancel.mockResolvedValue({})
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
})

async function mountIt() {
  w = mount(DashboardView, { props: { info: null }, attachTo: document.body })
  await flushPromises()
  return w
}

describe('DashboardView', () => {
  it('lists the campaigns as a table', async () => {
    const v = await mountIt()
    expect(v.findAll('tbody tr')).toHaveLength(2)
    expect(v.get('a.name').text()).toBe('Anneler gününe özel indirim')
  })

  it('has no summary tiles and no campaign count above the list', async () => {
    const v = await mountIt()
    for (const id of ['summary', 'total-sent', 'total-failed', 'total-pending', 'rate', 'active']) expect(v.find(`[data-testid="${id}"]`).exists()).toBe(false)
    expect(v.text()).not.toContain('aktif gönderim')
  })

  it('shows the speed of a sending campaign per channel, measured between refreshes', async () => {
    vi.useFakeTimers()
    try {
      const at = (sms: number, email: number): CampaignSummary => ({ ...sending, delivery: { sms: ch({ total: 1000, sent: sms, pending: 1000 - sms }), email: ch({ total: 1000, sent: email, pending: 1000 - email }), done: false } })
      list.mockResolvedValue([at(0, 0)])
      const v = await mountIt()
      expect(v.find('[data-testid="speed"]').exists()).toBe(false) // one reading is no speed
      list.mockResolvedValue([at(200, 190)])
      await vi.advanceTimersByTimeAsync(2000)
      await flushPromises()
      expect(v.get('[data-testid="speed"]').text()).toBe('SMS 100/sn · E-posta 95/sn')
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows the average speed and duration of a finished campaign', async () => {
    const done: CampaignSummary = {
      ...sending,
      status: 'completed',
      delivery: {
        sms: ch({ total: 100, sent: 100, first_sent_at: '2026-11-18T12:00:00Z', last_sent_at: '2026-11-18T12:00:25Z' }),
        email: ch({ total: 100, sent: 100, first_sent_at: '2026-11-18T12:00:01Z', last_sent_at: '2026-11-18T12:00:27Z' }),
        done: true,
      },
    }
    list.mockResolvedValue([done])
    const v = await mountIt()
    expect(v.get('[data-testid="speed"]').text()).toBe("ort. 4/sn, 27 sn'de bitti")
  })

  it('shows an empty state with a way to start when there are no campaigns', async () => {
    list.mockResolvedValue([])
    const v = await mountIt()
    expect(v.get('[data-testid="empty"]').text()).toContain('Henüz kampanya yok')
    expect(v.find('table').exists()).toBe(false)
  })

  it('asks in a dialog before cancelling, and names the campaign and its time', async () => {
    const v = await mountIt()
    expect(dialog()).toBeNull()
    await v.get('[data-testid="cancel"]').trigger('click')
    expect(dialog()).not.toBeNull()
    expect(dialog()!.textContent).toContain('Zamanlanmış gönderim iptal edilsin mi?')
    expect(dialog()!.textContent).toContain('Anneler gününe özel indirim')
    expect(cancel).not.toHaveBeenCalled()
  })

  it('does nothing when the dialog is dismissed', async () => {
    const v = await mountIt()
    await v.get('[data-testid="cancel"]').trigger('click')
    ;(Array.from(dialog()!.querySelectorAll('button')).find((b) => b.textContent === 'Vazgeç') as HTMLElement).click()
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(cancel).not.toHaveBeenCalled()
  })

  it('cancels the campaign once it is confirmed, then refreshes the list', async () => {
    const v = await mountIt()
    await v.get('[data-testid="cancel"]').trigger('click')
    list.mockResolvedValue([{ ...scheduled, status: 'cancelled' }, sending])
    byTestId('confirm-cancel')!.click()
    await flushPromises()
    expect(cancel).toHaveBeenCalledWith(7)
    expect(dialog()).toBeNull()
    expect(v.findAll('tbody tr')[0].text()).toContain('İptal edildi')
    expect(v.find('[data-testid="cancel"]').exists()).toBe(false)
  })

  it('shows the server’s refusal when the campaign has already started, and closes the dialog', async () => {
    const v = await mountIt()
    cancel.mockRejectedValue(new Error('gönderim başladığı için iptal edilemez'))
    await v.get('[data-testid="cancel"]').trigger('click')
    byTestId('confirm-cancel')!.click()
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(v.get('[data-testid="cancel-error"]').text()).toContain('gönderim başladığı için iptal edilemez')
  })
})
