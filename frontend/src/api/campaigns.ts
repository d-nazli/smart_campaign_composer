import type { ApproveResult, Campaign, MessageContent } from '../types'
import { requestJSON } from './http'

export function createCampaign(brief: string): Promise<Campaign> {
  return requestJSON<Campaign>('/api/campaigns', { method: 'POST', body: JSON.stringify({ brief }) })
}

export function getCampaign(id: number): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}`)
}

export function saveContent(id: number, content: MessageContent): Promise<Campaign> {
  return requestJSON<Campaign>(`/api/campaigns/${id}/content`, { method: 'PUT', body: JSON.stringify(content) })
}

/** Approves the campaign and queues the sending. Safe to repeat: a second call reports the same queue. */
export function approveCampaign(id: number): Promise<ApproveResult> {
  return requestJSON<ApproveResult>(`/api/campaigns/${id}/approve`, { method: 'POST' })
}
