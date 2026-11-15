<script setup lang="ts">
import { computed } from 'vue'
import type { Segment } from '../types'
import { describeSegment } from '../utils/formatRule'

const props = defineProps<{ segment: Segment }>()

const nf = new Intl.NumberFormat('tr-TR')
const rule = computed(() => describeSegment(props.segment))
const unreachable = computed(() => {
  const a = props.segment.audience
  return a ? a.matched - Math.max(a.sms_reachable, a.email_reachable) : 0
})
</script>

<template>
  <section class="card" aria-live="polite">
    <h2>Kitle</h2>

    <template v-if="segment.status === 'ok' && segment.audience">
      <p class="explanation">{{ segment.explanation }}</p>
      <ul class="conditions">
        <li v-for="line in rule.lines" :key="line">{{ line }}</li>
      </ul>
      <p v-if="segment.conditions.length > 1" class="muted small">{{ rule.match }}</p>

      <div class="stats">
        <div class="stat">
          <strong data-testid="matched">{{ nf.format(segment.audience.matched) }}</strong>
          <span>eşleşen kişi</span>
        </div>
        <div class="stat">
          <strong data-testid="sms-reachable">{{ nf.format(segment.audience.sms_reachable) }}</strong>
          <span>SMS ile ulaşılabilir</span>
        </div>
        <div class="stat">
          <strong data-testid="email-reachable">{{ nf.format(segment.audience.email_reachable) }}</strong>
          <span>e-posta ile ulaşılabilir</span>
        </div>
      </div>
      <p class="muted small">
        Ulaşılabilir = o kanal için izin vermiş ve telefon/e-postası kayıtlı. İzni olmayan kişiye o kanaldan mesaj gönderilmez.
        <template v-if="unreachable > 0">{{ nf.format(unreachable) }} kişiye hiçbir kanaldan ulaşılamıyor.</template>
      </p>
      <p v-if="segment.audience.matched === 0" class="notice warn">Bu kurala uyan kimse yok. Brief'i değiştirmeyi deneyin.</p>
    </template>

    <template v-else-if="segment.status === 'needs_clarification'">
      <p class="notice info"><strong>Biraz daha bilgi lazım:</strong> {{ segment.question }}</p>
      <p class="muted small">Brief'i buna göre güncelleyip tekrar deneyin.</p>
    </template>

    <template v-else-if="segment.status === 'unsupported'">
      <p class="notice warn"><strong>Bu brief desteklenmiyor:</strong> {{ segment.explanation }}</p>
    </template>
  </section>
</template>

<style scoped>
.explanation {
  margin: 0 0 8px;
  font-weight: 600;
}
.conditions {
  margin: 0 0 8px;
  padding-left: 20px;
}
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin: 16px 0 8px;
}
.stat {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.stat strong {
  font-size: 1.6rem;
  font-variant-numeric: tabular-nums;
}
.stat span {
  color: var(--muted);
  font-size: 0.85rem;
}
</style>
