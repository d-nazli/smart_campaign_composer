<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

// A number that eases to its new value in ~250 ms. With `fromZero` it counts up
// when it first appears. Respects prefers-reduced-motion (jumps straight there).
const props = withDefaults(defineProps<{ value: number; fromZero?: boolean; decimals?: number; duration?: number }>(), {
  fromZero: false,
  decimals: 0,
  duration: 280,
})

const shown = ref(props.fromZero ? 0 : props.value)
let frame = 0

const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

function animateTo(target: number) {
  cancelAnimationFrame(frame)
  const start = shown.value
  if (reduced() || start === target) {
    shown.value = target
    return
  }
  const t0 = performance.now()
  const step = (now: number) => {
    const p = Math.min(1, Math.max(0, (now - t0) / props.duration))
    const eased = 1 - Math.pow(1 - p, 3)
    shown.value = start + (target - start) * eased
    if (p < 1) frame = requestAnimationFrame(step)
  }
  frame = requestAnimationFrame(step)
}

onMounted(() => {
  if (props.fromZero) animateTo(props.value)
})
watch(() => props.value, animateTo)
onBeforeUnmount(() => cancelAnimationFrame(frame))

const text = computed(() =>
  new Intl.NumberFormat('tr-TR', { minimumFractionDigits: props.decimals, maximumFractionDigits: props.decimals }).format(shown.value),
)
const final = computed(() =>
  new Intl.NumberFormat('tr-TR', { minimumFractionDigits: props.decimals, maximumFractionDigits: props.decimals }).format(props.value),
)
</script>

<template>
  <!-- Screen readers get the final value at once, not every step of the count. -->
  <span class="anum">
    <span aria-hidden="true">{{ text }}</span>
    <span class="sr-only">{{ final }}</span>
  </span>
</template>

<style scoped>
.anum {
  font-variant-numeric: tabular-nums;
}
</style>
