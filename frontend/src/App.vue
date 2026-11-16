<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchInfo, llmLabel, type AppInfo } from './api/info'
import { pageFromHash, type Page } from './utils/dashboard'
import ComposeView from './views/ComposeView.vue'
import DashboardView from './views/DashboardView.vue'

const info = ref<AppInfo | null>(null)
const error = ref<string | null>(null)
const page = ref<Page>(pageFromHash(window.location.hash))

const isMock = computed(() => info.value?.llm_mode === 'mock')
// In Claude mode a quiet badge names the model; in mock mode the badge says so plainly.
const badge = computed(() => (info.value ? (isMock.value ? 'Mock modu' : llmLabel(info.value)) : null))

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
    <h1 class="brand">
      <svg class="brand-mark" viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="9" fill="var(--accent)" />
        <path d="M8 21V11l8 6 8-6v10" fill="none" stroke="var(--accent-fg)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <span>Smart Campaign Composer</span>
    </h1>
    <nav class="tabs" aria-label="Sayfalar">
      <a href="#/" :aria-current="page === 'compose' ? 'page' : undefined" data-testid="tab-compose">Yeni kampanya</a>
      <a href="#/dashboard" :aria-current="page === 'dashboard' ? 'page' : undefined" data-testid="tab-dashboard">Dashboard</a>
    </nav>
    <span
      v-if="badge"
      class="badge"
      :class="info?.llm_mode"
      :title="isMock ? 'Yanıtlar gerçek model yerine sahte (mock) modelden geliyor' : undefined"
      data-testid="llm-badge"
    >
      {{ badge }}
    </span>
    <span v-else-if="error" class="badge error">API ulaşılamıyor</span>
  </header>
  <main class="content" :class="{ wide: page === 'dashboard' }">
    <DashboardView v-if="page === 'dashboard'" :info="info" />
    <ComposeView v-else :info="info" />
  </main>
</template>
