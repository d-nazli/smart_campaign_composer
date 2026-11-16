import { STEPS, type StepKey } from './steps'

export const stepIndex = (k: StepKey): number => STEPS.findIndex((s) => s.key === k)

export const nextStep = (k: StepKey): StepKey => STEPS[Math.min(STEPS.length - 1, stepIndex(k) + 1)].key
export const prevStep = (k: StepKey): StepKey => STEPS[Math.max(0, stepIndex(k) - 1)].key

/** Slide direction: forward moves the panel to the left, back moves it to the right. */
export function direction(from: StepKey, to: StepKey): 'forward' | 'back' {
  return stepIndex(to) >= stepIndex(from) ? 'forward' : 'back'
}

/** Steps the user may open: everything up to the furthest step reached. No jumping ahead. */
export function reachableSteps(furthest: StepKey): StepKey[] {
  return STEPS.slice(0, stepIndex(furthest) + 1).map((s) => s.key)
}

export type BarAction =
  | 'create' // build the campaign from the brief
  | 'next' // just open the next step
  | 'generate' // start writing the messages
  | 'save-next' // save the edits, then open the approval step
  | 'ask' // first click of the two-step approval
  | 'send' // second click: really approve and queue
  | 'dashboard'

export interface BarInput {
  view: StepKey
  /** The campaign is approved: every earlier step is read-only. */
  frozen: boolean
  hasCampaign: boolean
  /** The brief text differs from the one the campaign was built from. */
  briefChanged: boolean
  hasContent: boolean
  confirming: boolean
  /** The person chose to send later (the main button says "Zamanla"). */
  scheduling?: boolean
}

/** The main button of the sticky bottom bar for the step being viewed. */
export function barAction(i: BarInput): { action: BarAction; label: string } {
  const next = { action: 'next' as const, label: 'İleri' }
  switch (i.view) {
    case 'brief':
      // Going back to an unchanged brief must not rebuild the campaign (it would throw the messages away).
      return !i.frozen && (!i.hasCampaign || i.briefChanged) ? { action: 'create', label: 'Kitleyi çıkar' } : next
    case 'audience':
      return i.frozen || i.hasContent ? next : { action: 'generate', label: 'Mesajları üret' }
    case 'messages':
      return i.frozen ? next : { action: 'save-next', label: 'Onayla' }
    case 'approve':
      if (i.frozen) return next
      if (i.scheduling) return i.confirming ? { action: 'send', label: 'Evet, zamanla' } : { action: 'ask', label: 'Zamanla' }
      return i.confirming ? { action: 'send', label: 'Evet, gönder' } : { action: 'ask', label: 'Gönder' }
    default:
      return { action: 'dashboard', label: "Dashboard'a git" }
  }
}
