/**
 * The sentence that answers "Bu kampanya kime gönderilsin? Tüm müşterilere mi?".
 * It is added to the brief (so the brief stays the one source of truth, and the
 * backend reads it like any other brief), not sent as a hidden flag.
 */
export const SEND_TO_ALL_SENTENCE = 'Tüm müşterilere gönder.'

export function withAllCustomers(brief: string): string {
  const b = brief.trimEnd()
  if (b.endsWith(SEND_TO_ALL_SENTENCE)) return b
  return `${b}\n\n${SEND_TO_ALL_SENTENCE}`
}
