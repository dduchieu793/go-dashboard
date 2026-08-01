# Architecture

The browser calls the Go REST API. The API owns configuration and checks the local Ollama HTTP service. Future data-source connectors and summarization workflows belong behind service boundaries rather than in HTTP handlers.

The system status API reports two independent facts: whether the Ollama HTTP API is reachable and whether the configured model appears in Ollama's installed-model list. A model is not required to run the Phase 0 dashboard.

No connector, inference, persistence, authentication, or background-processing behavior is included in the initial milestone.
