<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Component } from 'vue'

export interface SegmentOption {
  value: string
  label: string
  icon?: Component
  testid?: string
}

// A choice between a few options, one always selected (a radio group that looks like a toggle).
// Keyboard: arrow keys move the choice, Tab leaves the group.
const props = defineProps<{ modelValue: string; options: SegmentOption[]; label: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const buttons = ref<HTMLButtonElement[]>([])
const index = computed(() => Math.max(0, props.options.findIndex((o) => o.value === props.modelValue)))

function choose(i: number) {
  const opt = props.options[(i + props.options.length) % props.options.length]
  if (props.disabled || !opt) return
  emit('update:modelValue', opt.value)
  buttons.value[props.options.indexOf(opt)]?.focus()
}

function onKey(e: KeyboardEvent) {
  switch (e.key) {
    case 'ArrowRight':
    case 'ArrowDown':
      e.preventDefault()
      return choose(index.value + 1)
    case 'ArrowLeft':
    case 'ArrowUp':
      e.preventDefault()
      return choose(index.value - 1)
    case 'Home':
      e.preventDefault()
      return choose(0)
    case 'End':
      e.preventDefault()
      return choose(props.options.length - 1)
  }
}
</script>

<template>
  <div class="ui-seg" role="radiogroup" :aria-label="label" :aria-disabled="disabled || undefined" @keydown="onKey">
    <button
      v-for="(o, i) in options"
      :key="o.value"
      :ref="(el) => (buttons[i] = el as HTMLButtonElement)"
      type="button"
      role="radio"
      class="ui-seg__opt"
      :class="{ 'is-on': o.value === modelValue }"
      :aria-checked="o.value === modelValue"
      :tabindex="o.value === modelValue ? 0 : -1"
      :disabled="disabled"
      :data-testid="o.testid"
      @click="choose(i)"
    >
      <component :is="o.icon" v-if="o.icon" :size="15" aria-hidden="true" />
      {{ o.label }}
    </button>
  </div>
</template>

<style scoped>
.ui-seg {
  display: inline-flex;
  padding: 2px;
  gap: 2px;
  border-radius: var(--r-sm);
  background: var(--surface-2);
  border: 1px solid var(--border);
}
.ui-seg__opt {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  min-height: 30px;
  padding: 0 var(--s-4);
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--muted);
  font-weight: 500;
}
.ui-seg__opt:hover:not(:disabled):not(.is-on) {
  background: transparent;
  color: var(--fg);
}
.ui-seg__opt.is-on,
.ui-seg__opt.is-on:hover:not(:disabled) {
  background: var(--surface);
  border-color: var(--border-strong);
  color: var(--fg);
}
</style>
