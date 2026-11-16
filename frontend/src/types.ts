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
}

export interface Segment {
  status: SegmentStatus
  match?: 'all' | 'any'
  conditions: Condition[]
  explanation: string
  question?: string
  audience?: Audience
  offer?: Offer
}

export type ContentSource = 'llm' | 'fallback' | 'edited'

export interface StoredContent extends MessageContent {
  source: ContentSource
  report: Report
  updated_at: string
}

export type CampaignStatus = 'draft' | 'segment_ready' | 'content_ready' | 'approved' | 'sending' | 'completed'

export interface Campaign {
  id: number
  brief: string
  status: CampaignStatus
  segment?: Segment
  content?: StoredContent
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
