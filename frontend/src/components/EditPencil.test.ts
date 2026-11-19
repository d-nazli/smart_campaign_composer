// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EditPencil from './EditPencil.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

const make = (props: Record<string, unknown> = {}) => mount(EditPencil, { props: { label: 'Türkçe metni düzenle', ...props }, attachTo: document.body })

describe('the pencil', () => {
  it('is a real button with a name, reachable by keyboard', () => {
    const b = make().get('button')
    expect(b.text()).toBe('Düzenle')
    expect(b.attributes('aria-label')).toBe('Türkçe metni düzenle')
    expect(b.attributes('aria-label')!.toLowerCase()).toContain(b.text().toLowerCase()) // the visible word is in the name
    expect(b.attributes('type')).toBe('button')
    expect(b.attributes('disabled')).toBeUndefined()
    expect(b.attributes('tabindex')).not.toBe('-1')
    expect(b.find('svg').attributes('aria-hidden')).toBe('true')
  })

  it('asks to edit when pressed', async () => {
    const w = make()
    await w.get('button').trigger('click')
    expect(w.emitted('edit')).toHaveLength(1)
  })

  it('waits while something else runs', async () => {
    const w = make({ disabled: true })
    expect(w.get('button').attributes('disabled')).toBeDefined()
    await w.get('button').trigger('click')
    expect(w.emitted('edit')).toBeUndefined()
  })

  it('after approval stays focusable, says "Onaylandı" and does nothing', async () => {
    const w = make({ locked: true, disabled: true })
    const b = w.get('button')
    expect(b.attributes('disabled')).toBeUndefined()
    expect(b.attributes('aria-disabled')).toBe('true')
    expect(b.attributes('title')).toBe('Onaylandı')
    expect(b.attributes('aria-label')).toBe('Türkçe metni düzenle: Onaylandı')
    await b.trigger('click')
    expect(w.emitted('edit')).toBeUndefined()
  })
})
