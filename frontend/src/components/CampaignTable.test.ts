// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { CampaignSummary } from '../types'
import CampaignTable from './CampaignTable.vue'
import { shortWhen } from '../utils/schedule'

const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })
const LONG =
  'son 30 günde sepetine ürün ekleyip alışveriş yapmayan istanbul’daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun.'

const draft: CampaignSummary = { id: 1, brief: 'tüm müşterilere yaz', status: 'segment_ready', segment_status: 'ok', matched: 300, created_at: '2026-11-16T10:00:00Z', explanation: 'Tüm müşteriler', audience_languages: { tr: 180, en: 75, de: 45 } }
const sending: CampaignSummary = {
  id: 2,
  brief: LONG,
  status: 'sending',
  segment_status: 'ok',
  matched: 300,
  created_at: '2026-11-16T10:00:00Z',
  delivery: {
    sms: ch({ total: 10, sent: 6, failed: 1, pending: 3 }),
    email: ch({ total: 10, sent: 10 }),
    done: false,
    languages: { tr: { total: 12, sent: 12 }, en: { total: 5, sent: 3 }, de: { total: 3, sent: 1 } },
  },
}
const scheduled: CampaignSummary = { id: 3, brief: 'zamanlı kampanya', status: 'scheduled', segment_status: 'ok', matched: 50, created_at: '2026-11-16T10:00:00Z', scheduled_at: '2026-12-01T09:00:00Z' }
const failed: CampaignSummary = { id: 4, brief: 'bozuk', status: 'failed', created_at: '2026-11-16T10:00:00Z', scheduled_at: '2026-11-17T09:00:00Z', schedule_error: 'teklif sona erdi' }

const make = (campaigns: CampaignSummary[]) => mount(CampaignTable, { props: { campaigns } })

describe('CampaignTable', () => {
  it('lists the campaigns as rows of a table with named columns', () => {
    const w = make([draft, sending, scheduled])
    expect(w.findAll('thead th').map((h) => h.text())).toEqual(['Kampanya', 'Durum', 'Kitle', 'Diller', 'İlerleme', 'Zaman', 'İşlemler'])
    expect(w.findAll('tbody tr')).toHaveLength(3)
    expect(w.find('[data-testid="row-2"]').exists()).toBe(true)
  })

  it('titles a row with the brief: one line, capital first letter, ended with … and the whole text on hover', () => {
    const w = make([sending])
    const link = w.get('a.name')
    expect(link.text().startsWith('Son 30 günde')).toBe(true)
    expect(link.text().endsWith('…')).toBe(true)
    expect(link.text().length).toBeLessThanOrEqual(65)
    expect(link.text()).not.toContain('\n')
    expect(link.attributes('title')).toBe(LONG)
  })

  it('leaves a short brief whole', () => {
    const w = make([draft])
    expect(w.get('a.name').text()).toBe('Tüm müşterilere yaz')
    expect(w.get('a.name').attributes('title')).toBe('tüm müşterilere yaz') // the tooltip is the brief exactly as written
  })

  it('links the title to the wizard of that campaign', () => {
    expect(make([scheduled]).get('a.name').attributes('href')).toBe('/?c=3')
  })

  it('says who the audience is under the title and how many people', () => {
    const w = make([draft])
    expect(w.get('[data-testid="card-audience"]').text()).toBe('Tüm müşteriler')
    expect(w.get('tbody tr').findAll('td')[2].text()).toBe('300')
  })

  it('shows the status as a badge; a scheduled one says when ("Zamanlandı · 1 Ara 12:00")', () => {
    const w = make([draft, sending, scheduled, failed])
    const when = shortWhen(scheduled.scheduled_at!)
    expect(w.findAll('tbody tr').map((r) => r.findAll('td')[1].text())).toEqual(['Taslak', 'Gönderiliyor', `Zamanlandı · ${when}`, 'Başlatılamadı'])
    expect(when).toMatch(/^\d{1,2} Ara \d{2}:\d{2}$/)
  })

  it('lets a scheduled campaign be cancelled from its row, and only that one', async () => {
    const w = make([draft, sending, scheduled, failed])
    const buttons = w.findAll('[data-testid="cancel"]')
    expect(buttons).toHaveLength(1)
    await buttons[0].trigger('click')
    expect(w.emitted('cancel')?.[0]?.[0]).toMatchObject({ id: 3 })
  })

  it('shows languages as separate cells: the audience before sending, what was sent once it began', () => {
    const w = make([draft, sending])
    const cells = (row: number) => w.findAll('tbody tr')[row].findAll('.lang').map((l) => l.text())
    expect(cells(0)).toEqual(['TR180', 'EN75', 'DE45'])
    expect(cells(1)).toEqual(['TR12', 'EN3/5', 'DE1/3'])
  })

  it('shows progress as a bar and one line of numbers, with the details in a tooltip', () => {
    const w = make([sending])
    const bar = w.get('[role="progressbar"]')
    expect(bar.attributes('aria-valuenow')).toBe('85') // 16 sent and 1 failed of 20 are handled
    expect(w.get('[data-testid="sent"]').text()).toBe('16')
    expect(w.get('[data-testid="failed"]').text()).toContain('1 başarısız')
    expect(bar.attributes('title')).toContain('16 gönderildi')
  })

  it('shows a dash where there is no progress yet, and the reason where it could not start', () => {
    const w = make([draft, failed])
    expect(w.findAll('tbody tr')[0].get('.progress').text()).toBe('–')
    expect(w.findAll('tbody tr')[1].get('.progress').text()).toBe('teklif sona erdi')
  })

  it('shows the planned time of a scheduled campaign, and keeps it after it started', () => {
    const w = make([scheduled, { ...sending, scheduled_at: '2026-11-16T09:00:00Z' }])
    expect(w.findAll('[data-testid="card-planned"]')[0].text()).toMatch(/01\.12\.2026|1\.12\.2026/)
    expect(w.findAll('[data-testid="card-planned"]')[1].text()).toContain('zamanlanmıştı')
  })

  it('offers cancelling only for a scheduled campaign, and asks the page to do it', async () => {
    const w = make([draft, sending, scheduled])
    const buttons = w.findAll('[data-testid="cancel"]')
    expect(buttons).toHaveLength(1)
    await buttons[0].trigger('click')
    expect(w.emitted('cancel')).toEqual([[scheduled]])
  })
})
