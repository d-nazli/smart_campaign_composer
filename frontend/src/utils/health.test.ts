import { describe, expect, it } from 'vitest'
import { allHealthy, downItems, healthItems, healthTitle } from './health'

describe('system health', () => {
  it('is healthy only when every service answers', () => {
    const ok = healthItems({ api: 'ok', database: 'ok', worker: 'ok' }, false)
    expect(allHealthy(ok)).toBe(true)
    expect(downItems(ok)).toEqual([])
    expect(healthTitle(ok, 'Mock LLM')).toBe('API: çalışıyor\nWorker: çalışıyor\nVeritabanı: çalışıyor\nLLM: Mock LLM')
  })

  it('names the services that are down', () => {
    const items = healthItems({ api: 'ok', database: 'ok', worker: 'down' }, false)
    expect(allHealthy(items)).toBe(false)
    expect(downItems(items).map((i) => i.label)).toEqual(['Worker'])
  })

  it('knows nothing else when the api cannot be reached', () => {
    const items = healthItems(null, true)
    expect(downItems(items).map((i) => `${i.label}:${i.state}`)).toEqual(['API:down', 'Worker:unknown', 'Veritabanı:unknown'])
  })

  it('is neither healthy nor down before the first answer', () => {
    const items = healthItems(null, false)
    expect(allHealthy(items)).toBe(false)
    expect(downItems(items)).toEqual([])
  })
})
