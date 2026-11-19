import { describe, expect, it } from 'vitest'
import type { CampaignSummary, DeliveryProgress } from '../types'
import { briefPreview, deliveryTotals, filterCampaigns, isUnsupported, pageFromHash, statusLabel } from './dashboard'

const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })
const row = (o: Partial<CampaignSummary> = {}): CampaignSummary => ({
  id: 1,
  brief: 'b',
  status: 'draft',
  created_at: '2026-01-01T00:00:00Z',
  ...o,
})

describe('pageFromHash', () => {
  it('opens the dashboard for #/dashboard', () => {
    expect(pageFromHash('#/dashboard')).toBe('dashboard')
  })
  it('opens the composer for anything else, so a stray hash never shows a blank page', () => {
    expect(pageFromHash('')).toBe('compose')
    expect(pageFromHash('#/nope')).toBe('compose')
    expect(pageFromHash('#/dashboardx')).toBe('compose')
  })
})

describe('statusLabel', () => {
  it('names the sending states', () => {
    expect(statusLabel(row({ status: 'approved' }))).toBe('Onaylandı')
    expect(statusLabel(row({ status: 'sending' }))).toBe('Gönderiliyor')
    expect(statusLabel(row({ status: 'completed' }))).toBe('Tamamlandı')
  })
  it('names the states before approval', () => {
    expect(statusLabel(row({ status: 'segment_ready', segment_status: 'ok' }))).toBe('Kitle hazır')
    expect(statusLabel(row({ status: 'content_ready', segment_status: 'ok' }))).toBe('Onay bekliyor')
  })
  it('says why a draft never went anywhere instead of calling it a draft', () => {
    expect(statusLabel(row({ status: 'draft', segment_status: 'unsupported' }))).toBe('Desteklenmiyor')
    expect(statusLabel(row({ status: 'draft', segment_status: 'needs_clarification' }))).toBe('Soru bekliyor')
    expect(statusLabel(row({ status: 'draft' }))).toBe('Taslak')
  })
})

describe('briefPreview', () => {
  it('keeps a short brief as it is', () => {
    expect(briefPreview('kısa brief', 90)).toBe('kısa brief')
  })
  it('cuts a long one at a word and says so', () => {
    const out = briefPreview('bir iki üç dört beş altı yedi sekiz dokuz on', 20)
    expect(out.endsWith('…')).toBe(true)
    expect(out.length).toBeLessThanOrEqual(21)
    expect(out).not.toMatch(/\s…$/)
  })
  it('collapses line breaks', () => {
    expect(briefPreview('a\n\nb', 90)).toBe('a b')
  })
})

describe('deliveryTotals', () => {
  const d: DeliveryProgress = {
    sms: ch({ total: 10, sent: 6, failed: 1, skipped: 1, pending: 2, retries: 4, rate_limited: 3 }),
    email: ch({ total: 5, sent: 4, failed: 1, retries: 2, rate_limited: 1 }),
    done: false,
  }
  it('adds both channels', () => {
    expect(deliveryTotals(d)).toEqual({ total: 15, sent: 10, failed: 2, skipped: 1, pending: 2, retries: 6, rateLimited: 4 })
  })
  it('is all zeros when nothing was approved yet', () => {
    expect(deliveryTotals(undefined)).toEqual({ total: 0, sent: 0, failed: 0, skipped: 0, pending: 0, retries: 0, rateLimited: 0 })
  })
})

describe('the list filter', () => {
  const at = '2026-11-16T10:00:00Z'
  const rows: CampaignSummary[] = [
    { id: 1, brief: 'a', status: 'draft', segment_status: 'unsupported', created_at: at },
    { id: 2, brief: 'b', status: 'draft', segment_status: 'needs_clarification', created_at: at },
    { id: 3, brief: 'c', status: 'segment_ready', segment_status: 'ok', created_at: at },
    { id: 4, brief: 'd', status: 'content_ready', segment_status: 'ok', created_at: at },
    { id: 5, brief: 'e', status: 'scheduled', segment_status: 'ok', created_at: at },
    { id: 6, brief: 'f', status: 'sending', segment_status: 'ok', created_at: at },
    { id: 7, brief: 'g', status: 'completed', segment_status: 'ok', created_at: at },
  ]
  const ids = (l: CampaignSummary[]) => l.map((c) => c.id)
  it('hides "Desteklenmiyor" by default, in every filter', () => {
    expect(ids(filterCampaigns(rows, 'all'))).toEqual([2, 3, 4, 5, 6, 7])
    expect(ids(filterCampaigns(rows, 'draft'))).toEqual([2, 3, 4]) // drafts stay: segment_ready, content_ready, a question
    expect(ids(filterCampaigns(rows, 'active'))).toEqual([5, 6])
  })
  it('shows them when asked', () => {
    expect(ids(filterCampaigns(rows, 'all', true))).toEqual([1, 2, 3, 4, 5, 6, 7])
    expect(ids(filterCampaigns(rows, 'draft', true))).toEqual([1, 2, 3, 4])
    expect(ids(filterCampaigns(rows, 'active', true))).toEqual([5, 6])
  })
  it('"Desteklenmiyor" is an unapproved campaign the model could not serve', () => {
    expect(rows.filter(isUnsupported).map((c) => c.id)).toEqual([1])
  })
})
