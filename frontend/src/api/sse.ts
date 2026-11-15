import { ApiError, toApiError } from './http'

export interface SSEEvent {
  event: string
  data: unknown
}

/**
 * Incremental Server-Sent Events parser. Feed it text as it arrives (chunks may
 * end anywhere, even in the middle of a line); it returns the events completed
 * by that chunk. Comment lines (": keepalive") are ignored.
 *
 * EventSource cannot be used because the stream is started with POST.
 */
export class SSEParser {
  private buffer = ''

  push(text: string): SSEEvent[] {
    this.buffer += text.replace(/\r\n/g, '\n')
    const events: SSEEvent[] = []
    let sep: number
    while ((sep = this.buffer.indexOf('\n\n')) !== -1) {
      const block = this.buffer.slice(0, sep)
      this.buffer = this.buffer.slice(sep + 2)
      const parsed = parseBlock(block)
      if (parsed) events.push(parsed)
    }
    return events
  }
}

function parseBlock(block: string): SSEEvent | null {
  let event = 'message'
  const data: string[] = []
  for (const line of block.split('\n')) {
    if (line === '' || line.startsWith(':')) continue
    const colon = line.indexOf(':')
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'event') event = value
    else if (field === 'data') data.push(value)
  }
  if (data.length === 0) return null
  const raw = data.join('\n')
  try {
    return { event, data: JSON.parse(raw) }
  } catch {
    return { event, data: raw }
  }
}

/**
 * POSTs to url and calls onEvent for every server-sent event until the stream
 * ends. A non-2xx response (the server answers with JSON before it starts
 * streaming) is thrown as an ApiError.
 */
export async function streamPost(url: string, signal: AbortSignal, onEvent: (e: SSEEvent) => void): Promise<void> {
  let res: Response
  try {
    res = await fetch(url, { method: 'POST', signal, headers: { Accept: 'text/event-stream' } })
  } catch (e) {
    if (signal.aborted) throw e
    throw new ApiError(0, 'network', 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.')
  }
  if (!res.ok) throw await toApiError(res)
  if (!res.body) throw new ApiError(0, 'no_stream', 'Tarayıcı akış okumayı desteklemiyor.')

  const reader = res.body.getReader()
  const decoder = new TextDecoder() // stream:true below keeps multi-byte characters intact across chunks
  const parser = new SSEParser()
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    for (const ev of parser.push(decoder.decode(value, { stream: true }))) onEvent(ev)
  }
}
