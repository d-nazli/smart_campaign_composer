// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import DateTimeField from './DateTimeField.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

const base = { label: 'Gönderim zamanı', min: '2026-11-16T09:00', max: '2026-11-20T23:59', modelValue: '2026-11-18T10:00' }
const make = (props: Record<string, unknown> = {}) => mount(DateTimeField, { props: { ...base, ...props }, attachTo: document.body })
const openIt = async (w: VueWrapper) => {
  await w.get('.ui-dt__trigger').trigger('click')
}
const days = (w: VueWrapper) => w.findAll('.ui-dt__day')
const dayButton = (w: VueWrapper, n: number) => days(w).find((d) => !d.classes().includes('is-out') && d.text() === String(n))!

describe('DateTimeField trigger', () => {
  it('shows the chosen moment as dd.mm.yyyy hh:mm', () => {
    const w = make()
    expect(w.get('.ui-dt__text').text()).toBe('18.11.2026 10:00')
    expect(w.get('.ui-dt__trigger').attributes('aria-label')).toBe('Gönderim zamanı: 18.11.2026 10:00')
  })

  it('shows the placeholder when nothing is chosen', () => {
    const w = make({ modelValue: '' })
    expect(w.get('.ui-dt__text').text()).toBe('Tarih ve saat seçin')
    expect(w.get('.ui-dt__trigger').classes()).toContain('is-empty')
  })

  it('looks invalid and says so', () => {
    const w = make({ invalid: true })
    expect(w.get('.ui-dt__trigger').classes()).toContain('is-invalid')
    expect(w.get('.ui-dt__trigger').attributes('aria-invalid')).toBe('true')
  })

  it('is a button that opens a dialog, not a browser date input', () => {
    const w = make()
    expect(w.find('input').exists()).toBe(false)
    expect(w.get('.ui-dt__trigger').attributes('aria-haspopup')).toBe('dialog')
  })
})

describe('DateTimeField calendar', () => {
  it('opens on the month of the chosen moment, with six weeks', async () => {
    const w = make()
    expect(w.find('.ui-dt__pop').exists()).toBe(false)
    await openIt(w)
    expect(w.get('.ui-dt__month').text()).toBe('Kasım 2026')
    expect(days(w)).toHaveLength(42)
    expect(w.get('.ui-dt__trigger').attributes('aria-expanded')).toBe('true')
  })

  it('marks the chosen day', async () => {
    const w = make()
    await openIt(w)
    const on = days(w).filter((d) => d.attributes('aria-pressed') === 'true')
    expect(on).toHaveLength(1)
    expect(on[0].text()).toBe('18')
  })

  it('lets only the days inside the range be chosen', async () => {
    const w = make()
    await openIt(w)
    const enabled = days(w).filter((d) => d.attributes('disabled') === undefined).map((d) => d.text())
    expect(enabled).toEqual(['16', '17', '18', '19', '20'])
  })

  it('keeps the time when a day is chosen', async () => {
    const w = make()
    await openIt(w)
    await dayButton(w, 19).trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([['2026-11-19T10:00']])
  })

  it('ignores a day outside the range', async () => {
    const w = make()
    await openIt(w)
    const outside = days(w).find((d) => d.attributes('disabled') !== undefined && !d.classes().includes('is-out'))!
    await outside.trigger('click')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('does not go to months with nothing to choose', async () => {
    const w = make()
    await openIt(w)
    expect(w.get('[aria-label="Önceki ay"]').attributes('disabled')).toBeDefined()
    expect(w.get('[aria-label="Sonraki ay"]').attributes('disabled')).toBeDefined()
  })

  it('moves between months when the range allows', async () => {
    const w = make({ min: '2026-10-01T00:00', max: '2027-01-31T23:59' })
    await openIt(w)
    await w.get('[aria-label="Sonraki ay"]').trigger('click')
    expect(w.get('.ui-dt__month').text()).toBe('Aralık 2026')
    await w.get('[aria-label="Önceki ay"]').trigger('click')
    await w.get('[aria-label="Önceki ay"]').trigger('click')
    expect(w.get('.ui-dt__month').text()).toBe('Ekim 2026')
  })

  it('pulls the time inside the range when a boundary day is chosen', async () => {
    const w = make({ modelValue: '2026-11-18T07:00', min: '2026-11-16T14:30' })
    await openIt(w)
    await dayButton(w, 16).trigger('click')
    // 07:00 on the 16th is before the earliest allowed moment (14:30)
    expect(w.emitted('update:modelValue')).toEqual([['2026-11-16T14:30']])
  })
})

describe('DateTimeField time', () => {
  it('chooses an hour and a minute', async () => {
    const w = make()
    await openIt(w)
    const hours = w.get('[role="listbox"][aria-label="Saat"]').findAll('[role="option"]')
    const minutes = w.get('[role="listbox"][aria-label="Dakika"]').findAll('[role="option"]')
    expect(hours).toHaveLength(24)
    await hours[15].trigger('click')
    await minutes[3].trigger('click') // 15
    expect(w.emitted('update:modelValue')).toEqual([['2026-11-18T15:00'], ['2026-11-18T10:15']])
  })

  it('marks the current hour and minute', async () => {
    const w = make({ modelValue: '2026-11-18T10:35' })
    await openIt(w)
    expect(w.findAll('.ui-dt__cell.is-on').map((c) => c.text())).toEqual(['10', '35'])
  })

  it('turns off the hours before the earliest moment on the first day', async () => {
    const w = make({ modelValue: '2026-11-16T10:00', min: '2026-11-16T09:30' })
    await openIt(w)
    const hours = w.get('[role="listbox"][aria-label="Saat"]').findAll('[role="option"]')
    expect(hours[8].attributes('disabled')).toBeDefined() // 08:xx is entirely before 09:30
    expect(hours[9].attributes('disabled')).toBeUndefined() // 09:30 to 09:59 is still possible
    const minutes = w.get('[role="listbox"][aria-label="Dakika"]').findAll('[role="option"]')
    expect(minutes[0].attributes('disabled')).toBeUndefined() // 10:00 is fine
  })
})

describe('DateTimeField shortcuts and closing', () => {
  const shortcuts = [
    { label: '1 saat sonra', value: '2026-11-16T11:00' },
    { label: 'Çok erken', value: '2026-11-15T08:00' },
  ]

  it('applies a shortcut and closes', async () => {
    const w = make({ shortcuts })
    await openIt(w)
    await w.findAll('.ui-dt__chip')[0].trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([['2026-11-16T11:00']])
    expect(w.find('.ui-dt__pop').exists()).toBe(false)
  })

  it('turns off a shortcut that is outside the range', async () => {
    const w = make({ shortcuts })
    await openIt(w)
    const chips = w.findAll('.ui-dt__chip')
    expect(chips[0].attributes('disabled')).toBeUndefined()
    expect(chips[1].attributes('disabled')).toBeDefined()
  })

  it('closes with Escape and with "Tamam"', async () => {
    const w = make()
    await openIt(w)
    await w.get('.ui-dt').trigger('keydown', { key: 'Escape' })
    expect(w.find('.ui-dt__pop').exists()).toBe(false)
    await openIt(w)
    await w.get('.ui-dt__foot button.ui-btn').trigger('click')
    expect(w.find('.ui-dt__pop').exists()).toBe(false)
  })

  it('closes when the click lands outside', async () => {
    const w = make()
    await openIt(w)
    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await w.vm.$nextTick()
    expect(w.find('.ui-dt__pop').exists()).toBe(false)
  })

  it('needs a chosen moment before "Tamam" can be pressed', async () => {
    const w = make({ modelValue: '' })
    await openIt(w)
    expect(w.get('.ui-dt__foot button.ui-btn').attributes('disabled')).toBeDefined()
  })
})
