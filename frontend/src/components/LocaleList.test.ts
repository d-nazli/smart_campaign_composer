// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LocaleList, { type LocaleStatus } from './LocaleList.vue'
import type { LocaleInfo } from '../types'

const L = (code: string, extra: Partial<LocaleInfo> = {}): LocaleInfo => ({ code, name: code, native_name: code, direction: 'ltr', is_source: false, ...extra })
const locales = [L('tr-TR', { is_source: true }), L('ar-SA', { direction: 'rtl' }), L('de-DE'), L('en-GB')]
const st = (state: LocaleStatus['state'], extra: Partial<LocaleStatus> = {}): LocaleStatus => ({ state, writing: false, issues: 0, unsaved: false, ...extra })
const status: Record<string, LocaleStatus> = {
  'tr-TR': st('source'),
  'ar-SA': st('stale'),
  'de-DE': st('missing'),
  'en-GB': st('ok', { writing: true }),
}

const make = (modelValue = 'ar-SA') =>
  mount(LocaleList, { props: { locales, modelValue, status, counts: { 'tr-TR': 1984, 'ar-SA': 201 } }, attachTo: document.body })

describe('LocaleList', () => {
  it('puts the source in its own group above the translations', () => {
    const w = make()
    expect(w.findAll('.group').map((g) => g.text())).toEqual(['Kaynak', 'Çeviriler · 3'])
    expect(w.findAll('[role="option"]').map((o) => o.find('.code').text())).toEqual(['tr-TR', 'ar-SA', 'de-DE', 'en-GB'])
    expect(w.get('[data-testid="locale-tr-TR"]').text()).toContain('1.984 kişi')
  })

  it('marks the chosen locale and shows each state as a dot with a name', () => {
    const w = make()
    expect(w.get('[data-testid="locale-ar-SA"]').attributes('aria-selected')).toBe('true')
    expect(w.get('[data-testid="locale-ar-SA"] .dot').classes()).toContain('stale')
    expect(w.get('[data-testid="locale-de-DE"] .dot').attributes('aria-label')).toBe('Çeviri yok')
    expect(w.get('[data-testid="locale-en-GB"] .dot').classes()).toContain('busy')
  })

  it('switches the locale from the list and from the picker, which then closes', async () => {
    const w = make()
    await w.get('[data-testid="locale-de-DE"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['de-DE'])

    const picker = w.get('[data-testid="locale-picker"]')
    expect(picker.text()).toContain('ar-SA')
    await picker.trigger('click')
    expect(picker.attributes('aria-expanded')).toBe('true')
    await w.get('[data-testid="locale-en-GB"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[1]).toEqual(['en-GB'])
    expect(picker.attributes('aria-expanded')).toBe('false')
    w.unmount()
  })
})
