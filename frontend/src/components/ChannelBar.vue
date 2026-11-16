<script setup lang="ts">
import { computed } from 'vue'
import type { ChannelProgress } from '../types'
import { channelPercent } from '../utils/delivery'
import AnimatedNumber from './AnimatedNumber.vue'

// One channel's progress: sent (channel colour), failed (red), skipped (grey).
// Widths ease over 300 ms, so polling updates glide instead of jumping.
const props = defineProps<{ channel: 'sms' | 'email'; label: string; p: ChannelProgress; large?: boolean }>()

const pct = (n: number) => (props.p.total > 0 ? (n / props.p.total) * 100 : 0)
const percent = computed(() => channelPercent(props.p))
const sentW = computed(() => `${pct(props.p.sent)}%`)
const failedW = computed(() => `${pct(props.p.failed)}%`)
const skippedW = computed(() => `${pct(props.p.skipped)}%`)
</script>

<template>
  <div class="chan" :class="[channel, { large }]">
    <div class="head">
      <span class="name">{{ label }}</span>
      <span class="count num">
        <AnimatedNumber :value="p.sent + p.failed + p.skipped" /> / <AnimatedNumber :value="p.total" />
      </span>
      <span class="pct num">%<AnimatedNumber :value="percent" /></span>
    </div>
    <div
      class="track"
      role="progressbar"
      :aria-label="`${label} gönderim ilerlemesi`"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="percent"
    >
      <span class="seg sent" :style="{ width: sentW }" />
      <span class="seg failed" :style="{ width: failedW }" />
      <span class="seg skipped" :style="{ width: skippedW }" />
    </div>
  </div>
</template>

<style scoped>
.chan {
  --c: var(--sms);
}
.chan.email {
  --c: var(--email);
}
.head {
  display: grid;
  grid-template-columns: 1fr auto 3.2em;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 5px;
  font-size: 0.88rem;
}
.name {
  font-weight: 600;
  color: var(--c);
}
.count {
  color: var(--muted);
}
.pct {
  text-align: right;
  font-weight: 600;
}
.track {
  display: flex;
  height: 10px;
  border-radius: 10px;
  background: var(--surface-2);
  overflow: hidden;
}
.seg {
  display: block;
  height: 100%;
  transition: width var(--t-med) var(--ease);
}
.sent {
  background: var(--c);
}
.failed {
  background: var(--error);
}
.skipped {
  background: var(--muted);
  opacity: 0.45;
}

/* the sending step: big numbers and a thick bar */
.large .head {
  grid-template-columns: 1fr auto;
  grid-template-areas: 'name pct' 'count count';
  margin-bottom: 10px;
}
.large .name {
  grid-area: name;
  font-size: 1.05rem;
}
.large .pct {
  grid-area: pct;
  width: auto;
  font-size: 2.6rem;
  line-height: 1;
  min-width: 3.4ch;
}
.large .count {
  grid-area: count;
  font-size: 1rem;
}
.large .track {
  height: 20px;
  border-radius: 20px;
}
</style>
