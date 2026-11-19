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
    case 'age': {
      const n = Number(c.value)
      if (c.op === 'gte') return `Yaş ${n}+`
      if (c.op === 'eq') return `Yaş ${n}`
      return `Yaş ${SYMBOL[c.op] ?? c.op} ${n}`
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

/**
 * The age range an "all" rule's age conditions leave open, both ends included
 * ("gt 17" is "gte 18"), when it has exactly one lower and one upper bound.
 */
function ageRange(conds: Condition[]): [number, number] | null {
  let lo: number | null = null
  let hi: number | null = null
  for (const c of conds) {
    const n = Number(c.value)
    if (c.op === 'gte' || c.op === 'gt') {
      if (lo !== null) return null
      lo = c.op === 'gt' ? n + 1 : n
    } else if (c.op === 'lte' || c.op === 'lt') {
      if (hi !== null) return null
      hi = c.op === 'lt' ? n - 1 : n
    } else return null
  }
  return lo !== null && hi !== null && lo <= hi ? [lo, hi] : null
}

/**
 * One line per condition, except that the two ends of an age range in an "all"
 * rule are one line: "Yaş 18–25".
 */
export function describeConditions(conds: Condition[], match?: string): string[] {
  const ages = conds.filter((c) => c.field === 'age')
  const range = match !== 'any' && ages.length === 2 ? ageRange(ages) : null
  if (!range) return conds.map(describeCondition)
  const out: string[] = []
  let shown = false
  for (const c of conds) {
    if (c.field !== 'age') out.push(describeCondition(c))
    else if (!shown) {
      out.push(`Yaş ${range[0]}–${range[1]}`)
      shown = true
    }
  }
  return out
}

export function describeSegment(s: Segment): { match: string; lines: string[] } {
  return {
    match: s.match === 'any' ? 'Koşullardan herhangi biri sağlanmalı' : 'Tüm koşullar sağlanmalı',
    lines: describeConditions(s.conditions, s.match),
  }
}
