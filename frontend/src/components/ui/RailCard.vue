<script setup lang="ts">
import type { Component } from 'vue'

// A card for the right rail: a coloured top edge and an icon tile say what kind of
// note it is (info: blue, warn: amber, ok: green).
defineProps<{ tone: 'info' | 'warn' | 'ok'; title: string; icon?: Component }>()
</script>

<template>
  <section class="ui-rail" :class="`ui-rail--${tone}`">
    <header class="ui-rail__head">
      <span v-if="icon" class="ui-rail__tile" aria-hidden="true"><component :is="icon" :size="16" /></span>
      <h3 class="ui-rail__title">{{ title }}</h3>
    </header>
    <div class="ui-rail__body"><slot /></div>
  </section>
</template>

<style scoped>
.ui-rail {
  --tone: var(--accent);
  --tone-bg: var(--accent-soft);
  padding: var(--s-4) var(--s-5);
  border: 1px solid var(--border);
  border-top: 3px solid var(--tone);
  border-radius: var(--r-md);
  background: var(--surface);
  box-shadow: var(--shadow-card);
}
.ui-rail--warn {
  --tone: var(--email);
  --tone-bg: var(--warn-bg);
}
.ui-rail--ok {
  --tone: var(--ok);
  --tone-bg: var(--ok-bg);
}
.ui-rail__head {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin-bottom: var(--s-3);
}
.ui-rail__tile {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: var(--r-sm);
  background: var(--tone-bg);
  color: var(--tone);
}
.ui-rail__title {
  font-size: var(--text-base);
  color: var(--tone);
}
.ui-rail__body {
  font-size: var(--text-sm);
}
</style>
