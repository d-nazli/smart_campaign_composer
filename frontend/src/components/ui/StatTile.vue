<script setup lang="ts">
import AnimatedNumber from '../AnimatedNumber.vue'

// One figure and what it counts. A number eases to its new value; a string is shown as it is.
withDefaults(
  defineProps<{
    label: string
    value: number | string
    hint?: string
    tone?: 'default' | 'ok' | 'danger'
    decimals?: number
    suffix?: string
  }>(),
  { tone: 'default', decimals: 0 },
)
</script>

<template>
  <div class="ui-stat" :class="`ui-stat--${tone}`">
    <span class="ui-stat__label">{{ label }}</span>
    <strong class="ui-stat__value num">
      <AnimatedNumber v-if="typeof value === 'number'" :value="value" :decimals="decimals" />
      <template v-else>{{ value }}</template>
      <small v-if="suffix">{{ suffix }}</small>
    </strong>
    <span v-if="hint" class="ui-stat__hint">{{ hint }}</span>
  </div>
</template>

<style scoped>
.ui-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--s-4) var(--s-5);
  min-width: 0;
}
.ui-stat__label {
  color: var(--muted);
  font-size: var(--text-sm);
}
.ui-stat__value {
  font-size: var(--text-xl);
  font-weight: 600;
  line-height: 1.2;
  letter-spacing: -0.02em;
  min-height: 1.2em;
}
.ui-stat__value small {
  margin-left: 3px;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--muted);
  letter-spacing: 0;
}
.ui-stat__hint {
  color: var(--muted);
  font-size: var(--text-xs);
}
.ui-stat--ok .ui-stat__value {
  color: var(--ok);
}
.ui-stat--danger .ui-stat__value {
  color: var(--error);
}
</style>
