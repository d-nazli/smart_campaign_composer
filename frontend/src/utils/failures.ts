import type { FailureReason } from '../types'

const TEXT: Record<string, string> = {
  rejected: 'Sağlayıcı alıcıyı reddetti (geçersiz numara ya da adres)',
  gave_up: 'Tekrar denemeler tükendi, sağlayıcı yanıt vermedi',
  too_old: 'Çok uzun beklediği için gönderilmedi',
  too_long: 'Kişiselleştirme sonrası karakter sınırını aştığı için gönderilmedi',
  other: 'Başka bir nedenle gönderilemedi',
}

/** One plain sentence for a failure code from the API. */
export function failureText(reason: string): string {
  return TEXT[reason] ?? TEXT.other
}

/** Failures of both channels merged by reason, largest first (for the dashboard card). */
export function mergeFailures(list: FailureReason[] | undefined): { reason: string; count: number }[] {
  const by = new Map<string, number>()
  for (const f of list ?? []) by.set(f.reason, (by.get(f.reason) ?? 0) + f.count)
  return [...by].map(([reason, count]) => ({ reason, count })).sort((a, b) => b.count - a.count)
}
