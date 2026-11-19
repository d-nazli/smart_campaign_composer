import type { ApproveResult, Campaign, CampaignSummary, LocaleCode, MessageContent, Variant } from '../types'
import { requestJSON } from './http'

export function createCampaign(brief: string): Promise<Campaign> {
  return requestJSON<Campaign>('/api/campaigns', { method: 'POST', body: JSON.stringify({ brief }) })
}

export function getCampaign(id: number): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}`)
}

/** Saves a variant's edited source text (only the source is edited by hand). A changed source makes that variant's translations stale. */
export function saveContent(id: number, locales: Record<LocaleCode, MessageContent>, variant: Variant = 'A'): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/content`, { method: 'PUT', body: JSON.stringify({ locales, variant }) })
}

/** Translates a variant's source again into the locales (translations are never written by hand). */
export function translateCampaign(id: number, locales: LocaleCode[], variant: Variant = 'A'): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/translations`, { method: 'POST', body: JSON.stringify({ locales, variant }) })
}

/** Removes Variant B: everyone gets Variant A. */
export function deleteVariant(id: number, variant: Variant = 'B'): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/variants/${variant}`, { method: 'DELETE' })
}

/** Approves the campaign and queues the sending. Safe to repeat: a second call reports the same queue. */
export function approveCampaign(id: number, scheduledAt?: string): Promise<ApproveResult> {
  const init: RequestInit = { method: 'POST' }
  // No body sends now; a time (RFC 3339) schedules the sending.
  if (scheduledAt) init.body = JSON.stringify({ scheduled_at: scheduledAt })
  return requestJSON<ApproveResult>(`/api/campaigns/${id}/approve`, init)
}

/**
 * Answers the audience question of a campaign that waits for one: the audience is resolved
 * again on the SAME campaign (no new campaign). Refused (409) when it waits for no answer.
 */
export function clarifyCampaign(id: number, answer: string): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/clarify`, { method: 'POST', body: JSON.stringify({ answer }) })
}

/**
 * Replaces the brief of a campaign that was never approved: the audience is resolved again on
 * the SAME campaign (its old texts and answers go). Refused (409) once the campaign is approved.
 */
export function updateBrief(id: number, brief: string): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/brief`, { method: 'PUT', body: JSON.stringify({ brief }) })
}

/** Deletes a campaign that was never approved. Refused (409) once it is approved. */
export function deleteCampaign(id: number): Promise<void> {
  return requestJSON<void>(`/api/campaigns/${id}`, { method: 'DELETE' })
}

/** Cancels a scheduled campaign that has not started. Refused (409) once it has started. */
export function cancelCampaign(id: number): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/cancel`, { method: 'POST' })
}

/** Newest campaigns first, with sending progress for the approved ones. */
export async function listCampaigns(limit = 100): Promise<CampaignSummary[]> {
  const res = await requestJSON<{ campaigns: CampaignSummary[] }>(`/api/campaigns?limit=${limit}`)
  return res.campaigns
}
