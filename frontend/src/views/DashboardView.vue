<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Inbox, Plus } from 'lucide-vue-next'
import type { AppInfo } from '../api/info'
import { cancelCampaign, listCampaigns } from '../api/campaigns'
import type { CampaignSummary } from '../types'
import { healthWord, type HealthItem } from '../utils/health'
import { briefTitle, isSending, type ChannelRates } from '../utils/dashboard'
import { pushSample, ratePerSecond, type Sample } from '../utils/rate'
import { formatWhen } from '../utils/schedule'
import CampaignTable from '../components/CampaignTable.vue'
import Button from '../components/ui/Button.vue'
import Modal from '../components/ui/Modal.vue'

// down: the services that do not answer (the shell asks /api/system); a red alert tops the list.
defineProps<{ info: AppInfo | null; down?: HealthItem[] }>()

const POLL_MS = 2000

const campaigns = ref<CampaignSummary[] | null>(null)
const error = ref<string | null>(null)
// Sent-so-far readings of each sending campaign's channels, to measure the speed from (last 10 s).
const samples = ref<Record<string, Sample[]>>({})
let timer: number | null = null

async function refresh() {
  try {
    const list = await listCampaigns()
    campaigns.value = list
    error.value = null
    track(list)
  } catch (e) {
    // Keep showing the last list; say that it may be stale.
    error.value = e instanceof Error ? e.message : String(e)
  }
}

function track(list: CampaignSummary[]) {
  const at = performance.now()
  const next: Record<string, Sample[]> = {}
  for (const c of list) {
    if (!c.delivery || !isSending(c)) continue // a finished campaign shows its average instead
    for (const ch of ['sms', 'email'] as const) {
      const key = `${c.id}:${ch}`
      next[key] = pushSample(samples.value[key] ?? [], { at, sent: c.delivery[ch].sent })
    }
  }
  samples.value = next
}

// Cancelling asks first, in a dialog. The server refuses once a campaign has started;
// the refreshed list then shows where it stands.
const toCancel = ref<CampaignSummary | null>(null)
const cancelling = ref(false)
// Kept apart from the list error: the next refresh clears that one, and this one must stay readable.
const cancelError = ref<string | null>(null)
const askCancel = computed({
  get: () => toCancel.value !== null,
  set: (open: boolean) => {
    if (!open) toCancel.value = null
  },
})
async function confirmCancel() {
  const c = toCancel.value
  if (!c || cancelling.value) return
  cancelling.value = true
  cancelError.value = null
  try {
    await cancelCampaign(c.id)
  } catch (e) {
    cancelError.value = e instanceof Error ? e.message : String(e)
  } finally {
    cancelling.value = false
    toCancel.value = null
  }
  await refresh()
}
const cancelText = computed(() => {
  const c = toCancel.value
  if (!c) return ''
  const when = c.scheduled_at ? ` ${formatWhen(c.scheduled_at)} için` : ''
  return `“${briefTitle(c.brief, 60)}”${when} zamanlanmıştı. İptal edersen hiçbir mesaj gönderilmez.`
})

onMounted(() => {
  void refresh()
  timer = window.setInterval(refresh, POLL_MS)
})
onBeforeUnmount(() => {
  if (timer !== null) window.clearInterval(timer)
})

const active = computed(() => (campaigns.value ?? []).filter(isSending).length)
// Live speed per sending campaign and channel (needs two readings).
const rates = computed(() => {
  const out: Record<number, ChannelRates> = {}
  for (const c of campaigns.value ?? []) {
    const sms = samples.value[`${c.id}:sms`]
    const email = samples.value[`${c.id}:email`]
    if (sms && sms.length >= 2) out[c.id] = { sms: ratePerSecond(sms), email: ratePerSecond(email ?? []) }
  }
  return out
})

const newCampaign = () => (window.location.hash = '#/')
</script>

<template>
  <section class="dash">
    <p v-if="down?.length" class="notice error" role="alert" data-testid="system-alert">
      <strong>Servis yanıt vermiyor:</strong> {{ down.map((d) => `${d.label} (${healthWord(d.state)})`).join(', ') }}
    </p>
    <div class="page-head">
      <h1>Kampanyalar</h1>
      <Button variant="cta" :icon="Plus" data-testid="new-campaign" @click="newCampaign">Yeni kampanya</Button>
    </div>

    <p v-if="error" class="notice error" role="alert">Liste güncellenemedi ({{ error }}). Gösterilen bilgi eski olabilir.</p>
    <p v-if="cancelError" class="notice error" role="alert" data-testid="cancel-error">Gönderim iptal edilemedi: {{ cancelError }}</p>

    <p v-if="campaigns && campaigns.length > 0" class="active num" data-testid="active">
      Şu an aktif gönderim: <strong>{{ active }}</strong> kampanya
    </p>

    <p v-if="campaigns === null && !error" class="muted">Yükleniyor…</p>
    <div v-else-if="campaigns && campaigns.length === 0" class="empty" data-testid="empty">
      <span class="empty__icon" aria-hidden="true"><Inbox :size="22" /></span>
      <strong>Henüz kampanya yok</strong>
      <Button variant="cta" :icon="Plus" @click="newCampaign">Yeni kampanya</Button>
    </div>

    <CampaignTable v-if="campaigns && campaigns.length > 0" :campaigns="campaigns" :locales="info?.locales" :rates="rates" @cancel="toCancel = $event" />

    <Modal v-model="askCancel" title="Zamanlanmış gönderim iptal edilsin mi?" :description="cancelText">
      <template #footer>
        <Button variant="ghost" :disabled="cancelling" @click="toCancel = null">Vazgeç</Button>
        <Button variant="danger" :loading="cancelling" data-testid="confirm-cancel" @click="confirmCancel">Gönderimi iptal et</Button>
      </template>
    </Modal>
  </section>
</template>

<style scoped>
.dash {
  display: grid;
  gap: var(--s-5);
}
.active {
  margin: 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.active strong {
  color: var(--fg);
}
.empty {
  display: grid;
  justify-items: center;
  gap: var(--s-3);
  padding: var(--s-10) var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  text-align: center;
}
.empty__icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--accent-soft);
  color: var(--accent);
}
</style>
