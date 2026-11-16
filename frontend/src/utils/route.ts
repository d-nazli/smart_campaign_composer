// The address bar names the campaign being worked on (?c=12). These helpers keep
// that rule in one place so "new campaign" and "open campaign" cannot disagree.

/** The campaign id in the address, or null when there is none (or it is not a positive integer). */
export function campaignIdFromUrl(href: string): number | null {
  const raw = new URL(href).searchParams.get('c')
  if (raw === null || !/^\d+$/.test(raw)) return null
  const id = Number(raw)
  return Number.isSafeInteger(id) && id > 0 ? id : null
}

/** The same address without ?c= (other parameters stay). The hash is replaced by `hash`. */
export function urlWithoutCampaign(href: string, hash = ''): string {
  const u = new URL(href)
  u.searchParams.delete('c')
  return u.pathname + u.search + hash
}

/** The same address pointing at one campaign. The hash is replaced by `hash`. */
export function urlForCampaign(href: string, id: number, hash = ''): string {
  const u = new URL(href)
  u.searchParams.set('c', String(id))
  return u.pathname + u.search + hash
}
