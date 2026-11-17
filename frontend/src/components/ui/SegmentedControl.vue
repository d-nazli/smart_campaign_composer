<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Component } from 'vue'

export interface SegmentOption {
  value: string
  label: string
  icon?: Component
  testid?: string
}

// A choice between a few options, one always selected (a radio group). `inline` looks like a
// toggle; `rows` stacks the options as full-width rows with a radio mark (the panel's
// "Send now / Send later" pattern). Keyboard: arrow keys move the choice, Tab leaves the group.
const props = withDefaults(
  defineProps<{ modelValue: string; options: SegmentOption[]; label: string; disabled?: boolean; layout?: 'inline' | 'rows' }>(),
  { layout: 'inline' },
)
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
  <div class="ui-seg" :class="`ui-seg--${layout}`" role="radiogroup" :aria-label="label" :aria-disabled="disabled || undefined" @keydown="onKey">
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
      <i v-if="layout === 'rows'" class="ui-seg__mark" aria-hidden="true" />
      <component :is="o.icon" v-if="o.icon" :size="layout === 'rows' ? 18 : 15" class="ui-seg__icon" aria-hidden="true" />
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
.ui-seg--rows {
  display: grid;
  gap: var(--s-2);
  padding: 0;
  border: 0;
  background: none;
}
.ui-seg--rows .ui-seg__opt {
  justify-content: flex-start;
  gap: var(--s-3);
  min-height: 52px;
  padding: 0 var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--fg);
  font-size: var(--text-base);
  font-weight: 600;
}
.ui-seg--rows .ui-seg__opt:hover:not(:disabled):not(.is-on) {
  border-color: var(--border-strong);
  background: var(--surface);
}
.ui-seg--rows .ui-seg__opt.is-on,
.ui-seg--rows .ui-seg__opt.is-on:hover:not(:disabled) {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent-soft) 50%, var(--surface));
}
.ui-seg__mark {
  flex: none;
  width: 18px;
  height: 18px;
  border: 1.5px solid var(--border-strong);
  border-radius: 50%;
  background: var(--surface);
}
.is-on .ui-seg__mark {
  border: 5px solid var(--accent);
}
.ui-seg--rows .ui-seg__icon {
  color: var(--accent);
}
</style>
