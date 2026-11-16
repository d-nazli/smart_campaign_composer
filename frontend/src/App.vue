<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchInfo, llmLabel, type AppInfo } from './api/info'
import { pageFromHash, type Page } from './utils/dashboard'
import { urlWithoutCampaign } from './utils/route'
import Badge from './components/ui/Badge.vue'
import ComposeView from './views/ComposeView.vue'
import DashboardView from './views/DashboardView.vue'

const info = ref<AppInfo | null>(null)
const error = ref<string | null>(null)
const page = ref<Page>(pageFromHash(window.location.hash))

const isMock = computed(() => info.value?.llm_mode === 'mock')
// In Claude mode a quiet badge names the model; in mock mode the badge says so plainly.
const badge = computed(() => (info.value ? (isMock.value ? 'Mock modu' : llmLabel(info.value)) : null))

// ComposeView is rebuilt (new key) every time a NEW campaign starts, so nothing of the old one survives.
const composeKey = ref(0)

// "Yeni kampanya": a clean brief. The address loses ?c= first, otherwise the page would open the old campaign again.
function newCampaign() {
  window.history.pushState(null, '', urlWithoutCampaign(window.location.href, '#/'))
  page.value = 'compose'
  composeKey.value++
}

const onHash = () => {
  const next = pageFromHash(window.location.hash)
  // Coming back from the dashboard through a "#/" link also means a new campaign.
  if (next === 'compose' && page.value === 'dashboard') return newCampaign()
  page.value = next
}

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
    <div class="container topbar-in">
      <h1 class="brand">
        <svg class="brand-mark" viewBox="0 0 32 32" aria-hidden="true">
          <rect width="32" height="32" rx="8" fill="var(--accent)" />
          <path d="M8 21V11l8 6 8-6v10" fill="none" stroke="var(--accent-fg)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <span>Campaign Composer</span>
      </h1>
      <nav class="nav" aria-label="Sayfalar">
        <a href="#/" :aria-current="page === 'compose' ? 'page' : undefined" data-testid="tab-compose" @click.prevent="newCampaign">Yeni kampanya</a>
        <a href="#/dashboard" :aria-current="page === 'dashboard' ? 'page' : undefined" data-testid="tab-dashboard">Dashboard</a>
      </nav>
      <div class="topbar-end">
        <Badge
          v-if="badge"
          :tone="isMock ? 'warn' : 'ok'"
          dot
          :title="isMock ? 'Yanıtlar gerçek model yerine sahte (mock) modelden geliyor' : undefined"
          data-testid="llm-badge"
        >
          {{ badge }}
        </Badge>
        <Badge v-else-if="error" tone="danger" dot>API ulaşılamıyor</Badge>
      </div>
    </div>
  </header>
  <main class="container content">
    <DashboardView v-if="page === 'dashboard'" :info="info" />
    <ComposeView v-else :key="composeKey" :info="info" />
  </main>
</template>
