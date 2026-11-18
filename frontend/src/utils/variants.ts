import type { Approach, Campaign, Issue, LocaleCode, Variant, VariantCount, VariantProgress } from '../types'

/** Every variant a campaign can have, in order. There is no third one. */
export const VARIANTS: Variant[] = ['A', 'B']

/** How an approach reads on a tab. */
export const APPROACH_LABELS: Record<Approach, string> = {
  aciliyet: 'Aciliyet',
  duygusal: 'Duygusal',
  fayda: 'Fayda',
  merak: 'Merak',
  kisisel: 'Kişisel',
}

/** A tab's text: "Varyant A · Merak", or "Varyant A" for a text without an approach. */
export function variantLabel(c: Campaign | null, v: Variant): string {
  const a = forVariant(c, v)?.content?.approach
  return a ? `Varyant ${v} · ${APPROACH_LABELS[a] ?? a}` : `Varyant ${v}`
}

/** The campaign's variants: A, plus B once it was written. */
export function variantsOf(c: Campaign | null): Variant[] {
  return c?.variants?.length ? c.variants : ['A']
}

export const hasVariantB = (c: Campaign | null): boolean => variantsOf(c).includes('B')

/**
 * The campaign seen through one variant: content, translations, pending_locales and mismatches are
 * that variant's, everything else (status, segment, locales, delivery) is the campaign's. The locale
 * helpers (storedByLocale, localeState, draftsFrom, ...) then work on either variant unchanged.
 */
export function forVariant(c: Campaign | null, v: Variant): Campaign | null {
  if (!c || v === 'A') return c
  const b = c.variant_b
  return { ...c, content: b?.content, translations: b?.translations, pending_locales: b?.pending_locales ?? [], mismatches: b?.mismatches ?? [] }
}

/** The translations waiting in one variant. */
export const pendingOf = (c: Campaign | null, v: Variant): LocaleCode[] => forVariant(c, v)?.pending_locales ?? []

/** Variant B is the same text as Variant A. */
export const isDuplicate = (c: Campaign | null): boolean => !!c?.variant_b?.duplicate

/** An issue's variant ("A" when the server names none). */
export const issueVariant = (i: Issue): Variant => i.variant ?? 'A'

/** The split shown at approval: each variant's share of the people (rounded, adds up to 100). */
export function splitShares(counts: Record<Variant, VariantCount> | undefined): { variant: Variant; share: number; count: VariantCount }[] {
  if (!counts?.A || !counts?.B) return []
  const total = counts.A.people + counts.B.people
  const a = total > 0 ? Math.round((counts.A.people / total) * 100) : 50
  return [
    { variant: 'A', share: a, count: counts.A },
    { variant: 'B', share: 100 - a, count: counts.B },
  ]
}

/** The comparison table of the sending step, with a total row. */
export function comparisonRows(variants: VariantProgress[] | undefined): (VariantProgress & { label: string })[] {
  if (!variants || variants.length < 2) return []
  const rows = variants.map((v) => ({ ...v, label: `Varyant ${v.variant}` }))
  const total = rows.reduce(
    (t, v) => ({
      ...t,
      people: t.people + v.people,
      sms: { total: t.sms.total + v.sms.total, sent: t.sms.sent + v.sms.sent },
      email: { total: t.email.total + v.email.total, sent: t.email.sent + v.email.sent },
      failed: t.failed + v.failed,
    }),
    { variant: 'A' as Variant, label: 'Toplam', people: 0, sms: { total: 0, sent: 0 }, email: { total: 0, sent: 0 }, failed: 0 },
  )
  return [...rows, total]
}
