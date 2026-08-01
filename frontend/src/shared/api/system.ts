export type Health = {
  status: string
}

export type LLMStatus = {
  available: boolean
  model: string
  model_available: boolean
}

async function getJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) {
    throw new Error(`Request failed with status ${response.status}`)
  }
  return response.json() as Promise<T>
}

export function getHealth(signal: AbortSignal) {
  return getJSON<Health>('/health', signal)
}

export function getLLMStatus(signal: AbortSignal) {
  return getJSON<LLMStatus>('/api/v1/system/llm-status', signal)
}
