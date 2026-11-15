export type LLMMode = 'mock' | 'claude'

export interface AppInfo {
  llm_mode: LLMMode
  model?: string
}

/** Human-readable label for the active LLM mode, shown in the UI header. */
export function llmLabel(info: AppInfo): string {
  if (info.llm_mode === 'claude') {
    return info.model ? `Claude (${info.model})` : 'Claude'
  }
  return 'Mock LLM'
}

export async function fetchInfo(): Promise<AppInfo> {
  const res = await fetch('/api/info')
  if (!res.ok) {
    throw new Error(`GET /api/info failed: ${res.status}`)
  }
  return (await res.json()) as AppInfo
}
