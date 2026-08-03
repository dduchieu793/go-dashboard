export type Health = {
  status: string
}

export type LLMStatus = {
  available: boolean
  model: string
  model_available: boolean
}

export type ModelProfileStatus = {
  name: string
  provider: string
  model: string
  available: boolean
  model_available: boolean
  capabilities: string[]
}

export type CapabilityMetadata = {
  name: string
  description: string
  default_model_profile?: string
  llm_backed: boolean
}

export type WorkflowDefinition = {
  id: string
  name: string
  description: string
  version: number
  enabled: boolean
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

export function getModelStatuses(signal: AbortSignal) {
  return getJSON<{ profiles: ModelProfileStatus[] }>('/api/v1/system/model-statuses', signal)
}

export function getCapabilities(signal: AbortSignal) {
  return getJSON<{ capabilities: CapabilityMetadata[] }>('/api/v1/capabilities', signal)
}

export function getWorkflows(signal: AbortSignal) {
  return getJSON<{ workflows: WorkflowDefinition[] }>('/api/v1/workflows', signal)
}
