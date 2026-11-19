<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CheckCircle2, HelpCircle } from 'lucide-vue-next'
import type { CampaignStatus, DeliveryProgress, LocaleInfo } from '../types'
import { channelDuration, deliveryTotals, duration } from '../utils/dashboard'
import { pushSample, ratePerSecond, type Sample } from '../utils/rate'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LOCALES } from '../utils/locales'
import { comparisonRows, formatShare, splitShares } from '../utils/variants'
import HalfDonut from './ui/HalfDonut.vue'
import KpiGroup from './ui/KpiGroup.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

// The sending step, laid out like a campaign report. Each number appears once: one figure
// group per channel (sent, then failed / skipped / waiting), one bar for the whole campaign,
// the outcome (delivered / each failure reason) as a half ring, and the locales largest first.
const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus; locales?: LocaleInfo[] }>()

const finished = computed(() => props.status === 'completed')
const totals = computed(() => deliveryTotals(props.delivery))
const handled = computed(() => totals.value.sent + totals.value.failed + totals.value.skipped)
const reasons = computed(() => mergeFailures(props.delivery.failures))
const rows = computed(() => [
  { key: 'sms', label: 'SMS · gönderilen', p: props.delivery.sms },
  { key: 'email', label: 'E-posta · gönderilen', p: props.delivery.email },
])
// Sent messages per locale, largest first (a locale nobody receives is left out). While sending the
// order follows what goes there, so rows do not jump; once finished, what was sent. The bar is the
// locale's sent messages against the largest locale's number.
const byLocale = computed(() => {
  const all = props.locales ?? FALLBACK_LOCALES
  const key = (p: { total: number; sent: number }) => (finished.value ? p.sent : p.total)
  const rows = Object.entries(props.delivery.locales ?? {})
    .filter(([, p]) => p.total > 0)
    .map(([code, p]) => ({ code, p, name: all.find((l) => l.code === code)?.native_name ?? '' }))
    .sort((a, b) => key(b.p) - key(a.p) || a.code.localeCompare(b.code))
  const max = Math.max(1, ...rows.map((r) => key(r.p)))
  return rows.map((r) => ({ ...r, width: Math.min(100, Math.round((r.p.sent / max) * 1000) / 10) }))
})

// The outcome ring: every message of the campaign is the whole; delivered in blue, each failure
// reason in its own shade of red, what is still waiting is the empty track. Every colour also has
// a labelled row underneath, so the colour never carries the meaning alone.
// Speed per channel (the provider's limit is 100/s for each channel). The live rate is measured
// here from the progress the page polls; the busiest second comes from the server, and the time
// from its first and last accepted message. No average: messages over time includes the retry
// waits, so it reads far below the real sending speed.
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
// The provider's limit per channel; the busiest second ("En yüksek (1 sn)") must stay under it.
const LIMIT = 100
const PROVIDER_429 = 'Sağlayıcının test amaçlı döndürdüğü 429\'lar; bizim hız sınırımız aşılmadı.'
const TIME_HINT = 'Tekrar denenen mesajların beklemesi dahil.'
const speedRows = computed(() =>
  rows.value
    .filter((r) => r.p.total > 0)
    .map((r) => {
      const seconds = channelDuration(r.p)
      const live = samples.value[r.key as 'sms' | 'email']
      return {
        key: r.key,
        name: r.key === 'sms' ? 'SMS' : 'E-posta',
        live: sending.value && live.length >= 2 ? `${Math.round(ratePerSecond(live))}/sn` : '–',
        time: seconds !== null ? duration(seconds) : '–',
        peak: r.p.peak_per_second ? `${r.p.peak_per_second}/sn` : '–',
        over: (r.p.peak_per_second ?? 0) > LIMIT,
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
// The real split of the people, as on the approval step ("%51,6 / %48,4").
const comparisonShares = computed(() => {
  const v = props.delivery.variants
  const a = v?.find((x) => x.variant === 'A')
  const b = v?.find((x) => x.variant === 'B')
  if (!a || !b) return ''
  return splitShares({ A: { people: a.people, sms: 0, email: 0 }, B: { people: b.people, sms: 0, email: 0 } })
    .map((s) => formatShare(s.share))
    .join(' / ')
})
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
              <th scope="col" class="r" data-testid="speed-peak-head">En yüksek (1 sn)</th>
              <th scope="col" class="r" data-testid="speed-limit-head">Sınır</th>
              <th scope="col" class="r" data-testid="speed-time-head">
                <span class="q-head">{{ sending ? 'Geçen süre' : 'Süre' }}<span class="q" :title="TIME_HINT" :aria-label="TIME_HINT" role="img"><HelpCircle :size="13" aria-hidden="true" /></span></span>
              </th>
              <th scope="col" class="r">Tekrar deneme</th>
              <th scope="col" class="r" data-testid="speed-429-head">
                <span class="q-head">429 (provider)<span class="q" :title="PROVIDER_429" :aria-label="PROVIDER_429" role="img"><HelpCircle :size="13" aria-hidden="true" /></span></span>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in speedRows" :key="r.key" :data-testid="`speed-${r.key}`">
              <th scope="row">{{ r.name }}</th>
              <td v-if="sending" class="r num" data-testid="speed-live">{{ r.live }}</td>
              <td class="r num" :class="{ bad: r.over }" data-testid="speed-peak">{{ r.peak }}</td>
              <td class="r num muted" data-testid="speed-limit">{{ LIMIT }}/sn</td>
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

      <section v-if="byLocale.length" class="panel locpanel" data-testid="delivery-locales">
        <h3 class="label">Locale'e göre gönderilen</h3>
        <ol class="loclist">
          <li v-for="l in byLocale" :key="l.code" class="loc" :data-testid="`delivery-locale-${l.code}`">
            <span class="loc__code num">{{ l.code }}</span>
            <span class="loc__mid">
              <span class="loc__name"><bdi>{{ l.name }}</bdi></span>
              <span class="loc__bar" aria-hidden="true"><i :style="{ width: `${l.width}%` }" /></span>
            </span>
            <b class="loc__n num">{{ finished || l.p.sent === l.p.total ? count(l.p.sent) : `${count(l.p.sent)}/${count(l.p.total)}` }}</b>
          </li>
        </ol>
      </section>
    </div>

    <section v-if="comparison.length" class="panel" data-testid="variant-comparison">
      <h3 class="label" data-testid="variant-comparison-head">Varyant karşılaştırma<template v-if="comparisonShares"> · {{ comparisonShares }}</template></h3>
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
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
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
.q-head {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.q {
  display: inline-flex;
  color: var(--muted);
  cursor: help;
}
.speed .r {
  text-align: right;
}
/* Two columns, filled top to bottom so the largest locales read first. */
.loclist {
  margin: 0;
  padding: 0;
  list-style: none;
  columns: 2;
  column-gap: var(--s-5);
}
.loc {
  display: grid;
  grid-template-columns: 3.5rem minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-2) 0;
  break-inside: avoid;
  font-size: var(--text-sm);
}
.loc__code {
  font-weight: 500;
}
.loc__mid {
  display: grid;
  gap: 4px;
  min-width: 0;
}
.loc__name {
  overflow: hidden;
  color: var(--muted);
  font-size: var(--text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.loc__bar {
  display: block;
  height: 3px;
  border-radius: 2px;
  background: var(--surface-2);
  overflow: hidden;
}
.loc__bar i {
  display: block;
  height: 100%;
  background: var(--chart-1);
}
.loc__n {
  font-weight: 500;
  text-align: right;
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
/* Two columns only where a language name still fits next to its code and number. */
.locpanel {
  container-type: inline-size;
}
@container (max-width: 519px) {
  .loclist {
    columns: 1;
  }
}
</style>
