<script setup lang="ts">
// A half ring: each part is an arc of the semicircle in proportion to its value
// (used for the outcome of a campaign's messages). The slot sits below the ring, clear of the arc.
import { computed } from 'vue'

export interface Arc {
  value: number
  color: string
}
const props = defineProps<{ parts: Arc[]; max?: number; label: string }>()

const R = 70
const LEN = Math.PI * R
// Against a larger whole (max) a small part would be a hairline: it keeps a visible minimum so its colour can be seen.
const MIN_ARC = 3
const arcs = computed(() => {
  const total = props.max ?? props.parts.reduce((s, p) => s + p.value, 0)
  const lens = props.parts.map((p) => {
    const len = total > 0 ? (p.value / total) * LEN : 0
    return props.max !== undefined && p.value > 0 ? Math.max(len, MIN_ARC) : len
  })
  // The minimum can push the parts past the half ring: the largest part gives the room back.
  const over = lens.reduce((s, l) => s + l, 0) - LEN
  if (over > 0) {
    const big = lens.indexOf(Math.max(...lens))
    lens[big] = Math.max(0, lens[big] - over)
  }
  let at = 0
  return props.parts.map((p, i) => {
    const a = { color: p.color, dash: `${lens[i]} ${LEN * 2}`, offset: -at }
    at += lens[i]
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
/* Under the ring, never over it: the arc's ends reach the bottom of the drawing. */
figcaption {
  margin-top: var(--s-3);
  text-align: center;
}
</style>
