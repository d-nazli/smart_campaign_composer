<script setup lang="ts">
import { computed } from 'vue'
import { meterLevel } from '../utils/preview'

// Character counter as a ring (variant "ring") or a bar (default).
// Normal until 90% of the limit, yellow close to it, red beyond it.
const props = defineProps<{ used: number; max: number; variant?: 'ring' | 'bar' }>()

const level = computed(() => meterLevel(props.used, props.max))
const fraction = computed(() => (props.max > 0 ? Math.min(1, props.used / props.max) : 0))
const R = 15
const C = 2 * Math.PI * R
const dash = computed(() => `${C * fraction.value} ${C}`)
const label = computed(() =>
  level.value === 'over' ? `${props.used} / ${props.max} karakter, sınır aşıldı` : `${props.used} / ${props.max} karakter`,
)
</script>

<template>
  <span class="counter" :class="[level, `as-${variant ?? 'bar'}`]" role="status" :aria-label="label">
    <svg v-if="variant === 'ring'" viewBox="0 0 36 36" width="40" height="40" aria-hidden="true">
      <circle cx="18" cy="18" :r="R" class="track" />
      <circle cx="18" cy="18" :r="R" class="arc" :stroke-dasharray="dash" transform="rotate(-90 18 18)" />
    </svg>
    <span v-if="variant === 'ring'" class="ring-text" aria-hidden="true">
      <strong>{{ used }}</strong>
      <span>/ {{ max }}</span>
    </span>
    <template v-else>
      <span class="bar" aria-hidden="true"><span class="fill" :style="{ transform: `scaleX(${fraction})` }" /></span>
      <span class="text" aria-hidden="true">{{ used }} / {{ max }}</span>
    </template>
  </span>
</template>

<style scoped>
.counter {
  --c: var(--accent);
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-variant-numeric: tabular-nums;
  font-size: 0.8rem;
  color: var(--muted);
}
.counter.near {
  --c: var(--warn-line);
}
.counter.over {
  --c: var(--error);
}
.counter.near .text,
.counter.near .ring-text strong {
  color: var(--warn);
}
.counter.over .text,
.counter.over .ring-text strong {
  color: var(--error);
  font-weight: 650;
}
svg {
  display: block;
}
.track {
  fill: none;
  stroke: var(--border);
  stroke-width: 3.5;
}
.arc {
  fill: none;
  stroke: var(--c);
  stroke-width: 3.5;
  stroke-linecap: round;
  transition: stroke-dasharray var(--t-med) var(--ease), stroke var(--t-fast) var(--ease);
}
.ring-text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
}
.ring-text strong {
  font-size: 0.95rem;
  color: var(--fg);
}
.bar {
  width: 84px;
  height: 6px;
  border-radius: 6px;
  background: var(--border);
  overflow: hidden;
}
.fill {
  display: block;
  height: 100%;
  background: var(--c);
  transform-origin: left;
  transition: transform var(--t-med) var(--ease), background var(--t-fast) var(--ease);
}
</style>
