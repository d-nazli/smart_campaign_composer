// Shared test setup. Only matters for component tests (jsdom); in the plain Node tests there is no window.
if (typeof window !== 'undefined') {
  // jsdom has no matchMedia. Report "reduced motion" so AnimatedNumber shows its value at once.
  window.matchMedia ??= ((query: string) => ({
    matches: query.includes('prefers-reduced-motion'),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
  window.scrollTo = (() => {}) as typeof window.scrollTo
}
