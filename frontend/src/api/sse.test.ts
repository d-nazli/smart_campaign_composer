import { describe, expect, it } from 'vitest'
import { SSEParser } from './sse'

const wire =
  'event: attempt\ndata: {"n":1,"max":3}\n\n' +
  ': keepalive\n\n' +
  'event: delta\ndata: {"field":"sms_body","text":"Merhaba ş\\n\\"x\\""}\n\n' +
  'event: done\ndata: {"status":"content_ready"}\n\n'

describe('SSEParser', () => {
  it('parses a complete stream and ignores comments', () => {
    const events = new SSEParser().push(wire)
    expect(events.map((e) => e.event)).toEqual(['attempt', 'delta', 'done'])
    expect(events[0].data).toEqual({ n: 1, max: 3 })
    expect(events[1].data).toEqual({ field: 'sms_body', text: 'Merhaba ş\n"x"' })
  })

  it('gives the same result for every possible chunk boundary', () => {
    const expected = new SSEParser().push(wire)
    for (let i = 0; i <= wire.length; i++) {
      const p = new SSEParser()
      const got = [...p.push(wire.slice(0, i)), ...p.push(wire.slice(i))]
      expect(got).toEqual(expected)
    }
  })

  it('works one character at a time', () => {
    const p = new SSEParser()
    const got = [...wire].flatMap((ch) => p.push(ch))
    expect(got.map((e) => e.event)).toEqual(['attempt', 'delta', 'done'])
  })

  it('does not emit an event until its blank line arrives', () => {
    const p = new SSEParser()
    expect(p.push('event: delta\ndata: {"a":1}\n')).toEqual([])
    expect(p.push('\n')).toHaveLength(1)
  })

  it('handles CRLF line endings', () => {
    const events = new SSEParser().push('event: done\r\ndata: {"ok":true}\r\n\r\n')
    expect(events).toEqual([{ event: 'done', data: { ok: true } }])
  })

  it('joins multi-line data and tolerates non-JSON data', () => {
    const events = new SSEParser().push('event: note\ndata: hello\ndata: world\n\n')
    expect(events).toEqual([{ event: 'note', data: 'hello\nworld' }])
  })
})
