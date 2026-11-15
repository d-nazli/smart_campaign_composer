<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { fetchInfo, llmLabel, type AppInfo } from './api/info'

const info = ref<AppInfo | null>(null)
const error = ref<string | null>(null)

const badge = computed(() => (info.value ? llmLabel(info.value) : null))

onMounted(async () => {
  try {
    info.value = await fetchInfo()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
})
</script>

<template>
  <header class="topbar">
    <h1>Smart Campaign Composer</h1>
    <span v-if="badge" class="badge" :class="info?.llm_mode" data-testid="llm-badge">{{ badge }}</span>
    <span v-else-if="error" class="badge error">API ulaşılamıyor</span>
  </header>
  <main class="content">
    <p class="muted">Aşama 0: iskelet hazır. Kampanya akışı sonraki aşamalarda eklenecek.</p>
  </main>
</template>
