import type { WorkflowRun } from '../../shared/api/workflows'

type RunDetailsProps = {
  run: WorkflowRun | null
  isRefreshing?: boolean
  operationPending?: boolean
  operationError?: string
  onRetry?: (id: string) => void
  onCancel?: (id: string) => void
}

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '—'
}

function formatJSON(value?: Record<string, unknown>) {
  return value ? JSON.stringify(value, null, 2) : 'No data recorded.'
}

export function RunDetails({
  run,
  isRefreshing = false,
  operationPending = false,
  operationError,
  onRetry,
  onCancel,
}: RunDetailsProps) {
  if (!run) {
    return <section className="run-details empty-state">Select a workflow run to inspect its steps and artifacts.</section>
  }
  const steps = run.steps ?? []
  const artifacts = run.artifacts ?? []
  const finalArtifact = artifacts.find((artifact) => artifact.id === run.final_artifact_id)
  const final = finalArtifact?.content as { summary?: string; action_items?: string; models?: string[] } | undefined

  return (
    <section className="run-details" aria-labelledby="run-details-title">
      <div className="run-details-header">
        <div>
          <p className="eyebrow">Run details</p>
          <h2 id="run-details-title">{run.workflow_id}</h2>
        </div>
        <div className="run-state">
          {isRefreshing && (run.status === 'pending' || run.status === 'running') && <span className="live-indicator">Live</span>}
          <span className={`run-status status-${run.status}`}>{run.status.replace('_', ' ')}</span>
        </div>
      </div>

      <div className="run-actions">
        {run.status === 'failed' && onRetry && <button type="button" disabled={operationPending} onClick={() => onRetry(run.id)}>Retry failed steps</button>}
        {(run.status === 'pending' || run.status === 'running') && onCancel && (
          <button type="button" className="button-danger" disabled={operationPending} onClick={() => onCancel(run.id)}>Cancel run</button>
        )}
      </div>

      {operationError && <div className="message error compact" role="alert">{operationError}</div>}

      <dl className="run-metadata">
        <div><dt>Run ID</dt><dd>{run.id}</dd></div>
        <div><dt>Started</dt><dd>{formatTime(run.started_at ?? run.created_at)}</dd></div>
        <div><dt>Source</dt><dd>{run.request.source}</dd></div>
      </dl>

      {(run.status === 'failed' || run.status === 'cancelled') && (
        <div className="message error compact"><strong>{run.error_code}</strong><span>{run.error_message}</span></div>
      )}

      {final && (
        <div className="final-result">
          <p className="eyebrow">Final result</p>
          <h3>Summary</h3>
          <p>{final.summary}</p>
          <h3>Action items</h3>
          <p>{final.action_items}</p>
          {final.models && <p className="artifact-meta">Models: {final.models.join(', ')}</p>}
        </div>
      )}

      <div className="step-list">
        <h3>Execution steps</h3>
        {steps.map((step, index) => {
          const stepArtifacts = artifacts.filter((artifact) => artifact.step_run_id === step.id)
          return (
            <article className="step-row" key={step.id}>
              <div className="step-summary">
                <span className="step-index">{index + 1}</span>
                <div>
                  <strong>{step.step_id}</strong>
                  <p>{step.capability}{step.model ? ` · ${step.model}` : ''}</p>
                  <p>{step.attempt} {step.attempt === 1 ? 'attempt' : 'attempts'}</p>
                  {step.error_message && <p className="error">{step.error_message}</p>}
                </div>
                <span className={`run-status status-${step.status}`}>{step.status}</span>
              </div>
              <details className="execution-data">
                <summary>Inspect input, output, and artifacts</summary>
                <div className="data-grid">
                  <section><h4>Input</h4><pre>{formatJSON(step.input)}</pre></section>
                  <section><h4>Output</h4><pre>{formatJSON(step.output)}</pre></section>
                </div>
                <div className="artifact-list">
                  <h4>Artifacts ({stepArtifacts.length})</h4>
                  {stepArtifacts.length === 0 && <p className="muted">No artifact produced.</p>}
                  {stepArtifacts.map((artifact) => (
                    <section className="artifact-card" key={artifact.id}>
                      <div><strong>{artifact.type}</strong><span>{artifact.model || 'Go'}{artifact.prompt_version ? ` · ${artifact.prompt_version}` : ''}</span></div>
                      <pre>{formatJSON(artifact.content)}</pre>
                    </section>
                  ))}
                </div>
              </details>
            </article>
          )
        })}
      </div>

      <details className="source-details">
        <summary>Original request</summary>
        <p className="original-content">{run.request.content}</p>
      </details>
    </section>
  )
}
