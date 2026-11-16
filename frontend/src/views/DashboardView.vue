<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { AppInfo } from '../api/info'
import { cancelCampaign, listCampaigns } from '../api/campaigns'
import { fetchSystem, type SystemHealth as SystemState } from '../api/system'
import type { CampaignSummary } from '../types'
import { overallTotals } from '../utils/dashboard'
import { pushSample, ratePerSecond, type Sample } from '../utils/rate'
import AnimatedNumber from '../components/AnimatedNumber.vue'
import CampaignCard from '../components/CampaignCard.vue'
import SystemHealth from '../components/SystemHealth.vue'

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

async function cancel(id: number) {
  try {
    await cancelCampaign(id)
    error.value = null
  } catch (e) {
    // The server refuses when the campaign has already started; the refreshed list shows where it stands.
    error.value = e instanceof Error ? e.message : String(e)
  }
  await refresh()
}

async function refreshHealth() {
  try {
    system.value = await fetchSystem()
    apiUnreachable.value = false
  } catch {
    apiUnreachable.value = true
  }
}

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
</script>

<template>
  <section class="dash">
    <div class="top">
      <h2>Kampanyalar</h2>
      <SystemHealth :system="system" :unreachable="apiUnreachable" :info="info" />
    </div>

    <p v-if="error" class="notice error" role="alert">Liste güncellenemedi ({{ error }}). Gösterilen bilgi eski olabilir.</p>

    <div class="summary" data-testid="summary">
      <div class="stat sent">
        <span class="label">Toplam gönderilen</span>
        <strong class="value num" data-testid="total-sent"><AnimatedNumber :value="totals.sent" /></strong>
        <span class="sub muted small">SMS ve e-posta birlikte</span>
      </div>
      <div class="stat failed" :class="{ bad: totals.failed > 0 }">
        <span class="label">Başarısız</span>
        <strong class="value num" data-testid="total-failed"><AnimatedNumber :value="totals.failed" /></strong>
        <span class="sub small">{{ totals.failed > 0 ? 'Nedenleri kartlarda' : 'Sorun yok' }}</span>
      </div>
      <div class="stat queued">
        <span class="label">Kuyrukta</span>
        <strong class="value num" data-testid="total-pending"><AnimatedNumber :value="totals.pending" /></strong>
        <span class="sub muted small">gönderilmeyi bekliyor</span>
      </div>
      <div class="stat rate">
        <span class="label">Hız</span>
        <strong class="value num" data-testid="rate"><AnimatedNumber :value="shownRate" :decimals="1" /><small>/sn</small></strong>
        <span class="sub muted small">son 10 saniyede</span>
      </div>
    </div>

    <p v-if="campaigns === null && !error" class="muted">Yükleniyor…</p>
    <div v-else-if="campaigns && campaigns.length === 0" class="empty card" data-testid="empty">
      <strong>Henüz kampanya yok</strong>
      <p class="muted">“Yeni kampanya” sekmesinden bir brief yazarak ilk kampanyanı oluştur.</p>
      <a href="#/">Yeni kampanya başlat</a>
    </div>

    <div v-if="campaigns && campaigns.length > 0" class="list" data-testid="campaign-table">
      <CampaignCard v-for="c in campaigns" :key="c.id" :campaign="c" @cancel="cancel" />
    </div>

    <p class="muted small foot">
      Sayfa her {{ POLL_MS / 1000 }} saniyede güncellenir. “Tekrar” yeniden denemeleri, “429” bunların kaçının sağlayıcının hız
      sınırından geldiğini gösterir.
    </p>
  </section>
</template>

<style scoped>
.top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px 24px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.top h2 {
  margin: 0;
  font-size: 1.4rem;
}
.summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 22px;
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 16px 18px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
  min-height: 118px;
  transition: background var(--t-med) var(--ease), border-color var(--t-med) var(--ease);
}
.label {
  color: var(--muted);
  font-size: 0.88rem;
  font-weight: 550;
}
.value {
  font-size: 2.2rem;
  font-weight: 650;
  line-height: 1.15;
  /* a fixed height and tabular digits keep the card from changing size as the number grows */
  min-height: 1.15em;
}
.value small {
  font-size: 0.95rem;
  font-weight: 550;
  color: var(--muted);
  margin-left: 3px;
  letter-spacing: 0;
}
.stat.sent .value {
  color: var(--ok);
}
.stat.bad {
  background: var(--error-bg);
  border-color: var(--error-line);
}
.stat.bad .value,
.stat.bad .label,
.stat.bad .sub {
  color: var(--error);
}
.stat.failed .sub {
  color: var(--muted);
}
.list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 440px), 1fr));
  gap: 16px;
  align-items: start;
}
.empty {
  text-align: center;
  padding: 40px 20px;
}
.empty p {
  margin: 6px 0 14px;
}
.foot {
  margin-top: 22px;
  max-width: 80ch;
}

@media (max-width: 860px) {
  .summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 420px) {
  .value {
    font-size: 1.8rem;
  }
  .stat {
    padding: 12px 14px;
    min-height: 104px;
  }
}
</style>
