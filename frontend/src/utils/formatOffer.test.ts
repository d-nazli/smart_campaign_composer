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
