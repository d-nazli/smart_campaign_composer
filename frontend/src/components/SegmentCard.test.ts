// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Segment } from '../types'
import SegmentCard from './SegmentCard.vue'
import { describeOffer } from '../utils/formatOffer'
import { describeCondition } from '../utils/formatRule'

afterEach(() => {
  document.body.innerHTML = ''
})

const audience = { matched: 5000, sms_reachable: 3500, email_reachable: 3800, both_reachable: 2800 }

describe('SegmentCard: the audience note', () => {
  it('says why everyone was chosen for a special day that named nobody', () => {
    const segment: Segment = {
      status: 'ok',
      match: 'all',
      conditions: [{ field: 'all_customers', op: 'eq', value: true }],
      explanation: 'tüm müşteriler (izin vermiş olanlar)',
      audience,
      offer: { kind: 'percent', value: 20, code: '' },
      note: 'Kitle belirtilmediği için tüm müşteriler seçildi.',
    }
    const w = mount(SegmentCard, { props: { segment } })
    expect(w.get('[data-testid="audience-note"]').text()).toBe('Kitle belirtilmediği için tüm müşteriler seçildi.')
  })

  it('shows no note when the brief named the audience itself', () => {
    const segment: Segment = {
      status: 'ok',
      match: 'all',
      conditions: [{ field: 'gender', op: 'eq', value: 'female' }],
      explanation: 'kadın müşteriler',
      audience,
      offer: { kind: 'percent', value: 20, code: '' },
    }
    const w = mount(SegmentCard, { props: { segment } })
    expect(w.find('[data-testid="audience-note"]').exists()).toBe(false)
    expect(w.text()).toContain('Cinsiyet: kadın')
  })
})

describe('SegmentCard: what the rule leaves out of the brief', () => {
  const base: Segment = {
    status: 'ok',
    match: 'all',
    conditions: [{ field: 'days_since_last_cart_add', op: 'lte', value: 30 }],
    explanation: 'son 30 günde sepete ekleyenler',
    audience,
    offer: { kind: 'percent', value: 10, code: '' },
  }

  it('shows each warning next to the audience, which stays usable', () => {
    const warning = 'Brief\'te "almayanlara" var, kuralda yok. Kitle beklediğinden geniş olabilir.'
    const w = mount(SegmentCard, { props: { segment: { ...base, warnings: [warning] } } })
    const shown = w.findAll('[data-testid="audience-warning"]')
    expect(shown.map((p) => p.text())).toEqual([warning])
    expect(shown[0].classes()).toContain('warn')
    expect(w.text()).toContain('5.000') // the audience is still shown
  })

  it('shows none without warnings', () => {
    const w = mount(SegmentCard, { props: { segment: base } })
    expect(w.find('[data-testid="audience-warning"]').exists()).toBe(false)
  })
})

describe('describeOffer: combined, tiered and capped offers', () => {
  it('names every part of a combined offer', () => {
    expect(describeOffer({ kind: 'percent', value: 20, code: '', extras: [{ kind: 'free_shipping', value: 0 }] })).toBe('%20 indirim + ücretsiz kargo')
    expect(
      describeOffer({ kind: 'percent', value: 10, code: '', extras: [{ kind: 'amount', value: 50 }, { kind: 'free_shipping', value: 0 }] }),
    ).toBe('%10 indirim + ekstra 50 TL indirim + ücretsiz kargo')
  })

  it('writes a tier with its cap and a capped percentage', () => {
    expect(describeOffer({ kind: 'amount', value: 100, code: '', step_spend: 1000, max_discount: 300 })).toBe(
      'Her 1.000 TL alışverişe 100 TL indirim (toplamda en fazla 300 TL)',
    )
    expect(describeOffer({ kind: 'percent', value: 20, code: '', max_discount: 300 })).toBe('%20 indirim (en fazla 300 TL)')
  })

  it('keeps the terms after the parts', () => {
    expect(
      describeOffer({ kind: 'percent', value: 15, code: 'YAZ15', min_spend: 500, expires_at: '2026-12-31', extras: [{ kind: 'free_shipping', value: 0 }] }),
    ).toBe('%15 indirim + ücretsiz kargo (kupon kodu YAZ15, 500 TL ve üzeri alışverişte, son kullanım 31.12.2026)')
  })

  it('reads old offers without the new fields', () => {
    expect(describeOffer({ kind: 'free_shipping', value: 0, code: '', extras: null })).toBe('Ücretsiz kargo')
  })
})

describe('describeCondition: gender', () => {
  it('says the gender in Turkish', () => {
    expect(describeCondition({ field: 'gender', op: 'eq', value: 'female' })).toBe('Cinsiyet: kadın')
    expect(describeCondition({ field: 'gender', op: 'eq', value: 'male' })).toBe('Cinsiyet: erkek')
  })
})
