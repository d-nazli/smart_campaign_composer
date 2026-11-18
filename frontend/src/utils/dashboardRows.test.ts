import { describe, expect, it } from 'vitest'
import type { CampaignSummary, DeliveryProgress } from '../types'
import { briefTitle, localeLine, progressTitle, rowProgress, sentenceCase, statusBadge } from './dashboard'
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

describe('localeLine', () => {
  const order = [{ code: 'tr-TR' }, { code: 'de-DE' }, { code: 'en-GB' }, { code: 'it-IT' }]

  it('names the two largest locales and counts the rest; the tooltip lists them all', () => {
    const audience_locales = { 'tr-TR': 828, 'en-US': 174, 'de-DE': 40, 'it-IT': 12, 'en-GB': 0 }
    expect(localeLine({ audience_locales }, order)).toMatchObject({
      text: 'tr-TR 828 · en-US 174 · +2',
      title: ['tr-TR 828', 'en-US 174', 'de-DE 40', 'it-IT 12'].join('\n'),
    })
  })

  it('writes the codes as they are: never upper-cased', () => {
    expect(localeLine({ audience_locales: { 'it-IT': 5 } }, order)!.text).toBe('it-IT 5')
  })

  it('needs no "+" for two or fewer, and ties keep the table order', () => {
    expect(localeLine({ audience_locales: { 'en-GB': 3, 'de-DE': 3 } }, order)!.text).toBe('de-DE 3 · en-GB 3')
  })

  it('shows sent/total while sending, what was sent once done', () => {
    const delivery: DeliveryProgress = {
      sms: ch(),
      email: ch(),
      done: false,
      locales: { 'en-GB': { total: 72, sent: 60 }, 'tr-TR': { total: 171, sent: 171 }, 'de-DE': { total: 42, sent: 0 } },
    }
    expect(localeLine({ delivery }, order)!.text).toBe('tr-TR 171 · en-GB 60/72 · +1')
    expect(localeLine({ delivery: { ...delivery, done: true } }, order)!.title).toBe(['tr-TR 171', 'en-GB 60', 'de-DE 0'].join('\n'))
  })

  it('is absent without data', () => {
    expect(localeLine({}, order)).toBeNull()
    expect(localeLine({ audience_locales: { 'tr-TR': 0 } }, order)).toBeNull()
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
