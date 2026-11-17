import type { Offer } from '../types'

const nf = new Intl.NumberFormat('tr-TR', { maximumFractionDigits: 2 })

/** 2026-12-31 -> 31.12.2026, by splitting the text (never through Date: no time-zone shift). */
function formatDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso)
  return m ? `${m[3]}.${m[2]}.${m[1]}` : iso
}

function part(kind: string, value: number, o: Offer, extra: boolean): string {
  switch (kind) {
    case 'percent':
      return `%${nf.format(value)} indirim`
    case 'amount':
      if (o.step_spend && o.step_spend > 0) return `Her ${nf.format(o.step_spend)} TL alışverişe ${nf.format(value)} TL indirim`
      return extra ? `ekstra ${nf.format(value)} TL indirim` : `${nf.format(value)} TL indirim`
    case 'free_shipping':
      return 'Ücretsiz kargo'
  }
  return 'Bilinmeyen teklif'
}

/** The offer in plain Turkish, as the marketer wrote it. Always returns text: "no offer" is said out loud. */
export function describeOffer(o: Offer | undefined): string {
  if (!o || o.kind === 'none') return 'Brief’te teklif yok; mesajlarda indirim yazılmayacak'

  let main: string
  if (o.kind === 'coupon_code') {
    main = `Kupon kodu ${o.code}`
  } else if (o.kind === 'percent' || o.kind === 'amount' || o.kind === 'free_shipping') {
    // Every part of a combined offer, the main one first; a cap belongs to the main part.
    const extras = o.extras ?? []
    const hasPercent = o.kind === 'percent' || extras.some((e) => e.kind === 'percent')
    main = part(o.kind, o.value, o, false)
    if (o.max_discount && o.max_discount > 0) {
      main += o.step_spend && o.step_spend > 0 ? ` (toplamda en fazla ${nf.format(o.max_discount)} TL)` : ` (en fazla ${nf.format(o.max_discount)} TL)`
    }
    for (const e of extras) {
      const p = part(e.kind, e.value, o, e.kind === 'amount' && hasPercent)
      main += ' + ' + p.charAt(0).toLocaleLowerCase('tr-TR') + p.slice(1)
    }
  } else {
    return 'Bilinmeyen teklif'
  }

  // Terms of the offer. A minimum basket and a last day are part of WHAT is
  // promised; they never describe who receives the message.
  const terms: string[] = []
  if (o.code && o.kind !== 'coupon_code') terms.push(`kupon kodu ${o.code}`)
  if (o.min_spend && o.min_spend > 0) terms.push(`${nf.format(o.min_spend)} TL ve üzeri alışverişte`)
  if (o.expires_at) terms.push(`son kullanım ${formatDate(o.expires_at)}`)
  return terms.length > 0 ? `${main} (${terms.join(', ')})` : main
}
