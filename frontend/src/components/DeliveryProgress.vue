<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle2 } from 'lucide-vue-next'
import type { CampaignStatus, DeliveryProgress, LanguageInfo } from '../types'
import { deliveryTotals } from '../utils/dashboard'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LANGUAGES } from '../utils/languages'
import Badge from './ui/Badge.vue'
import HalfDonut from './ui/HalfDonut.vue'
import KpiGroup from './ui/KpiGroup.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

// The sending step, laid out like a campaign report. Each number appears once: one figure
// group per channel (sent, then failed / skipped / waiting), one bar for the whole campaign,
// the failure reasons as a half ring, and one line of languages.
const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus; languages?: LanguageInfo[] }>()

const finished = computed(() => props.status === 'completed')
const totals = computed(() => deliveryTotals(props.delivery))
const handled = computed(() => totals.value.sent + totals.value.failed + totals.value.skipped)
const reasons = computed(() => mergeFailures(props.delivery.failures))
const rows = computed(() => [
  { key: 'sms', label: 'SMS · gönderilen', p: props.delivery.sms },
  { key: 'email', label: 'E-posta · gönderilen', p: props.delivery.email },
])
// Sent messages per language, in language order (a language nobody receives is left out).
const byLanguage = computed(() =>
  (props.languages ?? FALLBACK_LANGUAGES)
    .map((l) => ({ code: l.code, display: l.display, p: props.delivery.languages?.[l.code] }))
    .filter((l) => l.p && l.p.total > 0),
)

const COLORS = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)', 'var(--chart-6)']
const failedTotal = computed(() => reasons.value.reduce((n, r) => n + r.count, 0))
const arcs = computed(() => reasons.value.map((r, i) => ({ value: r.count, color: COLORS[i % COLORS.length] })))
const share = (n: number) => (failedTotal.value > 0 ? ((n / failedTotal.value) * 100).toLocaleString('tr-TR', { maximumFractionDigits: 1 }) : '0')
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

    <div v-if="reasons.length || byLanguage.length" class="two">
      <section v-if="reasons.length" class="panel">
        <h3 class="label">Başarısız mesajlar</h3>
        <HalfDonut :parts="arcs" :label="`${failedTotal} başarısız mesaj`">
          <strong class="big num">{{ failedTotal }}</strong>
        </HalfDonut>
        <ul class="reasons" aria-label="Başarısızlık nedenleri">
          <li v-for="(r, i) in reasons" :key="r.reason" class="leader">
            <i class="dot" :style="{ background: COLORS[i % COLORS.length] }" aria-hidden="true" />
            <span>{{ failureText(r.reason) }}</span>
            <span class="leader__fill" />
            <span class="num">%{{ share(r.count) }} (<strong>{{ r.count }}</strong>)</span>
          </li>
        </ul>
      </section>

      <section v-if="byLanguage.length" class="panel" data-testid="delivery-languages">
        <h3 class="label">Dile göre gönderilen</h3>
        <div class="langs">
          <Badge v-for="l in byLanguage" :key="l.code">
            {{ l.code.toUpperCase() }} <b class="num">{{ finished || l.p!.sent === l.p!.total ? l.p!.sent : `${l.p!.sent}/${l.p!.total}` }}</b>
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
.langs {
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
