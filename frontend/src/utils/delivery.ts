import type { Audience, Campaign, ChannelProgress } from '../types'

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

/** A sentence when the queued numbers differ from what was shown before approval, otherwise null. */
export function approvalDifference(queued: { sms: number; email: number }, shown: Audience | undefined): string | null {
  if (!shown) return null
  const sentence = (label: string, was: number, now: number) =>
    was === now ? null : `Ekranda ${was} ${label} gösterilmişti; onay anında ${now} ${label} kuyruğa alındı`
  const parts = [
    sentence('SMS', shown.sms_reachable, queued.sms),
    sentence('e-posta', shown.email_reachable, queued.email),
  ].filter((s): s is string => s !== null)
  if (parts.length === 0) return null
  return parts.join('. ') + ' (veriler değişmiş olabilir).'
}
