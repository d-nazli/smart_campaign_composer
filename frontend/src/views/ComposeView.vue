<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AppInfo } from '../api/info'
import { approveCampaign, cancelCampaign, createCampaign, deleteVariant, getCampaign, saveContent, translateCampaign } from '../api/campaigns'
import { ApiError } from '../api/http'
import { useGenerationStream } from '../composables/useGenerationStream'
import { failedLocales, initialState, retryNotes } from '../composables/streamState'
import type { ApproveResult, Campaign, ContentField, Issue, LocaleCode, MessageContent, Variant } from '../types'
import { VARIANTS, forVariant, hasVariantB, isDuplicate, issueVariant, pendingOf, variantsOf } from '../utils/variants'
import { approvalDifference, shouldPoll } from '../utils/delivery'
import {
  FALLBACK_LOCALES,
  SOURCE_LOCALE,
  campaignLocales,
  changedLocales,
  countsLine,
  draftsFrom,
  emptyMessage,
  localeInfo,
  localeState,
  pendingLocales,
  stateLabel,
  storedByLocale,
} from '../utils/locales'
import { checkContent } from '../utils/limits'
import { campaignIdFromUrl, urlForCampaign, urlWithoutCampaign } from '../utils/route'
import { checkSchedule, formatWhen, fromLocalInput, toLocalInput, untilText } from '../utils/schedule'
import { ArrowLeft, Clock, FileText, Plus, RefreshCw, Sparkles, XCircle } from 'lucide-vue-next'
import { briefTitle, statusLabel } from '../utils/dashboard'
import { attemptLabel } from '../utils/preview'
import { withAllCustomers } from '../utils/sendToAll'
import { currentStep, type StepKey } from '../utils/steps'
import { barAction, direction, nextStep, prevStep, reachableSteps } from '../utils/wizard'
import ApprovalCard from '../components/ApprovalCard.vue'
import DeliveryProgress from '../components/DeliveryProgress.vue'
import LocaleList, { type LocaleStatus } from '../components/LocaleList.vue'
import MessageEditor from '../components/MessageEditor.vue'
import SegmentCard from '../components/SegmentCard.vue'
import StepBar from '../components/StepBar.vue'
import VariantTabs from '../components/VariantTabs.vue'
import Badge from '../components/ui/Badge.vue'
import Button from '../components/ui/Button.vue'
import Card from '../components/ui/Card.vue'
import KpiGroup from '../components/ui/KpiGroup.vue'
import Modal from '../components/ui/Modal.vue'
import StatTile from '../components/ui/StatTile.vue'

const props = defineProps<{ info: AppInfo | null }>()

// A sensible first choice for "later": the next full hour, at least an hour from now.
function defaultSendTime(): Date {
  const d = new Date(Date.now() + 60 * 60 * 1000)
  d.setMinutes(0, 0, 0)
  d.setHours(d.getHours() + 1)
  return d
}

const EXAMPLE_BRIEF =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

// ---- state ------------------------------------------------------------------------
// Everything the steps show lives here, in the parent, never inside a step: going back
// and forth only changes `view`, so nothing a step holds can be lost.
const brief = ref('')
const campaign = ref<Campaign | null>(null)
const creating = ref(false)
const createError = ref<ApiError | null>(null)

const { state: stream, start: startStream, reset: resetStream } = useGenerationStream()
// The variant the stream is writing; its live state is shown only on that variant's tab.
const streamVariant = ref<Variant>('A')
const shownStream = computed(() => (streamVariant.value === activeVariant.value ? stream.value : initialState()))

// One text per variant (A, and B if the marketer adds it) and locale of the campaign (the source and
// every locale with a reachable person). The tabs switch the variant, the locale column the locale;
// only each variant's source is edited, the translations are read-only.
const allLocales = computed(() => props.info?.locales ?? FALLBACK_LOCALES)
const sourceCode = computed(() => campaign.value?.source_locale ?? props.info?.source_locale ?? SOURCE_LOCALE)
const locales = computed(() => campaignLocales(campaign.value, allLocales.value))
const codes = computed(() => locales.value.map((l) => l.code))
const activeVariant = ref<Variant>('A')
// The campaign seen through the variant on screen: the locale helpers work on it unchanged.
const cv = computed(() => forVariant(campaign.value, activeVariant.value))
const hasB = computed(() => hasVariantB(campaign.value))
const draftsBy = ref<Record<Variant, Record<LocaleCode, MessageContent>>>({ A: draftsFrom(null, [SOURCE_LOCALE]), B: {} })
const drafts = computed<Record<LocaleCode, MessageContent>>({
  get: () => draftsBy.value[activeVariant.value],
  set: (v) => (draftsBy.value = { ...draftsBy.value, [activeVariant.value]: v }),
})
const activeLocale = ref<LocaleCode>(SOURCE_LOCALE)
const draft = computed<MessageContent>({
  get: () => drafts.value[activeLocale.value] ?? emptyMessage(),
  set: (v) => (drafts.value = { ...drafts.value, [activeLocale.value]: v }),
})
const activeInfo = computed(() => localeInfo(activeLocale.value, allLocales.value))
const activeIsSource = computed(() => activeLocale.value === sourceCode.value)
const activeStored = computed(() => storedByLocale(cv.value)[activeLocale.value] ?? null)
const activeState = computed(() => localeState(cv.value, activeLocale.value))
// Translations missing or stale: approval waits for them in both variants (the server refuses it too).
const pending = computed(() => pendingLocales(cv.value))
const pendingTotal = computed(() => variantsOf(campaign.value).reduce((n, v) => n + pendingOf(campaign.value, v).length, 0))
const translationTotal = computed(() => Math.max(codes.value.length - 1, 0))
const upToDate = computed(() => translationTotal.value - pending.value.length)
// Variant B exists only once written: while it is written (or failed to be) its tab is shown anyway.
const tabVariants = computed<Variant[]>(() =>
  hasB.value || (streamVariant.value === 'B' && stream.value.phase !== 'idle') ? ['A', 'B'] : ['A'],
)
const bMissing = computed(() => activeVariant.value === 'B' && !hasB.value)
const confirmRemove = ref(false)
const removing = ref(false)

// Translating again on request: which variant and locales are being written, why some could not be.
const translatingVariant = ref<Variant>('A')
const translating = ref<LocaleCode[]>([])
const translateError = ref<string | null>(null)
// "Onayla" with a changed source: how many translations will be made again before approval.
const retranslateAsk = ref<number | null>(null)
const saving = ref(false)
const saveError = ref<string | null>(null)
const serverIssues = ref<Issue[]>([])

const confirming = ref(false)
const approving = ref(false)

// Send now, or later: the audience is then decided by the worker when the time comes.
const sendMode = ref<'now' | 'later'>('now')
const sendWhen = ref(toLocalInput(defaultSendTime()))
const clockNow = ref(new Date())
const cancelling = ref(false)
const confirmCancel = ref(false)
const cancelError = ref<string | null>(null)
const approveError = ref<string | null>(null)
const approved = ref<ApproveResult | null>(null)

// After approval the text is frozen: earlier steps still open, read-only.
const frozen = computed(() => ['scheduled', 'approved', 'sending', 'completed', 'cancelled', 'failed'].includes(campaign.value?.status ?? ''))
const changedFor = (v: Variant) => changedLocales(forVariant(campaign.value, v), draftsBy.value[v] ?? {})
const changed = computed(() => changedFor(activeVariant.value))
// Edits are kept per variant: an unsaved edit of A is not lost by looking at B.
const unsavedVariants = computed(() => VARIANTS.filter((v) => changedFor(v).length > 0))
const unsaved = computed(() => unsavedVariants.value.length > 0)
const streaming = computed(() => stream.value.phase === 'streaming')
const mismatches = computed<Issue[]>(() => [
  ...(cv.value?.mismatches ?? []),
  ...serverIssues.value.filter(
    (i) => i.code === 'locale_mismatch' && issueVariant(i) === activeVariant.value && !(cv.value?.mismatches ?? []).some((m) => m.message === i.message),
  ),
])
const mismatchTotal = computed(() => (campaign.value?.mismatches?.length ?? 0) + (campaign.value?.variant_b?.mismatches?.length ?? 0))
const duplicate = computed(() => isDuplicate(campaign.value))
// Why the chosen time cannot be used; null when sending now or when the time is fine.
const scheduleError = computed(() =>
  sendMode.value === 'later' ? checkSchedule(fromLocalInput(sendWhen.value), campaign.value?.segment?.offer, clockNow.value) : null,
)
const canApprove = computed(
  () =>
    campaign.value?.status === 'content_ready' &&
    scheduleError.value === null &&
    !!campaign.value?.content &&
    pendingTotal.value === 0 &&
    mismatchTotal.value === 0 &&
    !duplicate.value &&
    translating.value.length === 0 &&
    !streaming.value &&
    !saving.value &&
    !unsaved.value &&
    !approving.value,
)
const difference = computed(() =>
  approved.value ? approvalDifference(approved.value.queued, campaign.value?.segment?.audience) : null,
)

const limits = computed(() => props.info?.limits)
const variables = computed(() => props.info?.variables ?? [])
const segmentReady = computed(() => campaign.value?.segment?.status === 'ok' && campaign.value.status !== 'draft')
const hasContent = computed(() => !!stream.value.locales[sourceCode.value]?.result || campaign.value?.content != null)
const briefChanged = computed(() => !!campaign.value && brief.value.trim() !== campaign.value.brief.trim())
const busy = computed(() => creating.value || streaming.value || saving.value || approving.value || removing.value || translating.value.length > 0)

// The limits are checked per locale of the variant on screen; saving needs every edited text clean.
const localChecks = computed(() => {
  const out: Record<LocaleCode, { issues: Issue[] }> = {}
  for (const code of codes.value) {
    out[code] = limits.value ? checkContent(drafts.value[code] ?? emptyMessage(), limits.value, variables.value) : { issues: [] }
  }
  return out
})
const anyLocalIssues = computed(
  () =>
    !!limits.value &&
    unsavedVariants.value.some((v) =>
      changedFor(v).some((code) => checkContent(draftsBy.value[v][code] ?? emptyMessage(), limits.value!, variables.value).issues.length > 0),
    ),
)

// ---- wizard -----------------------------------------------------------------------
const view = ref<StepKey>('brief')
// A campaign named in the address is fetched first; until then no panel shows, so the page never flashes the wrong step.
const ready = ref(campaignIdFromUrl(window.location.href) === null)
const loadError = ref<string | null>(null)
const slide = ref<'forward' | 'back'>('forward')

// How far the campaign has come decides which steps can be opened; `view` is only what is on screen.
const progress = computed(() =>
  currentStep({ campaign: campaign.value, streaming: streaming.value, hasContent: hasContent.value, unsaved: unsaved.value }),
)
const reachable = computed(() => reachableSteps(progress.value.current))

function go(key: StepKey) {
  if (key === view.value) return
  slide.value = direction(view.value, key)
  view.value = key
  confirming.value = false
  window.scrollTo({ top: 0, behavior: 'auto' })
}
function onStepBar(key: StepKey) {
  if (!busy.value && reachable.value.includes(key)) go(key)
}

const bar = computed(() =>
  barAction({
    view: view.value,
    frozen: frozen.value,
    hasCampaign: campaign.value !== null,
    briefChanged: briefChanged.value,
    hasContent: hasContent.value,
    // The question is asked in the alert dialog; the bar keeps its first label behind it.
    confirming: false,
    scheduling: sendMode.value === 'later',
  }),
)
const barLabel = computed(() => {
  switch (bar.value.action) {
    case 'create':
      return creating.value ? 'Kitle çıkarılıyor…' : bar.value.label
    case 'generate':
      return streaming.value ? 'Üretiliyor…' : bar.value.label
    case 'save-next':
      return saving.value ? 'Kaydediliyor…' : bar.value.label
    case 'send':
      return approving.value ? (sendMode.value === 'later' ? 'Zamanlanıyor…' : 'Gönderim başlıyor…') : bar.value.label
    default:
      return bar.value.label
  }
})
const barDisabled = computed(() => {
  switch (bar.value.action) {
    case 'create':
      return creating.value || brief.value.trim().length < 5
    case 'generate':
      return !segmentReady.value || streaming.value
    case 'save-next':
      return busy.value || !hasContent.value || (unsaved.value ? anyLocalIssues.value : campaign.value?.status !== 'content_ready')
    case 'ask':
      return !canApprove.value
    case 'send':
      return approving.value
    case 'next':
      return busy.value || (view.value === 'brief' && campaign.value === null)
    default:
      return false
  }
})
const barTestId = computed(() => (bar.value.action === 'ask' ? 'approve' : undefined))

async function onPrimary() {
  switch (bar.value.action) {
    case 'create':
      return submitBrief()
    case 'generate':
      return generate()
    case 'save-next':
      // A Turkish text changed (or translations wait, in either variant): translate first and let the person look.
      if (unsaved.value || pendingTotal.value > 0) {
        retranslateAsk.value = variantsOf(campaign.value).reduce(
          (n, v) => n + (unsavedVariants.value.includes(v) ? translationTotal.value : pendingOf(campaign.value, v).length),
          0,
        )
        return
      }
      return go('approve')
    case 'ask':
      confirming.value = true
      return
    case 'send':
      return approve()
    case 'next':
      return go(nextStep(view.value))
    case 'dashboard':
      window.location.hash = '#/dashboard'
  }
}
function onBack() {
  if (confirming.value) confirming.value = false
  else go(prevStep(view.value))
}

// ---- actions ----------------------------------------------------------------------
async function approve() {
  if (!campaign.value || !canApprove.value) return
  approving.value = true
  approveError.value = null
  try {
    const at = sendMode.value === 'later' ? fromLocalInput(sendWhen.value) : null
    approved.value = await approveCampaign(campaign.value.id, at ? at.toISOString() : undefined)
    campaign.value = await getCampaign(campaign.value.id)
    go('send')
  } catch (e) {
    const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    approveError.value = err.message
    confirming.value = false
    // A refused approval (e.g. a locale that gained a person needs its translation): show the state the server has.
    if (err.status === 409) {
      try {
        adopt(await getCampaign(campaign.value.id))
      } catch {
        /* keep what is on screen */
      }
    }
  } finally {
    approving.value = false
  }
}

// A scheduled campaign can be cancelled until it starts; if it has started, the server refuses.
async function cancelScheduled() {
  if (!campaign.value || cancelling.value) return
  cancelling.value = true
  cancelError.value = null
  try {
    campaign.value = await cancelCampaign(campaign.value.id)
    confirmCancel.value = false
  } catch (e) {
    cancelError.value = (e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')).message
    try {
      campaign.value = await getCampaign(campaign.value.id) // it may have started in the meantime
    } catch {
      /* keep what is on screen */
    }
    confirmCancel.value = false
  } finally {
    cancelling.value = false
  }
}

// Progress: ask the server once a second until the campaign is completed.
let timer: number | null = null
let clockTimer: number | null = null
function stopPolling() {
  if (timer !== null) window.clearInterval(timer)
  timer = null
}
function startPolling() {
  if (timer !== null) return
  timer = window.setInterval(async () => {
    const id = campaign.value?.id
    if (id === undefined) return stopPolling()
    try {
      const c = await getCampaign(id)
      campaign.value = c
      if (!shouldPoll(c)) stopPolling()
    } catch {
      /* a missed tick is fine; the next one tries again */
    }
  }, 1000)
}
watch(
  () => (campaign.value ? shouldPoll(campaign.value) : false),
  (poll) => (poll ? startPolling() : stopPolling()),
  { immediate: true },
)
onBeforeUnmount(stopPolling)
onMounted(() => (clockTimer = window.setInterval(() => (clockNow.value = new Date()), 15_000)))
onBeforeUnmount(() => clockTimer !== null && window.clearInterval(clockTimer))

// While streaming, every locale being written shows its live text (in the variant being written).
watch(
  () => stream.value.locales,
  (all) => {
    if (!streaming.value) return
    const v = streamVariant.value
    const next = { ...draftsBy.value[v] }
    for (const [code, l] of Object.entries(all)) if (l.attempt || l.result) next[code] = { ...l.draft }
    draftsBy.value = { ...draftsBy.value, [v]: next }
  },
  { deep: true },
)

// Warnings describe the SAVED text; while a new text streams in there is nothing to warn about yet.
function warningsOf(v: Variant): Record<LocaleCode, Issue[]> {
  const out: Record<LocaleCode, Issue[]> = {}
  if (streaming.value && streamVariant.value === v) return out
  for (const [code, sc] of Object.entries(storedByLocale(forVariant(campaign.value, v)))) {
    out[code] = (sc.report.warnings ?? []).map((w) => ({ ...w, locale: code, ...(v === 'B' ? { variant: v } : {}) }))
  }
  return out
}
const warnings = computed<Issue[]>(() => warningsOf(activeVariant.value)[activeLocale.value] ?? [])
const allWarnings = computed<Issue[]>(() =>
  variantsOf(campaign.value).flatMap((v) => {
    const w = warningsOf(v)
    return codes.value.flatMap((c) => w[c] ?? [])
  }),
)

// Where the text on screen came from, as a short badge (no sentence).
const sourceLabel = computed(() =>
  source.value === 'llm'
    ? 'Model metni'
    : source.value === 'fallback'
      ? 'Hazır şablon'
      : source.value === 'edited'
        ? 'Elle düzenlendi'
        : source.value === 'translated'
          ? 'Çeviri'
          : null,
)
// The colour of the campaign's state in the report header.
const statusTone = computed(() => {
  switch (campaign.value?.status) {
    case 'completed':
      return 'ok'
    case 'failed':
      return 'danger'
    case 'cancelled':
      return 'neutral'
    case 'scheduled':
      return 'plan'
    default:
      return 'accent'
  }
})
// A scheduled campaign: the estimated audience by channel and by locale.
const scheduledLocales = computed(() => countsLine(campaign.value?.segment?.locales, allLocales.value))
const scheduledCells = computed(() => (campaign.value?.segment?.audience ? 2 : 0) + (scheduledLocales.value ? 1 : 0) || 1)
const cancelText = computed(
  () =>
    `Kampanya${campaign.value?.scheduled_at ? ' ' + formatWhen(campaign.value.scheduled_at) + ' için' : ''} zamanlanmıştı. İptal edersen hiçbir mesaj gönderilmez.`,
)
const source = computed(() => shownStream.value.locales[activeLocale.value]?.result?.source ?? activeStored.value?.source ?? null)
// The variant on screen is being written by the model now.
const streamingHere = computed(() => streaming.value && streamVariant.value === activeVariant.value)
const translatingHere = computed(() => translatingVariant.value === activeVariant.value ? translating.value : [])

// What the locale column shows about each locale (of the variant on screen).
const localeStatus = computed<Record<LocaleCode, LocaleStatus>>(() => {
  const out: Record<LocaleCode, LocaleStatus> = {}
  const edited = new Set(changed.value)
  for (const code of codes.value) {
    const l = shownStream.value.locales[code]
    const writing = streamingHere.value
      ? code === sourceCode.value
        ? !l?.result
        : !!l && !l.result && !l.failed
      : translatingHere.value.includes(code)
    out[code] = {
      state: localeState(cv.value, code),
      writing,
      issues:
        (localChecks.value[code]?.issues.length ?? 0) +
        serverIssues.value.filter((i) => i.locale === code && issueVariant(i) === activeVariant.value).length +
        mismatches.value.filter((m) => m.locale === code).length,
      unsaved: edited.has(code),
    }
  }
  return out
})
// The tabs: a dot on a variant that needs attention, a spinner on one being written.
const variantStatus = computed<Record<Variant, { dot: boolean; writing: boolean }>>(() => {
  const out = {} as Record<Variant, { dot: boolean; writing: boolean }>
  for (const v of VARIANTS) {
    const view = forVariant(campaign.value, v)
    out[v] = {
      writing: (streaming.value && streamVariant.value === v) || (translating.value.length > 0 && translatingVariant.value === v),
      dot:
        !frozen.value &&
        (pendingOf(campaign.value, v).length > 0 || changedFor(v).length > 0 || (view?.mismatches?.length ?? 0) > 0 || (v === 'B' && duplicate.value)),
    }
  }
  return out
})
const notes = computed(() => retryNotes(shownStream.value))
// Issues of the locale on screen (the server names the locale of each one it rejects).
const serverIssuesHere = computed(() =>
  serverIssues.value.filter((i) => i.code !== 'locale_mismatch' && issueVariant(i) === activeVariant.value && (!i.locale || i.locale === activeLocale.value)),
)
const mismatchesHere = computed(() => mismatches.value.filter((m) => m.locale === activeLocale.value))
const activeAttempt = computed(() => shownStream.value.locales[activeLocale.value]?.attempt ?? null)
const failedNow = computed(() => [
  ...new Set([
    ...failedLocales(shownStream.value),
    ...(translatingVariant.value === activeVariant.value ? Object.keys(campaign.value?.translation_failures ?? {}) : []),
  ]),
])
const people = (code: LocaleCode) => (campaign.value?.segment?.locales?.[code] ?? 0).toLocaleString('tr-TR')

// What changed in the source since a stale translation was made (only the changed fields).
const FIELD_LABELS: Record<ContentField, string> = { sms_body: 'SMS', email_subject: 'E-posta konusu', email_body: 'E-posta metni' }
const sourceChanges = computed(() => {
  const was = activeStored.value?.stale ? activeStored.value.source_text : undefined
  const now = cv.value?.content
  if (!was || !now) return []
  return (Object.keys(FIELD_LABELS) as ContentField[]).filter((f) => was[f] !== now[f]).map((f) => ({ field: f, label: FIELD_LABELS[f], was: was[f], now: now[f] }))
})
const canTranslate = computed(() => !!cv.value?.content && !frozen.value && !busy.value && !unsaved.value)

const fallbackText = computed(() => {
  switch (shownStream.value.fallbackReason) {
    case 'llm_refused':
      return 'Model bu isteği yanıtlamayı reddetti.'
    case 'llm_unavailable':
      return 'Model servisine ulaşılamadı.'
    default:
      return 'Model 3 denemede sınırlara uyan bir metin üretemedi.'
  }
})

function setUrl(id: number | null) {
  const href = window.location.href
  window.history.replaceState(null, '', id === null ? urlWithoutCampaign(href, window.location.hash) : urlForCampaign(href, id, window.location.hash))
}

// Takes the server's campaign. The drafts of every variant follow it, except those in `keep`
// (variants with an edit not saved yet).
function adopt(c: Campaign, keep: Variant[] = []) {
  campaign.value = c
  const shown = campaignLocales(c, allLocales.value).map((l) => l.code)
  const next = { ...draftsBy.value }
  for (const v of VARIANTS) {
    if (keep.includes(v)) continue
    const view = forVariant(c, v)
    if (view?.content) next[v] = draftsFrom(view, shown)
    else if (v === 'B') next[v] = {}
  }
  draftsBy.value = next
  if (!shown.includes(activeLocale.value)) activeLocale.value = shown[0] ?? SOURCE_LOCALE
  if (activeVariant.value === 'B' && !hasVariantB(c) && streamVariant.value !== 'B') activeVariant.value = 'A'
}

// "Yeniden çevir" / "Güncel olmayanları çevir": translations are only ever made by the translator.
function askTranslate(list: LocaleCode[]) {
  if (canTranslate.value && list.length > 0) void runTranslate(list, activeVariant.value)
}
// "Çevir ve göster": save the Turkish texts, translate what is stale in each variant, stay here to review.
async function translateAndShow() {
  retranslateAsk.value = null
  if (unsaved.value) {
    await save()
    if (saveError.value) return
  }
  for (const v of variantsOf(campaign.value)) {
    const list = pendingOf(campaign.value, v)
    if (list.length > 0) await runTranslate(list, v)
    if (translateError.value) return
  }
}
async function runTranslate(list: LocaleCode[], v: Variant) {
  if (!campaign.value || list.length === 0) return
  translatingVariant.value = v
  translating.value = list
  translateError.value = null
  resetStream()
  try {
    const c = await translateCampaign(campaign.value.id, list, v)
    adopt(c)
    const failed = Object.keys(c.translation_failures ?? {})
    if (failed.length > 0) translateError.value = `Çevrilemedi: ${failed.join(', ')}`
  } catch (e) {
    translateError.value = (e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')).message
  } finally {
    translating.value = []
  }
}

async function submitBrief() {
  if (creating.value || brief.value.trim().length === 0) return
  creating.value = true
  createError.value = null
  campaign.value = null
  approved.value = null
  approveError.value = null
  resetStream()
  streamVariant.value = 'A'
  activeVariant.value = 'A'
  draftsBy.value = { A: draftsFrom(null, [SOURCE_LOCALE]), B: {} }
  activeLocale.value = SOURCE_LOCALE
  try {
    const c = await createCampaign(brief.value)
    adopt(c)
    setUrl(c.id)
    go('audience') // the result (audience, or the question about it) is shown right away
  } catch (e) {
    createError.value = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    setUrl(null)
    go('brief')
  } finally {
    creating.value = false
  }
}

// The answer to "Bu kampanya kime gönderilsin? Tüm müşterilere mi?": the sentence is
// added to the brief and the campaign is built again from it.
async function sendToAll() {
  brief.value = withAllCustomers(brief.value)
  await submitBrief()
}

// Writes a variant with the model: A ("Mesajları üret", "Yeniden üret"), or B ("+" and B's menu),
// which the model writes with A's text in view so that it says the same offer differently.
async function generate(v: Variant = 'A') {
  if (!campaign.value || streaming.value) return
  if (v === 'B' && !campaign.value.content) return
  serverIssues.value = []
  saveError.value = null
  go('messages') // the person watches the text arrive
  translateError.value = null
  streamVariant.value = v
  activeVariant.value = v
  await startStream(campaign.value.id, v)
  if (stream.value.phase === 'done') {
    try {
      adopt(await getCampaign(campaign.value.id), unsavedVariants.value.filter((u) => u !== v)) // pick up status = content_ready
    } catch {
      /* the stream result is already on screen */
    }
  }
}

// Saves the edited Turkish text of every variant that has one (each variant has its own source).
async function save() {
  if (!campaign.value || saving.value || anyLocalIssues.value) return
  saving.value = true
  saveError.value = null
  serverIssues.value = []
  const todo = unsavedVariants.value.map((v) => ({ v, texts: Object.fromEntries(changedFor(v).map((code) => [code, draftsBy.value[v][code]])) }))
  let current: Variant = 'A'
  try {
    for (const [i, job] of todo.entries()) {
      current = job.v
      adopt(await saveContent(campaign.value.id, job.texts, job.v), todo.slice(i + 1).map((t) => t.v))
    }
    resetStream()
  } catch (e) {
    const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    saveError.value = err.message
    serverIssues.value = (err.details?.report?.issues ?? []).map((i: Issue) => (current === 'B' ? { ...i, variant: current } : i))
    // Show the variant and the locale that have the first problem.
    activeVariant.value = current
    const first = serverIssues.value.find((i) => i.locale)
    if (first?.locale) activeLocale.value = first.locale
  } finally {
    saving.value = false
  }
}

// "Sil" in Variant B's menu: the campaign goes back to one text, everyone gets Variant A.
async function removeB() {
  if (!campaign.value || removing.value) return
  removing.value = true
  saveError.value = null
  try {
    if (hasB.value) adopt(await deleteVariant(campaign.value.id, 'B'), unsavedVariants.value.filter((v) => v === 'A'))
    resetStream()
    streamVariant.value = 'A'
    activeVariant.value = 'A'
    draftsBy.value = { ...draftsBy.value, B: {} }
    serverIssues.value = serverIssues.value.filter((i) => issueVariant(i) !== 'B')
    confirmRemove.value = false
  } catch (e) {
    saveError.value = (e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')).message
    confirmRemove.value = false
  } finally {
    removing.value = false
  }
}

onMounted(async () => {
  const id = campaignIdFromUrl(window.location.href)
  if (id === null) {
    ready.value = true
    return
  }
  try {
    const c = await getCampaign(id)
    brief.value = c.brief
    adopt(c)
    view.value = progress.value.current // open where the campaign stands
  } catch (e) {
    // Only a campaign that does not exist loses its place in the address. A failed
    // request (server briefly away) keeps it, so a reload can still open the campaign.
    if (e instanceof ApiError && e.status === 404) setUrl(null)
    else loadError.value = e instanceof ApiError ? e.message : 'Kampanya yüklenemedi. Sayfayı yenileyin.'
  } finally {
    ready.value = true
  }
})
</script>

<template>
  <div class="wizard">
    <StepBar :current="view" :done="progress.done" :reachable="reachable" @go="onStepBar">
      <template v-if="frozen" #end><Badge tone="ok" dot>Onaylandı</Badge></template>
    </StepBar>
    <p v-if="loadError" class="notice error" role="alert" data-testid="load-error">{{ loadError }}</p>

    <div class="stage">
      <Transition :name="slide === 'forward' ? 'slide-left' : 'slide-right'" mode="out-in">
        <div v-if="ready" :key="view" class="panel">
          <!-- 1 · Brief: the prompt card -->
          <section v-if="view === 'brief'" class="prompt" aria-labelledby="brief-title">
            <header class="prompt__head">
              <h2 id="brief-title">Brief</h2>
              <Button v-if="!frozen" variant="secondary" size="sm" :icon="FileText" :disabled="creating" @click="brief = EXAMPLE_BRIEF">Örnek brief</Button>
            </header>
            <div class="prompt__box" :class="{ 'is-working': creating }">
              <label for="brief" class="sr-only">Kampanya brief'i</label>
              <textarea
                id="brief"
                v-model="brief"
                rows="6"
                maxlength="2000"
                placeholder="Kime, ne zaman, ne göndermek istediğinizi doğal dille yazın…"
                :disabled="creating"
                :readonly="frozen"
                @keydown.ctrl.enter="!barDisabled && bar.action === 'create' && onPrimary()"
              />
              <p v-if="creating" class="prompt__status" role="status"><Sparkles :size="16" aria-hidden="true" class="spark" />Kitle çıkarılıyor…</p>
            </div>
            <p v-if="!frozen && campaign && briefChanged" class="notice warn">Brief değişti: kitle ve mesajlar yeniden oluşturulacak.</p>

            <div v-if="createError" class="notice error" role="alert">
              {{ createError.message }}
              <details v-if="createError.details?.attempts?.length">
                <summary>Ayrıntılar</summary>
                <ul>
                  <li v-for="(a, i) in createError.details.attempts" :key="i">Deneme {{ i + 1 }}: {{ a.join('; ') }}</li>
                </ul>
              </details>
            </div>
          </section>

          <!-- 2 · Kitle -->
          <template v-else-if="view === 'audience'">
            <SegmentCard v-if="campaign?.segment" :segment="campaign.segment" :locales="allLocales" :busy="creating || frozen" @send-to-all="sendToAll" />
            <Card v-else title="Kitle" role="status">
              <p class="muted">Kitle çıkarılıyor…</p>
            </Card>
          </template>

          <!-- 3 · Mesajlar -->
          <template v-else-if="view === 'messages'">
            <MessageEditor
              v-if="limits && (hasContent || streaming)"
              v-model="draft"
              :readonly="streaming || frozen || !activeIsSource || bMissing || translatingHere.includes(activeLocale)"
              :typing="streamingHere || translatingHere.includes(activeLocale)"
              :limits="limits"
              :variables="variables"
              :server-issues="serverIssuesHere"
              :warnings="warnings"
              :locale="activeLocale"
              :dir="activeInfo.direction"
            >
              <template #actions>
                <Badge v-if="translationTotal > 0 && cv?.content && !streaming" :tone="pending.length ? 'neutral' : 'ok'" data-testid="up-to-date">
                  Güncel <b class="num">{{ upToDate }}/{{ translationTotal }}</b>
                </Badge>
                <Badge v-if="unsaved && !frozen && !streaming" tone="warn" dot data-testid="unsaved">Kaydedilmedi</Badge>
                <Button
                  v-if="pending.length && !frozen && !streaming"
                  variant="outline"
                  size="sm"
                  :icon="Sparkles"
                  :disabled="!canTranslate"
                  :loading="translatingHere.length > 1"
                  data-testid="translate-pending"
                  @click="askTranslate(pending)"
                  >Güncel olmayanları çevir ({{ pending.length }})</Button
                >
                <Button
                  v-if="!frozen && !streaming && hasContent && segmentReady && !bMissing"
                  variant="outline"
                  size="sm"
                  :icon="RefreshCw"
                  :disabled="busy"
                  data-testid="regenerate"
                  @click="generate(activeVariant)"
                  >Yeniden üret</Button
                >
                <Button
                  v-if="tabVariants.length === 1 && !frozen && !streaming && !!campaign?.content"
                  variant="outline"
                  size="sm"
                  :icon="Plus"
                  :disabled="busy"
                  data-testid="variant-add"
                  @click="generate('B')"
                  >A/B testi</Button
                >
              </template>
              <template v-if="tabVariants.length > 1" #tabs>
                <VariantTabs
                  v-model="activeVariant"
                  :variants="tabVariants"
                  :status="variantStatus"
                  :locked="frozen || busy || !campaign?.content"
                  @regenerate="generate('B')"
                  @remove="confirmRemove = true"
                />
              </template>
              <template #locales>
                <LocaleList v-model="activeLocale" :locales="locales" :status="localeStatus" :counts="campaign?.segment?.locales" />
              </template>
              <template #top>
                <div class="loc-head" data-testid="locale-head">
                  <strong class="loc-head__code">{{ activeLocale }}</strong>
                  <span class="muted" :dir="activeInfo.direction">{{ activeInfo.native_name }}</span>
                  <Badge :tone="activeState === 'ok' ? 'ok' : activeState === 'source' ? 'accent' : activeState === 'stale' ? 'warn' : 'neutral'" data-testid="locale-state">
                    {{ stateLabel[activeState] }}
                  </Badge>
                  <Badge v-if="sourceLabel && hasContent && !streaming && activeStored" data-testid="source">{{ sourceLabel }}</Badge>
                  <Badge v-if="activeInfo.direction === 'rtl'" tone="plan">RTL</Badge>
                  <Badge v-if="activeIsSource && activeStored">v{{ activeStored.version }}</Badge>
                  <span class="loc-head__people muted num">{{ people(activeLocale) }} alıcı</span>
                  <Button
                    v-if="!activeIsSource && !frozen && !streaming"
                    variant="outline"
                    size="sm"
                    :icon="Sparkles"
                    :disabled="!canTranslate"
                    :loading="translatingHere.includes(activeLocale)"
                    data-testid="retranslate"
                    @click="askTranslate([activeLocale])"
                    >Yeniden çevir</Button
                  >
                </div>
                <p v-if="activeVariant === 'B' && duplicate && !streaming" class="notice error" role="alert" data-testid="variant-duplicate">Varyant B, Varyant A ile aynı.</p>
                <div v-if="streamingHere" class="agent" :class="{ 'agent--retry': (activeAttempt?.n ?? 1) > 1 }" role="status" data-testid="attempt">
                  <span class="agent__icon" aria-hidden="true"><Sparkles :size="16" /></span>
                  <span><strong>{{ activeLocale }}:</strong> {{ attemptLabel(activeAttempt) }}</span>
                  <ul v-if="notes.length" class="agent__notes">
                    <template v-for="n in notes" :key="n.locale">
                      <li v-for="p in n.problems" :key="n.locale + p">{{ n.locale }}: {{ p }}</li>
                    </template>
                  </ul>
                </div>
                <p v-if="shownStream.fallbackReason" class="notice warn" role="alert"><strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi.</p>
                <p v-if="shownStream.error" class="notice error" role="alert" data-testid="stream-error">{{ shownStream.error.message }}</p>
                <p v-if="failedNow.length && !streaming" class="notice error" role="alert" data-testid="translate-failed">Çevrilemedi: {{ failedNow.join(', ') }}</p>
                <p v-else-if="translateError" class="notice error" role="alert" data-testid="translate-error">{{ translateError }}</p>

                <details v-if="sourceChanges.length" class="diff" data-testid="source-diff">
                  <summary>{{ sourceCode }} değişikliği</summary>
                  <div v-for="c in sourceChanges" :key="c.field" class="diff__field">
                    <span class="label">{{ c.label }}</span>
                    <del>{{ c.was }}</del>
                    <ins>{{ c.now }}</ins>
                  </div>
                </details>
                <div v-if="mismatchesHere.length" class="notice error" role="alert" data-testid="mismatch">
                  <strong>{{ sourceCode }} ile aynı şeyi söylemiyor:</strong>
                  <span v-for="m in mismatchesHere" :key="m.message" class="block">• {{ m.message }}</span>
                </div>
              </template>
              <template #bottom>
                <p v-if="saveError" class="notice error" role="alert">{{ saveError }}</p>
              </template>
            </MessageEditor>

            <Card v-else title="Mesajlar">
              <p v-if="shownStream.fallbackReason" class="notice warn" role="alert"><strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi.</p>
              <p v-if="shownStream.error" class="notice error" role="alert">{{ shownStream.error.message }}</p>
              <div v-if="!streaming" class="empty">
                <p class="muted">Henüz mesaj yok.</p>
                <Button v-if="segmentReady" variant="primary" @click="generate('A')">Mesajları üret</Button>
              </div>
            </Card>
          </template>

          <!-- 4 · Onay -->
          <template v-else-if="view === 'approve'">
            <ApprovalCard
              v-if="campaign?.segment && (campaign.status === 'content_ready' || frozen)"
              :segment="campaign.segment"
              :variants="hasB ? campaign.variant_counts : undefined"
              :warnings="allWarnings"
              :locales="allLocales"
              :confirming="confirming"
              :frozen="frozen"
              :error="approveError"
              :mode="sendMode"
              :when="sendWhen"
              :schedule-error="scheduleError"
              @update:mode="sendMode = $event"
              @update:when="sendWhen = $event"
            />
            <Card v-else title="Onay">
              <p class="muted">Mesajlar henüz kaydedilmedi.</p>
            </Card>
          </template>

          <!-- 5 · Gönderim: the campaign's report -->
          <div v-else class="report" data-testid="delivery">
            <section v-if="campaign" class="meta">
              <Badge :tone="statusTone" :dot="campaign.status === 'sending' || campaign.status === 'scheduled'">{{ statusLabel(campaign) }}</Badge>
              <div class="meta__name">
                <strong :title="campaign.brief">{{ briefTitle(campaign.brief, 80) }}</strong>
                <span v-if="campaign.status === 'scheduled' && campaign.scheduled_at" class="meta__when">
                  <Clock :size="14" aria-hidden="true" /><strong class="num">{{ formatWhen(campaign.scheduled_at) }}</strong>
                  <span class="muted">({{ untilText(campaign.scheduled_at, clockNow) }})</span>
                </span>
              </div>
              <Button v-if="campaign.status === 'scheduled'" variant="outline" size="sm" :icon="XCircle" data-testid="cancel" @click="confirmCancel = true">
                Gönderimi iptal et
              </Button>
            </section>

            <template v-if="campaign?.status === 'scheduled'">
              <KpiGroup :count="scheduledCells" data-testid="scheduled">
                <StatTile
                  v-if="campaign.segment?.audience"
                  label="Tahmini alıcı · SMS"
                  :value="campaign.segment.audience.sms_reachable"
                  hint="Kitle gönderim anında yeniden belirlenir"
                />
                <StatTile v-if="campaign.segment?.audience" label="Tahmini alıcı · E-posta" :value="campaign.segment.audience.email_reachable" />
                <StatTile v-if="scheduledLocales" label="Tahmini locale'ler" value="" data-testid="scheduled-locales">
                  <span class="num">{{ scheduledLocales }}</span>
                </StatTile>
              </KpiGroup>
              <p v-if="cancelError" class="notice error" role="alert">{{ cancelError }}</p>
            </template>
            <p v-else-if="campaign?.status === 'cancelled'" class="notice info" role="status" data-testid="cancelled">
              Zamanlanmış gönderim iptal edildi; hiçbir mesaj gönderilmedi.
            </p>
            <p v-else-if="campaign?.status === 'failed'" class="notice error" role="alert" data-testid="failed">
              Zamanlanmış gönderim başlatılamadı: {{ campaign.schedule_error || 'bilinmeyen neden' }}. Hiçbir mesaj gönderilmedi.
            </p>
            <template v-else-if="campaign?.delivery">
              <p v-if="difference" class="notice info" role="status">{{ difference }}</p>
              <DeliveryProgress :delivery="campaign.delivery" :status="campaign.status" :locales="allLocales" />
            </template>
            <Card v-else title="Gönderim"><p class="muted">Kampanya henüz onaylanmadı.</p></Card>
          </div>
        </div>
      </Transition>
    </div>

    <!-- No box: the buttons sit on the page, on the column's edges. The main action comes
         first in the page so it is on top on a phone; on wider screens "Geri" moves left. -->
    <footer class="bar">
      <Badge v-if="pendingTotal && !frozen && (view === 'messages' || view === 'approve')" class="bar__note" tone="warn" dot title="Onay: 409 stale_translations" data-testid="pending-note">
        {{ pendingTotal }} çeviri güncel değil
      </Badge>
      <Badge v-else-if="duplicate && !frozen && (view === 'messages' || view === 'approve')" class="bar__note" tone="danger" dot data-testid="duplicate-note">
        Varyant B, Varyant A ile aynı
      </Badge>
      <Button class="main" variant="primary" :disabled="barDisabled" :data-testid="barTestId" data-action="primary" @click="onPrimary">
        {{ barLabel }}
      </Button>
      <Button v-if="view !== 'brief'" class="back" variant="outline" :icon="ArrowLeft" :disabled="busy" data-testid="back" @click="onBack">
        Geri
      </Button>
    </footer>

    <Modal
      :model-value="confirming && view === 'approve'"
      alert
      :persistent="approving"
      title="Emin misiniz?"
      :description="sendMode === 'later' ? 'Mesajlar seçtiğiniz zamanda kendiliğinden gönderilir. Başlamadan önce iptal edebilirsiniz.' : 'Mesajlar hemen gönderilir. Onaydan sonra metin ve kitle değişmez.'"
      @update:model-value="(v: boolean) => !v && (confirming = false)"
    >
      <template #footer>
        <Button variant="ghost" :disabled="approving" @click="confirming = false">Vazgeç</Button>
        <Button variant="primary" :loading="approving" data-testid="confirm-approve" @click="approve">{{ sendMode === 'later' ? 'Evet, zamanla' : 'Evet, gönder' }}</Button>
      </template>
    </Modal>

    <Modal :model-value="retranslateAsk !== null" title="Türkçe metni değiştirdiniz." @update:model-value="(v: boolean) => !v && (retranslateAsk = null)">
      <p class="retranslate" data-testid="retranslate-count">{{ retranslateAsk }} çeviri güncellenecek.</p>
      <template #footer>
        <Button variant="ghost" @click="retranslateAsk = null">Vazgeç</Button>
        <Button variant="primary" :icon="Sparkles" data-testid="translate-and-show" @click="translateAndShow">Çevir ve göster</Button>
      </template>
    </Modal>

    <Modal v-model="confirmRemove" alert :persistent="removing" title="Varyant B silinsin mi?">
      <template #footer>
        <Button variant="ghost" :disabled="removing" @click="confirmRemove = false">Vazgeç</Button>
        <Button variant="danger" :loading="removing" data-testid="confirm-remove-variant" @click="removeB">Sil</Button>
      </template>
    </Modal>

    <Modal v-model="confirmCancel" title="Zamanlanmış gönderim iptal edilsin mi?" :description="cancelText">
      <template #footer>
        <Button variant="ghost" :disabled="cancelling" @click="confirmCancel = false">Vazgeç</Button>
        <Button variant="danger" :loading="cancelling" data-testid="confirm-cancel" @click="cancelScheduled">Gönderimi iptal et</Button>
      </template>
    </Modal>
  </div>
</template>

<style scoped>
.wizard {
  display: flex;
  flex-direction: column;
  gap: var(--s-5);
}
.stage {
  /* clip (not hidden) keeps the previews' sticky position working */
  overflow-x: clip;
}
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--s-4);
}

/* sliding between steps: forward moves left, back moves right */
.slide-left-enter-active,
.slide-right-enter-active {
  transition: transform var(--t-med) var(--ease), opacity var(--t-med) var(--ease);
}
.slide-left-leave-active,
.slide-right-leave-active {
  transition: transform var(--t-fast) ease-in, opacity var(--t-fast) ease-in;
}
.slide-left-leave-to {
  transform: translateX(-20px);
  opacity: 0;
}
.slide-left-enter-from {
  transform: translateX(20px);
  opacity: 0;
}
.slide-right-leave-to {
  transform: translateX(20px);
  opacity: 0;
}
.slide-right-enter-from {
  transform: translateX(-20px);
  opacity: 0;
}

/* the brief: the AI prompt card */
.prompt {
  display: flex;
  flex-direction: column;
  gap: var(--s-4);
  padding: var(--s-8);
  border-radius: var(--r-md);
  background: var(--ai-surface);
}
.prompt__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
}
.prompt__head h2 {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 500;
}
.prompt__box {
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
  box-shadow: 0 6px 24px -14px rgba(60, 40, 140, 0.35);
}
.prompt__box.is-working {
  border: 2px solid transparent;
  background:
    linear-gradient(var(--surface), var(--surface)) padding-box,
    var(--ai-ring) border-box;
}
.prompt__box textarea {
  display: block;
  min-height: 170px;
  border: 0;
  border-radius: 10px;
  background: transparent;
  box-shadow: none;
}
.prompt__box textarea[readonly] {
  background: var(--surface-2);
}
.prompt__status {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 0 var(--s-4) var(--s-3);
  color: var(--ai-chip-fg);
  font-weight: 500;
}
.spark {
  animation: spin 2.4s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
details {
  margin-top: 6px;
}
details ul {
  margin: var(--s-1) 0 0;
  padding-left: var(--s-5);
}

/* the model at work, over the editor */
.agent {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  padding: var(--s-3) var(--s-4);
  border: 1.5px solid transparent;
  border-radius: 10px;
  background:
    var(--ai-surface) padding-box,
    var(--ai-ring) border-box;
}
.agent__icon {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--surface);
  color: var(--ai-chip-fg);
  animation: pulse 1.6s ease-in-out infinite;
}
.agent--retry .agent__icon {
  color: var(--warn);
}
.agent__notes {
  flex-basis: 100%;
  margin: 0;
  padding-left: calc(28px + var(--s-2) + var(--s-4));
  font-size: var(--text-sm);
}
@keyframes pulse {
  50% {
    opacity: 0.5;
  }
}
.block {
  display: block;
}
.empty {
  display: grid;
  justify-items: center;
  gap: var(--s-2);
  padding: var(--s-6) 0;
  text-align: center;
}
.empty p {
  margin: 0;
}

/* the sending step: a report */
.report {
  display: flex;
  flex-direction: column;
  gap: var(--s-4);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-3) var(--s-4);
  padding: var(--s-4) var(--s-5);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
}
.meta__name {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.meta__when {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: var(--text-sm);
  color: var(--plan);
}
.meta > :last-child:not(.meta__name) {
  margin-left: auto;
}

/* the buttons under the step */
.bar {
  display: flex;
  align-items: center;
  gap: var(--s-3);
}
.bar__note {
  margin-left: auto;
}
.bar__note + .main {
  margin-left: 0;
}
.loc-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
}
.loc-head__code {
  font-size: var(--text-md);
  font-variant-numeric: tabular-nums;
}
.loc-head__people {
  margin-left: auto;
  font-size: var(--text-sm);
}
.diff {
  padding: var(--s-2) var(--s-3);
  border-radius: var(--r-md);
  background: var(--surface-2);
  color: var(--muted);
  font-size: var(--text-sm);
}
.diff summary {
  cursor: pointer;
}
.diff__field {
  display: grid;
  gap: 2px;
  margin-top: var(--s-2);
}
.diff del {
  color: var(--error);
  white-space: pre-wrap;
}
.diff ins {
  color: var(--ok);
  text-decoration: none;
  white-space: pre-wrap;
}
.retranslate {
  margin: 0;
  font-variant-numeric: tabular-nums;
}
.back {
  order: -1;
}
.main {
  margin-left: auto;
  min-width: 170px;
}

@media (max-width: 767px) {
  .prompt {
    padding: var(--s-5) var(--s-4);
  }
  .prompt__head h2 {
    font-size: 1.375rem;
  }
  .bar {
    flex-direction: column;
    align-items: stretch;
  }
  .back {
    order: 0;
  }
  .main,
  .back {
    width: 100%;
    margin: 0;
  }
  .meta > :last-child:not(.meta__name) {
    width: 100%;
    margin-left: 0;
  }
}
</style>
