<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle2 } from 'lucide-vue-next'
import type { CampaignStatus, DeliveryProgress, LocaleInfo } from '../types'
import { deliveryTotals } from '../utils/dashboard'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LOCALES, localeSummary } from '../utils/locales'
import Badge from './ui/Badge.vue'
import HalfDonut from './ui/HalfDonut.vue'
import KpiGroup from './ui/KpiGroup.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

// The sending step, laid out like a campaign report. Each number appears once: one figure
// group per channel (sent, then failed / skipped / waiting), one bar for the whole campaign,
// the failure reasons as a half ring, and one line of locales.
const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus; locales?: LocaleInfo[] }>()

const finished = computed(() => props.status === 'completed')
const totals = computed(() => deliveryTotals(props.delivery))
const handled = computed(() => totals.value.sent + totals.value.failed + totals.value.skipped)
const reasons = computed(() => mergeFailures(props.delivery.failures))
const rows = computed(() => [
  { key: 'sms', label: 'SMS · gönderilen', p: props.delivery.sms },
  { key: 'email', label: 'E-posta · gönderilen', p: props.delivery.email },
])
// Sent messages per locale, in table order (a locale nobody receives is left out).
const byLocale = computed(() => {
  const progress = props.delivery.locales ?? {}
  const totals = Object.fromEntries(Object.entries(progress).map(([code, p]) => [code, p.total]))
  return localeSummary(totals, props.locales ?? FALLBACK_LOCALES).map((l) => ({ code: l.code, p: progress[l.code] }))
})

const COLORS = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)', 'var(--chart-6)']
const failedTotal = computed(() => reasons.value.reduce((n, r) => n + r.count, 0))
// The ring is drawn against everything that was to be sent, so it shows how big the problem is.
const pct = (n: number) => (totals.value.total > 0 ? ((n / totals.value.total) * 100).toLocaleString('tr-TR', { maximumFractionDigits: 2 }) : '0')
const count = (n: number) => n.toLocaleString('tr-TR')
const arcs = computed(() => reasons.value.map((r, i) => ({ value: r.count, color: COLORS[i % COLORS.length] })))
</script>

<template>
  <div class="delivery">
    <KpiGroup :count="2">
      <StatTile v-for="r in rows" :key="r.key" :label="r.label" :value="r.p.sent" tone="ok" :data-testid="`delivery-${r.key}`">
        <div class="leader">
          <span class="muted">Başarısız</span><span class="leader__fill" /><b class="num" :class="{ bad: r.p.failed > 0 }">{{ r.p.failed }}</b>
        </div>
        <div class="leader"><span class="muted">Atlanan</span><span class="leader__fill" /><b class="num">{{ r.p.skipped }}</b></div>
        <div class="leader"><span class="muted">Bekleyen</span><span class="leader__fill" /><b class="num">{{ r.p.pending }}</b></div>
      </StatTile>
    </KpiGroup>

    <section class="panel">
      <h3 class="label">Gönderim ilerlemesi</h3>
      <ProgressBar
        :segments="[
          { value: totals.sent, tone: 'ok' },
          { value: totals.failed + totals.skipped, tone: 'danger' },
        ]"
        :max="totals.total"
        label="Gönderim ilerlemesi"
      />
      <p class="caption num" role="status" data-testid="delivery-caption">
        <strong>{{ handled }}</strong> / {{ totals.total }} mesaj işlendi
      </p>
    </section>

    <div v-if="reasons.length || byLocale.length" class="two">
      <section v-if="reasons.length" class="panel">
        <h3 class="label">Başarısız mesajlar</h3>
        <HalfDonut :parts="arcs" :max="totals.total" :label="`${count(totals.total)} mesajın ${count(failedTotal)} tanesi başarısız, %${pct(failedTotal)}`">
          <strong class="big num" data-testid="failed-count">{{ count(failedTotal) }}</strong>
          <span class="of num muted" data-testid="failed-share">/ {{ count(totals.total) }} · %{{ pct(failedTotal) }}</span>
        </HalfDonut>
        <ul class="reasons" aria-label="Başarısızlık nedenleri">
          <li v-for="(r, i) in reasons" :key="r.reason" class="leader">
            <i class="dot" :style="{ background: COLORS[i % COLORS.length] }" aria-hidden="true" />
            <span>{{ failureText(r.reason) }}</span>
            <span class="leader__fill" />
            <span class="num">%{{ pct(r.count) }} (<strong>{{ r.count }}</strong>)</span>
          </li>
        </ul>
      </section>

      <section v-if="byLocale.length" class="panel" data-testid="delivery-locales">
        <h3 class="label">Locale'e göre gönderilen</h3>
        <div class="locs">
          <Badge v-for="l in byLocale" :key="l.code">
            {{ l.code }} <b class="num">{{ finished || l.p!.sent === l.p!.total ? l.p!.sent : `${l.p!.sent}/${l.p!.total}` }}</b>
          </Badge>
        </div>
      </section>
    </div>

    <p v-if="finished" class="notice ok done" role="status"><CheckCircle2 :size="16" aria-hidden="true" />Gönderim tamamlandı.</p>
  </div>
</template>

<style scoped>
.delivery {
  display: grid;
  gap: var(--s-4);
}
.panel {
  padding: var(--s-5) var(--s-6);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
  min-width: 0;
}
h3.label {
  margin: 0 0 var(--s-3);
}
.caption {
  margin: var(--s-2) 0 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.caption strong {
  color: var(--fg);
}
.bad {
  color: var(--error);
}
.two {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: var(--s-4);
  align-items: start;
}
.big {
  font-size: 1.875rem;
  font-weight: 400;
}
.of {
  display: block;
  font-size: var(--text-sm);
}
.reasons {
  margin: var(--s-3) 0 0;
  padding: 0;
  list-style: none;
}
.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  transform: translateY(-1px);
}
.locs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.done {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}
@media (max-width: 1023px) {
  .two {
    grid-template-columns: minmax(0, 1fr);
  }
}
@media (max-width: 767px) {
  .panel {
    padding: var(--s-4);
  }
}
</style>
