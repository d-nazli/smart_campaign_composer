<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { AlertTriangle, Clock, Send, Users } from 'lucide-vue-next'
import type { Issue, LanguageInfo, Segment } from '../types'
import { sentenceCase } from '../utils/dashboard'
import { describeOffer } from '../utils/formatOffer'
import { FALLBACK_LANGUAGES, languageSummary } from '../utils/languages'
import { quickTimes, scheduleBounds, toLocalInput } from '../utils/schedule'
import AnimatedNumber from './AnimatedNumber.vue'
import Badge from './ui/Badge.vue'
import Card from './ui/Card.vue'
import DateTimeField from './ui/DateTimeField.vue'
import RailCard from './ui/RailCard.vue'
import SegmentedControl, { type SegmentOption } from './ui/SegmentedControl.vue'

// What will be sent, to whom, and when: the choice of time and the summary on the left,
// the audience and the warnings in the right rail. The buttons live in the wizard's
// bottom bar; `confirming` is the second step of the approval: the card asks "are you sure?".
// The picker only offers times the server accepts: from a minute from now to 30 days
// ahead (or the offer's last day, if sooner).
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
  <div class="split">
    <Card title="Onay">
      <template #actions>
        <Badge v-if="frozen" tone="ok" dot>Onaylandı</Badge>
        <Badge v-else-if="later" tone="accent" data-testid="later-note">Tahmini sayılar</Badge>
      </template>

      <div class="body">
        <div v-if="!frozen" class="when">
          <span class="label">Gönderim zamanı</span>
          <SegmentedControl
            layout="rows"
            :model-value="mode"
            :options="options"
            label="Gönderim zamanı"
            :disabled="confirming"
            @update:model-value="emit('update:mode', $event as 'now' | 'later')"
          />
          <div v-if="mode === 'later'" class="when__pick">
            <DateTimeField
              :model-value="when"
              label="Gönderim zamanı"
              :min="earliest"
              :max="latest"
              :shortcuts="quick"
              :invalid="scheduleError !== null"
              testid="send-at"
              @update:model-value="emit('update:when', $event)"
            />
            <p v-if="scheduleError" class="when__error" role="alert" data-testid="schedule-error">{{ scheduleError }}</p>
          </div>
        </div>

        <dl class="ledger">
          <div class="ledger__row">
            <dt class="label">Teklif</dt>
            <dd>{{ sentenceCase(offer) }}</dd>
          </div>
          <div class="ledger__row">
            <dt class="label">Kitle</dt>
            <dd>{{ sentenceCase(segment.explanation) }}</dd>
          </div>
          <div v-if="spoken.length" class="ledger__row">
            <dt class="label">Diller</dt>
            <dd class="langs" data-testid="approval-languages">
              <Badge v-for="l in spoken" :key="l.code">{{ l.display }} <b class="num">{{ l.count }}</b></Badge>
            </dd>
          </div>
        </dl>

        <Transition name="fade">
          <p v-if="confirming" class="notice warn confirm" role="alertdialog" aria-labelledby="confirm-title">
            <strong id="confirm-title">Emin misiniz?</strong>
            <template v-if="mode === 'later'">Mesajlar seçtiğiniz zamanda kendiliğinden gönderilir. Başlamadan önce iptal edebilirsiniz.</template>
            <template v-else>Mesajlar hemen gönderilir. Onaydan sonra metin ve kitle değişmez.</template>
          </p>
        </Transition>
        <p v-if="error" class="notice error" role="alert">{{ error }}</p>
      </div>
    </Card>

    <aside aria-label="Göndermeden önce">
      <RailCard tone="info" title="Alıcılar" :icon="Users">
        <p class="people"><strong class="num"><AnimatedNumber :value="people" from-zero /></strong> <span class="muted">kişi</span></p>
        <div class="leader"><i class="mark sms" aria-hidden="true" /><span>SMS</span><span class="leader__fill" /><b class="num">{{ sms }}</b></div>
        <div class="leader"><i class="mark email" aria-hidden="true" /><span>E-posta</span><span class="leader__fill" /><b class="num">{{ email }}</b></div>
      </RailCard>
      <RailCard v-if="warnings.length" tone="warn" title="Uyarılar" :icon="AlertTriangle">
        <ul class="warnings" aria-label="Uyarılar">
          <li v-for="w in warnings" :key="(w.language ?? '') + w.field + w.code + w.message">
            <span><strong v-if="w.language">{{ w.language.toUpperCase() }}</strong> {{ w.message }}</span>
          </li>
        </ul>
      </RailCard>
    </aside>
  </div>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: var(--s-5);
}
.when {
  display: grid;
  gap: var(--s-2);
}
.when__pick {
  display: grid;
  gap: var(--s-2);
  padding-left: var(--s-8);
}
.when__error {
  margin: 0;
  color: var(--error);
  font-size: var(--text-sm);
}
.ledger {
  margin: 0;
}
.ledger__row {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr);
  gap: var(--s-3);
  align-items: baseline;
  padding: var(--s-3) 0;
  border-top: 1px dotted var(--border-strong);
}
dd {
  margin: 0;
}
.langs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.people {
  margin: 0 0 var(--s-2);
}
.people strong {
  font-size: 2rem;
  font-weight: 400;
}
.mark {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  transform: translateY(-1px);
}
.mark.sms {
  background: var(--sms);
}
.mark.email {
  background: var(--email);
}
.warnings {
  display: grid;
  gap: 6px;
  margin: 0;
  padding-left: var(--s-4);
}
.confirm strong {
  margin-right: 4px;
}
@media (max-width: 767px) {
  .when__pick {
    padding-left: 0;
  }
  .ledger__row {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--s-1);
  }
}
</style>
