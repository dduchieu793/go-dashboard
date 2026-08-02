import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import {
  cancelWorkflowRun,
  getWorkflowRun,
  listWorkflowRuns,
  retryWorkflowRun,
  type WorkflowRun,
} from '../../shared/api/workflows'
import { ManualWorkflowForm } from './ManualWorkflowForm'
import { RunDetails } from './RunDetails'

export function WorkflowDashboard() {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const queryClient = useQueryClient()
  const runsQuery = useQuery({
    queryKey: ['workflow-runs'],
    queryFn: ({ signal }) => listWorkflowRuns(signal),
    refetchInterval: (query) => query.state.data?.some((run) => run.status === 'pending' || run.status === 'running') ? 1_000 : false,
  })
  const runQuery = useQuery({
    queryKey: ['workflow-run', selectedId],
    queryFn: ({ signal }) => getWorkflowRun(selectedId!, signal),
    enabled: Boolean(selectedId),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'pending' || status === 'running' ? 500 : false
    },
  })
  const retryMutation = useMutation({
    mutationFn: retryWorkflowRun,
    onSuccess: refreshRun,
  })
  const cancelMutation = useMutation({
    mutationFn: cancelWorkflowRun,
    onSuccess: refreshRun,
  })

  function refreshRun(run: WorkflowRun) {
    queryClient.setQueryData(['workflow-run', run.id], run)
    void queryClient.invalidateQueries({ queryKey: ['workflow-runs'] })
  }

  function created(run: WorkflowRun) {
    setSelectedId(run.id)
    refreshRun(run)
  }

  useEffect(() => {
    if (!selectedId && runsQuery.data?.length) setSelectedId(runsQuery.data[0].id)
  }, [runsQuery.data, selectedId])

  useEffect(() => {
    const status = runQuery.data?.status
    if (status && status !== 'pending' && status !== 'running') {
      void queryClient.invalidateQueries({ queryKey: ['workflow-runs'] })
    }
  }, [queryClient, runQuery.data?.status])

  const selected = runQuery.data ?? runsQuery.data?.find((run) => run.id === selectedId) ?? null
  const operationError = retryMutation.error ?? cancelMutation.error

  return (
    <section className="orchestration" aria-labelledby="orchestration-title">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Local orchestration</p>
          <h2 id="orchestration-title">Workflow control center</h2>
        </div>
        <p>Every result is backed by a validated workflow, registered capabilities, and inspectable artifacts.</p>
      </div>

      <ManualWorkflowForm onCreated={created} />

      <div className="workflow-layout">
        <aside className="run-list" aria-label="Workflow runs">
          <div className="panel-heading"><h3>Recent runs</h3><span>{runsQuery.data?.length ?? 0}</span></div>
          {runsQuery.isLoading && <p className="muted">Loading runs…</p>}
          {runsQuery.isError && <p className="error" role="alert">Unable to load workflow runs.</p>}
          {runsQuery.data?.length === 0 && <p className="muted">No workflow runs yet.</p>}
          {runsQuery.data?.map((run) => (
            <button className={`run-list-item ${selectedId === run.id ? 'selected' : ''}`} key={run.id} onClick={() => setSelectedId(run.id)}>
              <span><strong>{run.workflow_id}</strong><small>{new Date(run.created_at).toLocaleString()}</small></span>
              <span className={`run-status status-${run.status}`}>{run.status}</span>
            </button>
          ))}
        </aside>
        <RunDetails
          run={selected}
          isRefreshing={runQuery.isFetching}
          operationPending={retryMutation.isPending || cancelMutation.isPending}
          operationError={operationError?.message}
          onRetry={(id) => retryMutation.mutate(id)}
          onCancel={(id) => cancelMutation.mutate(id)}
        />
      </div>
    </section>
  )
}
