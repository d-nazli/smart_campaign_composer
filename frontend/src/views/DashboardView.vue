<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Inbox, Plus } from 'lucide-vue-next'
import type { AppInfo } from '../api/info'
import { cancelCampaign, listCampaigns } from '../api/campaigns'
import { fetchSystem, type SystemHealth as SystemState } from '../api/system'
import type { CampaignSummary } from '../types'
import { briefTitle, overallTotals } from '../utils/dashboard'
import { pushSample, ratePerSecond, type Sample } from '../utils/rate'
import { formatWhen } from '../utils/schedule'
import CampaignTable from '../components/CampaignTable.vue'
import SystemHealth from '../components/SystemHealth.vue'
import Button from '../components/ui/Button.vue'
import Modal from '../components/ui/Modal.vue'
import StatTile from '../components/ui/StatTile.vue'

defineProps<{ info: AppInfo | null }>()

const POLL_MS = 2000
const HEALTH_MS = 5000

const campaigns = ref<CampaignSummary[] | null>(null)
const error = ref<string | null>(null)
const system = ref<SystemState | null>(null)
const apiUnreachable = ref(false)
const samples = ref<Sample[]>([])
let timer: number | null = null
let healthTimer: number | null = null

async function refresh() {
  try {
    const list = await listCampaigns()
    campaigns.value = list
    error.value = null
    samples.value = pushSample(samples.value, { at: performance.now(), sent: overallTotals(list).sent })
  } catch (e) {
    // Keep showing the last list; say that it may be stale.
    error.value = e instanceof Error ? e.message : String(e)
  }
}

async function refreshHealth() {
  try {
    system.value = await fetchSystem()
    apiUnreachable.value = false
  } catch {
    apiUnreachable.value = true
  }
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
  void refreshHealth()
  timer = window.setInterval(refresh, POLL_MS)
  healthTimer = window.setInterval(refreshHealth, HEALTH_MS)
})
onBeforeUnmount(() => {
  if (timer !== null) window.clearInterval(timer)
  if (healthTimer !== null) window.clearInterval(healthTimer)
})

const totals = computed(() => overallTotals(campaigns.value ?? []))
const rate = computed(() => ratePerSecond(samples.value))
// Rate is only meaningful while something is being sent.
const shownRate = computed(() => (totals.value.pending > 0 ? rate.value : 0))

const newCampaign = () => (window.location.hash = '#/')
</script>

<template>
  <section class="dash">
    <div class="head">
      <h2>Kampanyalar</h2>
      <Button variant="primary" :icon="Plus" data-testid="new-campaign" @click="newCampaign">Yeni kampanya</Button>
    </div>

    <p v-if="error" class="notice error" role="alert">Liste güncellenemedi ({{ error }}). Gösterilen bilgi eski olabilir.</p>
    <p v-if="cancelError" class="notice error" role="alert" data-testid="cancel-error">Gönderim iptal edilemedi: {{ cancelError }}</p>

    <div class="stats" data-testid="summary">
      <StatTile label="Gönderilen" :value="totals.sent" tone="ok" hint="SMS ve e-posta birlikte" data-testid="total-sent" />
      <StatTile label="Başarısız" :value="totals.failed" :tone="totals.failed > 0 ? 'danger' : 'default'" :hint="totals.failed > 0 ? 'Nedenleri satırlarda' : 'Sorun yok'" data-testid="total-failed" />
      <StatTile label="Kuyrukta" :value="totals.pending" hint="Gönderilmeyi bekliyor" data-testid="total-pending" />
      <StatTile label="Hız" :value="shownRate" :decimals="1" suffix="/sn" hint="Son 10 saniye" data-testid="rate" />
    </div>

    <p v-if="campaigns === null && !error" class="muted">Yükleniyor…</p>
    <div v-else-if="campaigns && campaigns.length === 0" class="empty" data-testid="empty">
      <Inbox :size="22" aria-hidden="true" />
      <strong>Henüz kampanya yok</strong>
      <p class="muted">Bir brief yazarak ilk kampanyayı oluşturun.</p>
      <Button variant="primary" :icon="Plus" @click="newCampaign">Yeni kampanya</Button>
    </div>

    <CampaignTable v-if="campaigns && campaigns.length > 0" :campaigns="campaigns" :languages="info?.languages" @cancel="toCancel = $event" />

    <div class="foot">
      <SystemHealth :system="system" :unreachable="apiUnreachable" :info="info" />
      <p class="muted small">Sayfa her {{ POLL_MS / 1000 }} saniyede güncellenir.</p>
    </div>

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
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
}
.head h2 {
  margin: 0;
  font-size: var(--text-lg);
}
/* four figures in one bordered strip, divided by lines */
.stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}
.stats > * + * {
  border-left: 1px solid var(--border);
}
.empty {
  display: grid;
  justify-items: center;
  gap: var(--s-2);
  padding: var(--s-10) var(--s-4);
  border: 1px dashed var(--border-strong);
  border-radius: var(--r-md);
  text-align: center;
  color: var(--muted);
}
.empty p {
  margin: 0 0 var(--s-2);
}
.empty strong {
  color: var(--fg);
}
.foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--s-3);
}
.foot p {
  margin: 0;
}
@media (max-width: 860px) {
  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .stats > :nth-child(3) {
    border-left: none;
  }
  .stats > :nth-child(n + 3) {
    border-top: 1px solid var(--border);
  }
}
</style>
