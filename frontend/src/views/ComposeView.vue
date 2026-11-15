<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { AppInfo } from '../api/info'
import { createCampaign, getCampaign, saveContent } from '../api/campaigns'
import { ApiError } from '../api/http'
import { useGenerationStream } from '../composables/useGenerationStream'
import { emptyContent } from '../composables/streamState'
import type { Campaign, Issue, MessageContent } from '../types'
import { checkContent } from '../utils/limits'
import MessageEditor from '../components/MessageEditor.vue'
import SegmentCard from '../components/SegmentCard.vue'

const props = defineProps<{ info: AppInfo | null }>()

const EXAMPLE_BRIEF =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

const brief = ref('')
const campaign = ref<Campaign | null>(null)
const creating = ref(false)
const createError = ref<ApiError | null>(null)

const { state: stream, start: startStream, reset: resetStream } = useGenerationStream()
const draft = ref<MessageContent>(emptyContent())
const saving = ref(false)
const saveError = ref<string | null>(null)
const serverIssues = ref<Issue[]>([])
const savedNote = ref(false)

const limits = computed(() => props.info?.limits)
const variables = computed(() => props.info?.variables ?? [])
const streaming = computed(() => stream.value.phase === 'streaming')
const segmentReady = computed(() => campaign.value?.segment?.status === 'ok' && campaign.value.status !== 'draft')
const hasContent = computed(() => stream.value.result !== null || campaign.value?.content != null)

const localCheck = computed(() =>
  limits.value ? checkContent(draft.value, limits.value, variables.value) : { smsLength: 0, subjectLength: 0, issues: [] },
)
const canSave = computed(() => hasContent.value && !streaming.value && !saving.value && localCheck.value.issues.length === 0)

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
  resetStream()
  draft.value = emptyContent()
  try {
    const c = await createCampaign(brief.value)
    adopt(c)
    setUrl(c.id)
  } catch (e) {
    createError.value = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
    setUrl(null)
  } finally {
    creating.value = false
  }
}

async function generate() {
  if (!campaign.value) return
  savedNote.value = false
  serverIssues.value = []
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
  if (!campaign.value || !canSave.value) return
  saving.value = true
  saveError.value = null
  serverIssues.value = []
  savedNote.value = false
  try {
    adopt(await saveContent(campaign.value.id, draft.value))
    resetStream()
    savedNote.value = true
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
  if (!Number.isInteger(id) || id < 1) return
  try {
    const c = await getCampaign(id)
    brief.value = c.brief
    adopt(c)
  } catch {
    setUrl(null)
  }
})
</script>

<template>
  <div class="compose">
    <section class="card">
      <h2>1 · Brief</h2>
      <label for="brief" class="sr-only">Kampanya brief'i</label>
      <textarea
        id="brief"
        v-model="brief"
        rows="4"
        maxlength="2000"
        placeholder="Kime, ne zaman, ne göndermek istediğinizi doğal dille yazın…"
        :disabled="creating"
        @keydown.ctrl.enter="submitBrief"
      />
      <div class="actions">
        <button class="primary" :disabled="creating || brief.trim().length < 5" @click="submitBrief">
          {{ creating ? 'Kitle çıkarılıyor…' : 'Kitleyi çıkar' }}
        </button>
        <button class="link" :disabled="creating" @click="brief = EXAMPLE_BRIEF">Örnek brief</button>
      </div>

      <p v-if="createError" class="notice error" role="alert">
        {{ createError.message }}
        <template v-if="createError.details?.attempts?.length">
          <details>
            <summary>Ayrıntılar</summary>
            <ul>
              <li v-for="(a, i) in createError.details.attempts" :key="i">
                Deneme {{ i + 1 }}: {{ a.join('; ') }}
              </li>
            </ul>
          </details>
        </template>
      </p>
    </section>

    <SegmentCard v-if="campaign?.segment" :segment="campaign.segment" />

    <section v-if="segmentReady && limits" class="card">
      <h2>2 · Mesajlar</h2>

      <div class="actions">
        <button class="primary" :disabled="streaming" @click="generate">
          {{ streaming ? 'Üretiliyor…' : hasContent ? 'Yeniden üret' : 'Mesajları üret' }}
        </button>
        <span v-if="streaming && stream.attempt" class="muted small" role="status">
          Deneme {{ stream.attempt.n }}/{{ stream.attempt.max }}
        </span>
      </div>

      <p v-if="streaming && stream.retryProblems.length" class="notice info" role="status">
        Önceki deneme sınırlara uymadığı için model düzeltiyor:
        <span v-for="p in stream.retryProblems" :key="p" class="block">• {{ p }}</span>
      </p>
      <p v-if="stream.fallbackReason" class="notice warn" role="alert">
        <strong>{{ fallbackText }}</strong> Hazır bir şablon yerleştirildi; lütfen metni düzenleyin. Şablon da tüm sınırlara uyar.
      </p>
      <p v-if="stream.error" class="notice error" role="alert">{{ stream.error.message }}</p>

      <MessageEditor
        v-if="hasContent || streaming"
        v-model="draft"
        :readonly="streaming"
        :limits="limits"
        :variables="variables"
        :server-issues="serverIssues"
      />

      <template v-if="hasContent && !streaming">
        <div class="actions">
          <button class="primary" :disabled="!canSave" @click="save">{{ saving ? 'Kaydediliyor…' : 'Düzenlemeyi kaydet' }}</button>
          <span v-if="source" class="muted small">
            Kaynak:
            {{ source === 'llm' ? 'model' : source === 'fallback' ? 'hazır şablon' : 'elle düzenlendi' }}
          </span>
        </div>
        <p v-if="saveError" class="notice error" role="alert">{{ saveError }}</p>
        <p v-if="savedNote" class="notice ok" role="status">Kaydedildi. Metin tüm sınırlara uyuyor.</p>
        <p class="muted small">Onay ve gönderim sonraki aşamada eklenecek.</p>
      </template>
    </section>
  </div>
</template>

<style scoped>
.compose {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.card textarea#brief {
  width: 100%;
  font: inherit;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg);
  color: var(--fg);
  resize: vertical;
}
.actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 12px 0;
  flex-wrap: wrap;
}
.block {
  display: block;
}
details {
  margin-top: 6px;
}
</style>
