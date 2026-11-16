// Shapes of the API responses (see backend/internal/campaign and httpapi).

export type ContentField = 'sms_body' | 'email_subject' | 'email_body'

export interface MessageContent {
  sms_body: string
  email_subject: string
  email_body: string
}

export interface Issue {
  field: ContentField
  code: string
  message: string
  length?: number
  limit?: number
}

export interface Report {
  ok: boolean
  issues: Issue[]
  /** Quality problems that do not block saving: offer missing, repeated word. */
  warnings?: Issue[]
  sms_length: number
  subject_length: number
}

export type ConditionValue = string | number | boolean | string[]

export interface Condition {
  field: string
  op: string
  value: ConditionValue
}

export interface Audience {
  matched: number
  sms_reachable: number
  email_reachable: number
  both_reachable: number
}

export type SegmentStatus = 'ok' | 'needs_clarification' | 'unsupported'

export type OfferKind = 'none' | 'percent' | 'amount' | 'free_shipping' | 'coupon_code'

/** The incentive the brief promises; the SMS and the email must state it. */
export interface Offer {
  kind: OfferKind
  value: number
  code: string
  /** Smallest basket (TRY) the offer applies to; 0 or absent = no minimum. A term of the offer, not an audience. */
  min_spend?: number
  /** Last day the offer can be used, YYYY-MM-DD; empty or absent = none. */
  expires_at?: string
}

export interface Segment {
  status: SegmentStatus
  match?: 'all' | 'any'
  conditions: Condition[]
  explanation: string
  question?: string
  audience?: Audience
  offer?: Offer
  /** A one-click answer to `question`: "all_customers" offers the "send to all" button. */
  suggestion?: 'all_customers'
}

export type ContentSource = 'llm' | 'fallback' | 'edited'

export interface StoredContent extends MessageContent {
  source: ContentSource
  report: Report
  updated_at: string
}

export type CampaignStatus = 'draft' | 'segment_ready' | 'content_ready' | 'approved' | 'sending' | 'completed'

/** How far one channel has come. `pending` includes rows in flight and waiting for a retry. */
export interface ChannelProgress {
  /** Times deliveries went back to the queue (429s included) and how many of those were HTTP 429. */
  retries: number
  rate_limited: number
  total: number
  pending: number
  sent: number
  failed: number
  skipped: number
}

/** Sending progress of an approved campaign. */
export interface DeliveryProgress {
  sms: ChannelProgress
  email: ChannelProgress
  /** Nothing is pending in either channel. */
  done: boolean
}

export interface ApproveResult {
  id: number
  status: CampaignStatus
  already_approved: boolean
  queued: { sms: number; email: number }
  /** Audience counted at approval time (absent on a repeated approval). */
  audience?: Audience
  warnings: Issue[]
}

export interface Campaign {
  id: number
  brief: string
  status: CampaignStatus
  segment?: Segment
  content?: StoredContent
  /** Present once the campaign has been approved. */
  delivery?: DeliveryProgress
  created_at: string
}

export interface VariableInfo {
  name: string
  budget: number
  fallback: string
}

export interface Limits {
  sms_max: number
  subject_max: number
  email_body_max: number
}

/** One line of the campaign list (the dashboard). */
export interface CampaignSummary {
  id: number
  brief: string
  status: CampaignStatus
  segment_status?: SegmentStatus
  matched?: number
  delivery?: DeliveryProgress
  created_at: string
}
