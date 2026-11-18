<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { X } from 'lucide-vue-next'

// A dialog for one decision. Escape and the backdrop close it (unless `persistent`),
// Tab stays inside, and focus goes back to whatever opened it.
// alert: a question that interrupts (role alertdialog), e.g. "Emin misiniz?" before sending.
const props = defineProps<{ modelValue: boolean; title: string; description?: string; persistent?: boolean; alert?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; close: [] }>()

const dialog = ref<HTMLElement | null>(null)
const titleId = `modal-title-${Math.random().toString(36).slice(2, 8)}`
let opener: HTMLElement | null = null

function close() {
  emit('update:modelValue', false)
  emit('close')
}

const FOCUSABLE = 'button:not(:disabled), [href], input:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && !props.persistent) {
    e.stopPropagation()
    return close()
  }
  if (e.key !== 'Tab' || !dialog.value) return
  const items = [...dialog.value.querySelectorAll<HTMLElement>(FOCUSABLE)]
  if (items.length === 0) return
  const first = items[0]
  const last = items[items.length - 1]
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (open) {
      opener = document.activeElement as HTMLElement | null
      document.body.style.overflow = 'hidden'
      await nextTick()
      // The safest choice first: the footer lists the way out (cancel) before the action,
      // so focus starts on its first button, else on the dialog itself.
      const first = dialog.value?.querySelector<HTMLElement>('.ui-modal__foot button:not(:disabled)')
      ;(first ?? dialog.value)?.focus()
    } else {
      document.body.style.overflow = ''
      opener?.focus?.()
      opener = null
    }
  },
)
onBeforeUnmount(() => {
  document.body.style.overflow = ''
})
</script>

<template>
  <Teleport to="body">
    <Transition name="ui-modal">
      <div v-if="modelValue" class="ui-modal" @keydown="onKey">
        <div class="ui-modal__backdrop" data-testid="modal-backdrop" @click="!persistent && close()" />
        <div ref="dialog" class="ui-modal__dialog" :role="alert ? 'alertdialog' : 'dialog'" aria-modal="true" :aria-labelledby="titleId" tabindex="-1">
          <header class="ui-modal__head">
            <h2 :id="titleId" class="ui-modal__title">{{ title }}</h2>
            <button v-if="!persistent" type="button" class="ui-modal__x" aria-label="Kapat" @click="close"><X :size="16" aria-hidden="true" /></button>
          </header>
          <div class="ui-modal__body">
            <p v-if="description" class="ui-modal__desc">{{ description }}</p>
            <slot />
          </div>
          <footer v-if="$slots.footer" class="ui-modal__foot"><slot name="footer" /></footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.ui-modal {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: var(--s-4);
}
.ui-modal__backdrop {
  position: absolute;
  inset: 0;
  background: rgba(14, 16, 21, 0.45);
}
.ui-modal__dialog {
  position: relative;
  width: min(480px, 100%);
  border-radius: var(--r-md);
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--fg);
  box-shadow: var(--shadow-pop);
  outline: none;
}
.ui-modal__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--s-3);
  padding: var(--s-5) var(--s-6) 0;
}
.ui-modal__title {
  margin: 0;
  font-size: var(--text-md);
}
.ui-modal__x {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--muted);
}
.ui-modal__body {
  padding: var(--s-3) var(--s-6) var(--s-5);
  color: var(--muted);
}
.ui-modal__desc {
  margin: 0;
}
.ui-modal__foot {
  display: flex;
  justify-content: space-between;
  gap: var(--s-2);
  padding: var(--s-3) var(--s-6);
  border-top: 1px solid var(--border);
  background: var(--surface-2);
  border-radius: 0 0 var(--r-md) var(--r-md);
}
.ui-modal-enter-active,
.ui-modal-leave-active {
  transition: opacity var(--t-fast) var(--ease);
}
.ui-modal-enter-active .ui-modal__dialog {
  transition: transform var(--t-med) var(--ease);
}
.ui-modal-enter-from,
.ui-modal-leave-to {
  opacity: 0;
}
.ui-modal-enter-from .ui-modal__dialog {
  transform: translateY(8px);
}
</style>
