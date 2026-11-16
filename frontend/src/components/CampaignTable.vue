<script setup lang="ts">
import { ref } from 'vue'
import { Clock, XCircle } from 'lucide-vue-next'
import type { CampaignSummary, LanguageInfo } from '../types'
import { briefTitle, languageCells, progressTitle, rowProgress, sentenceCase, statusBadge } from '../utils/dashboard'
import { FALLBACK_LANGUAGES } from '../utils/languages'
import { urlForCampaign } from '../utils/route'
import { formatWhen, untilText } from '../utils/schedule'
import Badge from './ui/Badge.vue'
import Button from './ui/Button.vue'
import ProgressBar from './ui/ProgressBar.vue'

// Every campaign is one row. The title is the brief, cut to one line (the tooltip has all of it);
// a row opens the wizard where the campaign stands.
const props = defineProps<{ campaigns: CampaignSummary[]; languages?: LanguageInfo[] }>()
const emit = defineEmits<{ cancel: [CampaignSummary] }>()

const TITLE_MAX = 64
const now = ref(new Date())
const order = () => props.languages ?? FALLBACK_LANGUAGES

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
          <th scope="col" class="r hide-sm matched">Kitle</th>
          <th scope="col" class="hide-sm langcol">Diller</th>
          <th scope="col" class="progress">İlerleme</th>
          <th scope="col" class="when">Zaman</th>
          <th scope="col" class="act"><span class="sr-only">İşlemler</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="c in campaigns" :key="c.id" class="row" :data-testid="`row-${c.id}`" @click="open(c, $event)">
          <td class="title">
            <a :href="href(c)" class="name" :title="c.brief.replace(/\s+/g, ' ').trim()">{{ briefTitle(c.brief, TITLE_MAX) }}</a>
            <span v-if="c.explanation" class="sub" :title="c.explanation" data-testid="card-audience">{{ sentenceCase(c.explanation) }}</span>
          </td>
          <td>
            <Badge :tone="statusBadge(c).tone" :dot="c.status === 'sending' || c.status === 'scheduled'">{{ statusBadge(c).text }}</Badge>
          </td>
          <td class="r hide-sm num">{{ c.matched != null ? c.matched.toLocaleString('tr-TR') : '–' }}</td>
          <td class="hide-sm" :class="{ 'is-empty': languageCells(c, order()).length === 0 }" data-testid="card-languages">
            <span class="langs">
              <span v-for="l in languageCells(c, order())" :key="l.code" class="lang"><b>{{ l.code }}</b><span class="num">{{ l.text }}</span></span>
              <span v-if="languageCells(c, order()).length === 0" class="muted">–</span>
            </span>
          </td>
          <td class="progress" :class="{ 'is-empty': rowProgress(c.delivery).total === 0 && c.status !== 'failed' }">
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
                <span v-if="rowProgress(c.delivery).failed > 0" class="bad" data-testid="failed">, {{ rowProgress(c.delivery).failed }} başarısız</span></span
              >
            </template>
            <span v-else-if="c.status === 'failed'" class="bad small" :title="c.schedule_error">{{ c.schedule_error || 'Başlatılamadı' }}</span>
            <span v-else class="muted">–</span>
          </td>
          <td class="when" data-testid="card-planned">
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
  overflow-x: auto;
}
.table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  min-width: 760px;
}
th {
  padding: var(--s-3) var(--s-4);
  border-bottom: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--muted);
  font-size: var(--text-sm);
  font-weight: 500;
  text-align: left;
  white-space: nowrap;
}
td {
  padding: var(--s-3) var(--s-4);
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
  background: var(--surface-2);
}
.r {
  text-align: right;
}
.title {
  width: 30%;
}
.status {
  width: 12%;
}
.matched {
  width: 7%;
}
.langcol {
  width: 19%;
}
.progress {
  width: 15%;
}
.when {
  width: 14%;
}
.act {
  width: 52px;
  padding-left: 0;
  text-align: right;
}
.icon-only {
  width: 28px;
  padding: 0;
}
.name {
  display: block;
  color: var(--fg);
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
.langs {
  display: flex;
  flex-wrap: nowrap;
  gap: var(--s-3);
  font-size: var(--text-sm);
  white-space: nowrap;
}
.lang {
  display: inline-flex;
  gap: 5px;
}
.lang b {
  color: var(--muted);
  font-weight: 500;
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
}
.t.plan {
  color: var(--accent);
  font-weight: 500;
}
/* On a phone each campaign becomes a small block: title, then status and time, then the bar. */
@media (max-width: 760px) {
  .hide-sm,
  thead {
    display: none;
  }
  .table,
  tbody {
    display: block;
    min-width: 0;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--s-2) var(--s-3);
    align-items: center;
    padding: var(--s-3) var(--s-4);
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
  }
  .title,
  .progress {
    grid-column: 1 / -1;
  }
  .row td.is-empty {
    display: none;
  }
  .when {
    grid-column: 1 / -1;
    display: flex;
    align-items: baseline;
    gap: var(--s-2);
    text-align: left;
  }
  .act {
    grid-column: 2;
    grid-row: 1;
    justify-self: end;
  }
}
</style>
