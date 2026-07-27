# AI Summary Dashboard

AI Summary Dashboard is a configurable web application that will collect information from external services, send selected content to a local LLM, and present concise summaries. This milestone establishes the project architecture only; connectors, prompts, and summarization are intentionally not implemented.

## Technology stack

- Backend: Go 1.25+, `net/http`, Chi, `slog`
- Frontend: React, TypeScript, Vite
- Local AI runtime: Ollama with `llama3.2:1b`
- API style: REST

## Folder structure

```text
.
├── backend/
│   ├── cmd/api/              # Application entry point
│   └── internal/
│       ├── config/           # Environment configuration
│       ├── httpapi/          # Router and HTTP handlers
│       ├── llm/              # LLM connectivity abstraction
│       └── summary/          # Future summary service
├── frontend/
│   └── src/
│       ├── app/              # Application composition
│       ├── features/dashboard/
│       └── shared/           # API and shared components
├── docs/
└── docker-compose.yml
```

## Prerequisites

- Go 1.25 or newer
- Node.js 20 or newer and npm
- Docker with Docker Compose (for Ollama)

## Setup

1. Start Ollama:

   ```sh
   docker compose up -d
   docker compose exec ollama ollama pull llama3.2:1b
   ```

2. Configure the backend. Copy `backend/.env.example` to `backend/.env`, then export those values in your shell. The Go process reads environment variables directly.

3. Install frontend dependencies:

   ```sh
   cd frontend
   npm install
   ```

## Run

Start the backend from `backend` after exporting its environment variables:

```sh
go run ./cmd/api
```

For PowerShell development:

```powershell
$env:APP_ENV="development"
$env:HTTP_PORT="8080"
$env:OLLAMA_BASE_URL="http://localhost:11434"
$env:OLLAMA_MODEL="llama3.2:1b"
$env:FRONTEND_ORIGIN="http://localhost:5173"
go run ./cmd/api
```

In another terminal, start the frontend:

```sh
cd frontend
npm run dev
```

Open http://localhost:5173.

## API endpoints

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Backend process health |
| `GET` | `/api/v1/system/llm-status` | Ollama connectivity and configured model |

## Current milestone

The skeleton includes configuration validation, structured logging, graceful shutdown, HTTP routing, a health check, an Ollama connectivity check, and a dashboard that displays both statuses. Data-source connectors, persistence, authentication, prompts, and AI inference are out of scope.
