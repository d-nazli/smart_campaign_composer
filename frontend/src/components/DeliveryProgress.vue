<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignStatus, DeliveryProgress, LanguageInfo } from '../types'
import { deliveryTotals } from '../utils/dashboard'
import { describeChannel } from '../utils/delivery'
import { failureText, mergeFailures } from '../utils/failures'
import { FALLBACK_LANGUAGES, sentLine } from '../utils/languages'
import AnimatedNumber from './AnimatedNumber.vue'
import ChannelBar from './ChannelBar.vue'

// The sending step: big bars and numbers that follow the queue.
const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus; languages?: LanguageInfo[] }>()

const finished = computed(() => props.status === 'completed')
const totals = computed(() => deliveryTotals(props.delivery))
const reasons = computed(() => mergeFailures(props.delivery.failures))
// "TR 171 · EN 72 · DE 42": the messages sent to each language ("TR 120/171" while some still wait).
const byLanguage = computed(() => sentLine(props.delivery.languages, props.languages ?? FALLBACK_LANGUAGES, props.delivery.done))
</script>

<template>
  <div class="delivery">
    <div class="totals">
      <div class="t sent">
        <strong class="num"><AnimatedNumber :value="totals.sent" /></strong>
        <span class="cap">gönderildi</span>
      </div>
      <div class="t failed" :class="{ bad: totals.failed > 0 }">
        <strong class="num"><AnimatedNumber :value="totals.failed" /></strong>
        <span class="cap">başarısız</span>
      </div>
      <div class="t">
        <strong class="num"><AnimatedNumber :value="totals.pending" /></strong>
        <span class="cap">bekliyor</span>
      </div>
    </div>

    <div class="chans">
      <div :data-testid="'delivery-sms'">
        <ChannelBar channel="sms" label="SMS" :p="delivery.sms" large />
        <p class="small muted line">{{ describeChannel(delivery.sms) }}</p>
      </div>
      <div :data-testid="'delivery-email'">
        <ChannelBar channel="email" label="E-posta" :p="delivery.email" large />
        <p class="small muted line">{{ describeChannel(delivery.email) }}</p>
      </div>
    </div>

    <p v-if="byLanguage" class="bylang" data-testid="delivery-languages">
      <span class="cap">Dile göre gönderilen</span>
      <strong class="num">{{ byLanguage }}</strong>
    </p>

    <ul v-if="reasons.length" class="reasons" aria-label="Başarısızlık nedenleri">
      <li v-for="r in reasons" :key="r.reason">
        <strong class="num">{{ r.count }}</strong> {{ failureText(r.reason) }}
      </li>
    </ul>

    <p v-if="finished" class="notice ok" role="status">Gönderim tamamlandı.</p>
    <p v-else class="muted small" role="status">Gönderiliyor… Bu sayfa kendiliğinden güncellenir.</p>
  </div>
</template>

<style scoped>
.bylang {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 12px;
  margin: 16px 0 0;
  padding: 10px 14px;
  border-radius: 10px;
  background: var(--accent-soft);
}
.bylang strong {
  letter-spacing: 0.02em;
}
.totals {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 26px;
}
.t {
  padding: 14px 18px;
  border-radius: 14px;
  background: var(--surface-2);
}
.t strong {
  display: block;
  font-size: 2.6rem;
  line-height: 1.1;
  font-weight: 650;
  min-height: 1.1em;
}
.t .cap {
  color: var(--muted);
  font-size: 0.9rem;
}
.t.sent strong {
  color: var(--ok);
}
.t.bad {
  background: var(--error-bg);
}
.t.bad strong,
.t.bad .cap {
  color: var(--error);
}
.chans {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 28px;
  margin-bottom: 18px;
}
.line {
  margin: 8px 0 0;
  min-height: 1.4em;
}
.reasons {
  list-style: none;
  margin: 0 0 12px;
  padding: 10px 14px;
  border-radius: 10px;
  background: var(--error-bg);
  color: var(--error);
  font-size: 0.9rem;
}
.reasons li + li {
  margin-top: 4px;
}
@media (max-width: 560px) {
  .chans {
    grid-template-columns: minmax(0, 1fr);
    gap: 22px;
  }
  .t {
    padding: 10px 12px;
  }
  .t strong {
    font-size: 1.8rem;
  }
}
</style>
