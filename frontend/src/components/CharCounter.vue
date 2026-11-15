<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ used: number; max: number }>()
const over = computed(() => props.used > props.max)
const near = computed(() => !over.value && props.used >= props.max * 0.9)
</script>

<template>
  <span class="counter" :class="{ over, near }" role="status" :aria-label="`${used} / ${max} karakter`">
    {{ used }} / {{ max }}
  </span>
</template>

<style scoped>
.counter {
  font-variant-numeric: tabular-nums;
  font-size: 0.8rem;
  color: var(--muted);
}
.counter.near {
  color: var(--warn);
}
.counter.over {
  color: var(--error);
  font-weight: 600;
}
</style>
