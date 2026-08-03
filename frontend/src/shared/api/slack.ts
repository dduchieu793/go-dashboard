import type { WorkflowRun } from './workflows'

export type SlackThread = {
  id: string
  workspace_id: string
  channel_id: string
  thread_ts: string
  context_version: number
  sync_status: 'uninitialized' | 'syncing' | 'synchronized' | 'dirty' | 'failed'
  last_synced_at?: string
  requested_analysis_version: number
  latest_workflow_run_id?: string
  updated_at: string
}

export type SlackMessage = {
  id: string
  slack_timestamp: string
  author_id: string
  text: string
  is_parent: boolean
  is_deleted: boolean
  edited_at?: string
}

export type SlackAttachment = {
  id: string
  message_id: string
  slack_file_id: string
  filename: string
  mime_type: string
  size_bytes: number
  download_status: string
  extraction_status: string
  is_removed: boolean
}

async function responseJSON<T>(response: Response): Promise<T> {
  if (!response.ok) throw new Error(`Request failed with status ${response.status}`)
  return response.json() as Promise<T>
}

export async function listSlackThreads(signal?: AbortSignal) {
  const response = await fetch('/api/v1/threads?limit=50', { signal })
  return (await responseJSON<{ threads: SlackThread[] }>(response)).threads
}

export async function listSlackMessages(threadID: string, signal?: AbortSignal) {
  const response = await fetch(`/api/v1/threads/${encodeURIComponent(threadID)}/messages`, { signal })
  return (await responseJSON<{ messages: SlackMessage[] }>(response)).messages
}

export async function listSlackAttachments(threadID: string, signal?: AbortSignal) {
  const response = await fetch(`/api/v1/threads/${encodeURIComponent(threadID)}/attachments`, { signal })
  return (await responseJSON<{ attachments: SlackAttachment[] }>(response)).attachments
}

export async function refreshSlackThread(threadID: string) {
  const response = await fetch(`/api/v1/threads/${encodeURIComponent(threadID)}/refresh`, { method: 'POST' })
  return responseJSON<{ thread: SlackThread; workflow_run: WorkflowRun | null }>(response)
}

export async function analyzeSlackThread(threadID: string) {
  const response = await fetch(`/api/v1/threads/${encodeURIComponent(threadID)}/analyze`, { method: 'POST' })
  return responseJSON<WorkflowRun>(response)
}
