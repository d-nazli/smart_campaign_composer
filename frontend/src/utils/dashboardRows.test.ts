import { describe, expect, it } from 'vitest'
import type { CampaignSummary, DeliveryProgress } from '../types'
import { briefTitle, languageCells, progressTitle, rowProgress, sentenceCase, statusBadge } from './dashboard'
import { quickTimes } from './schedule'

const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })
const row = (o: Partial<CampaignSummary> = {}): CampaignSummary => ({ id: 1, brief: 'b', status: 'draft', created_at: '2026-11-16T10:00:00Z', ...o })

describe('briefTitle', () => {
  it('starts with a capital, using Turkish rules', () => {
    expect(briefTitle('istanbul müşterilerine indirim', 80)).toBe('İstanbul müşterilerine indirim')
    expect(briefTitle('son 30 günde sepeti bırakanlara', 80)).toBe('Son 30 günde sepeti bırakanlara')
    expect(briefTitle('ışık kampanyası', 80)).toBe('Işık kampanyası')
  })

  it('keeps a short brief as it is, without an ellipsis', () => {
    expect(briefTitle('Tüm müşterilere %10 indirim', 80)).toBe('Tüm müşterilere %10 indirim')
  })

  it('cuts a long brief at a word and ends it with an ellipsis', () => {
    const t = briefTitle('son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul’daki kullanıcılara %30 indirim hatırlatması gönder', 50)
    expect(t.endsWith('…')).toBe(true)
    expect(t.length).toBeLessThanOrEqual(51)
    expect(t.startsWith('Son 30 günde')).toBe(true)
    expect(t).not.toMatch(/\s…$/) // no dangling space before the ellipsis
  })

  it('puts the brief on one line', () => {
    expect(briefTitle('birinci satır\n\n  ikinci   satır', 80)).toBe('Birinci satır ikinci satır')
  })

  it('is empty for an empty brief', () => {
    expect(briefTitle('   ', 40)).toBe('')
  })
})

describe('statusBadge', () => {
  it('colours what finished, what is moving and what needs attention', () => {
    expect(statusBadge(row({ status: 'completed' }))).toEqual({ text: 'Tamamlandı', tone: 'ok' })
    expect(statusBadge(row({ status: 'sending' })).tone).toBe('accent')
    expect(statusBadge(row({ status: 'scheduled' }))).toEqual({ text: 'Zamanlandı', tone: 'plan' })
    expect(statusBadge(row({ status: 'failed' })).tone).toBe('danger')
    expect(statusBadge(row({ status: 'cancelled' }))).toEqual({ text: 'İptal edildi', tone: 'neutral' })
  })

  it('calls every unfinished campaign a draft, except the ones waiting on the person', () => {
    expect(statusBadge(row({ status: 'segment_ready', segment_status: 'ok' })).text).toBe('Taslak')
    expect(statusBadge(row({ status: 'content_ready', segment_status: 'ok' })).text).toBe('Taslak')
    expect(statusBadge(row({ status: 'draft', segment_status: 'needs_clarification' }))).toEqual({ text: 'Soru bekliyor', tone: 'warn' })
    expect(statusBadge(row({ status: 'draft', segment_status: 'unsupported' }))).toEqual({ text: 'Desteklenmiyor', tone: 'warn' })
    expect(statusBadge(row({ status: 'draft' })).text).toBe('Taslak')
  })
})

describe('rowProgress and its tooltip', () => {
  const d: DeliveryProgress = { sms: ch({ total: 10, sent: 6, failed: 1, pending: 3 }), email: ch({ total: 5, sent: 4, skipped: 1, retries: 2, rate_limited: 1 }), done: false }

  it('adds both channels', () => {
    expect(rowProgress(d)).toEqual({ total: 15, sent: 10, failed: 1, skipped: 1, pending: 3, handled: 12 })
  })
  it('is all zero before sending', () => {
    expect(rowProgress(undefined).total).toBe(0)
    expect(progressTitle(undefined)).toBe('')
  })
  it('explains the numbers the table has no column for', () => {
    expect(progressTitle(d)).toBe('10 gönderildi, 1 başarısız, 1 atlandı, 3 bekliyor, 2 tekrar, 1 adet 429')
  })
})

describe('languageCells', () => {
  const order = [{ code: 'tr' }, { code: 'en' }, { code: 'de' }]

  it('shows what was sent per language, sent/total while some wait', () => {
    const delivery: DeliveryProgress = {
      sms: ch(),
      email: ch(),
      done: false,
      languages: { tr: { total: 171, sent: 171 }, en: { total: 72, sent: 60 }, de: { total: 42, sent: 0 } },
    }
    expect(languageCells({ delivery }, order)).toEqual([
      { code: 'TR', text: '171' },
      { code: 'EN', text: '60/72' },
      { code: 'DE', text: '0/42' },
    ])
    expect(languageCells({ delivery: { ...delivery, done: true } }, order).map((c) => c.text)).toEqual(['171', '60', '0'])
  })

  it('shows the audience before anything is sent', () => {
    expect(languageCells({ audience_languages: { de: 3, tr: 10, en: 0 } }, order)).toEqual([
      { code: 'TR', text: '10' },
      { code: 'DE', text: '3' },
    ])
  })

  it('shows nothing without data', () => {
    expect(languageCells({}, order)).toEqual([])
  })
})

describe('quickTimes', () => {
  it('offers an hour from now (on a five-minute mark), this evening and tomorrow morning', () => {
    const q = quickTimes(new Date(2026, 10, 16, 10, 7))
    expect(q.map((x) => x.label)).toEqual(['1 saat sonra', 'Bu akşam 18:00', 'Yarın 09:00'])
    expect(q[0].value).toBe('2026-11-16T11:10')
    expect(q[1].value).toBe('2026-11-16T18:00')
    expect(q[2].value).toBe('2026-11-17T09:00')
  })

  it('drops "this evening" once it is less than an hour away', () => {
    const q = quickTimes(new Date(2026, 10, 16, 17, 30))
    expect(q.map((x) => x.label)).toEqual(['1 saat sonra', 'Yarın 09:00'])
  })

  it('always leaves at least the minimum lead for "an hour from now"', () => {
    const now = new Date(2026, 10, 16, 23, 58)
    const first = new Date(quickTimes(now)[0].value)
    expect(first.getTime() - now.getTime()).toBeGreaterThanOrEqual(60 * 60 * 1000)
  })
})

describe('sentenceCase', () => {
  it('starts with a capital using Turkish rules and leaves the rest alone', () => {
    expect(sentenceCase('tüm müşteriler (izin vermiş olanlar)')).toBe('Tüm müşteriler (izin vermiş olanlar)')
    expect(sentenceCase('istanbul’daki müşteriler, son 30 günde')).toBe('İstanbul’daki müşteriler, son 30 günde')
    expect(sentenceCase('ışık')).toBe('Işık')
  })
  it('keeps what already starts with a capital or a symbol, and an empty text', () => {
    expect(sentenceCase('Ücretsiz kargo')).toBe('Ücretsiz kargo')
    expect(sentenceCase('%30 indirim')).toBe('%30 indirim')
    expect(sentenceCase('  ')).toBe('')
  })
})
