<script setup lang="ts">
import { computed } from 'vue'
import { ArrowLeft, Check, ChevronRight } from 'lucide-vue-next'
import { STEPS, type StepKey } from '../utils/steps'

// The flow bar: a way back to the campaign list, then the steps as a breadcrumb
// ("Brief › Kitle › …"). Steps that cannot be opened yet are greyed out. On a phone only
// "3/5 · Mesajlar" is shown. The `end` slot holds the campaign's state (e.g. "Onaylandı").
const props = defineProps<{ current: StepKey; done: StepKey[]; reachable: StepKey[] }>()
const emit = defineEmits<{ go: [StepKey] }>()

const at = computed(() => Math.max(0, STEPS.findIndex((s) => s.key === props.current)))
</script>

<template>
  <div class="flowbar">
    <a href="#/dashboard" class="flowbar__back"><ArrowLeft :size="16" aria-hidden="true" />Kampanyalar</a>
    <span class="flowbar__divider" aria-hidden="true" />
    <nav class="steps" aria-label="Kampanya adımları">
      <ol>
        <li
          v-for="(s, i) in STEPS"
          :key="s.key"
          :class="{ current: s.key === current, done: done.includes(s.key), future: !reachable.includes(s.key) }"
          :aria-current="s.key === current ? 'step' : undefined"
        >
          <ChevronRight v-if="i > 0" :size="14" class="sep" aria-hidden="true" />
          <button type="button" :disabled="!reachable.includes(s.key) || s.key === current" @click="emit('go', s.key)">
            <Check v-if="done.includes(s.key)" :size="14" :stroke-width="3" class="tick" aria-hidden="true" />
            <span class="step-name">{{ s.label }}</span>
            <span v-if="done.includes(s.key)" class="sr-only">(tamamlandı)</span>
          </button>
        </li>
      </ol>
    </nav>
    <p class="flowbar__compact" aria-hidden="true">
      <b class="num">{{ at + 1 }}/{{ STEPS.length }}</b> · {{ STEPS[at].label }}
    </p>
    <div v-if="$slots.end" class="flowbar__end"><slot name="end" /></div>
  </div>
</template>

<style scoped>
.flowbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--s-3);
  min-height: 56px;
  padding: var(--s-1) var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
}
.flowbar__back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
}
.flowbar__divider {
  width: 1px;
  height: 24px;
  background: var(--border);
}
ol {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}
li {
  display: inline-flex;
  align-items: center;
}
.sep {
  margin: 0 2px;
  color: var(--border-strong);
}
button {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-height: 40px;
  padding: 0 var(--s-2);
  border: 0;
  border-radius: 0;
  background: none;
  color: var(--muted);
  font-weight: 500;
}
button:hover:not(:disabled) {
  background: none;
  color: var(--fg);
}
.done button {
  color: var(--fg);
}
.tick {
  color: var(--ok);
}
.current button {
  color: var(--accent);
  font-weight: 600;
  box-shadow: inset 0 -2px 0 var(--accent);
}
.current button:disabled {
  opacity: 1;
  cursor: default;
}
.future button:disabled {
  opacity: 0.55;
}
.flowbar__compact {
  display: none;
  margin: 0;
  color: var(--muted);
  white-space: nowrap;
}
.flowbar__compact b {
  color: var(--accent);
}
.flowbar__end {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin-left: auto;
}
@media (max-width: 767px) {
  .flowbar {
    gap: var(--s-1) var(--s-2);
    padding: var(--s-2) var(--s-3);
  }
  .flowbar__divider,
  .steps {
    display: none;
  }
  .flowbar__compact {
    display: block;
    margin-left: auto;
  }
  .flowbar__end {
    margin-left: 0;
  }
}
</style>
