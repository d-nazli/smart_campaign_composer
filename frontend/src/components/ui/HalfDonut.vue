<script setup lang="ts">
// A half ring: each part is an arc of the semicircle in proportion to its value
// (used for the reasons deliveries failed). The slot sits in the middle, under the arc.
import { computed } from 'vue'

export interface Arc {
  value: number
  color: string
}
const props = defineProps<{ parts: Arc[]; max?: number; label: string }>()

const R = 70
const LEN = Math.PI * R
const arcs = computed(() => {
  const total = props.max ?? props.parts.reduce((s, p) => s + p.value, 0)
  let at = 0
  return props.parts.map((p) => {
    const len = total > 0 ? (p.value / total) * LEN : 0
    const a = { color: p.color, dash: `${len} ${LEN * 2}`, offset: -at }
    at += len
    return a
  })
})
</script>

<template>
  <figure class="gauge" role="img" :aria-label="label">
    <svg viewBox="0 0 180 100" aria-hidden="true">
      <path d="M20 90 A70 70 0 0 1 160 90" class="track" />
      <path v-for="(a, i) in arcs" :key="i" d="M20 90 A70 70 0 0 1 160 90" class="arc" :stroke="a.color" :stroke-dasharray="a.dash" :stroke-dashoffset="a.offset" />
    </svg>
    <figcaption><slot /></figcaption>
  </figure>
</template>

<style scoped>
.gauge {
  margin: 0;
  position: relative;
  max-width: 220px;
  margin-inline: auto;
}
svg {
  display: block;
  width: 100%;
}
path {
  fill: none;
  stroke-width: 16;
}
.track {
  stroke: var(--surface-2);
}
figcaption {
  position: absolute;
  inset: auto 0 0;
  text-align: center;
}
</style>
