// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

vi.mock('./api/info', async (orig) => ({
  ...(await orig<typeof import('./api/info')>()),
  fetchInfo: () => Promise.resolve({ llm_mode: 'mock' }),
}))
vi.mock('./api/system', () => ({ fetchSystem: () => Promise.resolve({ api: 'ok', database: 'ok', worker: 'ok' }) }))

import App from './App.vue'

// The wizard stands in as a stub: it can say it has unsaved work, and it has the step
// bar's "← Kampanyalar" link. Every mount is a new campaign (App re-keys it).
const mounts = vi.fn()
const ComposeStub = defineComponent({
  emits: ['dirty'],
  setup(_, { emit }) {
    mounts()
    return () =>
      h('div', [
        h('a', { href: '#/dashboard', 'data-testid': 'step-back' }, 'Kampanyalar'),
        h('button', { 'data-testid': 'make-dirty', onClick: () => emit('dirty', true) }),
        h('button', { 'data-testid': 'make-clean', onClick: () => emit('dirty', false) }),
      ])
  },
})

let w: VueWrapper | null = null
const dialog = () => document.body.querySelector('[role="alertdialog"]')
const click = (sel: string) => (document.body.querySelector(sel) as HTMLElement).click()

beforeEach(async () => {
  mounts.mockReset()
  window.history.replaceState(null, '', '/#/')
  w = mount(App, { attachTo: document.body, global: { stubs: { ComposeView: ComposeStub, DashboardView: true } } })
  await flushPromises()
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
  window.history.replaceState(null, '', '/')
})

describe('leaving a campaign with unsaved work', () => {
  it('without unsaved work, "Yeni" starts a new campaign at once', async () => {
    click('[data-testid="tab-compose"]')
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(mounts).toHaveBeenCalledTimes(2)
  })

  it('"Yeni" asks first; Vazgeç keeps the campaign, Çık starts a new one', async () => {
    click('[data-testid="make-dirty"]')
    await flushPromises()
    click('[data-testid="tab-compose"]')
    await flushPromises()
    expect(dialog()?.textContent).toContain('Kaydedilmemiş değişiklikler kaybolacak.')
    expect(mounts).toHaveBeenCalledTimes(1)

    click('[data-testid="leave-cancel"]')
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(mounts).toHaveBeenCalledTimes(1)

    click('[data-testid="tab-compose"]')
    await flushPromises()
    click('[data-testid="leave-confirm"]')
    await flushPromises()
    expect(dialog()).toBeNull()
    expect(mounts).toHaveBeenCalledTimes(2)
  })

  it.each([
    ['the step bar\'s "← Kampanyalar"', '[data-testid="step-back"]'],
    ['the breadcrumb', 'ol.crumbs a'],
    ['the rail', '[data-testid="tab-dashboard"]'],
  ])('%s asks first, and leaves on Çık', async (_, sel) => {
    click('[data-testid="make-dirty"]')
    await flushPromises()
    click(sel)
    await flushPromises()
    expect(dialog()).not.toBeNull()
    expect(window.location.hash).toBe('#/')

    click('[data-testid="leave-confirm"]')
    await flushPromises()
    expect(window.location.hash).toBe('#/dashboard')
  })

  it('closing or reloading the tab gets the browser warning only with unsaved work', async () => {
    const unload = () => {
      const e = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(e)
      return e.defaultPrevented
    }
    expect(unload()).toBe(false)
    click('[data-testid="make-dirty"]')
    await flushPromises()
    expect(unload()).toBe(true)
    click('[data-testid="make-clean"]')
    await flushPromises()
    expect(unload()).toBe(false)
  })
})
