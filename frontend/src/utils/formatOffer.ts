import type { Offer } from '../types'

const nf = new Intl.NumberFormat('tr-TR', { maximumFractionDigits: 2 })

/** 2026-12-31 -> 31.12.2026, by splitting the text (never through Date: no time-zone shift). */
function formatDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso)
  return m ? `${m[3]}.${m[2]}.${m[1]}` : iso
}

/** The offer in plain Turkish, as the marketer wrote it. Always returns text: "no offer" is said out loud. */
export function describeOffer(o: Offer | undefined): string {
  if (!o || o.kind === 'none') return 'Brief’te teklif yok; mesajlarda indirim yazılmayacak'

  let main: string
  switch (o.kind) {
    case 'percent':
      main = `%${nf.format(o.value)} indirim`
      break
    case 'amount':
      main = `${nf.format(o.value)} TL indirim`
      break
    case 'free_shipping':
      main = 'Ücretsiz kargo'
      break
    case 'coupon_code':
      main = `Kupon kodu ${o.code}`
      break
    default:
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
