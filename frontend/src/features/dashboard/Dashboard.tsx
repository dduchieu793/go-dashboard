import { useEffect, useState } from 'react'
import { getHealth, getLLMStatus, type Health, type LLMStatus } from '../../shared/api/system'
import { StatusCard } from '../../shared/components/StatusCard'

type DashboardState =
  | { phase: 'loading' }
  | { phase: 'ready'; health: Health; llm: LLMStatus }
  | { phase: 'error'; message: string }

export function Dashboard() {
  const [state, setState] = useState<DashboardState>({ phase: 'loading' })

  useEffect(() => {
    const controller = new AbortController()

    Promise.all([getHealth(controller.signal), getLLMStatus(controller.signal)])
      .then(([health, llm]) => setState({ phase: 'ready', health, llm }))
      .catch((error: unknown) => {
        if (!controller.signal.aborted) {
          setState({
            phase: 'error',
            message: error instanceof Error ? error.message : 'Unable to load system status',
          })
        }
      })

    return () => controller.abort()
  }, [])

  return (
    <main className="page">
      <section className="dashboard">
        <header>
          <p className="eyebrow">Local intelligence, clear overview</p>
          <h1>AI Summary Dashboard</h1>
          <p className="intro">System readiness for your private summary workspace.</p>
        </header>

        {state.phase === 'loading' && <div className="message" role="status">Loading system status…</div>}
        {state.phase === 'error' && (
          <div className="message error" role="alert">
            <strong>Could not reach the backend.</strong>
            <span>{state.message}</span>
          </div>
        )}
        {state.phase === 'ready' && (
          <div className="status-grid">
            <StatusCard label="Backend status" available={state.health.status === 'ok'} />
            <StatusCard
              label="Ollama runtime"
              available={state.llm.available}
              detail={state.llm.available ? 'API is reachable' : 'API is unreachable'}
            />
            <StatusCard
              label="Configured model"
              available={state.llm.model_available}
              detail={state.llm.model}
            />
          </div>
        )}
      </section>
    </main>
  )
}
