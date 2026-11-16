<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle2 } from 'lucide-vue-next'
import type { CampaignStatus, DeliveryProgress, LanguageInfo } from '../types'
import { deliveryTotals } from '../utils/dashboard'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LANGUAGES } from '../utils/languages'
import Badge from './ui/Badge.vue'
import ProgressBar from './ui/ProgressBar.vue'

// The sending step. Each number appears once: one bar for the whole campaign, one table
// by channel, one line by language.
const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus; languages?: LanguageInfo[] }>()

const finished = computed(() => props.status === 'completed')
const totals = computed(() => deliveryTotals(props.delivery))
const handled = computed(() => totals.value.sent + totals.value.failed + totals.value.skipped)
const reasons = computed(() => mergeFailures(props.delivery.failures))
const rows = computed(() => [
  { key: 'sms', label: 'SMS', p: props.delivery.sms },
  { key: 'email', label: 'E-posta', p: props.delivery.email },
])
// Sent messages per language, in language order (a language nobody receives is left out).
const byLanguage = computed(() =>
  (props.languages ?? FALLBACK_LANGUAGES)
    .map((l) => ({ code: l.code, display: l.display, p: props.delivery.languages?.[l.code] }))
    .filter((l) => l.p && l.p.total > 0),
)
</script>

<template>
  <div class="delivery">
    <div class="overall">
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
        <template v-if="!finished"> – bu sayfa kendiliğinden güncellenir</template>
      </p>
    </div>

    <table class="grid" data-testid="delivery-table">
      <thead>
        <tr>
          <th scope="col">Kanal</th>
          <th scope="col" class="r">Gönderilen</th>
          <th scope="col" class="r">Başarısız</th>
          <th scope="col" class="r">Atlanan</th>
          <th scope="col" class="r">Bekleyen</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.key" :data-testid="`delivery-${r.key}`">
          <th scope="row"><i class="mark" :class="r.key" aria-hidden="true" />{{ r.label }}</th>
          <td class="r num">{{ r.p.sent }}</td>
          <td class="r num" :class="{ bad: r.p.failed > 0 }">{{ r.p.failed }}</td>
          <td class="r num">{{ r.p.skipped }}</td>
          <td class="r num">{{ r.p.pending }}</td>
        </tr>
      </tbody>
    </table>

    <div v-if="byLanguage.length" class="langs" data-testid="delivery-languages">
      <span class="muted">Dile göre gönderilen</span>
      <Badge v-for="l in byLanguage" :key="l.code">
        {{ l.code.toUpperCase() }} <b class="num">{{ finished || l.p!.sent === l.p!.total ? l.p!.sent : `${l.p!.sent}/${l.p!.total}` }}</b>
      </Badge>
    </div>

    <ul v-if="reasons.length" class="reasons" aria-label="Başarısızlık nedenleri">
      <li v-for="r in reasons" :key="r.reason">
        <strong class="num">{{ r.count }}</strong> {{ failureText(r.reason) }}
      </li>
    </ul>

    <p v-if="finished" class="done" role="status"><CheckCircle2 :size="16" aria-hidden="true" />Gönderim tamamlandı.</p>
  </div>
</template>

<style scoped>
.delivery {
  display: grid;
  gap: var(--s-5);
}
.caption {
  margin: var(--s-2) 0 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.caption strong {
  color: var(--fg);
  font-weight: 600;
}
.grid {
  width: 100%;
  border-collapse: collapse;
}
.grid th,
.grid td {
  padding: var(--s-3) 0;
  border-top: 1px solid var(--border);
  text-align: left;
  font-weight: 400;
}
.grid thead th {
  border-top: none;
  padding-top: 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.grid tbody th {
  font-weight: 500;
}
.r {
  text-align: right !important;
}
.bad {
  color: var(--error);
  font-weight: 600;
}
.mark {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: var(--s-2);
  border-radius: 2px;
}
.mark.sms {
  background: var(--sms);
}
.mark.email {
  background: var(--email);
}
.langs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  padding-top: var(--s-4);
  border-top: 1px solid var(--border);
}
.langs .muted {
  margin-right: var(--s-2);
}
.langs b {
  color: var(--fg);
  font-weight: 600;
}
.reasons {
  list-style: none;
  margin: 0;
  padding: var(--s-3);
  border-radius: var(--r-sm);
  background: var(--error-bg);
  border: 1px solid var(--error-line);
  color: var(--error);
  font-size: var(--text-sm);
}
.reasons li + li {
  margin-top: 2px;
}
.done {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin: 0;
  color: var(--ok);
  font-weight: 500;
}
</style>
