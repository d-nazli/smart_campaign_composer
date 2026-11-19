<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertCircle, HelpCircle, Mail, MessageSquare, PencilLine } from 'lucide-vue-next'
import type { LocaleInfo, Segment } from '../types'
import { funnel } from '../utils/funnel'
import { describeOffer } from '../utils/formatOffer'
import { sentenceCase } from '../utils/dashboard'
import { describeConditions } from '../utils/formatRule'
import { FALLBACK_LOCALES, localeSummary } from '../utils/locales'
import Badge from './ui/Badge.vue'
import Button from './ui/Button.vue'
import Card from './ui/Card.vue'
import ProgressBar from './ui/ProgressBar.vue'
import StatTile from './ui/StatTile.vue'

// The audience step. A usable rule: the rule, the reach per channel, the offer and the
// locales on the left; the estimated audience as one figure with a bar in the right rail.
// A question from the model: the AI surface with a one-click answer when there is one.
// An answer to the question goes back to the same campaign (the parent sends it); the
// questions asked so far and their answers are listed with the audience and the question.
const props = defineProps<{ segment: Segment; busy?: boolean; locales?: LocaleInfo[] }>()
const emit = defineEmits<{ (e: 'send-to-all'): void; (e: 'answer', text: string): void; (e: 'edit-brief'): void }>()
const answer = ref('')
const ANSWER_MAX = 500
const canAnswer = computed(() => !props.busy && answer.value.trim().length > 0)
function sendAnswer() {
  if (!canAnswer.value) return
  emit('answer', answer.value.trim())
}
const history = computed(() => props.segment.clarifications ?? [])

const chips = computed(() => describeConditions(props.segment.conditions, props.segment.match))
const joiner = computed(() => (props.segment.match === 'any' ? 'veya' : 've'))
const offerText = computed(() => describeOffer(props.segment.offer))
const f = computed(() => (props.segment.audience ? funnel(props.segment.audience) : null))
const spoken = computed(() => localeSummary(props.segment.locales, props.locales ?? FALLBACK_LOCALES))
const hasOffer = computed(() => !!props.segment.offer && props.segment.offer.kind !== 'none')
const pct = (n: number) => (f.value && f.value.matched > 0 ? Math.round((n / f.value.matched) * 100) : 0)
const unreachableText = computed(() => (f.value && f.value.unreachable > 0 ? `${f.value.unreachable} kişiye ulaşılamıyor` : 'Hepsine ulaşılabiliyor'))
</script>

<template>
  <div v-if="segment.status === 'ok' && segment.audience && f" id="step-audience" class="split" aria-live="polite">
    <Card title="Kitle">
      <div class="body">
        <p class="explanation">{{ sentenceCase(segment.explanation) }}</p>
        <p v-if="segment.note" class="notice info" role="note" data-testid="audience-note">{{ segment.note }}</p>
        <p v-for="w in segment.warnings ?? []" :key="w" class="notice warn" role="note" data-testid="audience-warning">{{ w }}</p>

        <section v-if="history.length" data-testid="clarifications">
          <h3 class="label">Soru ve cevaplar</h3>
          <ol class="qa">
            <li v-for="(c, i) in history" :key="i" class="qa__item">
              <p><span class="qa__k">Soru:</span> {{ c.question }}</p>
              <p><span class="qa__k">Cevap:</span> {{ c.answer }}</p>
            </li>
          </ol>
        </section>

        <section v-if="chips.length">
          <h3 class="label">Kural</h3>
          <ul class="conds" aria-label="Kural koşulları">
            <template v-for="(c, i) in chips" :key="c">
              <li v-if="i > 0" class="joiner" aria-hidden="true">{{ joiner }}</li>
              <li><span class="chip">{{ c }}</span></li>
            </template>
          </ul>
        </section>

        <section>
          <h3 class="label">Kanallara göre ulaşılabilirlik</h3>
          <div class="reach">
            <div class="reach__row">
              <span class="reach__icon sms" aria-hidden="true"><MessageSquare :size="16" /></span>
              <span><strong class="num">{{ f.sms.toLocaleString('tr-TR') }}</strong> <span class="muted">SMS</span></span>
              <ProgressBar :value="f.sms" :max="f.matched" tone="accent" size="sm" label="SMS ile ulaşılabilenler" />
              <span class="num muted pct">%{{ pct(f.sms) }}</span>
            </div>
            <div class="reach__row">
              <span class="reach__icon email" aria-hidden="true"><Mail :size="16" /></span>
              <span><strong class="num">{{ f.email.toLocaleString('tr-TR') }}</strong> <span class="muted">E-posta</span></span>
              <ProgressBar :value="f.email" :max="f.matched" tone="accent" size="sm" label="E-posta ile ulaşılabilenler" />
              <span class="num muted pct">%{{ pct(f.email) }}</span>
            </div>
          </div>
        </section>

        <dl v-if="hasOffer || spoken.length" class="ledger">
          <div v-if="hasOffer" class="ledger__row" data-testid="offer">
            <dt class="label">Teklif</dt>
            <dd>{{ offerText }}</dd>
          </div>
          <div v-if="spoken.length" class="ledger__row" data-testid="audience-locales">
            <dt class="label">Locale</dt>
            <dd class="locs">
              <Badge v-for="l in spoken" :key="l.code">{{ l.code }} <b class="num">{{ l.count }}</b></Badge>
            </dd>
          </div>
        </dl>

        <p v-if="segment.audience.matched === 0" class="notice warn">Bu kurala uyan kimse yok.</p>
      </div>
    </Card>

    <aside aria-label="Tahmini kitle">
      <section class="stat-card">
        <StatTile label="Tahmini kitle" :value="f.matched" :sub="unreachableText" data-testid="matched-tile">
          <ProgressBar
            :segments="[
              { value: f.both, tone: 'accent' },
              { value: f.smsOnly, tone: 'sms' },
              { value: f.emailOnly, tone: 'email' },
            ]"
            :max="f.matched"
            label="Kanallara göre ulaşılabilirlik"
            size="sm"
            class="stack-bar"
          />
          <div class="leader"><i class="mark both" aria-hidden="true" /><span>İki kanaldan</span><span class="leader__fill" /><b class="num">{{ f.both }}</b></div>
          <div class="leader"><i class="mark sms" aria-hidden="true" /><span>Yalnız SMS</span><span class="leader__fill" /><b class="num">{{ f.smsOnly }}</b></div>
          <div class="leader"><i class="mark email" aria-hidden="true" /><span>Yalnız e-posta</span><span class="leader__fill" /><b class="num">{{ f.emailOnly }}</b></div>
          <div v-if="f.unreachable > 0" class="leader">
            <i class="mark none" aria-hidden="true" /><span>Ulaşılamayan</span><span class="leader__fill" /><b class="num">{{ f.unreachable }}</b>
          </div>
        </StatTile>
      </section>
      <!-- the same numbers for tests and screen readers; the card above is the visual copy -->
      <p class="sr-only">
        Eşleşen <span data-testid="matched">{{ f.matched }}</span>, SMS <span data-testid="sms-reachable">{{ f.sms }}</span>, e-posta
        <span data-testid="email-reachable">{{ f.email }}</span>, ulaşılamayan <span data-testid="unreachable">{{ f.unreachable }}</span>.
      </p>
    </aside>
  </div>

  <section v-else-if="segment.status === 'needs_clarification'" id="step-audience" class="ask" role="status">
    <p class="ask__title"><HelpCircle :size="18" aria-hidden="true" />Biraz daha bilgi lazım</p>
    <ol v-if="history.length" class="qa" data-testid="clarifications">
      <li v-for="(c, i) in history" :key="i" class="qa__item">
        <p><span class="qa__k">Soru:</span> {{ c.question }}</p>
        <p><span class="qa__k">Cevap:</span> {{ c.answer }}</p>
      </li>
    </ol>
    <p class="ask__text" data-testid="question">{{ segment.question }}</p>
    <p v-if="hasOffer" class="understood" data-testid="offer"><span class="chip ai">Anladığım teklif</span> {{ offerText }}</p>
    <div v-if="segment.suggestion === 'all_customers'" class="ask__option">
      <strong>Tüm müşteriler</strong>
      <Button variant="primary" :disabled="busy" data-testid="send-to-all" @click="emit('send-to-all')">Tüm müşterilere gönder</Button>
    </div>
    <form class="ask__answer" @submit.prevent="sendAnswer">
      <label for="answer" class="label">Cevabınız</label>
      <textarea
        id="answer"
        v-model="answer"
        rows="2"
        :maxlength="ANSWER_MAX"
        :disabled="busy"
        placeholder="Soruyu kendi cümlelerinizle cevaplayın…"
        data-testid="answer"
        @keydown.ctrl.enter="sendAnswer"
      />
      <Button type="submit" :variant="segment.suggestion === 'all_customers' ? 'secondary' : 'primary'" :disabled="!canAnswer" data-testid="send-answer">Cevapla</Button>
    </form>
    <p class="ask__alt">
      Cevap yerine brief'i baştan yazabilirsiniz; aynı kampanya güncellenir.
      <Button variant="ghost" size="sm" :icon="PencilLine" :disabled="busy" data-testid="edit-brief" @click="emit('edit-brief')">Brief'i düzenle</Button>
    </p>
  </section>

  <Card v-else-if="segment.status === 'unsupported'" id="step-audience" title="Kitle">
    <div class="bad" role="alert">
      <p class="bad__title"><AlertCircle :size="18" aria-hidden="true" />Bu brief desteklenmiyor</p>
      <p class="bad__text">{{ segment.explanation }}</p>
      <div class="bad__act">
        <Button variant="secondary" :icon="PencilLine" :disabled="busy" data-testid="edit-brief" @click="emit('edit-brief')">Brief'i düzenle</Button>
      </div>
    </div>
  </Card>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: var(--s-5);
}
.explanation {
  margin: 0;
  font-size: var(--text-md);
}
h3.label {
  margin: 0 0 var(--s-2);
}
.conds {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.joiner {
  color: var(--muted);
  font-size: var(--text-xs);
}
.reach {
  border: 1px solid var(--border);
  border-radius: var(--r-md);
}
.reach__row {
  display: grid;
  grid-template-columns: 34px minmax(120px, auto) minmax(0, 1fr) 48px;
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
}
.reach__row + .reach__row {
  border-top: 1px dotted var(--border-strong);
}
.reach__icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--surface-2);
}
.reach__icon.sms {
  color: var(--sms);
}
.reach__icon.email {
  color: var(--email);
}
.pct {
  text-align: right;
}
.ledger {
  margin: 0;
}
.ledger__row {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-3) 0;
  border-top: 1px dotted var(--border-strong);
}
dd {
  margin: 0;
}
.locs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.stat-card {
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  box-shadow: var(--shadow-card);
}
.stack-bar {
  margin: var(--s-2) 0 var(--s-1);
}
.mark {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  transform: translateY(-1px);
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
.mark.none {
  background: var(--border-strong);
}

/* the model asks a question: the AI surface */
.ask {
  display: grid;
  gap: var(--s-3);
  padding: var(--s-6);
  border-radius: var(--r-md);
  background: var(--ai-surface);
}
.ask__title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--ai-chip-fg);
  font-weight: 600;
}
.ask__text {
  margin: 0;
  font-size: var(--text-md);
}
.understood {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  margin: 0;
}
.chip.ai {
  border-color: transparent;
  background: var(--ai-chip-bg);
  color: var(--ai-chip-fg);
}
.ask__option {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}
.ask__option > :last-child {
  margin-left: auto;
}
.ask__answer {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: var(--s-2) var(--s-3);
}
.ask__answer .label {
  grid-column: 1 / -1;
}
.ask__answer textarea {
  width: 100%;
  min-height: 44px;
  padding: var(--s-2) var(--s-3);
  border: 1px solid var(--border-strong);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  resize: vertical;
}
.qa {
  display: grid;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.qa__item {
  padding: var(--s-2) var(--s-3);
  border-left: 3px solid var(--ai-chip-bg);
}
.qa__item p {
  margin: 0;
}
.qa__k {
  color: var(--muted);
  font-weight: 600;
}
.bad {
  padding-left: var(--s-4);
  border-left: 3px solid var(--error);
}
.bad__title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 var(--s-2);
  color: var(--error);
  font-weight: 600;
}
.bad__text {
  margin: 0;
}
.bad__act {
  margin-top: var(--s-3);
}
.ask__alt {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  margin: 0;
  color: var(--muted);
  font-size: var(--text-sm);
}
@media (max-width: 767px) {
  .reach__row {
    grid-template-columns: 34px minmax(0, 1fr) 48px;
  }
  .reach__row :deep(.ui-progress) {
    display: none;
  }
  .ledger__row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--s-1);
  }
  .ask {
    padding: var(--s-4);
  }
  .ask__option > :last-child {
    width: 100%;
    margin-left: 0;
  }
  .ask__answer {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
