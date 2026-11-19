// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { CampaignSummary } from '../types'

const list = vi.fn()
const cancel = vi.fn()
const remove = vi.fn()
vi.mock('../api/campaigns', () => ({
  listCampaigns: (...a: unknown[]) => list(...a),
  cancelCampaign: (...a: unknown[]) => cancel(...a),
  deleteCampaign: (...a: unknown[]) => remove(...a),
}))
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
  remove.mockReset()
  remove.mockResolvedValue(undefined)
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

  it('shows only the bar and "sent / total · failed" under İlerleme, never a speed line', async () => {
    const done: CampaignSummary = {
      ...sending,
      id: 9,
      status: 'completed',
      delivery: {
        sms: ch({ total: 430, sent: 425, failed: 5, first_sent_at: '2026-11-18T12:00:00Z', last_sent_at: '2026-11-18T12:00:17Z' }),
        email: ch({ total: 425, sent: 423, failed: 2, first_sent_at: '2026-11-18T12:00:00Z', last_sent_at: '2026-11-18T12:00:16Z' }),
        done: true,
      },
    }
    list.mockResolvedValue([sending, done])
    const v = await mountIt()
    for (const row of v.findAll('tbody tr')) {
      const cell = row.get('td.progress')
      expect(cell.find('[role="progressbar"]').exists()).toBe(true)
      expect(cell.find('[data-testid="speed"]').exists()).toBe(false)
      expect(cell.text()).not.toMatch(/\/sn|bitti|ort\./)
    }
    const doneCell = v.findAll('td.progress').find((c) => c.text().includes('848'))!
    expect(doneCell.text().replace(/\s+/g, ' ')).toBe('848 / 855 · 7 başarısız')
  })

  it('deletes a campaign left waiting for an answer, after asking, and refreshes the list', async () => {
    const waiting: CampaignSummary = { id: 11, brief: 'kış ürünleri', name: 'Ankara kış ürünleri · 200 TL', status: 'draft', segment_status: 'needs_clarification', created_at: '2026-11-16T10:00:00Z' }
    list.mockResolvedValue([waiting, scheduled])
    const v = await mountIt()
    await v.get('[data-testid="delete"]').trigger('click')
    await flushPromises()
    expect(dialog()?.textContent).toContain('Kampanya silinsin mi?')
    expect(dialog()?.textContent).toContain('Ankara kış ürünleri · 200 TL')
    expect(remove).not.toHaveBeenCalled()
    list.mockResolvedValue([scheduled])
    byTestId('confirm-delete')!.click()
    await flushPromises()
    expect(remove).toHaveBeenCalledWith(11)
    expect(v.findAll('tbody tr')).toHaveLength(1)
    expect(dialog()).toBeNull()
  })

  it('says so when the server refuses the deletion', async () => {
    const waiting: CampaignSummary = { id: 12, brief: 'x', status: 'draft', segment_status: 'needs_clarification', created_at: '2026-11-16T10:00:00Z' }
    list.mockResolvedValue([waiting])
    remove.mockRejectedValue(new Error('kampanya "approved" durumunda; yalnızca onaylanmamış kampanya silinebilir'))
    const v = await mountIt()
    await v.get('[data-testid="delete"]').trigger('click')
    await flushPromises()
    byTestId('confirm-delete')!.click()
    await flushPromises()
    expect(v.get('[data-testid="delete-error"]').text()).toContain('Kampanya silinemedi')
  })

  it('filters the list: Tümü / Aktif / Taslak, with "Desteklenmiyor" hidden until asked for', async () => {
    const at = '2026-11-16T10:00:00Z'
    const unsupported: CampaignSummary = { id: 21, brief: 'Kasım indirimi', name: 'Kasım indirimi · %15', status: 'draft', segment_status: 'unsupported', created_at: at }
    const ready: CampaignSummary = { id: 22, brief: 'Kasım indirimi', name: 'Kasım indirimi · %15', status: 'content_ready', segment_status: 'ok', matched: 10, created_at: at }
    const done: CampaignSummary = { id: 23, brief: 'Kasım indirimi', name: 'Kasım indirimi · %15', status: 'completed', segment_status: 'ok', matched: 10, created_at: at }
    list.mockResolvedValue([unsupported, ready, done, scheduled])
    const v = await mountIt()
    const rowIds = () => v.findAll('tbody tr a.name').map((a) => a.attributes('href'))

    // Default: everything but the "Desteklenmiyor" try; the button says how many are hidden.
    expect(rowIds()).toEqual(['/?c=22', '/?c=23', '/?c=7'])
    expect(v.get('[data-testid="filter-all"]').text()).toBe('Tümü · 3')
    expect(v.get('[data-testid="toggle-unsupported"]').text()).toBe('Desteklenmeyenleri göster (1)')

    await v.get('[data-testid="filter-active"]').trigger('click')
    expect(rowIds()).toEqual(['/?c=7']) // scheduled
    await v.get('[data-testid="filter-draft"]').trigger('click')
    expect(rowIds()).toEqual(['/?c=22']) // a draft with texts stays in the list

    await v.get('[data-testid="toggle-unsupported"]').trigger('click')
    expect(rowIds()).toEqual(['/?c=21', '/?c=22'])
    expect(v.get('[data-testid="toggle-unsupported"]').text()).toBe('Desteklenmeyenleri gizle')
    expect(v.findAll('[data-testid="delete"]')).toHaveLength(2) // the delete icon stays

    await v.get('[data-testid="filter-active"]').trigger('click')
    list.mockResolvedValue([unsupported])
    await v.get('[data-testid="toggle-unsupported"]').trigger('click')
    expect(v.find('table').exists()).toBe(true)
  })

  it('says so when a filter has nothing to show', async () => {
    list.mockResolvedValue([{ id: 31, brief: 'x', status: 'draft', segment_status: 'unsupported', created_at: '2026-11-16T10:00:00Z' } as CampaignSummary])
    const v = await mountIt()
    expect(v.find('table').exists()).toBe(false)
    expect(v.get('[data-testid="filter-empty"]').text()).toBe('Bu filtrede kampanya yok.')
    await v.get('[data-testid="toggle-unsupported"]').trigger('click')
    expect(v.findAll('tbody tr')).toHaveLength(1)
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
