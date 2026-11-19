<script setup lang="ts">
import { Pencil } from 'lucide-vue-next'
import Button from './ui/Button.vue'

// The text is read-only until "Düzenle" is pressed. After approval the button stays in
// place and in the tab order but does nothing ("Onaylandı"): aria-disabled instead of
// disabled, so a screen reader still reaches it and says why.
const props = defineProps<{
  /** What the button does, read by screen readers ("Türkçe metni düzenle"); it contains the visible "Düzenle". */
  label: string
  /** The campaign is approved: the text can no longer change. */
  locked?: boolean
  /** Something else is running (a save, a translation): try again in a moment. */
  disabled?: boolean
}>()
const emit = defineEmits<{ edit: [] }>()

function press() {
  if (!props.locked && !props.disabled) emit('edit')
}
</script>

<template>
  <Button
    variant="outline"
    size="sm"
    :class="{ 'pencil--locked': locked }"
    :icon="Pencil"
    :disabled="disabled && !locked"
    :aria-disabled="locked ? 'true' : undefined"
    :aria-label="locked ? `${label}: Onaylandı` : label"
    :title="locked ? 'Onaylandı' : label"
    data-testid="edit"
    @click="press"
    >Düzenle</Button
  >
</template>

<style scoped>
.pencil--locked {
  border-color: var(--border-strong);
  color: var(--muted);
  cursor: not-allowed;
  opacity: 0.6;
}
</style>
