import { describe, expect, it } from 'vitest'
import { SEND_TO_ALL_SENTENCE, withAllCustomers } from './sendToAll'

describe('withAllCustomers', () => {
  it('answers the question by adding the explicit sentence to the brief', () => {
    expect(withAllCustomers('anneler gününe özel 100 TL indirim')).toBe(`anneler gününe özel 100 TL indirim\n\n${SEND_TO_ALL_SENTENCE}`)
  })
  it('does not stack the sentence when clicked twice', () => {
    const once = withAllCustomers('indirim gönder')
    expect(withAllCustomers(once)).toBe(once)
  })
  it('ignores trailing whitespace', () => {
    expect(withAllCustomers('indirim gönder  \n')).toBe(`indirim gönder\n\n${SEND_TO_ALL_SENTENCE}`)
  })
  it('the sentence is one the backend reads as "everyone"', () => {
    expect(SEND_TO_ALL_SENTENCE).toBe('Tüm müşterilere gönder.')
  })
})
