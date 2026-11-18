<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { Loader2, MoreVertical, RefreshCw, Trash2 } from 'lucide-vue-next'
import type { Variant } from '../types'

// The variants of an A/B test as tabs, the way Insider's experiment screen shows them:
// "Varyant A · Merak | Varyant B · Fayda ⋮". The strip exists only while there is a Variant B
// (the card header's "A/B testi" button writes it); B's menu writes it again or removes it.
// A variant that needs attention (a stale translation, an unsaved edit, a problem) carries a
// dot; one being written spins.
const props = defineProps<{
  modelValue: Variant
  variants: Variant[]
  /** Each tab's text, e.g. "Varyant A · Merak". */
  labels: Record<Variant, string>
  /** Per variant: needs attention / being written now. */
  status: Record<Variant, { dot: boolean; writing: boolean }>
  /** B can be neither written again nor removed (approved, or busy). */
  locked: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [Variant]; regenerate: [Variant]; remove: [Variant] }>()

const menuOpen = ref(false)
const root = ref<HTMLElement | null>(null)

function onDocument(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) menuOpen.value = false
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && menuOpen.value) {
    menuOpen.value = false
    root.value?.querySelector<HTMLButtonElement>('[data-testid="variant-menu"]')?.focus()
  }
}
watch(menuOpen, (open) => {
  if (open) {
    document.addEventListener('mousedown', onDocument)
    document.addEventListener('keydown', onKey)
  } else {
    document.removeEventListener('mousedown', onDocument)
    document.removeEventListener('keydown', onKey)
  }
})
watch(
  () => props.locked,
  (l) => l && (menuOpen.value = false),
)
onBeforeUnmount(() => (menuOpen.value = false))

function pick(v: Variant) {
  menuOpen.value = false
  emit('update:modelValue', v)
}
function act(what: 'regenerate' | 'remove') {
  menuOpen.value = false
  if (what === 'regenerate') emit('regenerate', 'B')
  else emit('remove', 'B')
}
</script>

<template>
  <div ref="root" class="vt" data-testid="variant-tabs">
    <div role="tablist" aria-label="Varyantlar" class="vt__list">
      <template v-for="(v, i) in variants" :key="v">
        <span v-if="i > 0" class="vt__sep" aria-hidden="true">|</span>
        <button
          role="tab"
          type="button"
          class="vt__tab"
          :class="{ 'is-on': modelValue === v }"
          :aria-selected="modelValue === v"
          :data-testid="`variant-tab-${v}`"
          @click="pick(v)"
        >
          {{ labels[v] }}
          <Loader2 v-if="status[v]?.writing" :size="14" class="vt__spin" aria-label="yazılıyor" />
          <span v-else-if="status[v]?.dot" class="vt__dot" aria-label="dikkat gerekiyor" />
        </button>
        <div v-if="v === 'B' && !locked" class="vt__menu">
          <button
            type="button"
            class="vt__kebab"
            aria-label="Varyant B işlemleri"
            aria-haspopup="menu"
            :aria-expanded="menuOpen"
            data-testid="variant-menu"
            @click="menuOpen = !menuOpen"
          >
            <MoreVertical :size="18" aria-hidden="true" />
          </button>
          <ul v-if="menuOpen" role="menu" class="vt__pop">
            <li role="none">
              <button role="menuitem" type="button" class="vt__item" data-testid="variant-regenerate" @click="act('regenerate')">
                <RefreshCw :size="16" aria-hidden="true" />Yeniden üret
              </button>
            </li>
            <li role="none">
              <button role="menuitem" type="button" class="vt__item vt__item--danger" data-testid="variant-remove" @click="act('remove')">
                <Trash2 :size="16" aria-hidden="true" />Sil
              </button>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.vt {
  padding: 0 var(--s-6);
  border-bottom: 1px solid var(--border);
}
.vt__list {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-1);
}
.vt__tab {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  min-height: var(--control-h);
  padding: 0 var(--s-2);
  border: 0;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--muted);
  font: inherit;
  font-weight: 500;
  cursor: pointer;
}
.vt__tab:hover {
  color: var(--fg);
}
.vt__tab.is-on {
  color: var(--accent);
  border-bottom-color: var(--accent);
  font-weight: 600;
}
.vt__sep {
  padding: 0 var(--s-1);
  color: var(--border-strong);
}
.vt__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--warn);
}
.vt__spin {
  color: var(--ai-chip-fg);
  animation: vt-spin 1s linear infinite;
}
@keyframes vt-spin {
  to {
    transform: rotate(360deg);
  }
}
.vt__menu {
  position: relative;
}
.vt__kebab {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: var(--r-sm);
  background: none;
  color: var(--muted);
  cursor: pointer;
}
.vt__kebab:hover,
.vt__kebab[aria-expanded='true'] {
  background: var(--surface-2);
  color: var(--fg);
}
.vt__pop {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 20;
  min-width: 180px;
  margin: 0;
  padding: var(--s-1) 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-pop);
}
.vt__item {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  width: 100%;
  min-height: 40px;
  padding: 0 var(--s-4);
  border: 0;
  background: none;
  color: var(--fg);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.vt__item:hover {
  background: var(--surface-2);
}
.vt__item--danger {
  color: var(--error);
}
@media (max-width: 767px) {
  .vt {
    padding: 0 var(--s-4);
  }
}
@media (prefers-reduced-motion: reduce) {
  .vt__spin {
    animation: none;
  }
}
</style>
