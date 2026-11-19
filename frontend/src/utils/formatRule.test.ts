import { describe, expect, it } from 'vitest'
import type { Segment } from '../types'
import { describeCondition, describeConditions, describeSegment } from './formatRule'

describe('describeCondition', () => {
  it('describes the example brief', () => {
    const lines = describeSegment({
      status: 'ok',
      match: 'all',
      explanation: '',
      conditions: [
        { field: 'city', op: 'eq', value: 'İstanbul' },
        { field: 'days_since_last_cart_add', op: 'lte', value: 30 },
        { field: 'purchased_since_last_cart_add', op: 'eq', value: false },
      ],
    } satisfies Segment)
    expect(lines.lines).toEqual([
      'Şehir: İstanbul',
      'Son 30 gün içinde sepete ürün eklemiş',
      'Sepete eklemesinden sonra satın almamış',
    ])
    expect(lines.match).toBe('Tüm koşullar sağlanmalı')
  })

  it('handles lists, exclusions and comparisons', () => {
    expect(describeCondition({ field: 'city', op: 'in', value: ['Ankara', 'İzmir'] })).toBe('Şehir: Ankara, İzmir')
    expect(describeCondition({ field: 'city', op: 'not_in', value: ['Ankara'] })).toBe('Şehir: Ankara dışında')
    expect(describeCondition({ field: 'order_count', op: 'gte', value: 3 })).toBe('Sipariş sayısı ≥ 3')
    expect(describeCondition({ field: 'total_spent', op: 'lt', value: 500 })).toBe('Toplam harcama < 500 TL')
    expect(describeCondition({ field: 'days_since_last_cart_add', op: 'gte', value: 0 })).toContain('zamandan bağımsız')
    expect(describeCondition({ field: 'days_since_last_order', op: 'lte', value: 14 })).toBe('Son 14 gün içinde sipariş vermiş')
    expect(describeCondition({ field: 'days_since_last_order_or_never', op: 'gt', value: 30 })).toBe(
      'Son 30 günde sipariş vermemiş (hiç sipariş vermemişler dahil)',
    )
    expect(describeCondition({ field: 'days_since_last_order_or_never', op: 'gte', value: 30 })).toBe(
      'En az 30 gündür sipariş vermemiş (hiç sipariş vermemişler dahil)',
    )
  })

  it('falls back gracefully for unknown fields', () => {
    expect(describeCondition({ field: 'x', op: 'eq', value: 1 })).toBe('x eq 1')
  })
})

describe('describeCondition: everyone', () => {
  it('says plainly that the audience is everybody who may be contacted', () => {
    expect(describeCondition({ field: 'all_customers', op: 'eq', value: true })).toBe('Tüm müşteriler (iletişim izni olanlar)')
  })
})

describe('describeConditions: age', () => {
  const age = (op: string, value: number) => ({ field: 'age', op, value })
  const city = { field: 'city', op: 'eq', value: 'İstanbul' }

  it('shows the two ends of a range as one badge, where the first age condition was', () => {
    expect(describeConditions([age('gte', 18), city, age('lte', 25)], 'all')).toEqual(['Yaş 18–25', 'Şehir: İstanbul'])
    expect(describeConditions([city, age('lte', 25), age('gte', 18)], 'all')).toEqual(['Şehir: İstanbul', 'Yaş 18–25'])
  })

  it('counts whole years: gt 17 and lt 26 are 18–25', () => {
    expect(describeConditions([age('gt', 17), age('lt', 26)], 'all')).toEqual(['Yaş 18–25'])
  })

  it('keeps one-sided and "any" conditions as they are', () => {
    expect(describeConditions([age('gte', 65)], 'all')).toEqual(['Yaş 65+'])
    expect(describeConditions([age('lt', 30)], 'all')).toEqual(['Yaş < 30'])
    expect(describeConditions([age('eq', 30)], 'all')).toEqual(['Yaş 30'])
    expect(describeConditions([age('lt', 20), age('gt', 60)], 'any')).toEqual(['Yaş < 20', 'Yaş > 60'])
  })

  it('is used by describeSegment', () => {
    const s: Segment = { status: 'ok', match: 'all', explanation: '', conditions: [age('gte', 18), age('lte', 25)] }
    expect(describeSegment(s).lines).toEqual(['Yaş 18–25'])
  })
})
