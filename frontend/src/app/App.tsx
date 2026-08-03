import { Dashboard } from '../features/dashboard/Dashboard'
import { SlackThreadDashboard } from '../features/slack/SlackThreadDashboard'
import { WorkflowDashboard } from '../features/workflows/WorkflowDashboard'

export function App() {
  return (
    <main className="page">
      <div className="workspace">
        <Dashboard />
        <SlackThreadDashboard />
        <WorkflowDashboard />
      </div>
    </main>
  )
}
