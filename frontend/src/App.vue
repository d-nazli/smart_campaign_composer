<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchInfo, llmLabel, type AppInfo } from './api/info'
import { pageFromHash, type Page } from './utils/dashboard'
import ComposeView from './views/ComposeView.vue'
import DashboardView from './views/DashboardView.vue'

const info = ref<AppInfo | null>(null)
const error = ref<string | null>(null)
const page = ref<Page>(pageFromHash(window.location.hash))

const badge = computed(() => (info.value ? llmLabel(info.value) : null))

const onHash = () => (page.value = pageFromHash(window.location.hash))

onMounted(async () => {
  window.addEventListener('hashchange', onHash)
  try {
    info.value = await fetchInfo()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
})
onBeforeUnmount(() => window.removeEventListener('hashchange', onHash))
</script>

<template>
  <header class="topbar">
    <h1>Smart Campaign Composer</h1>
    <nav class="tabs" aria-label="Sayfalar">
      <a href="#/" :aria-current="page === 'compose' ? 'page' : undefined" data-testid="tab-compose">Yeni kampanya</a>
      <a href="#/dashboard" :aria-current="page === 'dashboard' ? 'page' : undefined" data-testid="tab-dashboard">Dashboard</a>
    </nav>
    <span v-if="badge" class="badge" :class="info?.llm_mode" data-testid="llm-badge">{{ badge }}</span>
    <span v-else-if="error" class="badge error">API ulaşılamıyor</span>
  </header>
  <main class="content" :class="{ wide: page === 'dashboard' }">
    <DashboardView v-if="page === 'dashboard'" />
    <ComposeView v-else :info="info" />
  </main>
</template>
