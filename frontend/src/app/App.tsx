import { Dashboard } from '../features/dashboard/Dashboard'
import { WorkflowDashboard } from '../features/workflows/WorkflowDashboard'

export function App() {
  return (
    <main className="page">
      <div className="workspace">
        <Dashboard />
        <WorkflowDashboard />
      </div>
    </main>
  )
}
