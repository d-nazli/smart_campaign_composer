<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AppInfo } from '../api/info'
import { approveCampaign, createCampaign, getCampaign, saveContent } from '../api/campaigns'
import { ApiError } from '../api/http'
import { useGenerationStream } from '../composables/useGenerationStream'
import { emptyContent } from '../composables/streamState'
import type { ApproveResult, Campaign, Issue, MessageContent } from '../types'
import { approvalDifference, shouldPoll } from '../utils/delivery'
import { checkContent } from '../utils/limits'
import { attemptLabel } from '../utils/preview'
import { withAllCustomers } from '../utils/sendToAll'
import { currentStep, type StepKey } from '../utils/steps'
import { barAction, direction, nextStep, prevStep, reachableSteps } from '../utils/wizard'
import ApprovalCard from '../components/ApprovalCard.vue'
import DeliveryProgress from '../components/DeliveryProgress.vue'
import MessageEditor from '../components/MessageEditor.vue'
import SegmentCard from '../components/SegmentCard.vue'
import StepBar from '../components/StepBar.vue'

const props = defineProps<{ info: AppInfo | null }>()

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
const draft = ref<MessageContent>(emptyContent())
const saving = ref(false)
const saveError = ref<string | null>(null)
const serverIssues = ref<Issue[]>([])

const confirming = ref(false)
const approving = ref(false)
const approveError = ref<string | null>(null)
const approved = ref<ApproveResult | null>(null)

// After approval the text is frozen: earlier steps still open, read-only.
const frozen = computed(() => ['approved', 'sending', 'completed'].includes(campaign.value?.status ?? ''))
const unsaved = computed(() => {
  const c = campaign.value?.content
  return !!c && (c.sms_body !== draft.value.sms_body || c.email_subject !== draft.value.email_subject || c.email_body !== draft.value.email_body)
})
const streaming = computed(() => stream.value.phase === 'streaming')
const canApprove = computed(
  () => campaign.value?.status === 'content_ready' && !streaming.value && !saving.value && !unsaved.value && !approving.value,
)
const difference = computed(() =>
  approved.value ? approvalDifference(approved.value.queued, campaign.value?.segment?.audience) : null,
)

const limits = computed(() => props.info?.limits)
const variables = computed(() => props.info?.variables ?? [])
const segmentReady = computed(() => campaign.value?.segment?.status === 'ok' && campaign.value.status !== 'draft')
const hasContent = computed(() => stream.value.result !== null || campaign.value?.content != null)
const briefChanged = computed(() => !!campaign.value && brief.value.trim() !== campaign.value.brief.trim())
const busy = computed(() => creating.value || streaming.value || saving.value || approving.value)

const localCheck = computed(() =>
  limits.value ? checkContent(draft.value, limits.value, variables.value) : { smsLength: 0, subjectLength: 0, issues: [] },
)

// ---- wizard -----------------------------------------------------------------------
const view = ref<StepKey>('brief')
// A campaign named in the address is fetched first; until then no panel shows, so the page never flashes the wrong step.
const ready = ref(!new URL(window.location.href).searchParams.get('c'))
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
      return approving.value ? 'Gönderim başlıyor…' : bar.value.label
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
      return busy.value || !hasContent.value || (unsaved.value ? localCheck.value.issues.length > 0 : campaign.value?.status !== 'content_ready')
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
    approved.value = await approveCampaign(campaign.value.id)
    campaign.value = await getCampaign(campaign.value.id)
    go('send')
  } catch (e) {
    approveError.value = (e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')).message
  } finally {
    approving.value = false
  }
}

// Progress: ask the server once a second until the campaign is completed.
let timer: number | null = null
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

// While streaming, the editor shows the live preview.
watch(
  () => stream.value.draft,
  (d) => {
    if (streaming.value) draft.value = { ...d }
  },
  { deep: true },
)
// When a generation finishes, the validated (or fallback) text becomes editable.
watch(
  () => stream.value.result,
  (r) => {
    if (r) {
      draft.value = { ...r.content }
      serverIssues.value = []
      saveError.value = null
    }
  },
)

// Warnings describe the SAVED text; while a new text streams in there is nothing to warn about yet.
const warnings = computed<Issue[]>(() => (streaming.value ? [] : (campaign.value?.content?.report.warnings ?? [])))

const source = computed(() => stream.value.result?.source ?? campaign.value?.content?.source ?? null)

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
  const url = new URL(window.location.href)
  if (id === null) url.searchParams.delete('c')
  else url.searchParams.set('c', String(id))
  window.history.replaceState(null, '', url)
}

function adopt(c: Campaign) {
  campaign.value = c
  if (c.content) draft.value = { sms_body: c.content.sms_body, email_subject: c.content.email_subject, email_body: c.content.email_body }
}

async function submitBrief() {
  if (creating.value || brief.value.trim().length === 0) return
  creating.value = true
  createError.value = null
  campaign.value = null
  approved.value = null
  approveError.value = null
  resetStream()
  draft.value = emptyContent()
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
  if (stream.value.result) {
    try {
      adopt(await getCampaign(campaign.value.id)) // pick up status = content_ready
    } catch {
      /* the stream result is already on screen */
    }
  }
}

async function save() {
  if (!campaign.value || saving.value || localCheck.value.issues.length > 0) return
  saving.value = true
  saveError.value = null
  serverIssues.value = []
  try {
    adopt(await saveContent(campaign.value.id, draft.value))
    resetStream()
  } catch (e) {
    const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    saveError.value = err.message
    serverIssues.value = err.details?.report?.issues ?? []
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  const id = Number(new URL(window.location.href).searchParams.get('c'))
  if (!Number.isInteger(id) || id < 1) {
    ready.value = true
    return
  }
  try {
    const c = await getCampaign(id)
    brief.value = c.brief
    adopt(c)
    view.value = progress.value.current // open where the campaign stands
  } catch {
    setUrl(null)
  } finally {
    ready.value = true
  }
})
</script>

<template>
  <div class="wizard">
    <StepBar :current="view" :done="progress.done" :reachable="reachable" @go="onStepBar" />

    <div class="stage">
      <Transition :name="slide === 'forward' ? 'slide-left' : 'slide-right'" mode="out-in">
        <div v-if="ready" :key="view" class="panel">
          <!-- 1 · Brief -->
          <section v-if="view === 'brief'" class="card">
            <h2>Brief</h2>
            <p v-if="frozen" class="notice info" role="status">Kampanya onaylandı; brief artık değiştirilemez.</p>
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
            <div class="actions">
              <button v-if="!frozen" class="link" :disabled="creating" @click="brief = EXAMPLE_BRIEF">Örnek brief</button>
              <span v-if="campaign && !frozen && briefChanged" class="muted small">
                Brief değişti: kitle ve mesajlar yeniden oluşturulacak.
              </span>
            </div>

            <p v-if="createError" class="notice error" role="alert">
              {{ createError.message }}
              <template v-if="createError.details?.attempts?.length">
                <details>
                  <summary>Ayrıntılar</summary>
                  <ul>
                    <li v-for="(a, i) in createError.details.attempts" :key="i">Deneme {{ i + 1 }}: {{ a.join('; ') }}</li>
                  </ul>
                </details>
              </template>
            </p>
          </section>

          <!-- 2 · Kitle -->
          <template v-else-if="view === 'audience'">
            <SegmentCard v-if="campaign?.segment" :segment="campaign.segment" :busy="creating || frozen" @send-to-all="sendToAll" />
            <section v-else class="card" role="status">
              <h2>Kitle</h2>
              <p class="muted">Kitle çıkarılıyor…</p>
            </section>
          </template>

          <!-- 3 · Mesajlar -->
          <section v-else-if="view === 'messages'" class="card">
            <div class="head">
              <h2>Mesajlar</h2>
              <span v-if="streaming" class="working" :class="{ fixing: (stream.attempt?.n ?? 1) > 1 }" role="status" data-testid="attempt">
                <span class="pulse" aria-hidden="true" />{{ attemptLabel(stream.attempt) }}
              </span>
              <button v-if="!frozen && !streaming && hasContent && segmentReady" class="regen" @click="generate">Yeniden üret</button>
            </div>

            <p v-if="frozen" class="notice info" role="status">Kampanya onaylandı; metin artık değiştirilemez.</p>

            <p v-if="streaming && stream.retryProblems.length" class="notice warn" role="status">
              Önceki deneme sınırlara uymadığı için model düzeltiyor:
              <span v-for="p in stream.retryProblems" :key="p" class="block">• {{ p }}</span>
            </p>
            <p v-if="stream.fallbackReason" class="notice warn" role="alert">
              <strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi; lütfen metni düzenleyin. Şablon da tüm sınırlara uyar.
            </p>
            <p v-if="stream.error" class="notice error" role="alert">{{ stream.error.message }}</p>

            <MessageEditor
              v-if="limits && (hasContent || streaming)"
              v-model="draft"
              :readonly="streaming || frozen"
              :typing="streaming"
              :limits="limits"
              :variables="variables"
              :server-issues="serverIssues"
              :warnings="warnings"
            />
            <div v-else-if="!streaming" class="empty">
              <p class="muted">Henüz mesaj yok.</p>
              <button v-if="segmentReady" class="primary" @click="generate">Mesajları üret</button>
            </div>

            <p v-if="source && hasContent && !streaming" class="muted small src">
              Kaynak: {{ source === 'llm' ? 'model' : source === 'fallback' ? 'hazır şablon' : 'elle düzenlendi' }}
              <template v-if="unsaved && !frozen"> · kaydedilmemiş düzenleme var, “Onayla” kaydeder</template>
            </p>
            <p v-if="saveError" class="notice error" role="alert">{{ saveError }}</p>
          </section>

          <!-- 4 · Onay -->
          <template v-else-if="view === 'approve'">
            <ApprovalCard
              v-if="campaign?.segment && (campaign.status === 'content_ready' || frozen)"
              :segment="campaign.segment"
              :warnings="warnings"
              :confirming="confirming"
              :frozen="frozen"
              :error="approveError"
            />
            <section v-else class="card">
              <h2>Onay</h2>
              <p class="muted">Onaya geçmek için önce mesajları üretip kaydedin.</p>
            </section>
          </template>

          <!-- 5 · Gönderim -->
          <section v-else class="card" data-testid="delivery">
            <h2>Gönderim</h2>
            <template v-if="campaign?.delivery">
              <p v-if="approved" class="muted small" data-testid="queued">
                Kuyruğa alınan: {{ approved.queued.sms }} SMS, {{ approved.queued.email }} e-posta.
                <template v-if="approved.already_approved">(Bu kampanya zaten onaylanmıştı.)</template>
              </p>
              <p v-if="difference" class="notice info" role="status">{{ difference }}</p>
              <DeliveryProgress :delivery="campaign.delivery" :status="campaign.status" />
            </template>
            <p v-else class="muted">Kampanya henüz onaylanmadı.</p>
          </section>
        </div>
      </Transition>
    </div>

    <footer class="bar">
      <div class="bar-in">
        <button v-if="view !== 'brief'" class="back" :disabled="busy" data-testid="back" @click="onBack">
          ← {{ confirming ? 'Vazgeç' : 'Geri' }}
        </button>
        <button class="primary main" :disabled="barDisabled" :data-testid="barTestId" data-action="primary" @click="onPrimary">
          {{ barLabel }}
        </button>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.wizard {
  display: flex;
  flex-direction: column;
}
.stage {
  /* clip (not hidden) keeps the previews' sticky position and shadows working */
  overflow-x: clip;
  padding-bottom: 24px;
}
.panel {
  display: flex;
  flex-direction: column;
  gap: 18px;
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
  transform: translateX(-36px);
  opacity: 0;
}
.slide-left-enter-from {
  transform: translateX(36px);
  opacity: 0;
}
.slide-right-leave-to {
  transform: translateX(36px);
  opacity: 0;
}
.slide-right-enter-from {
  transform: translateX(-36px);
  opacity: 0;
}

/* the buttons sit right under the step content */
.bar {
  padding: 4px 0 0;
}
.bar-in {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.main {
  margin-left: auto;
  min-width: 160px;
}
.back {
  background: none;
  border-color: transparent;
  color: var(--muted);
}
.back:hover:not(:disabled) {
  color: var(--fg);
}

.card textarea {
  min-height: 140px;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.head h2 {
  margin: 0;
}
.regen {
  margin-left: auto;
}
.working {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 3px 12px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--info);
  font-size: 0.82rem;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  transition: background var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
}
.working.fixing {
  background: var(--warn-bg);
  color: var(--warn);
}
.pulse {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
  animation: pulse 1s ease-in-out infinite;
}
@keyframes pulse {
  50% {
    opacity: 0.25;
  }
}
.actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 10px 0 0;
  flex-wrap: wrap;
}
.empty {
  padding: 28px 0;
  text-align: center;
}
.src {
  margin: 4px 0 0;
}
.block {
  display: block;
}
details {
  margin-top: 6px;
}

@media (max-width: 640px) {
  .main {
    flex: 1;
  }
}
</style>
