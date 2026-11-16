import { describe, expect, it } from 'vitest'
import { campaignIdFromUrl, urlForCampaign, urlWithoutCampaign } from './route'

describe('the campaign in the address', () => {
  it('is read from ?c=', () => {
    expect(campaignIdFromUrl('http://x/?c=12')).toBe(12)
    expect(campaignIdFromUrl('http://x/?a=1&c=7#/dashboard')).toBe(7)
  })

  it('is absent when there is no usable id', () => {
    expect(campaignIdFromUrl('http://x/')).toBeNull()
    for (const bad of ['', '0', '-3', '1.5', 'abc', '12abc', '99999999999999999999']) {
      expect(campaignIdFromUrl(`http://x/?c=${bad}`), bad).toBeNull()
    }
  })
})

describe('starting a new campaign', () => {
  it('removes ?c= so the old campaign is not opened again', () => {
    expect(urlWithoutCampaign('http://x/?c=12')).toBe('/')
    expect(urlWithoutCampaign('http://x/?c=12#/dashboard')).toBe('/')
  })

  it('keeps other parameters and sets the wanted hash', () => {
    expect(urlWithoutCampaign('http://x/app?a=1&c=12&b=2#/dashboard', '#/')).toBe('/app?a=1&b=2#/')
  })

  it('leaves an address without a campaign alone', () => {
    expect(urlWithoutCampaign('http://x/')).toBe('/')
    expect(campaignIdFromUrl('http://x' + urlWithoutCampaign('http://x/?c=5'))).toBeNull()
  })
})

describe('opening a campaign', () => {
  it('puts its id in the address and drops the dashboard hash', () => {
    expect(urlForCampaign('http://x/#/dashboard', 9)).toBe('/?c=9')
    expect(urlForCampaign('http://x/?c=1', 9, '#/')).toBe('/?c=9#/')
    expect(campaignIdFromUrl('http://x' + urlForCampaign('http://x/', 42))).toBe(42)
  })
})
