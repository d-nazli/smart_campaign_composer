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

describe('SegmentCard: the scope of the offer', () => {
  it('shows the products next to the offer, not as a rule condition', () => {
    const segment: Segment = {
      status: 'ok',
      match: 'all',
      conditions: [{ field: 'all_customers', op: 'eq', value: true }],
      explanation: 'tüm müşteriler (izin vermiş olanlar)',
      audience,
      offer: { kind: 'amount', value: 200, code: 'KIS200', scope: 'kış ürünleri' },
      note: 'Kitle belirtilmediği için tüm müşteriler seçildi.',
    }
    const w = mount(SegmentCard, { props: { segment } })
    expect(w.get('[data-testid="offer"]').text()).toContain('200 TL indirim · Kış ürünleri (kupon kodu KIS200)')
    expect(w.findAll('.chip').map((c) => c.text())).toEqual(['Tüm müşteriler (iletişim izni olanlar)'])
  })
})

describe('SegmentCard: age', () => {
  it('shows an age range as one badge', () => {
    const segment: Segment = {
      status: 'ok',
      match: 'all',
      conditions: [
        { field: 'age', op: 'gte', value: 18 },
        { field: 'age', op: 'lte', value: 25 },
        { field: 'city', op: 'eq', value: 'İstanbul' },
      ],
      explanation: "18–25 yaş arası, İstanbul'daki müşteriler",
      audience,
      offer: { kind: 'none', value: 0, code: '' },
    }
    const chips = mount(SegmentCard, { props: { segment } }).findAll('.chip').map((c) => c.text())
    expect(chips).toEqual(['Yaş 18–25', 'Şehir: İstanbul'])
  })
})

describe('SegmentCard: answering the question', () => {
  const asking: Segment = {
    status: 'needs_clarification',
    conditions: [],
    explanation: '',
    question: 'Bu kampanya kime gönderilsin? Tüm müşterilere mi?',
    suggestion: 'all_customers',
  }

  it('sends a typed answer, trimmed, and only a non-empty one', async () => {
    const w = mount(SegmentCard, { props: { segment: asking } })
    const send = w.get('[data-testid="send-answer"]')
    expect(send.attributes('disabled')).toBeDefined()
    await w.get('[data-testid="answer"]').setValue("  Ankara'daki müşterilere  ")
    expect(send.attributes('disabled')).toBeUndefined()
    await w.get('form').trigger('submit')
    expect(w.emitted('answer')).toEqual([["Ankara'daki müşterilere"]])
  })

  it('keeps the one-click answer next to the answer box', async () => {
    const w = mount(SegmentCard, { props: { segment: asking } })
    await w.get('[data-testid="send-to-all"]').trigger('click')
    expect(w.emitted('send-to-all')).toHaveLength(1)
  })

  it('does not send while busy', async () => {
    const w = mount(SegmentCard, { props: { segment: asking, busy: true } })
    await w.get('[data-testid="answer"]').setValue('evet')
    await w.get('form').trigger('submit')
    expect(w.emitted('answer')).toBeUndefined()
  })

  it('lists the earlier questions and answers above a new question, and with the audience', () => {
    const clarifications = [{ question: 'Kime gönderilsin?', answer: 'bilmiyorum', answered_at: '2026-11-19T10:00:00Z' }]
    const again = mount(SegmentCard, { props: { segment: { ...asking, question: 'Hangi şehir?', clarifications } } })
    expect(again.get('[data-testid="clarifications"]').findAll('p').map((p) => p.text())).toEqual(['Soru: Kime gönderilsin?', 'Cevap: bilmiyorum'])
    expect(again.get('[data-testid="question"]').text()).toBe('Hangi şehir?')

    const ok: Segment = {
      status: 'ok',
      match: 'all',
      conditions: [{ field: 'city', op: 'eq', value: 'Ankara' }],
      explanation: "Ankara'daki müşteriler",
      audience,
      clarifications: [{ question: 'Kime gönderilsin?', answer: "Ankara'dakilere", answered_at: '2026-11-19T10:00:00Z' }],
    }
    const done = mount(SegmentCard, { props: { segment: ok } })
    const shown = done.get('[data-testid="clarifications"]')
    expect(shown.get('h3').text()).toBe('Soru ve cevaplar')
    expect(shown.findAll('p').map((p) => p.text())).toEqual(['Soru: Kime gönderilsin?', "Cevap: Ankara'dakilere"])
  })

  it('shows no history when nothing was asked', () => {
    const w = mount(SegmentCard, { props: { segment: asking } })
    expect(w.find('[data-testid="clarifications"]').exists()).toBe(false)
  })
})

describe('SegmentCard: "Brief\'i düzenle"', () => {
  it('is offered on a question and on "Desteklenmiyor", not on an audience', async () => {
    const asking: Segment = { status: 'needs_clarification', conditions: [], explanation: '', question: 'Kime?' }
    const a = mount(SegmentCard, { props: { segment: asking } })
    await a.get('[data-testid="edit-brief"]').trigger('click')
    expect(a.emitted('edit-brief')).toHaveLength(1)

    const no: Segment = { status: 'unsupported', conditions: [], explanation: 'Yaş verisi yok.' }
    const u = mount(SegmentCard, { props: { segment: no } })
    expect(u.text()).toContain('Bu brief desteklenmiyor')
    await u.get('[data-testid="edit-brief"]').trigger('click')
    expect(u.emitted('edit-brief')).toHaveLength(1)

    const ok: Segment = { status: 'ok', match: 'all', conditions: [{ field: 'city', op: 'eq', value: 'Ankara' }], explanation: 'x', audience }
    expect(mount(SegmentCard, { props: { segment: ok } }).find('[data-testid="edit-brief"]').exists()).toBe(false)
  })
})
