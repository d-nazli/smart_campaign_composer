import { describe, expect, it } from 'vitest'
import type { Segment } from '../types'
import { describeCondition, describeSegment } from './formatRule'

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
