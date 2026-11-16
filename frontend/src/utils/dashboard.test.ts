import { describe, expect, it } from 'vitest'
import type { CampaignSummary, DeliveryProgress } from '../types'
import { briefPreview, deliveryTotals, pageFromHash, statusLabel } from './dashboard'

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
