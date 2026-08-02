import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { startManualWorkflow, type WorkflowRun } from '../../shared/api/workflows'

const maxContentLength = 50_000

type ManualWorkflowFormProps = {
  onCreated: (run: WorkflowRun) => void
}

export function ManualWorkflowForm({ onCreated }: ManualWorkflowFormProps) {
  const [content, setContent] = useState('')
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: (value: string) => startManualWorkflow(value),
    onSuccess: (run) => {
      onCreated(run)
      void queryClient.invalidateQueries({ queryKey: ['workflow-runs'] })
    },
  })
  const trimmed = content.trim()

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!trimmed || content.length > maxContentLength || mutation.isPending) return
    mutation.mutate(trimmed)
  }

  return (
    <section className="workflow-trigger" aria-labelledby="new-run-title">
      <div>
        <p className="eyebrow">New workflow run</p>
        <h2 id="new-run-title">Process manual content</h2>
        <p className="muted">Runs a controlled workflow that summarizes, extracts actions, and composes a dashboard result.</p>
      </div>
      <form onSubmit={submit}>
        <label htmlFor="workflow-content">Source content</label>
        <textarea
          id="workflow-content"
          value={content}
          maxLength={maxContentLength + 1}
          onChange={(event) => setContent(event.target.value)}
          placeholder="Paste a message, meeting note, or update…"
          rows={7}
        />
        <div className={`field-meta ${content.length > maxContentLength ? 'error' : ''}`}>
          <span>{content.length > maxContentLength ? 'Content is too long.' : 'The original request stays linked to this run.'}</span>
          <span>{content.length.toLocaleString()} / {maxContentLength.toLocaleString()}</span>
        </div>
        <button type="submit" disabled={!trimmed || content.length > maxContentLength || mutation.isPending}>
          {mutation.isPending ? 'Running workflow…' : 'Start workflow'}
        </button>
      </form>
      {mutation.isError && <div className="message error compact" role="alert">{mutation.error.message}</div>}
    </section>
  )
}
