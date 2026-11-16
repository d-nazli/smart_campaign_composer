import { describe, expect, it } from 'vitest'
import { barAction, direction, nextStep, prevStep, reachableSteps, type BarInput } from './wizard'

const base: BarInput = { view: 'brief', frozen: false, hasCampaign: false, briefChanged: false, hasContent: false, confirming: false }
const bar = (o: Partial<BarInput>) => barAction({ ...base, ...o })

describe('wizard navigation', () => {
  it('slides forward to the left and back to the right', () => {
    expect(direction('brief', 'audience')).toBe('forward')
    expect(direction('approve', 'audience')).toBe('back')
  })
  it('stays inside the five steps', () => {
    expect(nextStep('send')).toBe('send')
    expect(prevStep('brief')).toBe('brief')
    expect(nextStep('audience')).toBe('messages')
  })
  it('allows only steps up to the furthest one reached', () => {
    expect(reachableSteps('messages')).toEqual(['brief', 'audience', 'messages'])
    expect(reachableSteps('brief')).toEqual(['brief'])
  })
})

describe('the main button of each step', () => {
  it('builds the audience from a new or edited brief, otherwise just moves on', () => {
    expect(bar({}).action).toBe('create')
    expect(bar({ hasCampaign: true, briefChanged: true }).action).toBe('create')
    expect(bar({ hasCampaign: true, briefChanged: false }).action).toBe('next')
  })
  it('writes messages once, then only moves on', () => {
    expect(bar({ view: 'audience' })).toEqual({ action: 'generate', label: 'Mesajları üret' })
    expect(bar({ view: 'audience', hasContent: true }).action).toBe('next')
  })
  it('saves and continues from the messages step', () => {
    expect(bar({ view: 'messages' })).toEqual({ action: 'save-next', label: 'Onayla' })
  })
  it('approves in two steps', () => {
    expect(bar({ view: 'approve' })).toEqual({ action: 'ask', label: 'Gönder' })
    expect(bar({ view: 'approve', confirming: true }).action).toBe('send')
  })
  it('ends at the dashboard', () => {
    expect(bar({ view: 'send', frozen: true })).toEqual({ action: 'dashboard', label: "Dashboard'a git" })
  })
  it('opens earlier steps read-only: no action that changes anything', () => {
    for (const view of ['brief', 'audience', 'messages', 'approve'] as const) {
      expect(bar({ view, frozen: true, hasCampaign: true, hasContent: true }).action).toBe('next')
    }
  })
})
