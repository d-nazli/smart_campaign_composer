<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { Segment } from '../types'
import { funnel, share } from '../utils/funnel'
import { describeOffer } from '../utils/formatOffer'
import { describeCondition } from '../utils/formatRule'
import AnimatedNumber from './AnimatedNumber.vue'

const props = defineProps<{ segment: Segment; busy?: boolean }>()
const emit = defineEmits<{ (e: 'send-to-all'): void }>()

const chips = computed(() => props.segment.conditions.map(describeCondition))
const joiner = computed(() => (props.segment.match === 'any' ? 'veya' : 've'))
const offerText = computed(() => describeOffer(props.segment.offer))
const f = computed(() => (props.segment.audience ? funnel(props.segment.audience) : null))

// Bars start empty and grow to their share once the card is on screen.
const grown = ref(false)
onMounted(() => requestAnimationFrame(() => (grown.value = true)))
const w = (n: number) => (grown.value && f.value ? `${share(n, f.value.matched)}%` : '0%')
</script>

<template>
  <section id="step-audience" class="card segment" :class="segment.status" aria-live="polite">
    <h2>Kitle</h2>

    <template v-if="segment.status === 'ok' && segment.audience && f">
      <p class="explanation">{{ segment.explanation }}</p>

      <ul class="chips" aria-label="Kural koşulları">
        <template v-for="(c, i) in chips" :key="c">
          <li v-if="i > 0" class="joiner" aria-hidden="true">{{ joiner }}</li>
          <li class="chip">{{ c }}</li>
        </template>
      </ul>
      <p class="offer" data-testid="offer"><span class="chip offer-chip">Teklif</span> {{ offerText }}</p>

      <div class="funnel">
        <div class="row matched">
          <div class="meta">
            <span class="label">Eşleşen kişi</span>
            <strong class="value num" data-testid="matched"><AnimatedNumber :value="f.matched" from-zero /></strong>
          </div>
          <div class="track"><span class="bar" :style="{ width: grown ? '100%' : '0%' }" /></div>
        </div>

        <div class="split">
          <div class="row sms">
            <div class="meta">
              <span class="label">SMS ile ulaşılabilir</span>
              <strong class="value num" data-testid="sms-reachable"><AnimatedNumber :value="f.sms" from-zero /></strong>
            </div>
            <div class="track"><span class="bar" :style="{ width: w(f.sms) }" /></div>
          </div>
          <div class="row email">
            <div class="meta">
              <span class="label">E-posta ile ulaşılabilir</span>
              <strong class="value num" data-testid="email-reachable"><AnimatedNumber :value="f.email" from-zero /></strong>
            </div>
            <div class="track"><span class="bar" :style="{ width: w(f.email) }" /></div>
          </div>
          <div class="row none" :class="{ zero: f.unreachable === 0 }">
            <div class="meta">
              <span class="label">Hiçbir kanaldan ulaşılamayan</span>
              <strong class="value num" data-testid="unreachable"><AnimatedNumber :value="f.unreachable" from-zero /></strong>
            </div>
            <div class="track"><span class="bar" :style="{ width: w(f.unreachable) }" /></div>
          </div>
        </div>
      </div>

      <p class="why muted small">
        <template v-if="f.unreachable > 0">
          Ulaşılamayanlar o kanallar için iletişim izni vermemiş ya da kayıtlı telefonu / e-postası yok; onlara mesaj gönderilmez.
        </template>
        <template v-else>Eşleşen herkese en az bir kanaldan ulaşılabiliyor.</template>
        <template v-if="f.smsOnly > 0 || f.emailOnly > 0">
          {{ f.both }} kişiye iki kanaldan, {{ f.smsOnly }} kişiye yalnız SMS, {{ f.emailOnly }} kişiye yalnız e-posta ile ulaşılır.
        </template>
      </p>
      <p v-if="segment.audience.matched === 0" class="notice warn">Bu kurala uyan kimse yok. Brief'i değiştirmeyi deneyin.</p>
    </template>

    <template v-else-if="segment.status === 'needs_clarification'">
      <div class="box clarify" role="status">
        <strong>Biraz daha bilgi lazım</strong>
        <p>{{ segment.question }}</p>
        <p v-if="segment.offer && segment.offer.kind !== 'none'" class="offer" data-testid="offer">
          <span class="chip offer-chip">Anladığım teklif</span> {{ offerText }}
        </p>
        <div v-if="segment.suggestion === 'all_customers'" class="actions">
          <button class="primary" :disabled="busy" data-testid="send-to-all" @click="emit('send-to-all')">Tüm müşterilere gönder</button>
          <span class="small">İzin vermemiş olanlara yine de gönderilmez.</span>
        </div>
        <p class="small">Ya da brief'i kime göndereceğinizi yazarak güncelleyip tekrar deneyin.</p>
      </div>
    </template>

    <template v-else-if="segment.status === 'unsupported'">
      <div class="box unsupported" role="alert">
        <strong>Bu brief desteklenmiyor</strong>
        <p>{{ segment.explanation }}</p>
      </div>
    </template>
  </section>
</template>

<style scoped>
.explanation {
  margin: 0 0 12px;
  font-size: 1.02rem;
  font-weight: 600;
  max-width: 70ch;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
}
.chip {
  display: inline-block;
  padding: 3px 12px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--info);
  font-size: 0.86rem;
  font-weight: 550;
}
.joiner {
  color: var(--muted);
  font-size: 0.8rem;
}
.offer {
  margin: 0 0 20px;
}
.offer-chip {
  background: var(--warn-bg);
  color: var(--warn);
  margin-right: 4px;
}

.funnel {
  display: grid;
  gap: 14px;
}
.row .meta {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 5px;
}
.label {
  font-size: 0.9rem;
  color: var(--muted);
}
.value {
  font-size: 1.6rem;
  font-weight: 650;
  line-height: 1.1;
  min-width: 4ch;
  text-align: right;
}
.track {
  height: 12px;
  border-radius: 12px;
  background: var(--surface-2);
  overflow: hidden;
}
.bar {
  display: block;
  height: 100%;
  border-radius: 12px;
  background: var(--fg);
  transition: width 300ms var(--ease);
}
.matched .value {
  font-size: 2.3rem;
}
.sms .bar {
  background: var(--sms);
}
.sms .value {
  color: var(--sms);
}
.email .bar {
  background: var(--email);
}
.email .value {
  color: var(--email);
}
.none .bar {
  background: repeating-linear-gradient(135deg, var(--muted) 0 4px, transparent 4px 8px);
  opacity: 0.55;
}
.none .value {
  color: var(--muted);
}
/* the split rows hang off the matched bar like branches */
.split {
  display: grid;
  gap: 14px;
  margin-left: 14px;
  padding-left: 18px;
  border-left: 2px solid var(--border);
}
.why {
  margin: 14px 0 0;
  max-width: 75ch;
}

.box {
  border-radius: 12px;
  padding: 14px 16px;
  border: 1px solid;
}
.box p {
  margin: 6px 0 0;
}
.box .actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.clarify {
  background: var(--warn-bg);
  border-color: var(--warn-line);
  color: var(--warn);
}
.unsupported {
  background: var(--error-bg);
  border-color: var(--error-line);
  color: var(--error);
}

@media (max-width: 560px) {
  .value {
    font-size: 1.35rem;
  }
  .matched .value {
    font-size: 1.9rem;
  }
  .split {
    margin-left: 6px;
    padding-left: 12px;
  }
}
</style>
