<script setup lang="ts">
import type { Component } from 'vue'

// A short status or label. The colour says what kind of thing it is: neutral (plain fact),
// accent (in progress / selected), plan (scheduled for later), ok, warn, danger.
withDefaults(
  defineProps<{
    tone?: 'neutral' | 'accent' | 'plan' | 'ok' | 'warn' | 'danger'
    /** A small filled dot before the text (state). */
    dot?: boolean
    icon?: Component
  }>(),
  { tone: 'neutral' },
)
</script>

<template>
  <span class="ui-badge" :class="`ui-badge--${tone}`">
    <span v-if="dot" class="ui-badge__dot" aria-hidden="true" />
    <component :is="icon" v-else-if="icon" :size="12" aria-hidden="true" />
    <slot />
  </span>
</template>

<style scoped>
.ui-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 24px;
  padding: 0 var(--s-2);
  border-radius: var(--r-sm);
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--muted);
  font-size: var(--text-xs);
  font-weight: 500;
  white-space: nowrap;
}
.ui-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.ui-badge--accent {
  background: var(--accent-soft);
  border-color: transparent;
  color: var(--accent);
}
.ui-badge--plan {
  background: var(--plan-bg);
  border-color: transparent;
  color: var(--plan);
}
.ui-badge--ok {
  background: var(--ok-bg);
  border-color: var(--ok-line);
  color: var(--ok);
}
.ui-badge--warn {
  background: var(--warn-bg);
  border-color: var(--warn-line);
  color: var(--warn);
}
.ui-badge--danger {
  background: var(--error-bg);
  border-color: var(--error-line);
  color: var(--error);
}
</style>
