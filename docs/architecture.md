# Architecture

The product is a local, event-driven AI orchestration system. The dashboard is an output and management interface; future connectors submit normalized requests through the same application boundary.

The system status API reports whether the Ollama API is reachable and whether the configured general model is installed.

The current execution path follows a strict dependency direction:

```text
UI trigger
  -> normalized request
  -> validated manual-summary workflow
  -> persisted pending run
  -> asynchronous single-worker executor
  -> registered capabilities
  -> summary service / Ollama client
  -> structured artifacts
  -> SQLite persistence
  -> dashboard result
```

Go controls which workflows, capabilities, models, input mappings, timeouts, retries, and execution transitions are permitted. Model output cannot introduce capabilities, commands, filesystem access, or network calls. The current capability registry contains `summarize_text`, `extract_action_items`, and deterministic `compose_dashboard_result` implementations.

SQLite stores workflow versions, normalized requests, runs, step runs, artifacts, model names, prompt versions, timestamps, errors, and execution events. Runs are persisted before execution begins. Request IDs are unique, so replaying the same normalized request returns its existing run without executing capabilities again.

One background worker atomically claims pending runs and executes model-heavy steps sequentially. Cancellation is committed before the active execution context is interrupted, and persistence guards prevent a late capability result from overwriting the cancelled state. Retrying a failed run preserves completed upstream steps and artifacts, resets only failed and downstream work, and records cumulative attempts. Interrupted pending or running work is reset to pending and requeued at startup without duplicating completed artifacts.

The existing direct-summary API remains available for compatibility, but the workflow API is the primary product path. Formal multi-model routing, connector triggers, workflow selection, temporary planning, approval, and publishing destinations are introduced only in later milestones.
