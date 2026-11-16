import { describe, expect, it } from 'vitest'
import type { Campaign, CampaignStatus, CampaignSummary } from '../types'
import { statusLabel, statusTone } from './dashboard'
import { shouldPoll } from './delivery'
import { currentStep } from './steps'
import { barAction } from './wizard'

const summary = (status: CampaignStatus): Pick<CampaignSummary, 'status' | 'segment_status'> => ({ status, segment_status: 'ok' })
const campaign = (status: CampaignStatus): Campaign => ({
  id: 1,
  brief: 'b',
  status,
  segment: { status: 'ok', conditions: [], explanation: '' },
  created_at: '',
})
const input = { streaming: false, hasContent: true, unsaved: false }

describe('the states of a scheduled campaign', () => {
  it('are named in plain Turkish', () => {
    expect(statusLabel(summary('scheduled'))).toBe('Zamanlandı')
    expect(statusLabel(summary('cancelled'))).toBe('İptal edildi')
    expect(statusLabel(summary('failed'))).toBe('Başlatılamadı')
  })

  it('get their own colour; a campaign that could not start asks for attention', () => {
    expect(statusTone(summary('scheduled'))).toBe('scheduled')
    expect(statusTone(summary('failed'))).toBe('attention')
    expect(statusTone(summary('cancelled'))).toBe('draft')
  })

  it('is watched until it starts, so the screen can switch to the progress', () => {
    expect(shouldPoll(campaign('scheduled'))).toBe(true)
    expect(shouldPoll(campaign('cancelled'))).toBe(false)
    expect(shouldPoll(campaign('failed'))).toBe(false)
  })

  it('all show the last step: there is nothing left to edit', () => {
    for (const s of ['scheduled', 'cancelled', 'failed'] as const) {
      expect(currentStep({ campaign: campaign(s), ...input }).current).toBe('send')
    }
  })
})

describe('the main button when sending later', () => {
  const base = { view: 'approve' as const, frozen: false, hasCampaign: true, briefChanged: false, hasContent: true }

  it('says "Zamanla" and then "Evet, zamanla"', () => {
    expect(barAction({ ...base, confirming: false, scheduling: true })).toEqual({ action: 'ask', label: 'Zamanla' })
    expect(barAction({ ...base, confirming: true, scheduling: true })).toEqual({ action: 'send', label: 'Evet, zamanla' })
  })

  it('still says "Gönder" when sending now', () => {
    expect(barAction({ ...base, confirming: false })).toEqual({ action: 'ask', label: 'Gönder' })
    expect(barAction({ ...base, confirming: true, scheduling: false })).toEqual({ action: 'send', label: 'Evet, gönder' })
  })
})
