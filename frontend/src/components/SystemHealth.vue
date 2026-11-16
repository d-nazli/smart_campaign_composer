<script setup lang="ts">
import { computed } from 'vue'
import { llmLabel, type AppInfo } from '../api/info'
import type { Light, SystemHealth } from '../api/system'

// The health lights: api and worker (the api asks the worker's /healthz), the database, and the LLM mode.
const props = defineProps<{ system: SystemHealth | null; unreachable: boolean; info: AppInfo | null }>()

interface Item {
  key: string
  label: string
  state: Light | 'checking'
  text: string
}

const word = (s: Light | 'checking') =>
  s === 'ok' ? 'çalışıyor' : s === 'down' ? 'yanıt vermiyor' : s === 'unknown' ? 'bilinmiyor' : 'kontrol ediliyor'

const items = computed<Item[]>(() => {
  const s = props.system
  const api: Light | 'checking' = props.unreachable ? 'down' : (s?.api ?? 'checking')
  // When the api is gone we know nothing about the others.
  const rest = (v: Light | undefined): Light | 'checking' => (props.unreachable ? 'unknown' : (v ?? 'checking'))
  const list: Item[] = [
    { key: 'api', label: 'API', state: api, text: word(api) },
    { key: 'worker', label: 'Worker', state: rest(s?.worker), text: word(rest(s?.worker)) },
    { key: 'db', label: 'Veritabanı', state: rest(s?.database), text: word(rest(s?.database)) },
  ]
  return list
})

const llm = computed(() => (props.info ? llmLabel(props.info) : '…'))
const allOk = computed(() => items.value.every((i) => i.state === 'ok'))
</script>

<template>
  <div class="health" role="group" aria-label="Sistem sağlığı" data-testid="system-health">
    <span class="summary" :class="{ ok: allOk }">{{ allOk ? 'Sistem sağlıklı' : 'Sistem durumu' }}</span>
    <ul>
      <li v-for="i in items" :key="i.key" :data-testid="`health-${i.key}`">
        <span class="dot" :class="i.state" aria-hidden="true" />
        <span class="label">{{ i.label }}</span>
        <span class="state muted small">{{ i.text }}</span>
      </li>
      <li data-testid="health-llm">
        <span class="dot" :class="info?.llm_mode === 'claude' ? 'ok' : 'mock'" aria-hidden="true" />
        <span class="label">LLM</span>
        <span class="state muted small">{{ llm }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.health {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--s-1) var(--s-5);
  padding: var(--s-2) var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--surface);
  font-size: var(--text-sm);
}
.summary {
  font-weight: 600;
  color: var(--warn);
}
.summary.ok {
  color: var(--ok);
}
ul {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-1) var(--s-4);
  list-style: none;
  margin: 0;
  padding: 0;
}
li {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
}
.label {
  font-weight: 500;
}
.dot {
  align-self: center;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--muted);
  transition: background var(--t-med) var(--ease);
}
.dot.ok {
  background: var(--ok);
}
.dot.down {
  background: var(--error);
}
.dot.mock {
  background: var(--warn-line);
}
.dot.checking {
  background: var(--border-strong);
}
</style>
