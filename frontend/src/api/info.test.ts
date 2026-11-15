import { describe, expect, it } from 'vitest'
import { llmLabel } from './info'

describe('llmLabel', () => {
  it('labels mock mode', () => {
    expect(llmLabel({ llm_mode: 'mock' })).toBe('Mock LLM')
  })

  it('labels claude mode with the model', () => {
    expect(llmLabel({ llm_mode: 'claude', model: 'claude-sonnet-5-5' })).toBe('Claude (claude-sonnet-5-5)')
  })

  it('labels claude mode without a model', () => {
    expect(llmLabel({ llm_mode: 'claude' })).toBe('Claude')
  })
})
