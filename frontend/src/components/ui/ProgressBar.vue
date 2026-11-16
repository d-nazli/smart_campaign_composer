<script setup lang="ts">
import { computed } from 'vue'

export interface Segment {
  value: number
  tone: 'ok' | 'danger' | 'accent' | 'sms' | 'email'
}

// A thin bar made of one or more parts that add up against `max`. With nothing to count
// (max 0) it reads as complete, like an empty queue.
const props = withDefaults(
  defineProps<{
    /** Parts of the bar, in order. */
    segments?: Segment[]
    /** Shorthand for one part. */
    value?: number
    tone?: Segment['tone']
    max: number
    label: string
    size?: 'sm' | 'md'
  }>(),
  { tone: 'accent', size: 'md' },
)

const parts = computed<Segment[]>(() => props.segments ?? [{ value: props.value ?? 0, tone: props.tone }])
const done = computed(() => parts.value.reduce((n, p) => n + p.value, 0))
const percent = computed(() => (props.max <= 0 ? 100 : Math.min(100, Math.round((done.value / props.max) * 100))))
const width = (v: number) => (props.max <= 0 ? '0%' : `${Math.min(100, (v / props.max) * 100)}%`)
</script>

<template>
  <div
    class="ui-progress"
    :class="`ui-progress--${size}`"
    role="progressbar"
    :aria-label="label"
    aria-valuemin="0"
    aria-valuemax="100"
    :aria-valuenow="percent"
  >
    <span v-for="(p, i) in parts" :key="i" class="ui-progress__part" :class="`ui-progress__part--${p.tone}`" :style="{ width: width(p.value) }" />
  </div>
</template>

<style scoped>
.ui-progress {
  display: flex;
  width: 100%;
  height: 8px;
  border-radius: 999px;
  background: var(--surface-2);
  box-shadow: inset 0 0 0 1px var(--border);
  overflow: hidden;
}
.ui-progress--sm {
  height: 6px;
}
.ui-progress__part {
  height: 100%;
  transition: width var(--t-med) var(--ease);
}
.ui-progress__part--accent {
  background: var(--accent);
}
.ui-progress__part--ok {
  background: var(--ok);
}
.ui-progress__part--danger {
  background: var(--error);
}
.ui-progress__part--sms {
  background: var(--sms);
}
.ui-progress__part--email {
  background: var(--email);
}
</style>
