# AI Summary Dashboard

AI Summary Dashboard is a local, controlled AI orchestration system. Requests from the UI and future connectors are normalized, executed through predefined workflows and registered capabilities, persisted as structured runs and artifacts, and presented in a management dashboard.

## Technology stack

- Backend: Go 1.25+, `net/http`, Chi, SQLite, `slog`
- Frontend: React, TypeScript, Vite, TanStack Query
- Local AI runtime: Ollama with task-specific general, coding, and reasoning profiles
- API style: REST

## Folder structure

```text
.
├── backend/
│   ├── cmd/api/              # Application entry point
│   └── internal/
│       ├── capability/       # Registered executable capabilities
│       ├── config/           # Environment configuration
│       ├── httpapi/          # Router and HTTP handlers
│       ├── llm/              # LLM connectivity abstraction
│       ├── modelrouter/      # Capability-to-model profile routing
│       ├── orchestration/    # Deterministic workflow selection boundary
│       ├── storage/          # SQLite migrations and workflow persistence
│       ├── summary/          # Summary service and prompt construction
│       ├── trigger/          # Normalized incoming requests
│       └── workflow/         # Definitions, runs, validation, and execution
├── frontend/
│   └── src/
│       ├── app/              # Application composition
│       ├── features/         # Status and workflow-run experiences
│       └── shared/           # API clients and shared components
├── docs/
└── docker-compose.yml
```

## Prerequisites

- Go 1.25 or newer
- Node.js 20 or newer and Corepack
- Ollama installed locally, or Docker with Docker Compose

## Setup

1. Start Ollama and install the model profiles:

   ```sh
   ollama pull qwen3:4b
   ollama pull qwen2.5-coder:7b
   ollama pull deepseek-r1:8b
   ```

   To use the optional Docker runtime instead:

   ```sh
   docker compose up -d
   docker compose exec ollama ollama pull qwen3:4b
   ```

2. Copy `backend/.env.example` to `backend/.env`.

3. Install frontend dependencies:

   ```sh
   cd frontend
   corepack pnpm install
   ```

## Run

Start the backend from `backend` after exporting the environment file:

```sh
set -a
source .env
set +a
go run ./cmd/api
```

For PowerShell development:

```powershell
$env:APP_ENV="development"
$env:HTTP_PORT="8080"
$env:OLLAMA_BASE_URL="http://localhost:11434"
$env:OLLAMA_MODEL="qwen3:4b"
$env:OLLAMA_CODING_MODEL="qwen2.5-coder:7b"
$env:OLLAMA_REASONING_MODEL="deepseek-r1:8b"
$env:OLLAMA_GENERATE_TIMEOUT="60s"
$env:OLLAMA_KEEP_ALIVE="-1m"
$env:OLLAMA_CODING_KEEP_ALIVE="15m"
$env:OLLAMA_REASONING_KEEP_ALIVE="0s"
$env:FRONTEND_ORIGIN="http://localhost:5173"
$env:DATABASE_PATH="./data/dashboard.db"
go run ./cmd/api
```

In another terminal, start the frontend:

```sh
cd frontend
corepack pnpm dev
```

Open http://localhost:5173.

## API endpoints

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Backend process health |
| `GET` | `/api/v1/system/llm-status` | Ollama and configured-model status |
| `GET` | `/api/v1/system/model-statuses` | Readiness and capability mapping for every model profile |
| `GET` | `/api/v1/capabilities` | Registered capability metadata and schemas |
| `GET` | `/api/v1/workflows` | Enabled workflow definitions and routed steps |
| `POST` | `/api/v1/summaries/generate` | Legacy direct manual summary |
| `POST` | `/api/v1/workflows/manual-summary/runs` | Start the controlled manual-summary workflow |
| `GET` | `/api/v1/workflow-runs` | List persisted workflow runs |
| `GET` | `/api/v1/workflow-runs/{id}` | Inspect steps and artifacts for a run |
| `POST` | `/api/v1/workflow-runs/{id}/retry` | Resume failed and downstream steps safely |
| `POST` | `/api/v1/workflow-runs/{id}/cancel` | Cancel pending or running work |

### Start a workflow run

The current workflow uses Qwen3 for summarization, DeepSeek for action extraction, and Go for deterministic composition:

```sh
curl --request POST http://localhost:8080/api/v1/workflows/manual-summary/runs \
  --header 'Content-Type: application/json' \
  --data '{"content":"Text to process"}'
```

The start endpoint returns `202 Accepted` with a persisted pending run. A single background worker executes runs sequentially, while the dashboard polls for live state. The completed run contains the normalized request, ordered step runs, model-backed artifacts, prompt versions, and final dashboard artifact. `OLLAMA_GENERATE_TIMEOUT` defaults to `60s`; `OLLAMA_KEEP_ALIVE` defaults to `-1m` so the general model remains loaded.

Failed runs can be retried from the dashboard or retry endpoint. Completed upstream steps and artifacts are reused; only failed and downstream work executes again. Pending or running work can be cancelled, including the active Ollama HTTP request. Interrupted work remains recoverable and is requeued when the API starts again.

## Current milestone

The current milestone provides a versioned workflow registry, deterministic request-to-workflow selection with persisted audit metadata, four registered capabilities (including `classify_text`), config-driven multi-model routing, readiness catalogs, asynchronous single-worker execution, bounded retries, explicit retry and cancellation, SQLite persistence, startup recovery, and a workflow-run dashboard. The next enhancement is Slack thread synchronization and versioned text context. File extraction, dynamic planning, arbitrary tools, human approval, parallel execution, and multiple destinations remain later work.
