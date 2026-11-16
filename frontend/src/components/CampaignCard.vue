<script setup lang="ts">
import { computed } from 'vue'
import { ref } from 'vue'
import type { CampaignSummary } from '../types'
import { briefPreview, deliveryTotals, statusLabel, statusTone } from '../utils/dashboard'
import { failureText, mergeFailures } from '../utils/failures'
import { formatWhen, untilText } from '../utils/schedule'
import AnimatedNumber from './AnimatedNumber.vue'
import ChannelBar from './ChannelBar.vue'

const props = defineProps<{ campaign: CampaignSummary }>()
const emit = defineEmits<{ cancel: [id: number] }>()
const confirming = ref(false)
const now = ref(new Date())

const when = (iso: string) => new Date(iso).toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' })
const totals = computed(() => deliveryTotals(props.campaign.delivery))
const tone = computed(() => statusTone(props.campaign))
const reasons = computed(() => mergeFailures(props.campaign.delivery?.failures))
// The word on the badge: "Taslak", "Gönderiliyor", "Tamamlandı"; a stuck draft keeps its reason.
const badge = computed(() => {
  const label = statusLabel(props.campaign)
  return props.campaign.status === 'segment_ready' || props.campaign.status === 'content_ready' ? `Taslak · ${label}` : label
})
</script>

<template>
  <article class="ccard" :class="tone" :data-testid="`row-${campaign.id}`">
    <header>
      <a :href="`/?c=${campaign.id}`" class="id num">#{{ campaign.id }}</a>
      <span class="status" :class="tone">
        <span class="dot" aria-hidden="true" />{{ badge }}
      </span>
      <time class="muted small when" :datetime="campaign.created_at">{{ when(campaign.created_at) }}</time>
    </header>

    <p class="brief" :title="campaign.brief">{{ briefPreview(campaign.brief, 140) }}</p>

    <div v-if="campaign.status === 'scheduled'" class="sched" data-testid="scheduled">
      <p class="when-line">
        <strong>{{ campaign.scheduled_at ? formatWhen(campaign.scheduled_at) : '' }}</strong>
        <span v-if="campaign.scheduled_at" class="muted"> · {{ untilText(campaign.scheduled_at, now) }}</span>
      </p>
      <p class="muted small">Kitle gönderim anında belirlenecek; kişi sayısı değişebilir.</p>
      <div class="cancel">
        <button v-if="!confirming" class="link danger" data-testid="cancel" @click="confirming = true">İptal et</button>
        <template v-else>
          <button class="primary danger" data-testid="confirm-cancel" @click="emit('cancel', campaign.id)">Evet, iptal et</button>
          <button class="link" @click="confirming = false">Vazgeç</button>
        </template>
      </div>
    </div>
    <p v-else-if="campaign.status === 'failed'" class="notice error" role="alert">
      Başlatılamadı: {{ campaign.schedule_error || 'bilinmeyen neden' }}
    </p>
    <p v-else-if="campaign.status === 'cancelled'" class="muted small idle">Zamanlanmış gönderim iptal edildi; mesaj gönderilmedi.</p>

    <template v-else-if="campaign.delivery">
      <div class="bars">
        <ChannelBar channel="sms" label="SMS" :p="campaign.delivery.sms" />
        <ChannelBar channel="email" label="E-posta" :p="campaign.delivery.email" />
      </div>

      <dl class="stats">
        <div data-testid="sent">
          <dt>Gönderildi</dt>
          <dd class="num"><AnimatedNumber :value="totals.sent" /></dd>
        </div>
        <div data-testid="failed" :class="{ bad: totals.failed > 0 }">
          <dt>Başarısız</dt>
          <dd class="num"><AnimatedNumber :value="totals.failed" /></dd>
        </div>
        <div data-testid="retries">
          <dt>Tekrar</dt>
          <dd class="num"><AnimatedNumber :value="totals.retries" /></dd>
        </div>
        <div data-testid="rate-limited">
          <dt>429</dt>
          <dd class="num"><AnimatedNumber :value="totals.rateLimited" /></dd>
        </div>
      </dl>

      <ul v-if="reasons.length" class="reasons" aria-label="Başarısızlık nedenleri">
        <li v-for="r in reasons" :key="r.reason"><strong class="num">{{ r.count }}</strong> {{ failureText(r.reason) }}</li>
      </ul>
      <p v-else class="muted small noreasons">Başarısız gönderim yok.</p>
    </template>
    <p v-else class="muted small idle">
      {{ tone === 'attention' ? 'Brief tamamlanamadı; kampanya sayfasında devam edebilirsiniz.' : 'Henüz gönderilmedi.' }}
    </p>
  </article>
</template>

<style scoped>
.ccard {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px 20px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
  min-width: 0;
}
header {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.id {
  font-weight: 650;
  color: var(--fg);
  text-decoration: none;
}
.id:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.when {
  margin-left: auto;
}
.status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 11px 2px 9px;
  border-radius: 999px;
  font-size: 0.8rem;
  font-weight: 650;
  background: var(--surface-2);
  color: var(--muted);
  white-space: nowrap;
  transition: background var(--t-med) var(--ease), color var(--t-med) var(--ease);
}
.status .dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}
.status.sending {
  background: var(--info-bg);
  color: var(--info);
}
.status.sending .dot {
  animation: pulse 1s ease-in-out infinite;
}
.status.done {
  background: var(--ok-bg);
  color: var(--ok);
}
.status.scheduled {
  background: var(--accent-soft);
  color: var(--accent);
}
.sched {
  display: grid;
  gap: 6px;
}
.sched p {
  margin: 0;
}
.cancel {
  display: flex;
  gap: 10px;
  align-items: center;
}
.danger {
  color: var(--error);
}
.status.attention {
  background: var(--warn-bg);
  color: var(--warn);
}
@keyframes pulse {
  50% {
    opacity: 0.25;
  }
}
.brief {
  margin: 0;
  /* two lines are always reserved, so cards in a row stay level */
  min-height: 3em;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.bars {
  display: grid;
  gap: 12px;
}
.stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin: 0;
}
.stats div {
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--surface-2);
}
dt {
  font-size: 0.76rem;
  color: var(--muted);
}
dd {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 650;
  line-height: 1.25;
}
.stats .bad {
  background: var(--error-bg);
}
.stats .bad dt,
.stats .bad dd {
  color: var(--error);
}
.reasons {
  list-style: none;
  margin: 0;
  padding: 9px 12px;
  border-radius: 10px;
  background: var(--error-bg);
  color: var(--error);
  font-size: 0.86rem;
}
.reasons li + li {
  margin-top: 3px;
}
.noreasons,
.idle {
  margin: 0;
}
</style>
