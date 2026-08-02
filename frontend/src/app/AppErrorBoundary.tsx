import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }
type State = { failed: boolean }

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { failed: false }

  static getDerivedStateFromError(): State {
    return { failed: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Dashboard rendering failed', error, info)
  }

  render() {
    if (this.state.failed) {
      return (
        <main className="page error-page">
          <section className="message error" role="alert">
            <p className="eyebrow">Dashboard error</p>
            <h1>Something went wrong.</h1>
            <p>The workflow data is still saved. Reload the dashboard to try again.</p>
            <button type="button" onClick={() => window.location.reload()}>Reload dashboard</button>
          </section>
        </main>
      )
    }
    return this.props.children
  }
}
