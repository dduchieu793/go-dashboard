import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import {
  analyzeSlackThread,
  listSlackAttachments,
  listSlackMessages,
  listSlackThreads,
  refreshSlackThread,
} from '../../shared/api/slack'

function fileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function SlackThreadDashboard() {
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const queryClient = useQueryClient()
  const threads = useQuery({ queryKey: ['slack-threads'], queryFn: ({ signal }) => listSlackThreads(signal) })
  const messages = useQuery({
    queryKey: ['slack-messages', selectedID],
    queryFn: ({ signal }) => listSlackMessages(selectedID!, signal),
    enabled: Boolean(selectedID),
  })
  const attachments = useQuery({
    queryKey: ['slack-attachments', selectedID],
    queryFn: ({ signal }) => listSlackAttachments(selectedID!, signal),
    enabled: Boolean(selectedID),
  })
  const refresh = useMutation({
    mutationFn: refreshSlackThread,
    onSuccess: () => refreshSelection(),
  })
  const analyze = useMutation({
    mutationFn: analyzeSlackThread,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['workflow-runs'] })
      refreshSelection()
    },
  })

  function refreshSelection() {
    void queryClient.invalidateQueries({ queryKey: ['slack-threads'] })
    void queryClient.invalidateQueries({ queryKey: ['slack-messages', selectedID] })
    void queryClient.invalidateQueries({ queryKey: ['slack-attachments', selectedID] })
  }

  useEffect(() => {
    if (!selectedID && threads.data?.length) setSelectedID(threads.data[0].id)
  }, [selectedID, threads.data])

  const selected = threads.data?.find((thread) => thread.id === selectedID)
  const operationError = refresh.error ?? analyze.error

  return (
    <section className="slack-panel" aria-labelledby="slack-title">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Synchronized sources</p>
          <h2 id="slack-title">Slack threads</h2>
        </div>
        <p>Local message context and attachment processing state. Private file locations are never exposed.</p>
      </div>

      {threads.isLoading && <p className="muted">Loading Slack threads…</p>}
      {threads.isError && <p className="error" role="alert">Unable to load Slack threads.</p>}
      {threads.data?.length === 0 && <p className="slack-empty muted">No synchronized Slack threads yet.</p>}

      {threads.data && threads.data.length > 0 && (
        <div className="slack-layout">
          <aside className="slack-thread-list" aria-label="Slack threads">
            {threads.data.map((thread) => (
              <button key={thread.id} className={selectedID === thread.id ? 'selected' : ''} onClick={() => setSelectedID(thread.id)}>
                <span><strong>#{thread.channel_id}</strong><small>{thread.thread_ts}</small></span>
                <span className={`run-status status-${thread.sync_status}`}>{thread.sync_status}</span>
              </button>
            ))}
          </aside>

          <div className="slack-thread-detail">
            {selected && (
              <>
                <div className="slack-detail-heading">
                  <div>
                    <strong>Context version {selected.context_version}</strong>
                    <span>Requested analysis {selected.requested_analysis_version}</span>
                  </div>
                  <div>
                    <button type="button" disabled={refresh.isPending || analyze.isPending} onClick={() => refresh.mutate(selected.id)}>Refresh</button>
                    <button type="button" disabled={refresh.isPending || analyze.isPending} onClick={() => analyze.mutate(selected.id)}>Analyze</button>
                  </div>
                </div>
                {operationError && <p className="error" role="alert">{operationError.message}</p>}

                <div className="slack-source-grid">
                  <section>
                    <h3>Messages</h3>
                    {messages.isLoading && <p className="muted">Loading messages…</p>}
                    {messages.data?.map((message) => (
                      <article className={message.is_deleted ? 'removed' : ''} key={message.id}>
                        <div><strong>{message.author_id || 'Unknown author'}</strong><span>{message.is_parent ? 'Parent' : 'Reply'}{message.edited_at ? ' · edited' : ''}</span></div>
                        <p>{message.is_deleted ? 'Message deleted' : message.text}</p>
                      </article>
                    ))}
                  </section>
                  <section>
                    <h3>Attachments</h3>
                    {attachments.isLoading && <p className="muted">Loading attachments…</p>}
                    {attachments.data?.length === 0 && <p className="muted">No attachments.</p>}
                    {attachments.data?.map((attachment) => (
                      <article className={attachment.is_removed ? 'removed' : ''} key={attachment.id}>
                        <div><strong>{attachment.filename}</strong><span>{fileSize(attachment.size_bytes)}</span></div>
                        <p>{attachment.mime_type || 'Unknown type'}</p>
                        <div className="attachment-statuses">
                          <span>Download: {attachment.download_status}</span>
                          <span>Extraction: {attachment.extraction_status}</span>
                        </div>
                      </article>
                    ))}
                  </section>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </section>
  )
}
