// Shapes of the API responses (see backend/internal/campaign and httpapi).

export type ContentField = 'sms_body' | 'email_subject' | 'email_body'

export interface MessageContent {
  sms_body: string
  email_subject: string
  email_body: string
}

/** A locale code (BCP 47, "en-GB"): one row of the locales table (see GET /api/info). */
export type LocaleCode = string

export interface LocaleInfo {
  code: LocaleCode
  /** English name. */
  name: string
  /** The locale's own name. */
  native_name: string
  direction: 'ltr' | 'rtl'
  /** The locale every campaign is written in first; the others are translated from it. */
  is_source: boolean
  fallbacks?: Record<string, string>
}

export interface Issue {
  field: ContentField
  code: string
  message: string
  length?: number
  limit?: number
  /** Set when the issue belongs to one locale of the campaign. */
  locale?: LocaleCode
  /** "B" when the issue belongs to Variant B (absent: Variant A). */
  variant?: Variant
}

/** An A/B variant of a campaign. Every campaign has A; B is optional. */
export type Variant = 'A' | 'B'

/** How many people one variant reaches: people on at least one channel, SMS and e-mail deliveries. */
export interface VariantCount {
  people: number
  sms: number
  email: number
}

/** One variant's texts, like the top-level fields of Campaign for Variant A. */
export interface VariantView {
  content?: StoredContent
  translations?: Record<LocaleCode, StoredContent>
  pending_locales: LocaleCode[]
  mismatches: Issue[]
  /** The same text as Variant A: the campaign cannot be approved. */
  duplicate: boolean
}

/** One variant's share of a campaign's deliveries. */
export interface VariantProgress {
  variant: Variant
  people: number
  sms: { total: number; sent: number }
  email: { total: number; sent: number }
  failed: number
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
  /** Tiered discount: `value` TL for every `step_spend` TL of the basket; 0 or absent = not tiered. */
  step_spend?: number
  /** Cap of the discount in TL (total of a tier, or the most a percentage takes off); 0 or absent = none. */
  max_discount?: number
  /** Other incentives promised together with the main one ("%20 indirim + ücretsiz kargo"). */
  extras?: OfferPart[] | null
}

/** One incentive of a combined offer. */
export interface OfferPart {
  kind: 'percent' | 'amount' | 'free_shipping'
  value: number
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
  /** A sentence shown with an ok audience, e.g. why everyone was chosen. Set by the backend, never by the model. */
  note?: string
  /** How many reachable people each locale has, e.g. {"tr-TR": 120, "en-GB": 45}. */
  locales?: Record<LocaleCode, number>
  /** Parts of the brief the rule leaves out or turns around; shown, never blocking. Set by the backend. */
  warnings?: string[]
}

export type ContentSource = 'llm' | 'fallback' | 'edited' | 'translated'

/** The deliberate angle of a variant's source text, chosen by the model from a fixed list. */
export type Approach = 'aciliyet' | 'duygusal' | 'fayda' | 'merak' | 'kisisel'

export interface StoredContent extends MessageContent {
  locale: LocaleCode
  /** The source text's approach (absent on translations, the built-in template and older texts). */
  approach?: Approach
  source: ContentSource
  /** Changes of the source text; a translation keeps the source version it was made from. */
  version: number
  source_version?: number
  /** A translation made from an older source version. */
  stale: boolean
  /** The source text a stale translation was made from. */
  source_text?: MessageContent
  report: Report
  updated_at: string
}

export type CampaignStatus =
  | 'draft'
  | 'segment_ready'
  | 'content_ready'
  /** Approved for a later time: nothing is queued and the audience is decided when it starts. */
  | 'scheduled'
  | 'approved'
  | 'sending'
  | 'completed'
  /** A scheduled campaign a person cancelled before it started. */
  | 'cancelled'
  /** A scheduled campaign that could not start (see schedule_error). */
  | 'failed'

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
  /** When the first and the last message of this channel were accepted (absent until one was). */
  first_sent_at?: string
  last_sent_at?: string
  /** The most provider calls started in any one second (only on the campaign's own page; 0 or absent in the list). */
  peak_per_second?: number
}

/** Why some deliveries failed: a code from the API (rejected, gave_up, too_old, too_long, other). */
export interface FailureReason {
  channel: 'sms' | 'email'
  reason: string
  count: number
}

/** Sending progress of an approved campaign. */
export interface DeliveryProgress {
  sms: ChannelProgress
  email: ChannelProgress
  /** Nothing is pending in either channel. */
  done: boolean
  /** Failed deliveries grouped by reason, largest first (absent when none failed). */
  failures?: FailureReason[]
  /** Messages (SMS and email together) per locale: how many go there and how many were sent. */
  locales?: Record<LocaleCode, { total: number; sent: number }>
  /** Variant A against Variant B (only for a campaign sent with B). */
  variants?: VariantProgress[]
}

export interface ChannelCounts {
  sms: number
  email: number
}

export interface ApprovalCounts {
  /** Reachable recipients on screen when the campaign was approved. */
  shown: ChannelCounts
  /** Deliveries in the queue, every variant and locale. */
  queued: ChannelCounts
}

export interface ApproveResult {
  id: number
  status: CampaignStatus
  already_approved: boolean
  /** The deliveries queued (absent for a scheduled campaign that has not started). */
  queued?: ChannelCounts
  /** When a scheduled campaign starts (status "scheduled"). */
  scheduled_at?: string
  /** Audience counted at approval time (absent on a repeated approval). */
  audience?: Audience
  /** How many people receive each locale. */
  locales?: Record<LocaleCode, number>
  /** How many people each variant gets (with Variant B only). */
  variants?: Record<Variant, VariantCount>
  warnings: Issue[]
}

export interface Campaign {
  id: number
  brief: string
  /** The model's short campaign name (at most 40 characters); absent for older campaigns. */
  name?: string
  status: CampaignStatus
  segment?: Segment
  /** The locale the text is written in first. */
  source_locale?: LocaleCode
  /** The campaign's locales: the source and every locale with a reachable person, source first. */
  locales?: LocaleCode[]
  /** The source text; the translations are in `translations`. */
  content?: StoredContent
  translations?: Record<LocaleCode, StoredContent>
  /** Translations of the campaign that are missing or stale: approval waits for them. */
  pending_locales?: LocaleCode[]
  /** Up-to-date translations that do not say what the source says. */
  mismatches?: Issue[]
  /** Only in the answer to a translate request: the locales that could not be translated, with why. */
  translation_failures?: Record<LocaleCode, string>
  /** ["A"], or ["A", "B"] once Variant B was written. content / translations / pending_locales / mismatches are Variant A's. */
  variants?: Variant[]
  /** Variant B's texts (absent without B). */
  variant_b?: VariantView
  /** How many people each variant reaches (with Variant B only). */
  variant_counts?: Record<Variant, VariantCount>
  /** Present once the campaign has been approved. */
  delivery?: DeliveryProgress
  /**
   * Recipients on screen at approval against the deliveries really queued (rows of every
   * variant and locale). Present once the deliveries exist; the screen mentions it only when they differ.
   */
  approval?: ApprovalCounts
  /** When a scheduled campaign starts; why it could not start. */
  scheduled_at?: string
  schedule_error?: string
  created_at: string
}

export interface VariableInfo {
  name: string
  budget: number
  fallback: string
  /** The replacement word in every locale. */
  fallbacks?: Record<LocaleCode, string>
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
  /** The model's short campaign name (at most 40 characters); absent for older campaigns. */
  name?: string
  status: CampaignStatus
  segment_status?: SegmentStatus
  matched?: number
  /** The audience in words. */
  explanation?: string
  /** How many reachable people each locale has (an estimate until the campaign starts). */
  audience_locales?: Record<LocaleCode, number>
  delivery?: DeliveryProgress
  scheduled_at?: string
  schedule_error?: string
  created_at: string
}
