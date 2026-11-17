<script setup lang="ts">
import { HelpCircle } from 'lucide-vue-next'
import AnimatedNumber from '../AnimatedNumber.vue'

// One figure and what it counts: an upper-case name, the number, and (optionally) a
// line of data under it. What the figure means is a tooltip on the (?) mark, not a sentence.
// A number eases to its new value; a string is shown as it is.
withDefaults(
  defineProps<{
    label: string
    value: number | string
    /** What the figure means: shown as the (?) tooltip. */
    hint?: string
    /** A short line of data under the figure ("60 kişiye ulaşılamıyor"). */
    sub?: string
    tone?: 'default' | 'ok' | 'danger'
    decimals?: number
    suffix?: string
  }>(),
  { tone: 'default', decimals: 0 },
)
</script>

<template>
  <div class="ui-stat" :class="`ui-stat--${tone}`">
    <span class="ui-stat__label">
      {{ label }}
      <span v-if="hint" class="ui-stat__q" :title="hint" :aria-label="hint" role="img"><HelpCircle :size="13" aria-hidden="true" /></span>
    </span>
    <strong v-if="value !== ''" class="ui-stat__value num">
      <AnimatedNumber v-if="typeof value === 'number'" :value="value" :decimals="decimals" />
      <template v-else>{{ value }}</template>
      <small v-if="suffix">{{ suffix }}</small>
    </strong>
    <span v-if="sub" class="ui-stat__sub">{{ sub }}</span>
    <slot />
  </div>
</template>

<style scoped>
.ui-stat {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  padding: var(--s-5) var(--s-6);
  background: var(--surface);
  min-width: 0;
}
.ui-stat__label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--muted);
  font-size: var(--text-label);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.ui-stat__q {
  display: inline-grid;
  opacity: 0.7;
  cursor: help;
}
.ui-stat__value {
  font-size: 2.25rem;
  font-weight: 400;
  line-height: 1.15;
  letter-spacing: -0.01em;
  min-height: 1.15em;
}
.ui-stat__value small {
  margin-left: 3px;
  font-size: var(--text-md);
  font-weight: 400;
  color: var(--muted);
  letter-spacing: 0;
}
.ui-stat__sub {
  color: var(--muted);
  font-size: var(--text-sm);
}
.ui-stat--ok .ui-stat__value {
  color: var(--ok);
}
.ui-stat--danger .ui-stat__value {
  color: var(--error);
}
@media (max-width: 767px) {
  .ui-stat {
    padding: var(--s-4);
  }
  .ui-stat__value {
    font-size: 1.875rem;
  }
}
</style>
