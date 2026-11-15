import { describe, expect, it } from 'vitest'
import type { Limits, MessageContent, VariableInfo } from '../types'
import { chars, checkContent, effectiveLength } from './limits'

const limits: Limits = { sms_max: 160, subject_max: 80, email_body_max: 5000 }
const vars: VariableInfo[] = [
  { name: 'first_name', budget: 20, fallback: 'değerli müşterimiz' },
  { name: 'city', budget: 15, fallback: 'bölgeniz' },
]
const ok: MessageContent = { sms_body: 'Merhaba {{first_name}}!', email_subject: 'Konu', email_body: 'Gövde' }

describe('effectiveLength', () => {
  it('counts each variable as its fixed budget', () => {
    expect(effectiveLength('ab {{first_name}} {{city}}', vars)).toBe(3 + 20 + 1 + 15)
    expect(effectiveLength('ab {{ first_name }}', vars)).toBe(3 + 20)
  })

  it('counts characters, not UTF-16 units or bytes', () => {
    expect(chars('şğİ')).toBe(3)
    expect(chars('😀')).toBe(1)
    expect(effectiveLength('ş'.repeat(160), vars)).toBe(160)
  })
})

describe('checkContent', () => {
  it('accepts valid content', () => {
    expect(checkContent(ok, limits, vars).issues).toEqual([])
  })

  it('uses exactly 160 for SMS and 80 for the subject', () => {
    const fit = checkContent({ ...ok, sms_body: 'a'.repeat(160), email_subject: 'b'.repeat(80) }, limits, vars)
    expect(fit.issues).toEqual([])
    const over = checkContent({ ...ok, sms_body: 'a'.repeat(161), email_subject: 'b'.repeat(81) }, limits, vars)
    expect(over.issues.map((i) => i.code)).toEqual(['too_long', 'too_long'])
  })

  it('counts variables toward the limit', () => {
    expect(checkContent({ ...ok, sms_body: 'a'.repeat(140) + '{{first_name}}' }, limits, vars).issues).toEqual([])
    expect(checkContent({ ...ok, sms_body: 'a'.repeat(141) + '{{first_name}}' }, limits, vars).issues[0].code).toBe('too_long')
  })

  it('rejects a line break in the subject but allows it in the SMS and body', () => {
    expect(checkContent({ ...ok, email_subject: 'iki\nsatır' }, limits, vars).issues.map((i) => i.code)).toContain('newline_in_subject')
    expect(checkContent({ ...ok, sms_body: 'iki\nsatır', email_body: 'a\n\nb' }, limits, vars).issues).toEqual([])
  })

  it('rejects unknown variables and stray braces', () => {
    const codes = (sms: string) => checkContent({ ...ok, sms_body: sms }, limits, vars).issues.map((i) => i.code)
    expect(codes('Hi {{last_name}}')).toEqual(['unknown_variable'])
    expect(codes('Hi {first_name}')).toEqual(['malformed_placeholder'])
    expect(codes('Hi {{first_name}')).toEqual(['malformed_placeholder'])
  })

  it('requires every field', () => {
    const codes = checkContent({ sms_body: ' ', email_subject: '', email_body: '' }, limits, vars).issues.map((i) => i.code)
    expect(codes).toEqual(['required', 'required', 'required'])
  })
})
