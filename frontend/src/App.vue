<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ChevronRight, Home, LayoutList, Menu, PenSquare, X } from 'lucide-vue-next'
import { fetchInfo, llmLabel, type AppInfo } from './api/info'
import { fetchSystem, type SystemHealth } from './api/system'
import { allHealthy, downItems, healthItems, healthTitle } from './utils/health'
import { pageFromHash, type Page } from './utils/dashboard'
import { urlWithoutCampaign } from './utils/route'
import Badge from './components/ui/Badge.vue'
import Button from './components/ui/Button.vue'
import Modal from './components/ui/Modal.vue'
import ComposeView from './views/ComposeView.vue'
import DashboardView from './views/DashboardView.vue'

const info = ref<AppInfo | null>(null)
const error = ref<string | null>(null)
const page = ref<Page>(pageFromHash(window.location.hash))
const menuOpen = ref(false)

const isMock = computed(() => info.value?.llm_mode === 'mock')
// In Claude mode a quiet badge names the model; in mock mode the badge says so plainly.
const badge = computed(() => (info.value ? (isMock.value ? 'Mock modu' : llmLabel(info.value)) : null))

// System health: asked every 5 s. All well → a small "Sistem sağlıklı" badge (services in the
// tooltip); a service down → a red badge here and a red alert on the campaign list.
const HEALTH_MS = 5000
const system = ref<SystemHealth | null>(null)
const unreachable = ref(false)
const health = computed(() => healthItems(system.value, unreachable.value))
const healthy = computed(() => allHealthy(health.value))
const down = computed(() => downItems(health.value))
const healthTip = computed(() => healthTitle(health.value, info.value ? llmLabel(info.value) : null))
let healthTimer: number | null = null
async function refreshHealth() {
  try {
    system.value = await fetchSystem()
    unreachable.value = false
  } catch {
    unreachable.value = true
  }
}

// ComposeView is rebuilt (new key) every time a NEW campaign starts, so nothing of the old one survives.
const composeKey = ref(0)

// "Yeni kampanya": a clean brief. The address loses ?c= first, otherwise the page would open the old campaign again.
function newCampaign() {
  menuOpen.value = false
  window.history.pushState(null, '', urlWithoutCampaign(window.location.href, '#/'))
  page.value = 'compose'
  composeKey.value++
}

// Leaving a campaign with an open edit or an unsaved text asks first: the in-page links
// ("Yeni", "Kampanyalar" in the rail, the breadcrumb, the step bar and the mobile menu)
// wait for the answer, and closing or reloading the tab gets the browser's own warning.
const composeDirty = ref(false)
const leaving = ref<(() => void) | null>(null)
const mustAsk = () => page.value === 'compose' && composeDirty.value

function onLinkClick(e: MouseEvent) {
  if (!mustAsk() || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  const link = (e.target as Element | null)?.closest?.('a[href^="#/"]')
  if (!link) return
  const href = link.getAttribute('href') ?? ''
  // Capture phase: the link's own handler ("Yeni") and the navigation wait for the answer.
  e.preventDefault()
  e.stopPropagation()
  leaving.value = href === '#/' ? newCampaign : () => (window.location.hash = href)
}
function leave() {
  const go = leaving.value
  leaving.value = null
  composeDirty.value = false
  go?.()
}
function onBeforeUnload(e: BeforeUnloadEvent) {
  if (!mustAsk()) return
  e.preventDefault()
  e.returnValue = ''
}

const onHash = () => {
  menuOpen.value = false
  const next = pageFromHash(window.location.hash)
  // Coming back from the dashboard through a "#/" link also means a new campaign.
  if (next === 'compose' && page.value === 'dashboard') return newCampaign()
  page.value = next
}

onMounted(async () => {
  window.addEventListener('hashchange', onHash)
  document.addEventListener('click', onLinkClick, true)
  window.addEventListener('beforeunload', onBeforeUnload)
  void refreshHealth()
  healthTimer = window.setInterval(refreshHealth, HEALTH_MS)
  try {
    info.value = await fetchInfo()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('hashchange', onHash)
  document.removeEventListener('click', onLinkClick, true)
  window.removeEventListener('beforeunload', onBeforeUnload)
  if (healthTimer !== null) window.clearInterval(healthTimer)
})
</script>

<template>
  <div class="shell">
    <nav class="rail" aria-label="Sayfalar">
      <span class="rail-mark" aria-hidden="true">CC</span>
      <a href="#/dashboard" :aria-current="page === 'dashboard' ? 'page' : undefined" data-testid="tab-dashboard">
        <LayoutList :size="20" aria-hidden="true" /><span>Kampanyalar</span>
      </a>
      <a href="#/" :aria-current="page === 'compose' ? 'page' : undefined" data-testid="tab-compose" @click.prevent="newCampaign">
        <PenSquare :size="20" aria-hidden="true" /><span>Yeni</span>
      </a>
    </nav>

    <div class="main-col">
      <header class="topbar">
        <button
          type="button"
          class="burger"
          :aria-expanded="menuOpen"
          aria-controls="mobile-menu"
          :aria-label="menuOpen ? 'Menüyü kapat' : 'Menüyü aç'"
          @click="menuOpen = !menuOpen"
        >
          <X v-if="menuOpen" :size="22" aria-hidden="true" /><Menu v-else :size="22" aria-hidden="true" />
        </button>
        <ol class="crumbs" aria-label="Konum">
          <li><Home :size="15" aria-hidden="true" /><span class="sr-only">Ana sayfa</span></li>
          <li>
            <ChevronRight :size="14" aria-hidden="true" />
            <a v-if="page === 'compose'" href="#/dashboard">Kampanyalar</a>
            <template v-else>Kampanyalar</template>
          </li>
          <li v-if="page === 'compose'"><ChevronRight :size="14" aria-hidden="true" />Kampanya</li>
        </ol>
        <span class="product">Campaign Composer</span>
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
          <Badge v-if="healthy" tone="ok" dot class="health" :title="healthTip" :aria-label="'Sistem sağlıklı. ' + healthTip" data-testid="system-badge">Sistem sağlıklı</Badge>
          <Badge v-else-if="down.length" tone="danger" dot class="health" :title="healthTip" :aria-label="'Sistem sorunu. ' + healthTip" data-testid="system-badge">Sistem sorunu</Badge>
        </div>
      </header>

      <nav v-if="menuOpen" id="mobile-menu" class="mobile-menu" aria-label="Sayfalar (mobil)">
        <a href="#/dashboard" :aria-current="page === 'dashboard' ? 'page' : undefined" @click="menuOpen = false"><LayoutList :size="18" aria-hidden="true" />Kampanyalar</a>
        <a href="#/" :aria-current="page === 'compose' ? 'page' : undefined" @click.prevent="newCampaign"><PenSquare :size="18" aria-hidden="true" />Yeni kampanya</a>
      </nav>

      <main class="container content">
        <DashboardView v-if="page === 'dashboard'" :info="info" :down="down" />
        <ComposeView v-else :key="composeKey" :info="info" @dirty="(d: boolean) => (composeDirty = d)" />
      </main>
    </div>

    <Modal :model-value="leaving !== null" alert title="Kaydedilmemiş değişiklikler kaybolacak." @update:model-value="(v: boolean) => !v && (leaving = null)">
      <template #footer>
        <Button variant="ghost" data-testid="leave-cancel" @click="leaving = null">Vazgeç</Button>
        <Button variant="danger" data-testid="leave-confirm" @click="leave">Çık</Button>
      </template>
    </Modal>
  </div>
</template>
