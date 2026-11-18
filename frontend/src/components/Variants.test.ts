// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Campaign, DeliveryProgress as Delivery, Segment, StoredContent } from '../types'
import { comparisonRows, forVariant, hasVariantB, pendingOf, splitShares, variantsOf } from '../utils/variants'
import ApprovalCard from './ApprovalCard.vue'
import DeliveryProgress from './DeliveryProgress.vue'
import VariantTabs from './VariantTabs.vue'

afterEach(() => {
  document.body.innerHTML = ''
})

const stored = (sms: string, o: Partial<StoredContent> = {}): StoredContent => ({
  sms_body: sms,
  email_subject: 's',
  email_body: 'b',
  locale: 'tr-TR',
  source: 'llm',
  version: 1,
  stale: false,
  report: { ok: true, issues: [], sms_length: sms.length, subject_length: 1 },
  updated_at: '',
  ...o,
})

const campaign: Campaign = {
  id: 1,
  brief: 'b',
  status: 'content_ready',
  source_locale: 'tr-TR',
  locales: ['tr-TR', 'en-GB'],
  content: stored('A metni'),
  translations: { 'en-GB': stored('A text', { locale: 'en-GB', source: 'translated' }) },
  pending_locales: [],
  mismatches: [],
  variants: ['A', 'B'],
  variant_b: {
    content: stored('B metni', { version: 2, source: 'edited' }),
    translations: { 'en-GB': stored('B text', { locale: 'en-GB', source: 'translated', stale: true }) },
    pending_locales: ['en-GB'],
    mismatches: [],
    duplicate: false,
  },
  variant_counts: { A: { people: 502, sms: 400, email: 450 }, B: { people: 498, sms: 401, email: 440 } },
  created_at: '',
}

describe('the campaign seen through a variant', () => {
  it('A is the campaign itself; B swaps in its texts and keeps everything else', () => {
    expect(forVariant(campaign, 'A')).toBe(campaign)
    const b = forVariant(campaign, 'B')!
    expect(b.content?.sms_body).toBe('B metni')
    expect(b.translations?.['en-GB'].sms_body).toBe('B text')
    expect(b.pending_locales).toEqual(['en-GB'])
    expect(b.status).toBe('content_ready')
    expect(b.locales).toEqual(['tr-TR', 'en-GB'])
  })
  it('each variant has its own pending translations', () => {
    expect(pendingOf(campaign, 'A')).toEqual([])
    expect(pendingOf(campaign, 'B')).toEqual(['en-GB'])
  })
  it('a campaign without B has only A', () => {
    const one = { ...campaign, variants: ['A' as const], variant_b: undefined }
    expect(variantsOf(one)).toEqual(['A'])
    expect(hasVariantB(one)).toBe(false)
    expect(variantsOf(null)).toEqual(['A'])
    expect(pendingOf(one, 'B')).toEqual([])
  })
  it('the split adds up to 100', () => {
    const s = splitShares(campaign.variant_counts)
    expect(s.map((x) => x.share)).toEqual([50, 50])
    expect(s[0].share + s[1].share).toBe(100)
    expect(splitShares(undefined)).toEqual([])
  })
})

describe('VariantTabs', () => {
  const status = { A: { dot: false, writing: false }, B: { dot: true, writing: false } }
  const make = (o: Record<string, unknown> = {}) =>
    mount(VariantTabs, { props: { modelValue: 'A', variants: ['A', 'B'], status, locked: false, ...o }, attachTo: document.body })

  it('shows "Varyant A | Varyant B ⋮" with the selected one marked, and no third variant', () => {
    const w = make()
    const tabs = w.findAll('[role="tab"]')
    expect(tabs.map((t) => t.text())).toEqual(['Varyant A', 'Varyant B'])
    expect(tabs[0].attributes('aria-selected')).toBe('true')
    expect(w.find('[data-testid="variant-menu"]').exists()).toBe(true)
    expect(w.find('[data-testid="variant-add"]').exists()).toBe(false)
    expect(w.get('[data-testid="variant-tab-B"]').find('.vt__dot').exists()).toBe(true)
  })

  it('offers "+" while there is only A', async () => {
    const w = make({ variants: ['A'] })
    expect(w.find('[data-testid="variant-menu"]').exists()).toBe(false)
    await w.get('[data-testid="variant-add"]').trigger('click')
    expect(w.emitted('add')).toHaveLength(1)
  })

  it('B\'s menu writes B again or removes it; Escape closes it', async () => {
    const w = make()
    await w.get('[data-testid="variant-menu"]').trigger('click')
    expect(w.findAll('[role="menuitem"]').map((m) => m.text())).toEqual(['Yeniden üret', 'Sil'])
    await w.get('[data-testid="variant-remove"]').trigger('click')
    expect(w.emitted('remove')?.[0]).toEqual(['B'])
    await w.get('[data-testid="variant-menu"]').trigger('click')
    await w.get('[data-testid="variant-regenerate"]').trigger('click')
    expect(w.emitted('regenerate')?.[0]).toEqual(['B'])
    await w.get('[data-testid="variant-menu"]').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()
    expect(w.find('[role="menu"]').exists()).toBe(false)
  })

  it('switches the variant on a click', async () => {
    const w = make()
    await w.get('[data-testid="variant-tab-B"]').trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['B'])
  })

  it('after approval nothing can be added or removed', () => {
    const w = make({ locked: true })
    expect(w.find('[data-testid="variant-menu"]').exists()).toBe(false)
    expect(make({ locked: true, variants: ['A'] }).find('[data-testid="variant-add"]').exists()).toBe(false)
  })
})

describe('the approval with Variant B', () => {
  const segment: Segment = {
    status: 'ok',
    conditions: [],
    explanation: 'İstanbul’daki müşteriler',
    audience: { matched: 1200, sms_reachable: 801, email_reachable: 890, both_reachable: 700 },
    offer: { kind: 'percent', value: 30, code: '' },
  }
  const props = { segment, warnings: [], confirming: false, frozen: false, error: null, mode: 'now' as const, when: '', scheduleError: null }

  it('shows the 50 / 50 bar and the people per variant', () => {
    const w = mount(ApprovalCard, { props: { ...props, variants: campaign.variant_counts } })
    const split = w.get('[data-testid="approval-split"]')
    expect(split.get('[role="img"]').attributes('aria-label')).toBe('Varyant A yüzde 50, Varyant B yüzde 50')
    expect(split.text()).toContain('%50')
    expect(w.get('[data-testid="split-A"]').text()).toMatch(/Varyant A\s*502\s*400\s*450/)
    expect(w.get('[data-testid="split-B"]').text()).toMatch(/Varyant B\s*498\s*401\s*440/)
  })

  it('shows no split without Variant B', () => {
    expect(mount(ApprovalCard, { props }).find('[data-testid="approval-split"]').exists()).toBe(false)
  })

  it('names the variant of a warning', () => {
    const w = mount(ApprovalCard, { props: { ...props, warnings: [{ field: 'sms_body', code: 'offer_missing', message: 'teklif yok', locale: 'tr-TR', variant: 'B' }] } })
    expect(w.text()).toContain('Varyant B · tr-TR teklif yok')
  })
})

describe('the sending step with Variant B', () => {
  const ch = (o = {}) => ({ total: 0, pending: 0, sent: 0, failed: 0, skipped: 0, retries: 0, rate_limited: 0, ...o })
  const delivery: Delivery = {
    sms: ch({ total: 20, sent: 18, failed: 2 }),
    email: ch({ total: 22, sent: 22 }),
    done: true,
    variants: [
      { variant: 'A', people: 11, sms: { total: 10, sent: 9 }, email: { total: 11, sent: 11 }, failed: 1 },
      { variant: 'B', people: 12, sms: { total: 10, sent: 9 }, email: { total: 11, sent: 11 }, failed: 1 },
    ],
  }

  it('compares the variants in a table at the bottom: people, SMS, e-mail, failed, and a total', () => {
    const w = mount(DeliveryProgress, { props: { delivery, status: 'completed' } })
    const table = w.get('[data-testid="variant-comparison"]')
    expect(table.findAll('thead th').map((t) => t.text())).toEqual(['Varyant', 'Kişi', 'SMS', 'E-posta', 'Başarısız'])
    expect(w.get('[data-testid="compare-A"]').findAll('td').map((t) => t.text())).toEqual(['11', '9', '11', '1'])
    expect(w.get('[data-testid="compare-total"]').findAll('td').map((t) => t.text())).toEqual(['23', '18', '22', '2'])
    // it is the last panel of the report
    const panels = w.findAll('.panel')
    expect(panels[panels.length - 1].attributes('data-testid')).toBe('variant-comparison')
  })

  it('shows sent / queued while messages wait', () => {
    const live = { ...delivery, done: false, variants: [{ ...delivery.variants![0], sms: { total: 10, sent: 4 } }, delivery.variants![1]] }
    const w = mount(DeliveryProgress, { props: { delivery: live, status: 'sending' } })
    expect(w.get('[data-testid="compare-A"]').findAll('td')[1].text()).toBe('4 / 10')
  })

  it('shows no comparison for a campaign without B', () => {
    expect(comparisonRows(undefined)).toEqual([])
    const w = mount(DeliveryProgress, { props: { delivery: { ...delivery, variants: undefined }, status: 'completed' } })
    expect(w.find('[data-testid="variant-comparison"]').exists()).toBe(false)
  })
})
