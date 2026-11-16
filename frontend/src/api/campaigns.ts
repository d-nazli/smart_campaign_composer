import type { ApproveResult, Campaign, CampaignSummary, LanguageCode, MessageContent } from '../types'
import { requestJSON } from './http'

export function createCampaign(brief: string): Promise<Campaign> {
  return requestJSON<Campaign>('/api/campaigns', { method: 'POST', body: JSON.stringify({ brief }) })
}

export function getCampaign(id: number): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}`)
}

/** Saves the text of every language at once: the whole set must say the same thing. */
export function saveContent(id: number, languages: Record<LanguageCode, MessageContent>): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/content`, { method: 'PUT', body: JSON.stringify({ languages }) })
}

/** Approves the campaign and queues the sending. Safe to repeat: a second call reports the same queue. */
export function approveCampaign(id: number): Promise<ApproveResult> {
  return requestJSON<ApproveResult>(`/api/campaigns/${id}/approve`, { method: 'POST' })
}

/** Newest campaigns first, with sending progress for the approved ones. */
export async function listCampaigns(limit = 100): Promise<CampaignSummary[]> {
  const res = await requestJSON<{ campaigns: CampaignSummary[] }>(`/api/campaigns?limit=${limit}`)
  return res.campaigns
}
