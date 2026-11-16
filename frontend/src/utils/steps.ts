import type { Campaign } from '../types'

export type StepKey = 'brief' | 'audience' | 'messages' | 'approve' | 'send'

export const STEPS: { key: StepKey; label: string }[] = [
  { key: 'brief', label: 'Brief' },
  { key: 'audience', label: 'Kitle' },
  { key: 'messages', label: 'Mesajlar' },
  { key: 'approve', label: 'Onay' },
  { key: 'send', label: 'Gönderim' },
]

export interface StepInput {
  campaign: Campaign | null
  /** A message generation is running right now. */
  streaming: boolean
  /** Some message text exists (generated or saved). */
  hasContent: boolean
  /** The editor holds changes that are not saved yet. */
  unsaved: boolean
}

/** How far the campaign has come. Everything before `current` counts as done. */
export function currentStep(i: StepInput): { current: StepKey; done: StepKey[] } {
  const c = i.campaign
  let current: StepKey
  // A brief whose audience is not usable waits on the audience step: that is where the question or the reason is shown.
  if (!c) current = 'brief'
  else if (c.segment?.status !== 'ok' || c.status === 'draft') current = 'audience'
  else if (c.status === 'completed' || c.status === 'approved' || c.status === 'sending') current = 'send'
  else if (i.streaming || i.unsaved) current = 'messages'
  else if (c.status === 'content_ready') current = 'approve'
  else current = i.hasContent ? 'messages' : 'audience'

  const at = STEPS.findIndex((s) => s.key === current)
  const done = STEPS.slice(0, at).map((s) => s.key)
  // A finished campaign has nothing left to do: the last step is done too.
  if (c?.status === 'completed') done.push('send')
  return { current, done }
}
