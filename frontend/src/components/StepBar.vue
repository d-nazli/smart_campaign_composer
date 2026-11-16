<script setup lang="ts">
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
            <svg v-if="done.includes(s.key)" viewBox="0 0 16 16" width="12" height="12">
              <path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
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
  margin-bottom: 20px;
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
  height: 2px;
  margin: 0 10px;
  border-radius: 2px;
  background: linear-gradient(var(--accent), var(--accent)) left / 0% 100% no-repeat, var(--border);
  transition: background-size var(--t-med) var(--ease);
}
li.done:not(:last-child)::after {
  background-size: 100% 100%;
}
button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px 4px 4px;
  border: none;
  background: none;
  border-radius: 999px;
  color: var(--muted);
  font-weight: 550;
  white-space: nowrap;
}
button:disabled {
  opacity: 1;
  cursor: default;
}
.dot {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 2px solid var(--border);
  background: var(--surface);
  font-family: var(--font-num);
  font-size: 0.78rem;
  font-weight: 650;
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
  box-shadow: 0 0 0 4px var(--accent-soft);
}
.current button {
  color: var(--fg);
  font-weight: 700;
  background: var(--accent-soft);
  padding-right: 14px;
}
.current .label {
  color: var(--accent);
}
/* steps that cannot be opened yet fade back */
.future {
  opacity: 0.45;
}
.done:not(.current) button:hover .dot {
  box-shadow: 0 0 0 3px var(--accent-soft);
}

@media (max-width: 560px) {
  /* Phone width: only the current step keeps its name. */
  li:not(.current) .label {
    display: none;
  }
  li:not(:last-child)::after {
    margin: 0 6px;
  }
}
</style>
