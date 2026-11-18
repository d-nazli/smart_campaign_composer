import type { ApprovalCounts, Campaign, ChannelProgress } from '../types'

/** Share of a channel's recipients that reached a final state (0-100). Never 100 while one is pending. */
export function channelPercent(p: ChannelProgress): number {
  if (p.total === 0) return 100
  const final = p.sent + p.failed + p.skipped
  const pct = Math.round((final / p.total) * 100)
  return final < p.total ? Math.min(pct, 99) : 100
}

/** "6 gönderildi · 2 başarısız · …": what happened to everyone, zero counts left out. */
export function describeChannel(p: ChannelProgress): string {
  if (p.total === 0) return 'Alıcı yok'
  const parts: string[] = []
  if (p.sent) parts.push(`${p.sent} gönderildi`)
  if (p.failed) parts.push(`${p.failed} başarısız`)
  if (p.skipped) parts.push(`${p.skipped} atlandı`)
  if (p.pending) parts.push(`${p.pending} bekliyor`)
  return parts.join(' · ')
}

/** True while the screen should keep asking the server for progress. */
export function shouldPoll(c: Campaign): boolean {
  // The server marks a campaign completed a moment after its last row is final,
  // so "completed" (not just "done") is what ends the polling.
  // A scheduled campaign is watched too: the screen switches to the progress when it starts.
  return c.status === 'scheduled' || c.status === 'approved' || c.status === 'sending'
}

const fmt = (n: number) => n.toLocaleString('tr-TR')

/**
 * "Onaydan sonra 27 SMS alıcısı değişti (1.677 → 1.650)." when the deliveries really queued differ from
 * the recipients on screen at approval; null when they are the same (or not queued yet).
 */
export function approvalChange(a: ApprovalCounts | undefined): string | null {
  if (!a) return null
  const parts = [
    { label: 'SMS', was: a.shown.sms, now: a.queued.sms },
    { label: 'e-posta', was: a.shown.email, now: a.queued.email },
  ]
    .filter((c) => c.was !== c.now)
    .map((c) => `${fmt(Math.abs(c.was - c.now))} ${c.label} alıcısı (${fmt(c.was)} → ${fmt(c.now)})`)
  if (parts.length === 0) return null
  if (parts.length === 1) return `Onaydan sonra ${parts[0].replace(' (', ' değişti (')}.`
  return `Onaydan sonra ${parts.join(' ve ')} değişti.`
}
