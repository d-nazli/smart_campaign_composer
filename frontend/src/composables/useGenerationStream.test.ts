// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ApiError } from '../api/http'
import type { SSEEvent } from '../api/sse'

// Every call of streamPost is a stream the test drives: push events, close it, or fail it.
interface FakeStream {
  url: string
  signal: AbortSignal
  push: (e: SSEEvent) => void
  close: () => void
  fail: (e: unknown) => void
}
let streams: FakeStream[] = []
vi.mock('../api/sse', () => ({
  streamPost: (url: string, signal: AbortSignal, onEvent: (e: SSEEvent) => void) =>
    new Promise<void>((resolve, reject) => {
      streams.push({ url, signal, push: onEvent, close: resolve, fail: reject })
    }),
}))

import { useGenerationStream } from './useGenerationStream'

type Stream = ReturnType<typeof useGenerationStream>
let w: VueWrapper | null = null
let gen: Stream

beforeEach(() => {
  streams = []
  w = mount(
    defineComponent({
      setup() {
        gen = useGenerationStream()
        return () => null
      },
    }),
  )
})
afterEach(() => {
  w?.unmount()
  w = null
})

const delta = (text: string): SSEEvent => ({ event: 'delta', data: { locale: 'tr-TR', field: 'sms_body', text } })

describe('useGenerationStream', () => {
  it('streams into the state and ends with done', async () => {
    const run = gen.start(1)
    expect(streams[0].url).toBe('/api/campaigns/1/messages/stream')
    expect(gen.state.value.phase).toBe('streaming')
    streams[0].push(delta('Merhaba'))
    expect(gen.state.value.locales['tr-TR'].draft.sms_body).toBe('Merhaba')
    streams[0].push({ event: 'done', data: {} })
    streams[0].close()
    await run
    expect(gen.state.value.phase).toBe('done')
    expect(gen.state.value.error).toBeNull()
  })

  it('a stream the server closes without a final event is an error', async () => {
    const run = gen.start(1, 'B')
    expect(streams[0].url).toBe('/api/campaigns/1/messages/stream?variant=B')
    streams[0].push(delta('yarım'))
    streams[0].close()
    await run
    expect(gen.state.value.phase).toBe('error')
    expect(gen.state.value.error).toEqual({ code: 'stream_closed', message: 'Bağlantı beklenmedik şekilde kapandı. Tekrar deneyin.' })
  })

  it('a stale stream cannot write over the new one', async () => {
    const first = gen.start(1)
    const second = gen.start(1)
    expect(streams[0].signal.aborted).toBe(true) // the old request is cancelled
    // The old stream still delivers (and ends) late: none of it reaches the state.
    streams[0].push(delta('ESKİ'))
    streams[0].push({ event: 'error', data: { code: 'x', message: 'eski hata' } })
    streams[0].close()
    await first
    streams[1].push(delta('yeni'))
    expect(gen.state.value.phase).toBe('streaming')
    expect(gen.state.value.locales['tr-TR'].draft.sms_body).toBe('yeni')
    streams[1].push({ event: 'done', data: {} })
    streams[1].close()
    await second
    expect(gen.state.value.phase).toBe('done')
  })

  it('a refused start (HTTP error) becomes the error state; an aborted one is silent', async () => {
    const refused = gen.start(1)
    streams[0].fail(new ApiError(409, 'invalid_state', 'kampanya onaylandı'))
    await refused
    expect(gen.state.value.error).toEqual({ code: 'invalid_state', message: 'kampanya onaylandı' })

    const aborted = gen.start(1)
    gen.reset()
    streams[1].fail(new DOMException('aborted', 'AbortError'))
    await aborted
    await flushPromises()
    expect(gen.state.value.phase).toBe('idle')
    expect(gen.state.value.error).toBeNull()
  })
})
