<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CheckCircle2 } from 'lucide-vue-next'
import type { CampaignStatus, DeliveryProgress, LocaleInfo } from '../types'
import { channelAverage, deliveryTotals, duration } from '../utils/dashboard'
import { pushSample, ratePerSecond, type Sample } from '../utils/rate'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LOCALES, localeSummary } from '../utils/locales'
import { comparisonRows } from '../utils/variants'
import Badge from './ui/Badge.vue'
import HalfDonut from './ui/HalfDonut.vue'
import KpiGroup from './ui/KpiGroup.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

// The sending step, laid out like a campaign report. Each number appears once: one figure
// group per channel (sent, then failed / skipped / waiting), one bar for the whole campaign,
// the outcome (delivered / each failure reason) as a half ring, and one line of locales.
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

// The outcome ring: every message of the campaign is the whole; delivered in blue, each failure
// reason in its own shade of red, what is still waiting is the empty track. Every colour also has
// a labelled row underneath, so the colour never carries the meaning alone.
// Speed per channel (the provider's limit is 100/s for each channel). The live rate is measured
// here from the progress the page polls; the average and the time come from the server's
// first and last accepted message.
const samples = ref<Record<'sms' | 'email', Sample[]>>({ sms: [], email: [] })
watch(
  () => [props.delivery.sms.sent, props.delivery.email.sent] as const,
  ([sms, email]) => {
    const at = performance.now()
    samples.value = { sms: pushSample(samples.value.sms, { at, sent: sms }), email: pushSample(samples.value.email, { at, sent: email }) }
  },
  { immediate: true },
)
const sending = computed(() => !finished.value && !props.delivery.done)
const speedRows = computed(() =>
  rows.value
    .filter((r) => r.p.total > 0)
    .map((r) => {
      const avg = channelAverage(r.p)
      const live = samples.value[r.key as 'sms' | 'email']
      return {
        key: r.key,
        name: r.key === 'sms' ? 'SMS' : 'E-posta',
        live: sending.value && live.length >= 2 ? `${Math.round(ratePerSecond(live))}/sn` : '–',
        avg: avg ? `${Math.round(avg.perSecond)}/sn` : '–',
        time: avg ? duration(avg.seconds) : '–',
        retries: r.p.retries,
        limited: r.p.rate_limited,
      }
    }),
)

const SENT_COLOR = 'var(--chart-1)'
const REDS = ['var(--error)', 'color-mix(in srgb, var(--error) 70%, var(--surface))', 'color-mix(in srgb, var(--error) 50%, var(--surface))', 'color-mix(in srgb, var(--error) 35%, var(--surface))']
const SKIPPED_COLOR = 'var(--muted)'
const failedTotal = computed(() => reasons.value.reduce((n, r) => n + r.count, 0))
const pct = (n: number) => (totals.value.total > 0 ? ((n / totals.value.total) * 100).toLocaleString('tr-TR', { maximumFractionDigits: 2 }) : '0')
const count = (n: number) => n.toLocaleString('tr-TR')
const outcome = computed(() => [
  { key: 'sent', text: 'Başarılı', value: totals.value.sent, color: SENT_COLOR },
  ...reasons.value.map((r, i) => ({ key: r.reason, text: failureText(r.reason), value: r.count, color: REDS[Math.min(i, REDS.length - 1)] })),
  ...(totals.value.skipped > 0 ? [{ key: 'skipped', text: 'Atlandı (izin geri çekildi ya da adres yok)', value: totals.value.skipped, color: SKIPPED_COLOR }] : []),
])
const arcs = computed(() => outcome.value.map((o) => ({ value: o.value, color: o.color })))
// Variant A against Variant B (only for a campaign sent with B): sent / queued per channel.
const comparison = computed(() => comparisonRows(props.delivery.variants))
const sentOf = (p: { total: number; sent: number }) => (finished.value || p.sent === p.total ? count(p.sent) : `${count(p.sent)} / ${count(p.total)}`)
const ringLabel = computed(
  () => `Gönderim sonucu: ${count(totals.value.total)} mesajın ${count(totals.value.sent)} tanesi başarılı, ${count(failedTotal.value)} tanesi başarısız (%${pct(failedTotal.value)})`,
)
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

    <section v-if="speedRows.length" class="panel" data-testid="speed">
      <h3 class="label">Hız · kanal başına en fazla 100/sn</h3>
      <div class="speed-wrap">
        <table class="speed">
          <thead>
            <tr>
              <th scope="col">Kanal</th>
              <th v-if="sending" scope="col" class="r">Anlık</th>
              <th scope="col" class="r">Ortalama</th>
              <th scope="col" class="r">{{ sending ? 'Geçen süre' : 'Süre' }}</th>
              <th scope="col" class="r">Tekrar deneme</th>
              <th scope="col" class="r">429</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in speedRows" :key="r.key" :data-testid="`speed-${r.key}`">
              <th scope="row">{{ r.name }}</th>
              <td v-if="sending" class="r num" data-testid="speed-live">{{ r.live }}</td>
              <td class="r num" data-testid="speed-avg">{{ r.avg }}</td>
              <td class="r num" data-testid="speed-time">{{ r.time }}</td>
              <td class="r num">{{ count(r.retries) }}</td>
              <td class="r num">{{ count(r.limited) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div class="two">
      <section class="panel" data-testid="outcome">
        <h3 class="label">Gönderim sonucu</h3>
        <HalfDonut :parts="arcs" :max="totals.total" :label="ringLabel">
          <strong class="big num" data-testid="outcome-total">{{ count(totals.sent) }} / {{ count(totals.total) }}</strong>
          <span class="of num muted" data-testid="outcome-share">başarılı · %{{ pct(failedTotal) }} başarısız</span>
        </HalfDonut>
        <ul class="reasons" aria-label="Gönderim sonucu ayrıntısı">
          <li v-for="o in outcome" :key="o.key" class="leader" :data-testid="`outcome-${o.key}`">
            <i class="dot" :style="{ background: o.color }" aria-hidden="true" />
            <span>{{ o.text }}</span>
            <span class="leader__fill" />
            <span class="num">%{{ pct(o.value) }} (<strong>{{ count(o.value) }}</strong>)</span>
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

    <section v-if="comparison.length" class="panel" data-testid="variant-comparison">
      <h3 class="label">Varyant karşılaştırma · %50 / %50</h3>
      <div class="speed-wrap">
        <table class="speed">
          <thead>
            <tr>
              <th scope="col">Varyant</th>
              <th scope="col" class="r">Kişi</th>
              <th scope="col" class="r">SMS</th>
              <th scope="col" class="r">E-posta</th>
              <th scope="col" class="r">Başarısız</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in comparison" :key="r.label" :class="{ total: r.label === 'Toplam' }" :data-testid="`compare-${r.label === 'Toplam' ? 'total' : r.variant}`">
              <th scope="row">
                <i v-if="r.label !== 'Toplam'" class="vkey" :class="`vkey--${r.variant}`" aria-hidden="true" />{{ r.label }}
              </th>
              <td class="r num"><b>{{ count(r.people) }}</b></td>
              <td class="r num">{{ sentOf(r.sms) }}</td>
              <td class="r num">{{ sentOf(r.email) }}</td>
              <td class="r num" :class="{ bad: r.failed > 0 }">{{ count(r.failed) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

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
  font-size: 1.375rem;
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
.speed-wrap {
  overflow-x: auto;
}
.speed {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}
.speed th,
.speed td {
  padding: var(--s-2) var(--s-3);
  border-top: 1px solid var(--border);
  text-align: left;
  white-space: nowrap;
}
.speed thead th {
  border-top: none;
  color: var(--muted);
  font-weight: 500;
}
.speed tbody th {
  font-weight: 500;
}
.speed .r {
  text-align: right;
}
.locs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.speed tr.total {
  background: var(--surface-2);
}
.speed tr.total th {
  font-weight: 600;
}
.vkey {
  display: inline-block;
  width: 10px;
  height: 10px;
  margin-right: var(--s-2);
  border-radius: 2px;
}
.vkey--A {
  background: var(--accent);
}
.vkey--B {
  background: var(--chart-3);
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
