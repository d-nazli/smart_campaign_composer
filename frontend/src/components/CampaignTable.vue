<script setup lang="ts">
import { ref } from 'vue'
import { Clock, XCircle } from 'lucide-vue-next'
import type { CampaignSummary, LocaleInfo } from '../types'
import { briefTitle, localeLine, progressTitle, rowProgress, sentenceCase, statusBadge } from '../utils/dashboard'
import { FALLBACK_LOCALES } from '../utils/locales'
import { urlForCampaign } from '../utils/route'
import { formatWhen, shortWhen, untilText } from '../utils/schedule'
import Badge from './ui/Badge.vue'
import Button from './ui/Button.vue'
import ProgressBar from './ui/ProgressBar.vue'

// Every campaign is one row. The title is the brief, cut to one line (the tooltip has all of it);
// a row opens the wizard where the campaign stands. A scheduled campaign says when in its
// status ("Zamanlandı · 24 Kas 14:00") and can be cancelled from its row.
// Below 1024px the rows are laid out as cards (CSS only: the same elements).
const props = defineProps<{ campaigns: CampaignSummary[]; locales?: LocaleInfo[] }>()
const emit = defineEmits<{ cancel: [CampaignSummary] }>()

const TITLE_MAX = 64
const now = ref(new Date())
const order = () => props.locales ?? FALLBACK_LOCALES

const href = (c: CampaignSummary) => urlForCampaign(window.location.href, c.id)
function open(c: CampaignSummary, e: MouseEvent) {
  // Real links and buttons inside the row do their own thing.
  if ((e.target as HTMLElement).closest('a, button')) return
  window.location.href = href(c)
}
const day = (iso: string) => formatWhen(iso)
</script>

<template>
  <div class="wrap">
    <table class="table" data-testid="campaign-table">
      <thead>
        <tr>
          <th scope="col" class="title">Kampanya</th>
          <th scope="col" class="status">Durum</th>
          <th scope="col" class="r matched">Kitle</th>
          <th scope="col" class="loccol">Locale</th>
          <th scope="col" class="progress">İlerleme</th>
          <th scope="col" class="when">Zaman</th>
          <th scope="col" class="act"><span class="sr-only">İşlemler</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="c in campaigns" :key="c.id" class="row" :data-testid="`row-${c.id}`" @click="open(c, $event)">
          <td class="title">
            <a :href="href(c)" class="name" :title="c.brief.replace(/\s+/g, ' ').trim()" data-testid="card-name">{{ c.name || briefTitle(c.brief, TITLE_MAX) }}</a>
            <span v-if="c.explanation" class="sub" :title="c.explanation" data-testid="card-audience">{{ sentenceCase(c.explanation) }}</span>
          </td>
          <td>
            <Badge :tone="statusBadge(c).tone" :dot="c.status === 'sending' || c.status === 'scheduled'" data-testid="status"
              >{{ statusBadge(c).text }}<template v-if="c.status === 'scheduled' && c.scheduled_at"> · {{ shortWhen(c.scheduled_at) }}</template></Badge
            >
          </td>
          <td class="r num matched-cell" data-label="Kitle">{{ c.matched != null ? c.matched.toLocaleString('tr-TR') : '–' }}</td>
          <td class="locs-cell" :class="{ 'is-empty': !localeLine(c, order()) }" data-label="Locale" data-testid="card-locales">
            <span v-if="localeLine(c, order())" class="locs num" :title="localeLine(c, order())!.title" :aria-label="localeLine(c, order())!.text"
              ><template v-for="(p, i) in localeLine(c, order())!.parts" :key="p"><span v-if="i > 0" class="sep" aria-hidden="true"> · </span><span class="part">{{ p }}</span></template></span
            >
            <span v-else class="muted">–</span>
          </td>
          <td class="progress" data-label="İlerleme" :class="{ 'is-empty': rowProgress(c.delivery).total === 0 && c.status !== 'failed' }">
            <template v-if="rowProgress(c.delivery).total > 0">
              <ProgressBar
                size="sm"
                label="Gönderim ilerlemesi"
                :segments="[
                  { value: rowProgress(c.delivery).sent, tone: 'ok' },
                  { value: rowProgress(c.delivery).failed + rowProgress(c.delivery).skipped, tone: 'danger' },
                ]"
                :max="rowProgress(c.delivery).total"
                :title="progressTitle(c.delivery)"
              />
              <span class="cap num"
                ><span data-testid="sent">{{ rowProgress(c.delivery).sent }}</span> / {{ rowProgress(c.delivery).total }}
                <span v-if="rowProgress(c.delivery).failed > 0" class="bad" data-testid="failed">· {{ rowProgress(c.delivery).failed }} başarısız</span></span
              >
            </template>
            <span v-else-if="c.status === 'failed'" class="bad small" :title="c.schedule_error">{{ c.schedule_error || 'Başlatılamadı' }}</span>
            <span v-else class="muted">–</span>
          </td>
          <td class="when" data-label="Zaman" data-testid="card-planned">
            <template v-if="c.scheduled_at">
              <span class="t num" :class="{ plan: c.status === 'scheduled' }"><Clock :size="13" aria-hidden="true" />{{ day(c.scheduled_at) }}</span>
              <span class="sub">{{ c.status === 'scheduled' ? untilText(c.scheduled_at, now) : 'zamanlanmıştı' }}</span>
            </template>
            <template v-else>
              <span class="t num">{{ day(c.created_at) }}</span>
              <span class="sub">oluşturuldu</span>
            </template>
          </td>
          <td class="act">
            <Button v-if="c.status === 'scheduled'" variant="ghost" size="sm" :icon="XCircle" class="icon-only" title="Gönderimi iptal et" :aria-label="`#${c.id} zamanlı gönderimi iptal et`" data-testid="cancel" @click="emit('cancel', c)" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.wrap {
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
}
.table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
th {
  padding: var(--s-3) var(--s-4);
  border-bottom: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--muted);
  font-size: var(--text-label);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  text-align: left;
  white-space: nowrap;
}
th:first-child {
  border-top-left-radius: var(--r-md);
}
th:last-child {
  border-top-right-radius: var(--r-md);
}
td {
  padding: var(--s-4);
  border-top: 1px solid var(--border);
  vertical-align: middle;
}
tbody tr:first-child td {
  border-top: none;
}
.row {
  cursor: pointer;
  transition: background var(--t-fast) var(--ease);
}
.row:hover {
  background: color-mix(in srgb, var(--accent-soft) 45%, var(--surface));
}
.r {
  text-align: right;
}
.title {
  width: 30%;
}
.status {
  width: 17%;
}
.matched {
  width: 8%;
}
.loccol {
  width: 16%;
}
.progress {
  width: 14%;
}
.when {
  width: 13%;
}
.act {
  width: 64px;
  padding-left: 0;
  text-align: right;
}
.icon-only {
  width: 34px;
  padding: 0;
  color: var(--accent);
}
.name {
  display: block;
  color: var(--accent);
  font-weight: 500;
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.name:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}
.sub {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted);
  font-size: var(--text-xs);
}
.locs {
  font-size: var(--text-sm);
  cursor: help;
}
.locs .part {
  white-space: nowrap;
}
.cap {
  display: block;
  margin-top: 4px;
  color: var(--muted);
  font-size: var(--text-xs);
  white-space: nowrap;
}
.bad {
  color: var(--error);
}
.t {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
  font-size: var(--text-sm);
}
.t.plan {
  color: var(--plan);
  font-weight: 500;
}
/* Below 1024px a table would need sideways scrolling: each campaign becomes a card.
   Title, then status (and the cancel button), then the bar and the labelled facts. */
@media (max-width: 1023px) {
  thead {
    display: none;
  }
  .table,
  tbody {
    display: block;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--s-2) var(--s-3);
    align-items: center;
    padding: var(--s-4);
    border-top: 1px solid var(--border);
  }
  .row:first-child {
    border-top: none;
  }
  .row td,
  tbody tr:first-child td {
    display: block;
    width: auto;
    padding: 0;
    border: none;
    text-align: left;
  }
  .title {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .row td:nth-child(2) {
    grid-column: 1;
    grid-row: 1;
  }
  .act {
    grid-column: 2;
    grid-row: 1;
    justify-self: end;
  }
  .name,
  .sub {
    white-space: normal;
  }
  .row td.progress,
  .row td.matched-cell,
  .row td.locs-cell,
  .row td.when {
    grid-column: 1 / -1;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--s-1) var(--s-3);
  }
  .row td.progress {
    flex-direction: column;
    align-items: stretch;
    gap: 0;
  }
  .matched-cell::before,
  .locs-cell::before,
  .when::before {
    content: attr(data-label);
    flex: none;
    width: 64px;
    color: var(--muted);
    font-size: var(--text-sm);
  }
  .row td.is-empty {
    display: none;
  }
  .when .sub {
    display: inline;
  }
}
</style>
