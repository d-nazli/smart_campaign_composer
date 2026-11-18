<script setup lang="ts">
// A bordered panel: the only container the screens use. The header (title and actions) is
// separated from the body by a line, so sections read as a ledger and not as cards in cards.
// The shadow is the panel's faint lift (a token, and none in dark mode).
defineProps<{ title?: string; flush?: boolean }>()
</script>

<template>
  <section class="ui-card">
    <header v-if="title || $slots.actions" class="ui-card__head">
      <h2 v-if="title" class="ui-card__title">{{ title }}</h2>
      <div v-if="$slots.actions" class="ui-card__actions"><slot name="actions" /></div>
    </header>
    <div class="ui-card__body" :class="{ 'ui-card__body--flush': flush }"><slot /></div>
    <footer v-if="$slots.footer" class="ui-card__foot"><slot name="footer" /></footer>
  </section>
</template>

<style scoped>
.ui-card {
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
  min-width: 0;
}
.ui-card__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
  min-height: 60px;
  padding: var(--s-3) var(--s-6);
  border-bottom: 1px solid var(--border);
}
.ui-card__title {
  margin: 0;
  font-size: var(--text-md);
}
.ui-card__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
}
.ui-card__body {
  padding: var(--s-6);
}
.ui-card__body--flush {
  padding: 0;
}
.ui-card__foot {
  padding: var(--s-3) var(--s-6);
  border-top: 1px solid var(--border);
  background: var(--surface-2);
  border-radius: 0 0 var(--r-md) var(--r-md);
}
@media (max-width: 767px) {
  .ui-card__head,
  .ui-card__body,
  .ui-card__foot {
    padding-inline: var(--s-4);
  }
}
</style>
