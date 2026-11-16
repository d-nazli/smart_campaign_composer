<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { listCampaigns } from '../api/campaigns'
import type { CampaignSummary, ChannelProgress } from '../types'
import { briefPreview, deliveryTotals, statusLabel } from '../utils/dashboard'
import { channelPercent } from '../utils/delivery'

const POLL_MS = 2000

const campaigns = ref<CampaignSummary[] | null>(null)
const error = ref<string | null>(null)
let timer: number | null = null

async function refresh() {
  try {
    campaigns.value = await listCampaigns()
    error.value = null
  } catch (e) {
    // Keep showing the last list; say that it may be stale.
    error.value = e instanceof Error ? e.message : String(e)
  }
}

onMounted(() => {
  void refresh()
  timer = window.setInterval(refresh, POLL_MS)
})
onBeforeUnmount(() => {
  if (timer !== null) window.clearInterval(timer)
})

const nf = new Intl.NumberFormat('tr-TR')
const when = (iso: string) => new Date(iso).toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' })
const channels = (c: CampaignSummary): { key: string; label: string; p: ChannelProgress }[] =>
  c.delivery
    ? [
        { key: 'sms', label: 'SMS', p: c.delivery.sms },
        { key: 'email', label: 'E-posta', p: c.delivery.email },
      ]
    : []
</script>

<template>
  <section class="dash" aria-live="polite">
    <h2>Kampanyalar</h2>
    <p v-if="error" class="notice error" role="alert">
      Liste güncellenemedi ({{ error }}). Gösterilen bilgi eski olabilir.
    </p>
    <p v-if="campaigns === null && !error" class="muted">Yükleniyor…</p>
    <p v-else-if="campaigns && campaigns.length === 0" class="muted" data-testid="empty">
      Henüz kampanya yok. “Yeni kampanya” sekmesinden bir brief yazın.
    </p>

    <table v-if="campaigns && campaigns.length > 0" data-testid="campaign-table">
      <thead>
        <tr>
          <th>#</th>
          <th>Brief</th>
          <th>Durum</th>
          <th>Gönderim</th>
          <th class="num">Gönderildi</th>
          <th class="num">Başarısız</th>
          <th class="num">Tekrar</th>
          <th class="num">429</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="c in campaigns" :key="c.id" :data-testid="`row-${c.id}`">
          <td>
            <a :href="`/?c=${c.id}`">{{ c.id }}</a>
          </td>
          <td class="brief">
            <span :title="c.brief">{{ briefPreview(c.brief, 90) }}</span>
            <span class="muted small block">{{ when(c.created_at) }}</span>
          </td>
          <td>
            <span class="status" :class="c.status">{{ statusLabel(c) }}</span>
          </td>
          <td class="progress-cell">
            <template v-if="c.delivery">
              <div v-for="r in channels(c)" :key="r.key" class="chan">
                <span class="label">{{ r.label }}</span>
                <progress :value="channelPercent(r.p)" max="100" :aria-label="`${r.label} ilerlemesi`" />
                <span class="small muted">{{ nf.format(r.p.sent + r.p.failed + r.p.skipped) }}/{{ nf.format(r.p.total) }}</span>
              </div>
            </template>
            <span v-else class="muted small">—</span>
          </td>
          <td class="num" data-testid="sent">{{ c.delivery ? nf.format(deliveryTotals(c.delivery).sent) : '—' }}</td>
          <td class="num" :class="{ bad: c.delivery && deliveryTotals(c.delivery).failed > 0 }" data-testid="failed">
            {{ c.delivery ? nf.format(deliveryTotals(c.delivery).failed) : '—' }}
          </td>
          <td class="num" data-testid="retries">{{ c.delivery ? nf.format(deliveryTotals(c.delivery).retries) : '—' }}</td>
          <td class="num" data-testid="rate-limited">
            {{ c.delivery ? nf.format(deliveryTotals(c.delivery).rateLimited) : '—' }}
          </td>
        </tr>
      </tbody>
    </table>
    <p class="muted small">
      Sayfa her {{ POLL_MS / 1000 }} saniyede güncellenir. “Tekrar” yeniden denemeleri, “429” bunların kaçının provider’ın hız
      sınırından geldiğini gösterir.
    </p>
  </section>
</template>

<style scoped>
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.92rem;
}
th,
td {
  text-align: left;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
}
th {
  color: var(--muted);
  font-weight: 600;
  font-size: 0.8rem;
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.bad {
  color: var(--error);
  font-weight: 600;
}
.brief {
  max-width: 320px;
}
.block {
  display: block;
}
.status {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--surface);
  border: 1px solid var(--border);
  font-size: 0.8rem;
  white-space: nowrap;
}
.status.sending,
.status.approved {
  background: var(--info-bg);
  color: var(--info);
}
.status.completed {
  background: var(--ok-bg);
  color: var(--ok);
}
.progress-cell {
  min-width: 190px;
}
.chan {
  display: grid;
  grid-template-columns: 52px 1fr auto;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}
.label {
  font-size: 0.8rem;
}
progress {
  width: 100%;
  height: 10px;
  accent-color: var(--accent);
}
</style>
