import type { Condition, Segment } from '../types'

const SYMBOL: Record<string, string> = { eq: '=', gt: '>', gte: '≥', lt: '<', lte: '≤' }

function days(field: 'cart' | 'order', op: string, n: number): string {
  const what = field === 'cart' ? 'sepete ürün eklemiş' : 'sipariş vermiş'
  switch (op) {
    case 'lte':
      return `Son ${n} gün içinde ${what}`
    case 'lt':
      return `${n} günden daha kısa süre önce ${what}`
    case 'gte':
      return n === 0 ? `Daha önce ${what} (zamandan bağımsız)` : `En az ${n} gün önce ${what}`
    case 'gt':
      return `${n} günden daha uzun süre önce ${what}`
    default:
      return `Tam ${n} gün önce ${what}`
  }
}

/** One human-readable (Turkish) line per condition. */
export function describeCondition(c: Condition): string {
  const list = Array.isArray(c.value) ? c.value.join(', ') : String(c.value)
  switch (c.field) {
    case 'city':
      if (c.op === 'neq' || c.op === 'not_in') return `Şehir: ${list} dışında`
      return `Şehir: ${list}`
    case 'days_since_last_cart_add':
      return days('cart', c.op, Number(c.value))
    case 'days_since_last_order':
      return days('order', c.op, Number(c.value))
    case 'days_since_last_order_or_never': {
      const n = Number(c.value)
      const tail = '(hiç sipariş vermemişler dahil)'
      return c.op === 'gte' ? `En az ${n} gündür sipariş vermemiş ${tail}` : `Son ${n} günde sipariş vermemiş ${tail}`
    }
    case 'gender':
      return c.value === 'female' ? 'Cinsiyet: kadın' : c.value === 'male' ? 'Cinsiyet: erkek' : `Cinsiyet: ${list}`
    case 'all_customers':
      return 'Tüm müşteriler (iletişim izni olanlar)'
    case 'order_count':
      return `Sipariş sayısı ${SYMBOL[c.op] ?? c.op} ${c.value}`
    case 'total_spent':
      return `Toplam harcama ${SYMBOL[c.op] ?? c.op} ${c.value} TL`
    case 'purchased_since_last_cart_add':
      return c.value === true ? 'Sepete eklemesinden sonra satın almış' : 'Sepete eklemesinden sonra satın almamış'
    default:
      return `${c.field} ${c.op} ${list}`
  }
}

export function describeSegment(s: Segment): { match: string; lines: string[] } {
  return {
    match: s.match === 'any' ? 'Koşullardan herhangi biri sağlanmalı' : 'Tüm koşullar sağlanmalı',
    lines: s.conditions.map(describeCondition),
  }
}
