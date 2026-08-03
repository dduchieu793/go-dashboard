# Architecture

The product is a local, event-driven AI orchestration system. The dashboard is an output and management interface; future connectors submit normalized requests through the same application boundary.

The readiness APIs report the backend, each configured Ollama model profile, registered capabilities, and enabled workflows. The general profile routes to Qwen3, coding routes to Qwen Coder, and deep reasoning routes to DeepSeek; profile names and models are environment-configurable.

The current execution path follows a strict dependency direction:

```text
UI trigger
  -> normalized request
  -> deterministic workflow selector
  -> versioned workflow registry
  -> validated manual-summary workflow
  -> persisted pending run
  -> asynchronous single-worker executor
  -> registered capabilities
  -> capability-to-model router / Ollama client
  -> structured artifacts
  -> SQLite persistence
  -> dashboard result
```

Go controls which workflows, capabilities, models, input mappings, timeouts, retries, and execution transitions are permitted. Model output cannot introduce capabilities, commands, filesystem access, or network calls. The current capability registry contains `summarize_text`, `classify_text`, `extract_action_items`, and deterministic `compose_dashboard_result` implementations. Each capability declares schemas, whether it uses an LLM, and its default model profile.

Workflow selection is deterministic: an explicit enabled workflow ID wins, otherwise a configured normalized-request-type rule is used. The selected workflow, method, and reason are written into request metadata before persistence. The current executor serves the `manual-summary` workflow; future executors can be added behind the same application boundary.

Model routing is also deterministic. `summarize_text` and `classify_text` use the general profile, `extract_action_items` uses the reasoning profile, and workflow steps may explicitly override a capability's default profile. The coding profile is installed and visible but remains unloaded unless a coding capability is registered and invoked.

SQLite stores workflow versions, normalized requests, runs, step runs, artifacts, model names, prompt versions, timestamps, errors, and execution events. Runs are persisted before execution begins. Request IDs are unique, so replaying the same normalized request returns its existing run without executing capabilities again.

One background worker atomically claims pending runs and executes model-heavy steps sequentially. Cancellation is committed before the active execution context is interrupted, and persistence guards prevent a late capability result from overwriting the cancelled state. Retrying a failed run preserves completed upstream steps and artifacts, resets only failed and downstream work, and records cumulative attempts. Interrupted pending or running work is reset to pending and requeued at startup without duplicating completed artifacts.

The existing direct-summary API remains available for compatibility, but the workflow API is the primary product path. The next product slice adds Slack event ingestion, local thread synchronization, incremental replies, and versioned text context before calling this existing workflow boundary. File extraction follows later in an explicit order: metadata, secure download, CSV, XLSX, PDF text, then image OCR.
