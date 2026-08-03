# Architecture

The product is a local, event-driven AI orchestration system. The dashboard is an output and management interface; manual input and Slack submit normalized requests through the same application boundary.

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

The Slack text path precedes that shared workflow path:

```text
signed Slack event
  -> event deduplication
  -> resolve local thread identity
  -> initial conversations.replies synchronization when missing
  -> otherwise transactional incremental message upsert
  -> increment context version only for material changes
  -> bounded chronological text context with source references
  -> normalized slack_thread request
  -> existing manual-summary workflow
```

Slack thread and message records are authoritative; the system does not store a permanently concatenated thread as source-of-truth. Deleted messages remain persisted for traceability but are excluded from active context. Each workflow request records its Slack thread ID, channel, thread timestamp, context version, and the exact bounded source messages analyzed. The request ID is deterministic for a thread/context-version pair, preventing duplicate runs for repeated events.

Slack files are represented as attachment metadata records associated with their exact message and thread. New, updated, removed, and restored metadata is handled in the same transaction as its message and advances the context version once. Download and extraction statuses begin as `pending`. Local paths are server-only fields and are never serialized through the API. The extracted-document table and repository boundary exist for later extractors, but this milestone does not download or inspect file content.

Slack event bodies are authenticated with Slack's HMAC signing secret and a five-minute replay window. Event IDs are claimed transactionally. Completed events are ignored, active duplicate delivery receives an error so Slack retries, and stale claims can be recovered. Bot tokens are used only in the server-side Slack API adapter and never returned in APIs, logs, or artifacts.

Go controls which workflows, capabilities, models, input mappings, timeouts, retries, and execution transitions are permitted. Model output cannot introduce capabilities, commands, filesystem access, or network calls. The current capability registry contains `summarize_text`, `classify_text`, `extract_action_items`, and deterministic `compose_dashboard_result` implementations. Each capability declares schemas, whether it uses an LLM, and its default model profile.

Workflow selection is deterministic: an explicit enabled workflow ID wins, otherwise a configured normalized-request-type rule is used. The selected workflow, method, and reason are written into request metadata before persistence. The current executor serves the `manual-summary` workflow; future executors can be added behind the same application boundary.

Model routing is also deterministic. `summarize_text` and `classify_text` use the general profile, `extract_action_items` uses the reasoning profile, and workflow steps may explicitly override a capability's default profile. The coding profile is installed and visible but remains unloaded unless a coding capability is registered and invoked.

SQLite stores workflow versions, normalized requests, runs, step runs, artifacts, model names, prompt versions, timestamps, errors, and execution events. Runs are persisted before execution begins. Request IDs are unique, so replaying the same normalized request returns its existing run without executing capabilities again.

One background worker atomically claims pending runs and executes model-heavy steps sequentially. Cancellation is committed before the active execution context is interrupted, and persistence guards prevent a late capability result from overwriting the cancelled state. Retrying a failed run preserves completed upstream steps and artifacts, resets only failed and downstream work, and records cumulative attempts. Interrupted pending or running work is reset to pending and requeued at startup without duplicating completed artifacts.

The existing direct-summary API remains available for compatibility, but the workflow API is the primary product path. Slack text ingestion now calls this boundary, and Slack file metadata is persisted independently. File processing follows later in an explicit order: secure download, CSV, XLSX, PDF text, then image OCR.
