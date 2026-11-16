<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { Issue, LanguageInfo, Segment } from '../types'
import { describeOffer } from '../utils/formatOffer'
import { FALLBACK_LANGUAGES, languageSummary } from '../utils/languages'
import { dayText, scheduleBounds, toLocalInput } from '../utils/schedule'
import AnimatedNumber from './AnimatedNumber.vue'

// The summary of what will be sent. The buttons live in the wizard's bottom bar;
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

// The earliest and latest times the form offers move with the clock.
const clock = ref(new Date())
let tick: number | null = null
onMounted(() => (tick = window.setInterval(() => (clock.value = new Date()), 30_000)))
onBeforeUnmount(() => tick !== null && window.clearInterval(tick))
const bounds = computed(() => scheduleBounds(props.segment.offer, clock.value))
const earliest = computed(() => toLocalInput(bounds.value.earliest))
const latest = computed(() => toLocalInput(bounds.value.latest))
const limit = computed(() =>
  bounds.value.limitedByOffer && props.segment.offer?.expires_at
    ? `En geç teklifin son günü (${dayText(props.segment.offer.expires_at)}) seçilebilir.`
    : 'En fazla 30 gün sonrası seçilebilir.',
)

const sms = computed(() => props.segment.audience?.sms_reachable ?? 0)
const email = computed(() => props.segment.audience?.email_reachable ?? 0)
const people = computed(() => {
  const a = props.segment.audience
  return a ? a.sms_reachable + a.email_reachable - a.both_reachable : 0
})
const offer = computed(() => describeOffer(props.segment.offer))
const spoken = computed(() => languageSummary(props.segment.languages, props.languages ?? FALLBACK_LANGUAGES))
</script>

<template>
  <section class="card approval">
    <h2>Onay</h2>
    <p v-if="frozen" class="notice info" role="status">Kampanya onaylandı; bu özet artık değiştirilemez.</p>

    <fieldset v-if="!frozen" class="when" :disabled="confirming">
      <legend>Ne zaman gönderilsin?</legend>
      <label class="choice" :class="{ on: mode === 'now' }">
        <input type="radio" name="send-mode" value="now" :checked="mode === 'now'" data-testid="mode-now" @change="emit('update:mode', 'now')" />
        Hemen
      </label>
      <label class="choice" :class="{ on: mode === 'later' }">
        <input type="radio" name="send-mode" value="later" :checked="mode === 'later'" data-testid="mode-later" @change="emit('update:mode', 'later')" />
        İleri bir zamanda
      </label>
      <div v-if="mode === 'later'" class="picker">
        <label for="send-at" class="sr-only">Gönderim zamanı</label>
        <input
          id="send-at"
          type="datetime-local"
          :value="when"
          :min="earliest"
          :max="latest"
          :aria-invalid="scheduleError !== null"
          data-testid="send-at"
          @input="emit('update:when', ($event.target as HTMLInputElement).value)"
        />
        <p class="muted small">{{ limit }}</p>
        <p v-if="scheduleError" class="issue" role="alert" data-testid="schedule-error">{{ scheduleError }}</p>
      </div>
    </fieldset>

    <div class="summary">
      <div class="people">
        <strong class="num big"><AnimatedNumber :value="people" from-zero /></strong>
        <span class="muted">{{ mode === 'later' && !frozen ? 'kişiye gönderilmesi bekleniyor (tahmini)' : 'kişiye gönderilecek' }}</span>
      </div>
      <div class="channels">
        <div class="tile sms">
          <strong class="num"><AnimatedNumber :value="sms" from-zero /></strong>
          <span class="cap">SMS</span>
        </div>
        <div class="tile email">
          <strong class="num"><AnimatedNumber :value="email" from-zero /></strong>
          <span class="cap">e-posta</span>
        </div>
      </div>
    </div>

    <dl class="facts">
      <div>
        <dt>Teklif</dt>
        <dd>{{ offer }}</dd>
      </div>
      <div>
        <dt>Kitle</dt>
        <dd>{{ segment.explanation }}</dd>
      </div>
      <div v-if="spoken.length">
        <dt>Diller</dt>
        <dd data-testid="approval-languages">
          <span v-for="l in spoken" :key="l.code" class="lang">{{ l.display }} <strong class="num">{{ l.count }}</strong></span>
        </dd>
      </div>
    </dl>

    <p v-if="mode === 'later' && !frozen" class="notice info" role="status" data-testid="later-note">
      Kişi sayısı gönderim anında değişebilir: kitle şimdi değil, gönderim zamanı geldiğinde, o andaki verilerle yeniden belirlenir
      (yeni gelenler eklenir; izni kaldıranlar ve alışveriş yapanlar çıkar). Yukarıdaki sayılar yalnızca tahmindir.
    </p>

    <ul v-if="warnings.length" class="warnings" aria-label="Uyarılar">
      <li v-for="w in warnings" :key="(w.language ?? '') + w.field + w.code + w.message" class="notice warn">
        ⚠ <strong v-if="w.language">{{ w.language.toUpperCase() }}:</strong> {{ w.message }}
      </li>
    </ul>

    <Transition name="fade">
      <div v-if="confirming" class="confirm" role="alertdialog" aria-labelledby="confirm-title">
        <p v-if="mode === 'later'" id="confirm-title">
          <strong>Emin misiniz?</strong>
          Mesajlar seçtiğiniz zamanda otomatik gönderilecek; kimsenin o anda burada olması gerekmez. Kitle o anda belirlenir, bu yüzden
          kişi sayısı yaklaşık <strong>{{ sms }} SMS</strong> ve <strong>{{ email }} e-posta</strong> değerinden farklı olabilir. Onaydan sonra
          metin ve kitle kuralı değiştirilemez; gönderim başlamadan önce kampanyayı iptal edebilirsiniz.
        </p>
        <p v-else id="confirm-title">
          <strong>Emin misiniz?</strong>
          Mesajlar <strong>{{ sms }} SMS</strong> ve <strong>{{ email }} e-posta</strong> olarak gönderilecek. Onaydan sonra metin ve kitle
          değiştirilemez.
        </p>
      </div>
    </Transition>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.summary {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px 32px;
  margin-bottom: 18px;
}
.people {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.big {
  font-size: 3rem;
  line-height: 1;
  font-weight: 650;
  min-width: 3ch;
}
.channels {
  display: flex;
  gap: 10px;
}
.tile {
  min-width: 112px;
  padding: 10px 16px;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
}
.tile strong {
  font-size: 1.6rem;
  line-height: 1.15;
}
.tile .cap {
  font-size: 0.85rem;
}
.tile.sms {
  background: var(--sms-soft);
  color: var(--sms);
}
.tile.email {
  background: var(--email-soft);
  color: var(--email);
}
.facts {
  margin: 0 0 12px;
  display: grid;
  gap: 8px;
}
.facts div {
  display: grid;
  grid-template-columns: 5.5rem 1fr;
  gap: 12px;
}
dt {
  color: var(--muted);
}
dd {
  margin: 0;
}
.when {
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 10px 14px 12px;
  margin: 0 0 18px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
  align-items: center;
}
.when legend {
  padding: 0 6px;
  font-weight: 600;
}
.choice {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.choice.on {
  font-weight: 600;
}
.picker {
  flex-basis: 100%;
}
.picker .small {
  margin: 4px 0 0;
}
.issue {
  color: var(--error);
  font-size: 0.85rem;
  margin: 4px 0 0;
}
.lang {
  display: inline-block;
  margin-right: 14px;
}
.warnings {
  list-style: none;
  margin: 0;
  padding: 0;
}
.warnings .notice {
  margin: 8px 0;
}
.confirm {
  margin-top: 14px;
  padding: 14px 16px;
  border-radius: 12px;
  background: var(--accent-soft);
  border: 1px solid var(--accent);
}
.confirm p {
  margin: 0;
}
@media (max-width: 560px) {
  .big {
    font-size: 2.3rem;
  }
  .facts div {
    grid-template-columns: 1fr;
    gap: 0;
  }
}
</style>
