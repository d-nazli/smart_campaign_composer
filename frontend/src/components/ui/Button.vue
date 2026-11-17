<script setup lang="ts">
import type { Component } from 'vue'
import { Loader2 } from 'lucide-vue-next'

// The one button. primary: the single main action of a screen; secondary: everything else;
// outline: a visible way back that sits on the page background; ghost: quiet actions;
// danger: irreversible ones; cta: the page's create button (gradient, upper case).
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'outline' | 'ghost' | 'danger' | 'cta'
    size?: 'md' | 'sm'
    icon?: Component
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit'
  }>(),
  { variant: 'secondary', size: 'md', type: 'button' },
)
</script>

<template>
  <button :type="type" :disabled="disabled || loading" :aria-busy="loading || undefined" class="ui-btn" :class="[`ui-btn--${variant}`, `ui-btn--${size}`]">
    <Loader2 v-if="loading" class="ui-btn__spin" :size="16" aria-hidden="true" />
    <component :is="icon" v-else-if="icon" :size="16" aria-hidden="true" />
    <slot />
  </button>
</template>

<style scoped>
.ui-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--s-2);
  min-height: var(--control-h);
  padding: 0 var(--s-5);
  border-radius: var(--r-sm);
  border: 1px solid var(--border-strong);
  background: var(--surface);
  color: var(--fg);
  font-weight: 500;
  white-space: nowrap;
}
.ui-btn--sm {
  min-height: 34px;
  padding: 0 var(--s-3);
  font-size: var(--text-sm);
}
.ui-btn--secondary:hover:not(:disabled) {
  background: var(--surface-2);
}
.ui-btn--primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-fg);
}
.ui-btn--primary:hover:not(:disabled) {
  background: var(--accent-hover);
  border-color: var(--accent-hover);
}
.ui-btn--outline {
  background: var(--surface);
  border-color: var(--accent);
  color: var(--accent);
}
.ui-btn--outline:hover:not(:disabled) {
  background: var(--accent-soft);
}
.ui-btn--cta {
  background: var(--cta-gradient);
  border-color: transparent;
  color: #fff;
  font-size: var(--text-sm);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}
.ui-btn--cta:hover:not(:disabled) {
  background: var(--cta-gradient);
  filter: brightness(1.08);
}
.ui-btn--ghost {
  background: transparent;
  border-color: transparent;
  color: var(--muted);
}
.ui-btn--ghost:hover:not(:disabled) {
  background: var(--surface-2);
  color: var(--fg);
}
.ui-btn--danger {
  background: var(--error);
  border-color: var(--error);
  color: #fff;
}
.ui-btn--danger:hover:not(:disabled) {
  filter: brightness(0.92);
  background: var(--error);
}
.ui-btn__spin {
  animation: ui-spin 0.8s linear infinite;
}
@keyframes ui-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
