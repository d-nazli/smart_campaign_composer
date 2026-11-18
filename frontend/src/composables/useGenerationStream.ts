import { onBeforeUnmount, ref } from 'vue'
import { ApiError } from '../api/http'
import { streamPost } from '../api/sse'
import { initialState, reduce, startedState, type StreamState } from './streamState'

/** Starts message generation for a campaign (Variant A, or B) and exposes the live state. */
export function useGenerationStream() {
  const state = ref<StreamState>(initialState())
  let controller: AbortController | null = null

  async function start(campaignId: number, variant: 'A' | 'B' = 'A'): Promise<void> {
    controller?.abort()
    const mine = new AbortController()
    controller = mine
    state.value = startedState()

    const query = variant === 'B' ? '?variant=B' : ''
    try {
      await streamPost(`/api/campaigns/${campaignId}/messages/stream${query}`, mine.signal, (ev) => {
        if (controller === mine) state.value = reduce(state.value, ev)
      })
      // The server closed the stream without a final event: treat as an error.
      if (controller === mine && state.value.phase === 'streaming') {
        state.value = reduce(state.value, {
          event: 'error',
          data: { code: 'stream_closed', message: 'Bağlantı beklenmedik şekilde kapandı. Tekrar deneyin.' },
        })
      }
    } catch (e) {
      if (mine.signal.aborted) return
      const err = e instanceof ApiError ? e : new ApiError(0, 'unknown', 'Beklenmeyen bir hata oluştu.')
      state.value = reduce(state.value, { event: 'error', data: { code: err.code, message: err.message } })
    }
  }

  function reset(): void {
    controller?.abort()
    controller = null
    state.value = initialState()
  }

  onBeforeUnmount(() => controller?.abort())

  return { state, start, reset }
}
