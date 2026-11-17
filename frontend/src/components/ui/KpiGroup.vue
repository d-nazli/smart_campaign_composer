<script setup lang="ts">
// One white panel split into equal cells by hairlines: the row of figures at the top of a page.
// `count` cells per row on wide screens, at most two below 1024px, one on a phone.
defineProps<{ count: number }>()
</script>

<template>
  <div class="ui-kpis" :style="{ '--n': count, '--n2': Math.min(count, 2) }">
    <slot />
  </div>
</template>

<style scoped>
/* 1px gaps over a border-coloured background draw the dividers in every layout */
.ui-kpis {
  display: grid;
  grid-template-columns: repeat(var(--n), minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--border);
  box-shadow: var(--shadow-card);
}
@media (max-width: 1023px) {
  .ui-kpis {
    grid-template-columns: repeat(var(--n2), minmax(0, 1fr));
  }
  .ui-kpis > :deep(:last-child:nth-child(odd)) {
    grid-column: 1 / -1;
  }
}
@media (max-width: 767px) {
  .ui-kpis {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
