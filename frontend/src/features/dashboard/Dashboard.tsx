import { useEffect, useState } from 'react'
import {
  getCapabilities,
  getHealth,
  getLLMStatus,
  getModelStatuses,
  getWorkflows,
  type CapabilityMetadata,
  type Health,
  type LLMStatus,
  type ModelProfileStatus,
  type WorkflowDefinition,
} from '../../shared/api/system'
import { StatusCard } from '../../shared/components/StatusCard'

type DashboardState =
  | { phase: 'loading' }
  | {
      phase: 'ready'
      health: Health
      llm: LLMStatus
      profiles: ModelProfileStatus[]
      capabilities: CapabilityMetadata[]
      workflows: WorkflowDefinition[]
    }
  | { phase: 'error'; message: string }

export function Dashboard() {
  const [state, setState] = useState<DashboardState>({ phase: 'loading' })

  useEffect(() => {
    const controller = new AbortController()

    Promise.all([
      getHealth(controller.signal),
      getLLMStatus(controller.signal),
      getModelStatuses(controller.signal),
      getCapabilities(controller.signal),
      getWorkflows(controller.signal),
    ])
      .then(([health, llm, models, capabilityCatalog, workflowCatalog]) => setState({
        phase: 'ready',
        health,
        llm,
        profiles: models.profiles,
        capabilities: capabilityCatalog.capabilities,
        workflows: workflowCatalog.workflows,
      }))
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
      <section className="dashboard system-panel" aria-labelledby="dashboard-title">
        <header>
          <p className="eyebrow">Local intelligence, clear overview</p>
          <h1 id="dashboard-title">AI Summary Dashboard</h1>
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
            {state.profiles.map((profile) => (
              <StatusCard
                key={profile.name}
                label={`${profile.name[0].toUpperCase()}${profile.name.slice(1)} model`}
                available={profile.available && profile.model_available}
                detail={`${profile.model}${profile.capabilities.length > 0 ? ` · ${profile.capabilities.length} capabilities` : ' · ready for routing'}`}
              />
            ))}
            <StatusCard
              label="Capability catalog"
              available={state.capabilities.length > 0}
              detail={`${state.capabilities.length} registered`}
            />
            <StatusCard
              label="Workflow catalog"
              available={state.workflows.length > 0}
              detail={`${state.workflows.length} enabled`}
            />
          </div>
        )}
      </section>
  )
}
