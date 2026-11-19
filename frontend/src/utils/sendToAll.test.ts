import { describe, expect, it } from 'vitest'
import { SEND_TO_ALL_SENTENCE } from './sendToAll'

describe('the one-click answer', () => {
  it('is a sentence the backend reads as "everyone"', () => {
    expect(SEND_TO_ALL_SENTENCE).toBe('Tüm müşterilere gönder.')
  })
})
