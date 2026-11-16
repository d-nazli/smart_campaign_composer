<script setup lang="ts">
import { computed } from 'vue'
import type { CampaignStatus, ChannelProgress, DeliveryProgress } from '../types'
import { channelPercent, describeChannel } from '../utils/delivery'

const props = defineProps<{ delivery: DeliveryProgress; status: CampaignStatus }>()

const rows = computed<{ key: string; label: string; p: ChannelProgress }[]>(() => [
  { key: 'sms', label: 'SMS', p: props.delivery.sms },
  { key: 'email', label: 'E-posta', p: props.delivery.email },
])
const finished = computed(() => props.status === 'completed')
const failedTotal = computed(() => props.delivery.sms.failed + props.delivery.email.failed)
</script>

<template>
  <div class="delivery" aria-live="polite">
    <div v-for="r in rows" :key="r.key" class="row" :data-testid="`delivery-${r.key}`">
      <div class="head">
        <strong>{{ r.label }}</strong>
        <span class="muted small">{{ r.p.total }} alıcı</span>
      </div>
      <progress :value="channelPercent(r.p)" max="100" :aria-label="`${r.label} gönderim ilerlemesi`" />
      <p class="small" :class="{ bad: r.p.failed > 0 }">{{ describeChannel(r.p) }}</p>
    </div>

    <p v-if="finished" class="notice ok" role="status">Gönderim tamamlandı.</p>
    <p v-else class="muted small" role="status">Gönderiliyor… Bu sayfa kendiliğinden güncellenir.</p>
    <p v-if="finished && failedTotal > 0" class="muted small">
      Başarısız olanlar genellikle geçersiz telefon / e-posta adresidir; bunlar yeniden denenmez.
    </p>
  </div>
</template>

<style scoped>
.row {
  margin-bottom: 14px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}
progress {
  width: 100%;
  height: 12px;
  accent-color: var(--accent);
}
p {
  margin: 4px 0 0;
}
.bad {
  color: var(--error);
}
</style>
