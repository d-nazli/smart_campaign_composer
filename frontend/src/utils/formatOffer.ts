import type { Offer } from '../types'

const nf = new Intl.NumberFormat('tr-TR', { maximumFractionDigits: 2 })

/** The offer in plain Turkish, as the marketer wrote it. Always returns text: "no offer" is said out loud. */
export function describeOffer(o: Offer | undefined): string {
  if (!o || o.kind === 'none') return 'Brief’te teklif yok; mesajlarda indirim yazılmayacak'
  const code = o.code ? ` (kupon kodu ${o.code})` : ''
  switch (o.kind) {
    case 'percent':
      return `%${nf.format(o.value)} indirim${code}`
    case 'amount':
      return `${nf.format(o.value)} TL indirim${code}`
    case 'free_shipping':
      return `Ücretsiz kargo${code}`
    case 'coupon_code':
      return `Kupon kodu ${o.code}`
    default:
      return 'Bilinmeyen teklif'
  }
}
