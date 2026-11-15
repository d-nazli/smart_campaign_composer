import type { ContentField, Issue, Limits, MessageContent, VariableInfo } from '../types'

// Mirrors backend/internal/content/validate.go so the editor can show problems
// while typing. The server remains the authority: it validates again on save.

const PLACEHOLDER = /\{\{\s*([^{}\s]*)\s*\}\}/g
// \n, \r, NEL, LINE SEPARATOR, PARAGRAPH SEPARATOR (built from char codes so no
// invisible separator characters live in this source file).
const LINE_BREAKS = new RegExp('[\\n\\r' + String.fromCharCode(0x85, 0x2028, 0x2029) + ']')

/** Characters (code points), like the backend's rune count: "ş" is 1, not 2. */
export function chars(s: string): number {
  return [...s].length
}

/** Length with every known {{variable}} counted as its fixed budget. */
export function effectiveLength(text: string, variables: VariableInfo[]): number {
  const budget = new Map(variables.map((v) => [v.name, v.budget]))
  let total = 0
  let last = 0
  for (const m of text.matchAll(PLACEHOLDER)) {
    const start = m.index ?? 0
    total += chars(text.slice(last, start)) + (budget.get(m[1]) ?? 0)
    last = start + m[0].length
  }
  return total + chars(text.slice(last))
}

export interface LocalCheck {
  smsLength: number
  subjectLength: number
  issues: Issue[]
}

/** Quick client-side check of the three fields. */
export function checkContent(c: MessageContent, limits: Limits, variables: VariableInfo[]): LocalCheck {
  const issues: Issue[] = []
  const known = new Set(variables.map((v) => v.name))
  const add = (field: ContentField, code: string, message: string, length?: number, limit?: number) =>
    issues.push({ field, code, message, length, limit })

  const check = (field: ContentField, label: string, text: string, max: number, singleLine: boolean) => {
    const t = text.trim()
    if (t === '') {
      add(field, 'required', `${label} boş olamaz.`)
      return 0
    }
    if (singleLine && LINE_BREAKS.test(t)) {
      add(field, 'newline_in_subject', `${label} tek satır olmalı; satır sonu içeremez.`)
    }
    for (const m of t.matchAll(PLACEHOLDER)) {
      if (!known.has(m[1])) {
        add(field, 'unknown_variable', `${label}: bilinmeyen değişken {{${m[1]}}}. Kullanılabilir: ${variables.map((v) => `{{${v.name}}}`).join(', ')}.`)
      }
    }
    if (/[{}]/.test(t.replace(PLACEHOLDER, ''))) {
      add(field, 'malformed_placeholder', `${label}: hatalı süslü parantez. Değişkenler {{değişken_adı}} biçiminde yazılmalı.`)
    }
    const n = effectiveLength(t, variables)
    if (n > max) {
      add(field, 'too_long', `${label} en fazla ${max} karakter olmalı; değişkenler paylarıyla sayıldığında ${n} karakter (${n - max} fazla).`, n, max)
    }
    return n
  }

  const smsLength = check('sms_body', 'SMS', c.sms_body, limits.sms_max, false)
  const subjectLength = check('email_subject', 'E-posta konusu', c.email_subject, limits.subject_max, true)
  check('email_body', 'E-posta metni', c.email_body, limits.email_body_max, false)
  return { smsLength, subjectLength, issues }
}
