import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WorkflowRun } from '../../shared/api/workflows'
import { cancelWorkflowRun, retryWorkflowRun } from '../../shared/api/workflows'
import { ManualWorkflowForm } from './ManualWorkflowForm'
import { RunDetails } from './RunDetails'
import { SlackThreadDashboard } from '../slack/SlackThreadDashboard'

function testRun(): WorkflowRun {
  return {
    id: 'run_1', workflow_id: 'manual-summary', workflow_version: 1, status: 'completed',
    request: { id: 'request_1', source: 'ui', type: 'manual_text', content: 'Original source', received_at: '2026-08-02T00:00:00Z' },
    created_at: '2026-08-02T00:00:00Z', final_artifact_id: 'artifact_final',
    steps: [
      {
        id: 'step_1', step_id: 'summarize', capability: 'summarize_text', model: 'qwen3:4b', status: 'completed', attempt: 1,
        input: { content: 'Original source' }, output: { summary: 'Release summary' },
      },
      { id: 'step_2', step_id: 'actions', capability: 'extract_action_items', model: 'qwen3:4b', status: 'completed', attempt: 1 },
      { id: 'step_3', step_id: 'compose', capability: 'compose_dashboard_result', status: 'completed', attempt: 1 },
    ],
    artifacts: [{
      id: 'artifact_final', step_run_id: 'step_3', type: 'dashboard_result', created_at: '2026-08-02T00:00:01Z',
      content: { summary: 'Release summary', action_items: 'Ship on Friday', models: ['qwen3:4b'] },
    }],
  }
}

function renderWithQuery(component: React.ReactNode) {
  return render(<QueryClientProvider client={new QueryClient()}>{component}</QueryClientProvider>)
}

afterEach(() => vi.unstubAllGlobals())

describe('workflow UI', () => {
  it('starts the controlled manual workflow', async () => {
    const run = testRun()
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify(run), {
      status: 202, headers: { 'Content-Type': 'application/json' },
    })))
    vi.stubGlobal('fetch', fetchMock)
    const onCreated = vi.fn()
    const user = userEvent.setup()
    renderWithQuery(<ManualWorkflowForm onCreated={onCreated} />)

    await user.type(screen.getByLabelText('Source content'), 'Original source')
    await user.click(screen.getByRole('button', { name: 'Start workflow' }))

    expect(onCreated).toHaveBeenCalledWith(run)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workflows/manual-summary/runs', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ content: 'Original source' }),
    }))
  })

  it('shows the final result, execution steps, models, and source', async () => {
    const user = userEvent.setup()
    render(<RunDetails run={testRun()} />)

    expect(screen.getByText('Release summary')).toBeInTheDocument()
    expect(screen.getByText('Ship on Friday')).toBeInTheDocument()
    expect(screen.getByText(/summarize_text · qwen3:4b/)).toBeInTheDocument()
    expect(screen.getAllByText('1 attempt')).toHaveLength(3)
    await user.click(screen.getAllByText('Inspect input, output, and artifacts')[0])
    expect(screen.getByText(/"content": "Original source"/)).toBeInTheDocument()
    await user.click(screen.getByText('Original request'))
    expect(screen.getByText('Original source')).toBeInTheDocument()
  })

  it('shows the exact Slack source messages and context version', () => {
    const run = testRun()
    run.request = {
      ...run.request,
      source: 'slack',
      type: 'slack_thread',
      metadata: { context_version: '3', channel_id: 'C1' },
      sources: [{
        id: 'message_1', kind: 'slack_message', external_id: '1710000000.000001', author_id: 'U1',
        content: 'Slack source text', occurred_at: '2026-08-02T00:00:00Z',
      }],
    }

    render(<RunDetails run={run} />)

    expect(screen.getByText('Source messages')).toBeInTheDocument()
    expect(screen.getByText('Version 3')).toBeInTheDocument()
    expect(screen.getByText('Slack source text')).toBeInTheDocument()
    expect(screen.getByText(/U1/)).toBeInTheDocument()
  })

  it('offers retry for failed runs and cancellation for active runs', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    const failed = { ...testRun(), status: 'failed' as const, error_code: 'timeout', error_message: 'Model timed out.' }
    const { rerender } = render(<RunDetails run={failed} onRetry={onRetry} />)
    await user.click(screen.getByRole('button', { name: 'Retry failed steps' }))
    expect(onRetry).toHaveBeenCalledWith('run_1')

    const onCancel = vi.fn()
    rerender(<RunDetails run={{ ...testRun(), status: 'running' }} onCancel={onCancel} />)
    await user.click(screen.getByRole('button', { name: 'Cancel run' }))
    expect(onCancel).toHaveBeenCalledWith('run_1')
  })

  it('renders a pending API response when artifacts are not present yet', () => {
    const pending = { ...testRun(), status: 'pending' as const }
    Reflect.deleteProperty(pending, 'artifacts')

    render(<RunDetails run={pending as WorkflowRun} />)

    expect(screen.getByText('pending')).toBeInTheDocument()
    expect(screen.getByText('Execution steps')).toBeInTheDocument()
  })

  it('calls the controlled lifecycle endpoints', async () => {
    const run = testRun()
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify(run), {
      status: 202, headers: { 'Content-Type': 'application/json' },
    })))
    vi.stubGlobal('fetch', fetchMock)

    await retryWorkflowRun(run.id)
    await cancelWorkflowRun(run.id)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/workflow-runs/run_1/retry', { method: 'POST' })
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/workflow-runs/run_1/cancel', { method: 'POST' })
  })

  it('shows synchronized Slack attachment processing state', async () => {
    const fetchMock = vi.fn().mockImplementation((input: string) => {
      const body = input.includes('/attachments')
        ? { attachments: [{ id: 'attachment_1', message_id: 'message_1', slack_file_id: 'F1', filename: 'plan.pdf', mime_type: 'application/pdf', size_bytes: 2048, download_status: 'pending', extraction_status: 'pending', is_removed: false }] }
        : input.includes('/messages')
          ? { messages: [{ id: 'message_1', slack_timestamp: '1710000000.000001', author_id: 'U1', text: 'Review the plan', is_parent: true, is_deleted: false }] }
          : { threads: [{ id: 'thread_1', workspace_id: 'T1', channel_id: 'C1', thread_ts: '1710000000.000001', context_version: 2, sync_status: 'dirty', requested_analysis_version: 2, updated_at: '2026-08-02T00:00:00Z' }] }
      return Promise.resolve(new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQuery(<SlackThreadDashboard />)

    expect(await screen.findByText('plan.pdf')).toBeInTheDocument()
    expect(screen.getByText('Download: pending')).toBeInTheDocument()
    expect(screen.getByText('Extraction: pending')).toBeInTheDocument()
    expect(screen.getByText('Review the plan')).toBeInTheDocument()
  })
})
