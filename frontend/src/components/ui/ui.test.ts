// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import Badge from './Badge.vue'
import Button from './Button.vue'
import Card from './Card.vue'
import Modal from './Modal.vue'
import ProgressBar from './ProgressBar.vue'
import SegmentedControl from './SegmentedControl.vue'
import StatTile from './StatTile.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('Button', () => {
  it('shows its text, emits clicks and is a plain button by default', async () => {
    const w = mount(Button, { slots: { default: 'Kaydet' } })
    expect(w.text()).toBe('Kaydet')
    expect(w.attributes('type')).toBe('button')
    await w.trigger('click')
    expect(w.emitted('click')).toHaveLength(1)
  })

  it('takes its look from the variant and size', () => {
    const w = mount(Button, { props: { variant: 'primary', size: 'sm' } })
    expect(w.classes()).toEqual(expect.arrayContaining(['ui-btn--primary', 'ui-btn--sm']))
    expect(mount(Button).classes()).toContain('ui-btn--secondary')
  })

  it('cannot be clicked while loading, and says it is busy', async () => {
    const w = mount(Button, { props: { loading: true }, slots: { default: 'Gönder' } })
    expect(w.attributes('disabled')).toBeDefined()
    expect(w.attributes('aria-busy')).toBe('true')
    await w.trigger('click')
    expect(w.emitted('click')).toBeUndefined()
  })

  it('passes other attributes (test ids, labels) to the button', () => {
    const w = mount(Button, { attrs: { 'data-testid': 'go', 'aria-label': 'Git' } })
    expect(w.attributes('data-testid')).toBe('go')
    expect(w.attributes('aria-label')).toBe('Git')
  })
})

describe('Badge', () => {
  it('has a tone and an optional dot', () => {
    const w = mount(Badge, { props: { tone: 'ok', dot: true }, slots: { default: 'Tamamlandı' } })
    expect(w.text()).toBe('Tamamlandı')
    expect(w.classes()).toContain('ui-badge--ok')
    expect(w.find('.ui-badge__dot').exists()).toBe(true)
    expect(mount(Badge).classes()).toContain('ui-badge--neutral')
    expect(mount(Badge).find('.ui-badge__dot').exists()).toBe(false)
  })
})

describe('Card', () => {
  it('has a title and actions in its header, and a body', () => {
    const w = mount(Card, { props: { title: 'Onay' }, slots: { default: '<p id="b">içerik</p>', actions: '<span id="a">x</span>' } })
    expect(w.find('h2').text()).toBe('Onay')
    expect(w.find('#a').exists()).toBe(true)
    expect(w.find('.ui-card__body #b').exists()).toBe(true)
  })

  it('has no header when it has neither title nor actions', () => {
    expect(mount(Card, { slots: { default: 'x' } }).find('header').exists()).toBe(false)
  })

  it('can drop the body padding (for a table)', () => {
    expect(mount(Card, { props: { flush: true } }).find('.ui-card__body--flush').exists()).toBe(true)
  })
})

describe('StatTile', () => {
  it('shows a number and a label; what it means is a tooltip, not a sentence', () => {
    const w = mount(StatTile, { props: { label: 'Gönderilen', value: 1234, hint: 'SMS ve e-posta' } })
    expect(w.text()).toContain('Gönderilen')
    expect(w.find('.ui-stat__value').text()).toContain('1.234') // Turkish thousands separator
    expect(w.text()).not.toContain('SMS ve e-posta')
    expect(w.get('.ui-stat__q').attributes('title')).toBe('SMS ve e-posta')
  })

  it('shows a text value as it is, and a suffix', () => {
    const w = mount(StatTile, { props: { label: 'Hız', value: '12,5', suffix: '/sn' } })
    expect(w.find('.ui-stat__value').text()).toBe('12,5/sn')
  })

  it('takes a colour from its tone', () => {
    expect(mount(StatTile, { props: { label: 'x', value: 1, tone: 'danger' } }).classes()).toContain('ui-stat--danger')
  })
})

describe('ProgressBar', () => {
  it('reports its percentage to assistive technology', () => {
    const w = mount(ProgressBar, { props: { value: 25, max: 100, label: 'İlerleme' } })
    expect(w.attributes('role')).toBe('progressbar')
    expect(w.attributes('aria-label')).toBe('İlerleme')
    expect(w.attributes('aria-valuenow')).toBe('25')
  })

  it('adds up its parts against the maximum', () => {
    const w = mount(ProgressBar, {
      props: { max: 200, label: 'x', segments: [{ value: 100, tone: 'ok' }, { value: 50, tone: 'danger' }] },
    })
    expect(w.attributes('aria-valuenow')).toBe('75')
    const parts = w.findAll('.ui-progress__part')
    expect(parts.map((p) => (p.element as HTMLElement).style.width)).toEqual(['50%', '25%'])
    expect(parts[1].classes()).toContain('ui-progress__part--danger')
  })

  it('never shows more than full, and reads as complete when there is nothing to count', () => {
    expect(mount(ProgressBar, { props: { value: 500, max: 100, label: 'x' } }).attributes('aria-valuenow')).toBe('100')
    expect(mount(ProgressBar, { props: { value: 0, max: 0, label: 'x' } }).attributes('aria-valuenow')).toBe('100')
  })
})

describe('SegmentedControl', () => {
  const options = [
    { value: 'now', label: 'Hemen', testid: 'now' },
    { value: 'later', label: 'Zamanla', testid: 'later' },
  ]
  const make = (value = 'now', extra = {}) => mount(SegmentedControl, { props: { modelValue: value, options, label: 'Gönderim', ...extra }, attachTo: document.body })

  it('is a radio group with the chosen option checked', () => {
    const w = make('later')
    expect(w.attributes('role')).toBe('radiogroup')
    expect(w.attributes('aria-label')).toBe('Gönderim')
    const radios = w.findAll('[role="radio"]')
    expect(radios.map((r) => r.attributes('aria-checked'))).toEqual(['false', 'true'])
    // only the chosen one is in the tab order
    expect(radios.map((r) => r.attributes('tabindex'))).toEqual(['-1', '0'])
  })

  it('emits the clicked option', async () => {
    const w = make('now')
    await w.get('[data-testid="later"]').trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([['later']])
  })

  it('moves with the arrow keys, wrapping around, and with Home and End', async () => {
    const w = make('now')
    await w.trigger('keydown', { key: 'ArrowRight' })
    await w.trigger('keydown', { key: 'ArrowLeft' }) // from "now" back wraps to "later"
    await w.trigger('keydown', { key: 'End' })
    await w.trigger('keydown', { key: 'Home' })
    expect(w.emitted('update:modelValue')).toEqual([['later'], ['later'], ['later'], ['now']])
  })

  it('does nothing when disabled', async () => {
    const w = make('now', { disabled: true })
    await w.get('[data-testid="later"]').trigger('click')
    await w.trigger('keydown', { key: 'ArrowRight' })
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })
})

describe('Modal', () => {
  const footer = '<button id="no">Vazgeç</button><button id="yes">Sil</button>'
  const make = (props = {}) =>
    mount(Modal, { props: { modelValue: true, title: 'Silinsin mi?', description: 'Geri alınamaz.', ...props }, slots: { footer }, attachTo: document.body })
  const dialog = () => document.body.querySelector('[role="dialog"]') as HTMLElement | null

  it('is an alert dialog when it interrupts with a question ("Emin misiniz?")', () => {
    const w = make({ alert: true, title: 'Emin misiniz?' })
    const d = document.body.querySelector('[role="alertdialog"]') as HTMLElement
    expect(d).not.toBeNull()
    expect(dialog()).toBeNull()
    expect(document.getElementById(d.getAttribute('aria-labelledby')!)!.textContent).toBe('Emin misiniz?')
    w.unmount()
  })

  it('is a labelled modal dialog', () => {
    const w = make()
    const d = dialog()!
    expect(d).not.toBeNull()
    expect(d.getAttribute('aria-modal')).toBe('true')
    expect(document.getElementById(d.getAttribute('aria-labelledby')!)!.textContent).toBe('Silinsin mi?')
    expect(d.textContent).toContain('Geri alınamaz.')
    w.unmount()
  })

  it('renders nothing while closed', () => {
    const w = make({ modelValue: false })
    expect(dialog()).toBeNull()
    w.unmount()
  })

  it('closes with Escape, with the close button and with a click on the backdrop', async () => {
    const w = make()
    dialog()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    ;(document.body.querySelector('.ui-modal__x') as HTMLElement).click()
    ;(document.body.querySelector('[data-testid="modal-backdrop"]') as HTMLElement).click()
    expect(w.emitted('update:modelValue')).toEqual([[false], [false], [false]])
    w.unmount()
  })

  it('cannot be dismissed by accident when persistent', () => {
    const w = make({ persistent: true })
    dialog()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    ;(document.body.querySelector('[data-testid="modal-backdrop"]') as HTMLElement).click()
    expect(w.emitted('update:modelValue')).toBeUndefined()
    expect(document.body.querySelector('.ui-modal__x')).toBeNull()
    w.unmount()
  })

  it('puts the focus on the first footer button (the way out) when it opens', async () => {
    const w = make({ modelValue: false })
    await w.setProps({ modelValue: true })
    await nextTick()
    await nextTick()
    expect(document.activeElement?.id).toBe('no')
    w.unmount()
  })

  it('keeps Tab inside the dialog', async () => {
    const w = make()
    const last = document.getElementById('yes')!
    last.focus()
    dialog()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(document.body.querySelector('.ui-modal__x'))
    // and backwards from the first element to the last
    dialog()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(last)
    w.unmount()
  })

  it('locks the page scroll while open and releases it after', async () => {
    const w = make({ modelValue: false })
    await w.setProps({ modelValue: true })
    expect(document.body.style.overflow).toBe('hidden')
    await w.setProps({ modelValue: false })
    expect(document.body.style.overflow).toBe('')
    w.unmount()
  })
})
