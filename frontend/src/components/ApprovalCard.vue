<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { AlertTriangle, Clock, Info, Send } from 'lucide-vue-next'
import type { Issue, LanguageInfo, Segment } from '../types'
import { sentenceCase } from '../utils/dashboard'
import { describeOffer } from '../utils/formatOffer'
import { FALLBACK_LANGUAGES, languageSummary } from '../utils/languages'
import { dayText, quickTimes, scheduleBounds, toLocalInput } from '../utils/schedule'
import AnimatedNumber from './AnimatedNumber.vue'
import Badge from './ui/Badge.vue'
import Card from './ui/Card.vue'
import DateTimeField from './ui/DateTimeField.vue'
import SegmentedControl, { type SegmentOption } from './ui/SegmentedControl.vue'

// What will be sent, to whom, and when. The buttons live in the wizard's bottom bar;
// `confirming` is the second step of the approval: the card asks "are you sure?".
const props = defineProps<{
  segment: Segment
  warnings: Issue[]
  confirming: boolean
  /** Already approved: the card is read-only. */
  frozen: boolean
  error: string | null
  languages?: LanguageInfo[]
  /** Send now, or at a chosen time (the audience is then decided when the time comes). */
  mode: 'now' | 'later'
  /** The chosen time as a datetime-local value (the viewer's own zone). */
  when: string
  /** Why the chosen time cannot be used (null when it can). */
  scheduleError: string | null
}>()
const emit = defineEmits<{ 'update:mode': ['now' | 'later']; 'update:when': [string] }>()

const options: SegmentOption[] = [
  { value: 'now', label: 'Hemen', icon: Send, testid: 'mode-now' },
  { value: 'later', label: 'Zamanla', icon: Clock, testid: 'mode-later' },
]

// The earliest and latest times the picker offers move with the clock.
const clock = ref(new Date())
let tick: number | null = null
onMounted(() => (tick = window.setInterval(() => (clock.value = new Date()), 30_000)))
onBeforeUnmount(() => tick !== null && window.clearInterval(tick))
const bounds = computed(() => scheduleBounds(props.segment.offer, clock.value))
const earliest = computed(() => toLocalInput(bounds.value.earliest))
const latest = computed(() => toLocalInput(bounds.value.latest))
const quick = computed(() => quickTimes(clock.value))
const limit = computed(() =>
  bounds.value.limitedByOffer && props.segment.offer?.expires_at
    ? `En geç ${dayText(props.segment.offer.expires_at)} (teklifin son günü)`
    : 'En fazla 30 gün sonrası',
)

const sms = computed(() => props.segment.audience?.sms_reachable ?? 0)
const email = computed(() => props.segment.audience?.email_reachable ?? 0)
const people = computed(() => {
  const a = props.segment.audience
  return a ? a.sms_reachable + a.email_reachable - a.both_reachable : 0
})
const offer = computed(() => describeOffer(props.segment.offer))
const spoken = computed(() => languageSummary(props.segment.languages, props.languages ?? FALLBACK_LANGUAGES))
const later = computed(() => props.mode === 'later' && !props.frozen)
</script>

<template>
  <Card title="Onay">
    <template #actions>
      <Badge v-if="frozen" tone="ok" dot>Onaylandı</Badge>
      <Badge v-else-if="later" tone="accent">Tahmini sayılar</Badge>
    </template>

    <div v-if="!frozen" class="when">
      <div class="when__row">
        <span class="when__label">Gönderim</span>
        <SegmentedControl :model-value="mode" :options="options" label="Gönderim zamanı" :disabled="confirming" @update:model-value="emit('update:mode', $event as 'now' | 'later')" />
        <DateTimeField
          v-if="mode === 'later'"
          :model-value="when"
          label="Gönderim zamanı"
          :min="earliest"
          :max="latest"
          :shortcuts="quick"
          :invalid="scheduleError !== null"
          testid="send-at"
          @update:model-value="emit('update:when', $event)"
        />
      </div>
      <p v-if="mode === 'later'" class="when__hint">{{ limit }}</p>
      <p v-if="scheduleError" class="when__error" role="alert" data-testid="schedule-error">{{ scheduleError }}</p>
    </div>

    <dl class="ledger">
      <div class="ledger__row">
        <dt>Teklif</dt>
        <dd>{{ sentenceCase(offer) }}</dd>
      </div>
      <div class="ledger__row">
        <dt>Kitle</dt>
        <dd>{{ sentenceCase(segment.explanation) }}</dd>
      </div>
      <div class="ledger__row">
        <dt>Alıcılar</dt>
        <dd class="recipients">
          <span class="people"><strong class="num"><AnimatedNumber :value="people" from-zero /></strong> <span class="muted">kişi</span></span>
          <span class="chan"><i class="mark sms" aria-hidden="true" />SMS <b class="num">{{ sms }}</b></span>
          <span class="chan"><i class="mark email" aria-hidden="true" />E-posta <b class="num">{{ email }}</b></span>
        </dd>
      </div>
      <div v-if="spoken.length" class="ledger__row">
        <dt>Diller</dt>
        <dd class="langs" data-testid="approval-languages">
          <Badge v-for="l in spoken" :key="l.code">{{ l.display }} <b class="num">{{ l.count }}</b></Badge>
        </dd>
      </div>
    </dl>

    <p v-if="later" class="note" data-testid="later-note">
      <Info :size="14" aria-hidden="true" />
      Kitle gönderim anında yeniden belirlenir; kişi sayısı değişebilir.
    </p>

    <ul v-if="warnings.length" class="warnings" aria-label="Uyarılar">
      <li v-for="w in warnings" :key="(w.language ?? '') + w.field + w.code + w.message">
        <AlertTriangle :size="14" aria-hidden="true" />
        <span><strong v-if="w.language">{{ w.language.toUpperCase() }}</strong> {{ w.message }}</span>
      </li>
    </ul>

    <Transition name="fade">
      <p v-if="confirming" class="confirm" role="alertdialog" aria-labelledby="confirm-title">
        <strong id="confirm-title">Emin misiniz?</strong>
        <template v-if="mode === 'later'">Mesajlar seçtiğiniz zamanda kendiliğinden gönderilir. Başlamadan önce iptal edebilirsiniz.</template>
        <template v-else>Mesajlar hemen gönderilir. Onaydan sonra metin ve kitle değişmez.</template>
      </p>
    </Transition>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  </Card>
</template>

<style scoped>
.when {
  padding-bottom: var(--s-5);
  margin-bottom: var(--s-5);
  border-bottom: 1px solid var(--border);
}
.when__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-3);
}
.when__label {
  width: 7rem;
  color: var(--muted);
}
.when__hint {
  margin: var(--s-2) 0 0 calc(7rem + var(--s-3));
  color: var(--muted);
  font-size: var(--text-sm);
}
.when__error {
  margin: var(--s-2) 0 0 calc(7rem + var(--s-3));
  color: var(--error);
  font-size: var(--text-sm);
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
.ledger__row:first-child {
  border-top: none;
  padding-top: 0;
}
dt {
  color: var(--muted);
}
dd {
  margin: 0;
  min-width: 0;
}
.recipients {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: var(--s-1) var(--s-6);
}
.people strong {
  font-size: var(--text-md);
  font-weight: 600;
}
.chan {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.chan b {
  font-weight: 600;
}
.mark {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  background: var(--muted);
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
  gap: var(--s-2);
}
.langs b {
  color: var(--fg);
  font-weight: 600;
}

.note {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin: var(--s-4) 0 0;
  padding-top: var(--s-4);
  border-top: 1px solid var(--border);
  color: var(--muted);
  font-size: var(--text-sm);
}
.warnings {
  list-style: none;
  margin: var(--s-4) 0 0;
  padding: 0;
  display: grid;
  gap: var(--s-2);
}
.warnings li {
  display: flex;
  gap: var(--s-2);
  align-items: flex-start;
  padding: var(--s-2) var(--s-3);
  border-radius: var(--r-sm);
  background: var(--warn-bg);
  border: 1px solid var(--warn-line);
  color: var(--warn);
  font-size: var(--text-sm);
}
.warnings svg {
  flex: none;
  margin-top: 2px;
}
.confirm {
  margin: var(--s-4) 0 0;
  padding: var(--s-3) var(--s-4);
  border-radius: var(--r-sm);
  background: var(--accent-soft);
  border: 1px solid var(--accent);
  font-size: var(--text-sm);
}
.confirm strong {
  margin-right: var(--s-2);
}

@media (max-width: 640px) {
  .when__label {
    width: 100%;
  }
  .when__hint,
  .when__error {
    margin-left: 0;
  }
  .ledger__row {
    grid-template-columns: 1fr;
    gap: 2px;
  }
}
</style>
