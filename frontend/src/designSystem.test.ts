import { describe, expect, it } from 'vitest'
// @ts-expect-error the project has no Node typings; the test runner runs on Node
import { readFileSync } from 'node:fs'

// These tests guard the design system itself, not one screen: they read the sources.
const sources = import.meta.glob('./**/*.vue', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
const read = (file: string): string => readFileSync(new URL(file, import.meta.url), 'utf-8')
const tokens = read('./styles/tokens.css')
const styles = read('./style.css')

describe('no browser-styled controls', () => {
  // The browser's own radio buttons, date boxes and drop-downs are the part of a page that looks unfinished.
  const raw = /<input[^>]*type=["']?(radio|checkbox|date|datetime-local|time|month|week|range|color)\b/i
  const select = /<select[\s>]/i

  it('none of the screens uses one', () => {
    const offenders = Object.entries(sources)
      .filter(([, src]) => raw.test(src) || select.test(src))
      .map(([file]) => file)
    expect(offenders).toEqual([])
  })

  it('looked at the components and the screens (not an empty list)', () => {
    const files = Object.keys(sources)
    expect(files.length).toBeGreaterThan(20)
    expect(files).toEqual(expect.arrayContaining(['./components/ui/DateTimeField.vue', './views/DashboardView.vue', './App.vue']))
  })
})

describe('the shared building blocks exist', () => {
  it('has all eight', () => {
    for (const name of ['Button', 'Card', 'StatTile', 'Badge', 'SegmentedControl', 'DateTimeField', 'ProgressBar', 'Modal']) {
      expect(Object.keys(sources), name).toContain(`./components/ui/${name}.vue`)
    }
  })
})

describe('the screens are built from them', () => {
  it('Dashboard uses the table, a button and a modal', () => {
    const dash = sources['./views/DashboardView.vue']
    for (const part of ['CampaignTable', 'ui/Button.vue', 'ui/Modal.vue']) expect(dash, part).toContain(part)
  })
  it('Approval uses the toggle and the date-time field', () => {
    const a = sources['./components/ApprovalCard.vue']
    for (const part of ['SegmentedControl', 'DateTimeField', 'ui/Card.vue']) expect(a, part).toContain(part)
  })
  it('the wizard uses Card, Button, Badge and Modal', () => {
    const c = sources['./views/ComposeView.vue']
    for (const part of ['ui/Card.vue', 'ui/Button.vue', 'ui/Badge.vue', 'ui/Modal.vue']) expect(c, part).toContain(part)
  })
})

describe('one column for every page', () => {
  it('the shell is a navigation rail, a top bar and one content column', () => {
    const app = sources['./App.vue']
    expect(app).toMatch(/class="shell"/)
    expect(app).toMatch(/<nav class="rail"/)
    expect(app).toMatch(/<header class="topbar"/)
    expect(app).toMatch(/<main class="container content"/)
  })
  it('the container takes its width from one token, with the gutter on both sides', () => {
    expect(tokens).toMatch(/--page:\s*1600px/)
    expect(tokens).toMatch(/--gutter:\s*40px/)
    expect(styles).toMatch(/\.container\s*\{[^}]*max-width:\s*calc\(var\(--page\) \+ 2 \* var\(--gutter\)\)[^}]*margin:\s*0 auto/s)
  })
  it('no screen sets a width of its own', () => {
    for (const file of ['./views/DashboardView.vue', './views/ComposeView.vue']) {
      expect(sources[file], file).not.toMatch(/^\s*max-width:\s*\d+px/m) // a media query is fine, a width rule is not
    }
  })
})

describe('tokens', () => {
  it('has a light and a dark palette with one accent', () => {
    expect(tokens).toMatch(/--accent:\s*#1f33cc/i)
    expect(tokens).toMatch(/prefers-color-scheme:\s*dark/)
    expect(tokens).toMatch(/--bg:[^;]+;[\s\S]*prefers-color-scheme:\s*dark[\s\S]*--bg:/)
  })
  it('sets the type scale and the space scale', () => {
    for (const t of ['--text-xs', '--text-sm', '--text-base', '--text-md', '--text-lg', '--text-xl', '--s-1', '--s-4', '--s-8', '--r-sm', '--r-md']) {
      expect(tokens, t).toContain(t + ':')
    }
  })
  it('uses Geist for all text', () => {
    expect(tokens).toMatch(/--font:\s*'Geist Variable'/)
    expect(styles).toMatch(/font-family:\s*var\(--font\)/)
  })
  it('panels have only the faint card shadow from the token, and none in dark mode', () => {
    const card = sources['./components/ui/Card.vue']
    expect(card).toMatch(/box-shadow:\s*var\(--shadow-card\)/)
    expect(card).not.toMatch(/box-shadow:\s*0/)
    expect(tokens).toMatch(/prefers-color-scheme:\s*dark[\s\S]*--shadow-card:\s*none/)
  })
  it('uses a 16px base and 44px controls', () => {
    expect(tokens).toMatch(/--text-base:\s*1rem/)
    expect(tokens).toMatch(/--control-h:\s*44px/)
  })
})

describe('motion and accessibility floor', () => {
  it('respects reduced motion', () => {
    expect(styles).toMatch(/prefers-reduced-motion:\s*reduce/)
  })
  it('keeps a visible keyboard focus', () => {
    expect(styles).toMatch(/:focus-visible\s*\{[^}]*outline:\s*2px solid var\(--accent\)/)
  })
})

describe('locale codes are written as they are', () => {
  // A Turkish page upper-cases "it-IT" to "İT-İT": no locale code may sit in an upper-cased element
  // (the .label class, table heads), and no script upper-cases one.
  const shouty = /<(\w+)[^>]*\bclass="[^"]*\b(label|group)\b[^"]*"[^>]*>([^<]*)<|<th\b[^>]*>([^<]*)</g
  const codeInside = /\{\{[^}]*\b(code|locale|activeLocale|sourceCode)\b[^}]*\}\}/

  it('no upper-cased element shows a locale code', () => {
    const offenders: string[] = []
    for (const [file, src] of Object.entries(sources)) {
      for (const m of src.matchAll(shouty)) {
        const inner = m[3] ?? m[4] ?? ''
        if (codeInside.test(inner)) offenders.push(`${file}: ${inner.trim()}`)
      }
    }
    expect(offenders).toEqual([])
  })

  it('no script upper-cases text with toUpperCase (Turkish-aware toLocaleUpperCase only for sentences)', () => {
    const scripts = import.meta.glob('./**/*.ts', { query: '?raw', import: 'default', eager: true }) as Record<string, string>
    const offenders = Object.entries({ ...sources, ...scripts })
      .filter(([file]) => !file.endsWith('.test.ts'))
      .filter(([, src]) => /\.toUpperCase\(/.test(src))
      .map(([file]) => file)
    expect(offenders).toEqual([])
  })

  it('the locale detail has no yellow "stale" box: the badge and the translate button say it', () => {
    const compose = Object.entries(sources).find(([file]) => file.endsWith('ComposeView.vue'))![1]
    expect(compose).not.toContain('data-testid="stale"')
    expect(compose).not.toContain('çeviri v')
    expect(compose).toContain('data-testid="source-diff"') // the "tr-TR değişikliği" fold stays
    expect(compose).toContain('Güncel olmayanları çevir')
  })
})
