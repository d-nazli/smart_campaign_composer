<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import { STEPS, type StepKey } from '../utils/steps'

defineProps<{ current: StepKey; done: StepKey[]; reachable: StepKey[] }>()
const emit = defineEmits<{ go: [StepKey] }>()
</script>

<template>
  <nav class="steps" aria-label="Kampanya adımları">
    <ol>
      <li
        v-for="(s, i) in STEPS"
        :key="s.key"
        :class="{ current: s.key === current, done: done.includes(s.key), future: !reachable.includes(s.key) }"
        :aria-current="s.key === current ? 'step' : undefined"
      >
        <button type="button" :disabled="!reachable.includes(s.key) || s.key === current" @click="emit('go', s.key)">
          <span class="dot" aria-hidden="true">
            <Check v-if="done.includes(s.key)" :size="12" :stroke-width="3" />
            <template v-else>{{ i + 1 }}</template>
          </span>
          <span class="label">{{ s.label }}</span>
          <span v-if="done.includes(s.key)" class="sr-only">(tamamlandı)</span>
        </button>
      </li>
    </ol>
  </nav>
</template>

<style scoped>
.steps {
  margin-bottom: var(--s-6);
}
ol {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  align-items: center;
}
li {
  display: flex;
  align-items: center;
  flex: 1 1 0;
  min-width: 0;
}
li:last-child {
  flex: 0 0 auto;
}
/* the line between two steps fills once the step before it is done */
li:not(:last-child)::after {
  content: '';
  flex: 1;
  height: 1px;
  margin: 0 var(--s-3);
  background: linear-gradient(var(--accent), var(--accent)) left / 0% 100% no-repeat, var(--border-strong);
  transition: background-size var(--t-med) var(--ease);
}
li.done:not(:last-child)::after {
  background-size: 100% 100%;
}
button {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  padding: 4px var(--s-2) 4px 4px;
  border: 1px solid transparent;
  background: none;
  border-radius: 999px;
  color: var(--muted);
  font-weight: 500;
  white-space: nowrap;
}
button:hover:not(:disabled) {
  background: none;
}
button:disabled {
  opacity: 1;
  cursor: default;
}
.dot {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 1px solid var(--border-strong);
  background: var(--surface);
  font-size: var(--text-xs);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  transition: background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
}
.done .dot {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-fg);
}
.done button {
  color: var(--fg);
  cursor: pointer;
}
.done button:hover .label {
  text-decoration: underline;
  text-underline-offset: 3px;
}
/* The step being worked on stands out: filled dot with a halo, bold label on a tinted pill. */
.current .dot {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-fg);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.current button {
  color: var(--fg);
  font-weight: 700;
  background: var(--accent-soft);
  padding-right: var(--s-3);
}
.current .label {
  color: var(--accent);
}
/* steps that cannot be opened yet fade back */
.future {
  opacity: 0.5;
}

@media (max-width: 560px) {
  /* Phone width: only the current step keeps its name. */
  li:not(.current) .label {
    display: none;
  }
  li:not(:last-child)::after {
    margin: 0 var(--s-2);
  }
}
</style>
