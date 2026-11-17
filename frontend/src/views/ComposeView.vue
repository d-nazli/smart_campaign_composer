<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AppInfo } from '../api/info'
import { approveCampaign, cancelCampaign, createCampaign, getCampaign, saveContent } from '../api/campaigns'
import { ApiError } from '../api/http'
import { useGenerationStream } from '../composables/useGenerationStream'
import { allResults, retryNotes } from '../composables/streamState'
import type { ApproveResult, Campaign, Issue, LanguageCode, MessageContent } from '../types'
import { approvalDifference, shouldPoll } from '../utils/delivery'
import { DEFAULT_LANGUAGE, FALLBACK_LANGUAGES, changedLanguages, draftsFrom, emptyMessage, hasAllLanguages, storedByLanguage } from '../utils/languages'
import { checkContent } from '../utils/limits'
import { campaignIdFromUrl, urlForCampaign, urlWithoutCampaign } from '../utils/route'
import { checkSchedule, formatWhen, fromLocalInput, toLocalInput, untilText } from '../utils/schedule'
import { countsLine } from '../utils/languages'
import { ArrowLeft, Clock, FileText, RefreshCw, Sparkles, XCircle } from 'lucide-vue-next'
import { briefTitle, statusLabel } from '../utils/dashboard'
import { attemptLabel } from '../utils/preview'
import { withAllCustomers } from '../utils/sendToAll'
import { currentStep, type StepKey } from '../utils/steps'
import { barAction, direction, nextStep, prevStep, reachableSteps } from '../utils/wizard'
import ApprovalCard from '../components/ApprovalCard.vue'
import DeliveryProgress from '../components/DeliveryProgress.vue'
import LanguageTabs, { type TabStatus } from '../components/LanguageTabs.vue'
import MessageEditor from '../components/MessageEditor.vue'
import SegmentCard from '../components/SegmentCard.vue'
import StepBar from '../components/StepBar.vue'
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

// One text per language. The tabs switch which one the editor shows; saving sends all of them,
// because the languages must say the same thing.
const languages = computed(() => props.info?.languages ?? FALLBACK_LANGUAGES)
const codes = computed(() => languages.value.map((l) => l.code))
const drafts = ref<Record<LanguageCode, MessageContent>>(draftsFrom(null, FALLBACK_LANGUAGES))
const activeLang = ref<LanguageCode>(DEFAULT_LANGUAGE)
const draft = computed<MessageContent>({
  get: () => drafts.value[activeLang.value] ?? emptyMessage(),
  set: (v) => (drafts.value = { ...drafts.value, [activeLang.value]: v }),
})
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
const unsaved = computed(() => changedLanguages(campaign.value, drafts.value).length > 0)
const streaming = computed(() => stream.value.phase === 'streaming')
const mismatches = computed<Issue[]>(() => [
  ...(campaign.value?.mismatches ?? []),
  ...serverIssues.value.filter((i) => i.code === 'language_mismatch' && !(campaign.value?.mismatches ?? []).some((m) => m.message === i.message)),
])
// Why the chosen time cannot be used; null when sending now or when the time is fine.
const scheduleError = computed(() =>
  sendMode.value === 'later' ? checkSchedule(fromLocalInput(sendWhen.value), campaign.value?.segment?.offer, clockNow.value) : null,
)
const canApprove = computed(
  () =>
    campaign.value?.status === 'content_ready' &&
    scheduleError.value === null &&
    hasAllLanguages(campaign.value, languages.value) &&
    (campaign.value?.mismatches?.length ?? 0) === 0 &&
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
const hasContent = computed(() => allResults(stream.value, codes.value) || campaign.value?.content != null)
const briefChanged = computed(() => !!campaign.value && brief.value.trim() !== campaign.value.brief.trim())
const busy = computed(() => creating.value || streaming.value || saving.value || approving.value)

// The limits are checked per language; saving needs every language clean.
const localChecks = computed(() => {
  const out: Record<LanguageCode, { issues: Issue[] }> = {}
  for (const code of codes.value) {
    out[code] = limits.value ? checkContent(drafts.value[code] ?? emptyMessage(), limits.value, variables.value) : { issues: [] }
  }
  return out
})
const anyLocalIssues = computed(() => Object.values(localChecks.value).some((c) => c.issues.length > 0))

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
    confirming: confirming.value,
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
const barTestId = computed(() => (bar.value.action === 'ask' ? 'approve' : bar.value.action === 'send' ? 'confirm-approve' : undefined))

async function onPrimary() {
  switch (bar.value.action) {
    case 'create':
      return submitBrief()
    case 'generate':
      return generate()
    case 'save-next':
      if (unsaved.value) {
        await save()
        if (saveError.value) return
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
    approveError.value = (e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')).message
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

// While streaming, every language's tab shows its live preview.
watch(
  () => stream.value.langs,
  (langs) => {
    if (!streaming.value) return
    const next = { ...drafts.value }
    for (const [code, l] of Object.entries(langs)) next[code] = { ...l.draft }
    drafts.value = next
  },
  { deep: true },
)
// When a generation finishes, the validated (or fallback) texts become editable.
watch(
  () => allResults(stream.value, codes.value),
  (done) => {
    if (!done) return
    const next = { ...drafts.value }
    for (const code of codes.value) {
      const r = stream.value.langs[code]?.result
      if (r) next[code] = { ...r.content }
    }
    drafts.value = next
    serverIssues.value = []
    saveError.value = null
  },
)

// Warnings describe the SAVED text; while a new text streams in there is nothing to warn about yet.
const warningsByLang = computed<Record<LanguageCode, Issue[]>>(() => {
  const out: Record<LanguageCode, Issue[]> = {}
  if (streaming.value) return out
  for (const [code, sc] of Object.entries(storedByLanguage(campaign.value))) {
    out[code] = (sc.report.warnings ?? []).map((w) => ({ ...w, language: code }))
  }
  return out
})
const warnings = computed<Issue[]>(() => warningsByLang.value[activeLang.value] ?? [])
const allWarnings = computed<Issue[]>(() => codes.value.flatMap((c) => warningsByLang.value[c] ?? []))

// Where the text on screen came from, as a short badge (no sentence).
const sourceLabel = computed(() =>
  source.value === 'llm' ? 'Model metni' : source.value === 'fallback' ? 'Hazır şablon' : source.value === 'edited' ? 'Elle düzenlendi' : null,
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
// A scheduled campaign: the estimated audience by channel and by language.
const scheduledLanguages = computed(() => countsLine(campaign.value?.segment?.languages, languages.value))
const scheduledCells = computed(() => (campaign.value?.segment?.audience ? 2 : 0) + (scheduledLanguages.value ? 1 : 0) || 1)
const cancelText = computed(
  () =>
    `Kampanya${campaign.value?.scheduled_at ? ' ' + formatWhen(campaign.value.scheduled_at) + ' için' : ''} zamanlanmıştı. İptal edersen hiçbir mesaj gönderilmez.`,
)
const source = computed(() => stream.value.langs[activeLang.value]?.result?.source ?? storedByLanguage(campaign.value)[activeLang.value]?.source ?? null)

// What the tabs show about each language.
const tabStatus = computed<Record<LanguageCode, TabStatus>>(() => {
  const out: Record<LanguageCode, TabStatus> = {}
  const changed = new Set(changedLanguages(campaign.value, drafts.value))
  for (const code of codes.value) {
    const l = stream.value.langs[code]
    out[code] = {
      issues: (localChecks.value[code]?.issues.length ?? 0) + serverIssues.value.filter((i) => i.language === code && i.code !== 'language_mismatch').length,
      writing: streaming.value && !l?.result,
      mismatch: mismatches.value.some((m) => m.language === code),
      edited: changed.has(code),
    }
  }
  return out
})
const notes = computed(() => retryNotes(stream.value))
// Issues of the language on screen (the server names the language of each one it rejects).
const serverIssuesHere = computed(() => serverIssues.value.filter((i) => i.code !== 'language_mismatch' && (!i.language || i.language === activeLang.value)))
const activeAttempt = computed(() => stream.value.langs[activeLang.value]?.attempt ?? null)
const langName = (code: LanguageCode) => languages.value.find((l) => l.code === code)?.display ?? code

const fallbackText = computed(() => {
  switch (stream.value.fallbackReason) {
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

function adopt(c: Campaign) {
  campaign.value = c
  if (c.content) drafts.value = draftsFrom(c, languages.value)
}

async function submitBrief() {
  if (creating.value || brief.value.trim().length === 0) return
  creating.value = true
  createError.value = null
  campaign.value = null
  approved.value = null
  approveError.value = null
  resetStream()
  drafts.value = draftsFrom(null, languages.value)
  activeLang.value = DEFAULT_LANGUAGE
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

async function generate() {
  if (!campaign.value || streaming.value) return
  serverIssues.value = []
  go('messages') // the person watches the text arrive
  await startStream(campaign.value.id)
  if (allResults(stream.value, codes.value)) {
    try {
      adopt(await getCampaign(campaign.value.id)) // pick up status = content_ready
    } catch {
      /* the stream result is already on screen */
    }
  }
}

async function save() {
  if (!campaign.value || saving.value || anyLocalIssues.value) return
  saving.value = true
  saveError.value = null
  serverIssues.value = []
  try {
    adopt(await saveContent(campaign.value.id, drafts.value))
    resetStream()
  } catch (e) {
    const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    saveError.value = err.message
    serverIssues.value = err.details?.report?.issues ?? []
    // Show the language that has the first problem.
    const first = serverIssues.value.find((i) => i.language && i.code !== 'language_mismatch')
    if (first?.language) activeLang.value = first.language
  } finally {
    saving.value = false
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
            <SegmentCard v-if="campaign?.segment" :segment="campaign.segment" :languages="languages" :busy="creating || frozen" @send-to-all="sendToAll" />
            <Card v-else title="Kitle" role="status">
              <p class="muted">Kitle çıkarılıyor…</p>
            </Card>
          </template>

          <!-- 3 · Mesajlar -->
          <template v-else-if="view === 'messages'">
            <MessageEditor
              v-if="limits && (hasContent || streaming)"
              v-model="draft"
              :readonly="streaming || frozen"
              :typing="streaming"
              :limits="limits"
              :variables="variables"
              :server-issues="serverIssuesHere"
              :warnings="warnings"
              :language="activeLang"
              :language-name="langName(activeLang)"
            >
              <template #actions>
                <Badge v-if="sourceLabel && hasContent && !streaming" data-testid="source">{{ sourceLabel }}</Badge>
                <Badge v-if="unsaved && !frozen && !streaming" tone="warn" dot data-testid="unsaved">Kaydedilmedi</Badge>
                <Button v-if="!frozen && !streaming && hasContent && segmentReady" variant="outline" size="sm" :icon="RefreshCw" @click="generate">Yeniden üret</Button>
              </template>
              <template #top>
                <div v-if="streaming" class="agent" :class="{ 'agent--retry': (activeAttempt?.n ?? 1) > 1 }" role="status" data-testid="attempt">
                  <span class="agent__icon" aria-hidden="true"><Sparkles :size="16" /></span>
                  <span><strong>{{ langName(activeLang) }}:</strong> {{ attemptLabel(activeAttempt) }}</span>
                  <ul v-if="notes.length" class="agent__notes">
                    <template v-for="n in notes" :key="n.language">
                      <li v-for="p in n.problems" :key="n.language + p">{{ langName(n.language) }}: {{ p }}</li>
                    </template>
                  </ul>
                </div>
                <p v-if="stream.fallbackReason" class="notice warn" role="alert"><strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi.</p>
                <p v-if="stream.error" class="notice error" role="alert">{{ stream.error.message }}</p>
                <LanguageTabs v-model="activeLang" :languages="languages" :status="tabStatus" />
                <div v-if="mismatches.length" class="notice error" role="alert" data-testid="mismatch">
                  <strong>Diller aynı şeyi söylemiyor:</strong>
                  <span v-for="m in mismatches" :key="m.message" class="block">• {{ m.message }}</span>
                </div>
              </template>
              <template #bottom>
                <p v-if="saveError" class="notice error" role="alert">{{ saveError }}</p>
              </template>
            </MessageEditor>

            <Card v-else title="Mesajlar">
              <p v-if="stream.fallbackReason" class="notice warn" role="alert"><strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi.</p>
              <p v-if="stream.error" class="notice error" role="alert">{{ stream.error.message }}</p>
              <div v-if="!streaming" class="empty">
                <p class="muted">Henüz mesaj yok.</p>
                <Button v-if="segmentReady" variant="primary" @click="generate">Mesajları üret</Button>
              </div>
            </Card>
          </template>

          <!-- 4 · Onay -->
          <template v-else-if="view === 'approve'">
            <ApprovalCard
              v-if="campaign?.segment && (campaign.status === 'content_ready' || frozen)"
              :segment="campaign.segment"
              :warnings="allWarnings"
              :languages="languages"
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
                <StatTile v-if="scheduledLanguages" label="Tahmini diller" value="" data-testid="scheduled-languages">
                  <span class="num">{{ scheduledLanguages }}</span>
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
              <DeliveryProgress :delivery="campaign.delivery" :status="campaign.status" :languages="languages" />
            </template>
            <Card v-else title="Gönderim"><p class="muted">Kampanya henüz onaylanmadı.</p></Card>
          </div>
        </div>
      </Transition>
    </div>

    <!-- No box: the buttons sit on the page, on the column's edges. The main action comes
         first in the page so it is on top on a phone; on wider screens "Geri" moves left. -->
    <footer class="bar">
      <Button class="main" variant="primary" :disabled="barDisabled" :data-testid="barTestId" data-action="primary" @click="onPrimary">
        {{ barLabel }}
      </Button>
      <Button v-if="view !== 'brief'" class="back" variant="outline" :icon="ArrowLeft" :disabled="busy" data-testid="back" @click="onBack">
        {{ confirming ? 'Vazgeç' : 'Geri' }}
      </Button>
    </footer>

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
