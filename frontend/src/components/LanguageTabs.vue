<script setup lang="ts">
import type { LanguageCode, LanguageInfo } from '../types'

export interface TabStatus {
  /** Problems found in this language's text (limits, unknown variables). */
  issues: number
  /** The model is writing this language right now. */
  writing: boolean
  /** The languages disagree about something and this one is named. */
  mismatch: boolean
  /** This language's text has unsaved edits. */
  edited: boolean
}

defineProps<{
  languages: LanguageInfo[]
  modelValue: LanguageCode
  status: Record<LanguageCode, TabStatus>
}>()
const emit = defineEmits<{ 'update:modelValue': [LanguageCode] }>()
</script>

<template>
  <div class="tabs" role="tablist" aria-label="Mesaj dili">
    <button
      v-for="l in languages"
      :key="l.code"
      type="button"
      role="tab"
      class="tab"
      :class="{ active: l.code === modelValue }"
      :aria-selected="l.code === modelValue"
      :data-testid="'lang-' + l.code"
      @click="emit('update:modelValue', l.code)"
    >
      <span class="code">{{ l.code.toUpperCase() }}</span>
      <span class="name">{{ l.display }}</span>
      <span v-if="status[l.code]?.writing" class="dot writing" title="Yazılıyor" aria-label="yazılıyor" />
      <span v-else-if="status[l.code]?.issues" class="dot bad" :title="status[l.code].issues + ' sorun'" :aria-label="status[l.code].issues + ' sorun'" />
      <span v-else-if="status[l.code]?.mismatch" class="dot bad" title="Diğer dillerle uyuşmuyor" aria-label="diğer dillerle uyuşmuyor" />
      <span v-else-if="status[l.code]?.edited" class="dot edited" title="Kaydedilmemiş düzenleme" aria-label="kaydedilmemiş düzenleme" />
    </button>
  </div>
</template>

<style scoped>
/* Underlined tabs: the active language is blue with a blue line under it. */
.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-5);
  margin: 0 0 var(--s-2);
  border-bottom: 1px solid var(--border);
}
.tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 44px;
  margin-bottom: -1px;
  padding: 0 2px;
  border: 0;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  background: none;
  color: var(--muted);
  font: inherit;
  cursor: pointer;
  transition: color var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease);
}
.tab:hover:not(.active) {
  background: none;
  color: var(--fg);
}
.tab.active {
  border-bottom-color: var(--accent);
  color: var(--accent);
}
.code {
  font-weight: 700;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.dot.writing {
  background: var(--ai-chip-fg);
  animation: blink 1s ease-in-out infinite;
}
.dot.bad {
  background: var(--error);
}
.dot.edited {
  background: var(--email);
}
@keyframes blink {
  50% {
    opacity: 0.25;
  }
}
@media (prefers-reduced-motion: reduce) {
  .dot.writing {
    animation: none;
  }
}
</style>
