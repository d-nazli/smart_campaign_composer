<script setup lang="ts">
import { computed } from 'vue'
import { AlertCircle, HelpCircle } from 'lucide-vue-next'
import type { LanguageInfo, Segment } from '../types'
import { funnel } from '../utils/funnel'
import { describeOffer } from '../utils/formatOffer'
import { sentenceCase } from '../utils/dashboard'
import { describeCondition } from '../utils/formatRule'
import { FALLBACK_LANGUAGES, languageSummary } from '../utils/languages'
import Badge from './ui/Badge.vue'
import Button from './ui/Button.vue'
import Card from './ui/Card.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

const props = defineProps<{ segment: Segment; busy?: boolean; languages?: LanguageInfo[] }>()
const emit = defineEmits<{ (e: 'send-to-all'): void }>()

const chips = computed(() => props.segment.conditions.map(describeCondition))
const joiner = computed(() => (props.segment.match === 'any' ? 'veya' : 've'))
const offerText = computed(() => describeOffer(props.segment.offer))
const f = computed(() => (props.segment.audience ? funnel(props.segment.audience) : null))
const spoken = computed(() => languageSummary(props.segment.languages, props.languages ?? FALLBACK_LANGUAGES))
const hasOffer = computed(() => !!props.segment.offer && props.segment.offer.kind !== 'none')
</script>

<template>
  <Card id="step-audience" title="Kitle" aria-live="polite">
    <template v-if="segment.status === 'ok' && segment.audience && f">
      <p class="explanation">{{ sentenceCase(segment.explanation) }}</p>

      <ul class="conds" aria-label="Kural koşulları">
        <template v-for="(c, i) in chips" :key="c">
          <li v-if="i > 0" class="joiner" aria-hidden="true">{{ joiner }}</li>
          <li><Badge>{{ c }}</Badge></li>
        </template>
      </ul>

      <div class="stats">
        <StatTile label="Eşleşen kişi" :value="f.matched" :hint="f.unreachable > 0 ? `${f.unreachable} kişiye ulaşılamıyor` : 'Hepsine ulaşılabiliyor'" data-testid="matched-tile" />
        <StatTile label="SMS ile ulaşılabilir" :value="f.sms" />
        <StatTile label="E-posta ile ulaşılabilir" :value="f.email" />
      </div>
      <!-- the same three numbers for tests and screen readers; the tiles above are the visual copy -->
      <p class="sr-only">
        Eşleşen <span data-testid="matched">{{ f.matched }}</span>, SMS <span data-testid="sms-reachable">{{ f.sms }}</span>, e-posta
        <span data-testid="email-reachable">{{ f.email }}</span>, ulaşılamayan <span data-testid="unreachable">{{ f.unreachable }}</span>.
      </p>

      <ProgressBar
        :segments="[
          { value: f.both, tone: 'accent' },
          { value: f.smsOnly, tone: 'sms' },
          { value: f.emailOnly, tone: 'email' },
        ]"
        :max="f.matched"
        label="Kanallara göre ulaşılabilirlik"
        size="sm"
      />
      <p class="legend small muted">
        <span><i class="mark both" aria-hidden="true" />İki kanaldan {{ f.both }}</span>
        <span><i class="mark sms" aria-hidden="true" />Yalnız SMS {{ f.smsOnly }}</span>
        <span><i class="mark email" aria-hidden="true" />Yalnız e-posta {{ f.emailOnly }}</span>
        <span v-if="f.unreachable > 0"><i class="mark none" aria-hidden="true" />Ulaşılamayan {{ f.unreachable }}</span>
      </p>

      <dl class="ledger">
        <div v-if="hasOffer" class="ledger__row" data-testid="offer">
          <dt>Teklif</dt>
          <dd>{{ offerText }}</dd>
        </div>
        <div v-if="spoken.length" class="ledger__row" data-testid="audience-languages">
          <dt>Mesaj dili</dt>
          <dd class="langs"><Badge v-for="l in spoken" :key="l.code">{{ l.display }} <b class="num">{{ l.count }}</b></Badge></dd>
        </div>
      </dl>

      <p v-if="f.unreachable > 0" class="hint">İzin vermeyen ya da telefonu / e-postası olmayan kişilere mesaj gönderilmez.</p>
      <p v-if="segment.audience.matched === 0" class="notice warn">Bu kurala uyan kimse yok. Brief'i değiştirmeyi deneyin.</p>
    </template>

    <template v-else-if="segment.status === 'needs_clarification'">
      <div class="box" role="status">
        <p class="box__title"><HelpCircle :size="16" aria-hidden="true" />Biraz daha bilgi lazım</p>
        <p class="box__text">{{ segment.question }}</p>
        <p v-if="hasOffer" class="understood" data-testid="offer"><Badge tone="warn">Anladığım teklif</Badge> {{ offerText }}</p>
        <div v-if="segment.suggestion === 'all_customers'" class="actions">
          <Button variant="primary" :disabled="busy" data-testid="send-to-all" @click="emit('send-to-all')">Tüm müşterilere gönder</Button>
          <span class="hint">İzin vermemiş olanlara yine de gönderilmez.</span>
        </div>
        <p class="hint">Ya da brief'i kime göndereceğinizi yazarak güncelleyip tekrar deneyin.</p>
      </div>
    </template>

    <template v-else-if="segment.status === 'unsupported'">
      <div class="box box--bad" role="alert">
        <p class="box__title"><AlertCircle :size="16" aria-hidden="true" />Bu brief desteklenmiyor</p>
        <p class="box__text">{{ segment.explanation }}</p>
      </div>
    </template>
  </Card>
</template>

<style scoped>
.explanation {
  margin: 0 0 var(--s-3);
  font-size: var(--text-md);
  font-weight: 500;
  max-width: 70ch;
}
.conds {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  list-style: none;
  margin: 0 0 var(--s-5);
  padding: 0;
}
.joiner {
  color: var(--muted);
  font-size: var(--text-xs);
}
/* three figures in one bordered strip, divided by lines */
.stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  margin-bottom: var(--s-4);
}
.stats > * + * {
  border-left: 1px solid var(--border);
}
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-1) var(--s-5);
  margin: var(--s-2) 0 var(--s-5);
}
.legend span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.mark {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  background: var(--border-strong);
}
.mark.both {
  background: var(--accent);
}
.mark.sms {
  background: var(--sms);
}
.mark.email {
  background: var(--email);
}
.ledger {
  margin: 0;
}
.ledger__row {
  display: grid;
  grid-template-columns: 7rem minmax(0, 1fr);
  gap: var(--s-3);
  padding: var(--s-3) 0;
  border-top: 1px solid var(--border);
}
dt {
  color: var(--muted);
}
dd {
  margin: 0;
}
.langs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
}
.langs b {
  color: var(--fg);
  font-weight: 600;
}
.hint {
  margin: var(--s-3) 0 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
.box {
  padding: var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--surface-2);
}
.box--bad {
  border-color: var(--error-line);
  background: var(--error-bg);
  color: var(--error);
}
.box__title {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin: 0;
  font-weight: 600;
}
.box__text {
  margin: var(--s-2) 0 0;
}
.understood {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--s-2);
  margin: var(--s-3) 0 0;
}
.actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--s-3);
  margin-top: var(--s-4);
}
@media (max-width: 640px) {
  .stats {
    grid-template-columns: 1fr;
  }
  .stats > * + * {
    border-left: none;
    border-top: 1px solid var(--border);
  }
  .ledger__row {
    grid-template-columns: 1fr;
    gap: 2px;
  }
}
</style>
