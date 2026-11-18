<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import type { LocaleCode, LocaleInfo } from '../types'
import { stateLabel, type LocaleState } from '../utils/locales'

export interface LocaleStatus {
  state: LocaleState
  /** The model is writing this locale right now. */
  writing: boolean
  /** Problems found in this locale's text (limits, unknown variables, disagreeing with the source). */
  issues: number
  /** This locale's text has unsaved edits. */
  unsaved: boolean
}

// The campaign's locales: a narrow column inside the Mesajlar card (the source on top, then
// the translations), and below 1280px a button that opens the same list.
const props = defineProps<{
  locales: LocaleInfo[]
  modelValue: LocaleCode
  status: Record<LocaleCode, LocaleStatus>
  counts?: Record<LocaleCode, number>
}>()
const emit = defineEmits<{ 'update:modelValue': [LocaleCode] }>()

const source = computed(() => props.locales.filter((l) => l.is_source))
const translations = computed(() => props.locales.filter((l) => !l.is_source))
const current = computed(() => props.locales.find((l) => l.code === props.modelValue) ?? props.locales[0])

const open = ref(false)
const root = ref<HTMLElement | null>(null)

function choose(code: LocaleCode) {
  emit('update:modelValue', code)
  open.value = false
}

// The dot: what the person must look at first wins (writing, problems, unsaved, then the state).
function dot(code: LocaleCode): { cls: string; label: string } {
  const s = props.status[code]
  if (!s) return { cls: 'missing', label: stateLabel.missing }
  if (s.writing) return { cls: 'busy', label: 'Yazılıyor' }
  if (s.issues > 0) return { cls: 'bad', label: `${s.issues} sorun` }
  if (s.unsaved) return { cls: 'unsaved', label: 'Kaydedilmedi' }
  return { cls: s.state, label: stateLabel[s.state] }
}

const people = (code: LocaleCode) => (props.counts?.[code] ?? 0).toLocaleString('tr-TR')

function onDocument(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}
watch(open, (o) => {
  if (o) {
    document.addEventListener('mousedown', onDocument)
    document.addEventListener('keydown', onKey)
  } else {
    document.removeEventListener('mousedown', onDocument)
    document.removeEventListener('keydown', onKey)
  }
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocument)
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div ref="root" class="locales">
    <button
      type="button"
      class="picker"
      aria-haspopup="listbox"
      :aria-expanded="open"
      aria-controls="locale-list"
      data-testid="locale-picker"
      @click="open = !open"
    >
      <span class="code">{{ current?.code }}</span>
      <span class="dot" :class="dot(modelValue).cls" aria-hidden="true" />
      <span class="state">{{ dot(modelValue).label }}</span>
      <span class="n num">{{ people(modelValue) }} kişi</span>
      <ChevronDown :size="16" aria-hidden="true" class="chev" :class="{ up: open }" />
    </button>

    <div id="locale-list" class="list" :class="{ open }" role="listbox" aria-label="Locale" :aria-activedescendant="'loc-' + modelValue">
      <template v-for="group in [{ title: 'Kaynak', items: source }, { title: `Çeviriler · ${translations.length}`, items: translations }]" :key="group.title">
        <div v-if="group.items.length" class="group" role="presentation">{{ group.title }}</div>
        <button
          v-for="l in group.items"
          :id="'loc-' + l.code"
          :key="l.code"
          type="button"
          role="option"
          class="item"
          :aria-selected="l.code === modelValue"
          :title="l.native_name"
          :data-testid="'locale-' + l.code"
          @click="choose(l.code)"
        >
          <span class="code">{{ l.code }}</span>
          <span class="dot" :class="dot(l.code).cls" :title="dot(l.code).label" :aria-label="dot(l.code).label" />
          <span class="n num">{{ people(l.code) }} kişi</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.locales {
  position: relative;
  min-width: 0;
}
.picker {
  display: none;
}
.list {
  display: flex;
  flex-direction: column;
  padding: var(--s-2) 0;
}
.group {
  padding: var(--s-3) var(--s-4) var(--s-1);
  color: var(--muted);
  font-size: var(--text-label);
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 0 var(--s-2);
  width: 100%;
  min-height: 44px;
  padding: var(--s-2) var(--s-4) var(--s-2) calc(var(--s-4) - 3px);
  border: 0;
  border-left: 3px solid transparent;
  border-radius: 0;
  background: none;
  color: var(--fg);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.item:hover {
  background: var(--surface-2);
}
.item[aria-selected='true'] {
  border-left-color: var(--accent);
  background: var(--accent-soft);
}
.item[aria-selected='true'] .code {
  color: var(--accent);
}
.item .dot {
  grid-row: 1 / span 2;
  grid-column: 2;
}
.code {
  font-weight: 700;
  font-size: var(--text-sm);
  font-variant-numeric: tabular-nums;
}
.n {
  color: var(--muted);
  font-size: var(--text-xs);
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex: none;
}
.dot.source {
  background: var(--accent);
}
.dot.ok {
  background: var(--ok);
}
.dot.stale {
  background: var(--warn);
}
.dot.missing {
  box-shadow: inset 0 0 0 2px var(--border-strong);
}
.dot.bad {
  background: var(--error);
}
.dot.unsaved {
  background: var(--email);
}
.dot.busy {
  background: var(--ai-chip-fg);
  animation: blink 1s ease-in-out infinite;
}
@keyframes blink {
  50% {
    opacity: 0.25;
  }
}
@media (prefers-reduced-motion: reduce) {
  .dot.busy {
    animation: none;
  }
}

/* Below 1280px: a button that opens the same list over the editor. */
@media (max-width: 1279px) {
  .picker {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    width: 100%;
    min-height: var(--control-h);
    padding: 0 var(--s-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--r-sm);
    background: var(--surface);
    color: var(--fg);
    font: inherit;
    cursor: pointer;
  }
  .picker .state {
    color: var(--muted);
    font-size: var(--text-sm);
  }
  .picker .n {
    margin-left: auto;
  }
  .chev {
    transition: transform var(--t-fast) var(--ease);
  }
  .chev.up {
    transform: rotate(180deg);
  }
  .list {
    display: none;
    position: absolute;
    z-index: 15;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    max-height: 60vh;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--surface);
    box-shadow: var(--shadow-pop);
  }
  .list.open {
    display: flex;
  }
}
</style>
