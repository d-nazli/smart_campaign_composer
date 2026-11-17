import { afterEach, describe, expect, it, vi } from 'vitest'
import { approveCampaign } from './campaigns'
import { ApiError } from './http'

afterEach(() => {
  vi.unstubAllGlobals()
})

// The server refuses a send time in the past or more than 30 days ahead with
// 422 invalid_schedule; the wizard shows its message under the approval card.
describe('approving for a time the server does not allow', () => {
  it('becomes an ApiError with status 422, code invalid_schedule and the server message', async () => {
    const body = { error: { code: 'invalid_schedule', message: 'gönderim zamanı en fazla 30 gün sonrası olabilir' } }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 422, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    const far = new Date(Date.now() + 31 * 24 * 60 * 60 * 1000).toISOString()
    const err = await approveCampaign(7, far).catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 422, code: 'invalid_schedule', message: 'gönderim zamanı en fazla 30 gün sonrası olabilir' })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/campaigns/7/approve')
    expect(JSON.parse(init.body as string)).toEqual({ scheduled_at: far })
  })
})
