export type RunStatus = 'pending' | 'running' | 'waiting_approval' | 'completed' | 'failed' | 'cancelled'
export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'cancelled'

export type WorkflowStep = {
  id: string
  step_id: string
  capability: string
  model_profile?: string
  model?: string
  status: StepStatus
  attempt: number
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  error_code?: string
  error_message?: string
  started_at?: string
  completed_at?: string
}

export type WorkflowArtifact = {
  id: string
  step_run_id: string
  type: string
  content: Record<string, unknown>
  model?: string
  prompt_version?: string
  created_at: string
}

export type WorkflowSource = {
  id: string
  kind: string
  external_id: string
  author_id?: string
  content: string
  occurred_at: string
  metadata?: Record<string, string>
}

export type WorkflowRun = {
  id: string
  workflow_id: string
  workflow_version: number
  request: {
    id: string
    source: string
    type: string
    content: string
    metadata?: Record<string, string>
    sources?: WorkflowSource[]
    received_at: string
  }
  status: RunStatus
  final_artifact_id?: string
  error_code?: string
  error_message?: string
  started_at?: string
  completed_at?: string
  created_at: string
  steps: WorkflowStep[]
  artifacts: WorkflowArtifact[]
}

type APIError = { error?: { message?: string } }

async function parseResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = (await response.json().catch(() => ({}))) as APIError
    throw new Error(error.error?.message ?? `Request failed with status ${response.status}`)
  }
  return response.json() as Promise<T>
}

export async function startManualWorkflow(content: string, signal?: AbortSignal): Promise<WorkflowRun> {
  const response = await fetch('/api/v1/workflows/manual-summary/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
    signal,
  })
  return parseResponse<WorkflowRun>(response)
}

export async function listWorkflowRuns(signal?: AbortSignal): Promise<WorkflowRun[]> {
  const response = await fetch('/api/v1/workflow-runs?limit=25', { signal })
  const body = await parseResponse<{ runs: WorkflowRun[] }>(response)
  return body.runs
}

export async function getWorkflowRun(id: string, signal?: AbortSignal): Promise<WorkflowRun> {
  const response = await fetch(`/api/v1/workflow-runs/${encodeURIComponent(id)}`, { signal })
  return parseResponse<WorkflowRun>(response)
}

async function runLifecycleOperation(id: string, operation: 'retry' | 'cancel'): Promise<WorkflowRun> {
  const response = await fetch(`/api/v1/workflow-runs/${encodeURIComponent(id)}/${operation}`, { method: 'POST' })
  return parseResponse<WorkflowRun>(response)
}

export function retryWorkflowRun(id: string): Promise<WorkflowRun> {
  return runLifecycleOperation(id, 'retry')
}

export function cancelWorkflowRun(id: string): Promise<WorkflowRun> {
  return runLifecycleOperation(id, 'cancel')
}
