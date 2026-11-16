import { describe, expect, it } from 'vitest'
import { describeOffer } from './formatOffer'

describe('describeOffer', () => {
  it('writes every kind of offer', () => {
    expect(describeOffer({ kind: 'percent', value: 15, code: '' })).toBe('%15 indirim')
    expect(describeOffer({ kind: 'percent', value: 12.5, code: '' })).toBe('%12,5 indirim')
    expect(describeOffer({ kind: 'amount', value: 100, code: '' })).toBe('100 TL indirim')
    expect(describeOffer({ kind: 'amount', value: 1000, code: '' })).toBe('1.000 TL indirim')
    expect(describeOffer({ kind: 'free_shipping', value: 0, code: '' })).toBe('Ücretsiz kargo')
    expect(describeOffer({ kind: 'coupon_code', value: 0, code: 'HOSGELDIN10' })).toBe('Kupon kodu HOSGELDIN10')
    expect(describeOffer({ kind: 'percent', value: 15, code: 'YAZ25' })).toBe('%15 indirim (kupon kodu YAZ25)')
  })

  it('says out loud when there is no offer, also for campaigns saved before offers existed', () => {
    const none = 'Brief’te teklif yok; mesajlarda indirim yazılmayacak'
    expect(describeOffer({ kind: 'none', value: 0, code: '' })).toBe(none)
    expect(describeOffer(undefined)).toBe(none)
  })
})

describe('describeOffer: the terms of the offer', () => {
  it('states the minimum basket and the last day', () => {
    expect(describeOffer({ kind: 'amount', value: 100, code: '', min_spend: 1000, expires_at: '2026-12-31' })).toBe(
      '100 TL indirim (1.000 TL ve üzeri alışverişte, son kullanım 31.12.2026)',
    )
    expect(describeOffer({ kind: 'percent', value: 10, code: '', min_spend: 1500.5 })).toBe('%10 indirim (1.500,5 TL ve üzeri alışverişte)')
    expect(describeOffer({ kind: 'free_shipping', value: 0, code: '', expires_at: '2027-01-05' })).toBe('Ücretsiz kargo (son kullanım 05.01.2027)')
    expect(describeOffer({ kind: 'percent', value: 15, code: 'YAZ25', min_spend: 1000 })).toBe(
      '%15 indirim (kupon kodu YAZ25, 1.000 TL ve üzeri alışverişte)',
    )
    expect(describeOffer({ kind: 'coupon_code', value: 0, code: 'YAZ25', expires_at: '2026-12-31' })).toBe('Kupon kodu YAZ25 (son kullanım 31.12.2026)')
  })

  it('says nothing extra when there are no terms (0 and empty mean none)', () => {
    expect(describeOffer({ kind: 'percent', value: 15, code: '', min_spend: 0, expires_at: '' })).toBe('%15 indirim')
  })

  it('does not shift the day with the browser time zone', () => {
    expect(describeOffer({ kind: 'amount', value: 1, code: '', expires_at: '2026-01-01' })).toBe('1 TL indirim (son kullanım 01.01.2026)')
  })
})
