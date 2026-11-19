// Editing a campaign's text ends with the approval: from then on the server refuses every
// change with 409 (the text is what gets sent). These helpers keep that rule in one place.

/** The states after approval: the text is frozen, earlier steps still open, read-only. */
const FROZEN = ['scheduled', 'approved', 'sending', 'completed', 'cancelled', 'failed']

export const isFrozen = (status: string | null | undefined): boolean => FROZEN.includes(status ?? '')

export const LOCKED_MESSAGE = 'Kampanya onaylandı, içerik artık değiştirilemez.'

/**
 * What to show for a write the server refused with 409, given the campaign's status as the
 * server has it now (fetched again after the refusal): approved in the meantime (another tab,
 * another person) gets the fixed sentence; any other refusal keeps the server's message.
 */
export function conflictMessage(serverMessage: string, statusNow: string | null | undefined): string {
  return isFrozen(statusNow) ? LOCKED_MESSAGE : serverMessage
}
