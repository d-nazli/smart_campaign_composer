import { describe, expect, it } from 'vitest'
import { LOCKED_MESSAGE, conflictMessage, isFrozen } from './editing'

describe('the text is frozen from the approval on', () => {
  it('every state after approval freezes it, the states before do not', () => {
    for (const s of ['scheduled', 'approved', 'sending', 'completed', 'cancelled', 'failed']) expect(isFrozen(s), s).toBe(true)
    for (const s of ['draft', 'segment_ready', 'content_ready', undefined, null]) expect(isFrozen(s), String(s)).toBe(false)
  })
})

describe('a write refused with 409', () => {
  it('says the campaign was approved when the server now has it approved', () => {
    expect(conflictMessage('kampanya "approved" durumunda; içerik düzenlenemez', 'approved')).toBe(LOCKED_MESSAGE)
    expect(conflictMessage('x', 'scheduled')).toBe('Kampanya onaylandı, içerik artık değiştirilemez.')
  })
  it('keeps the server message for any other refusal', () => {
    expect(conflictMessage('kampanya "draft" durumunda; içerik düzenlenemez', 'draft')).toBe('kampanya "draft" durumunda; içerik düzenlenemez')
  })
})
