# Architecture

The browser calls the Go REST API. The API owns configuration and checks the local Ollama HTTP service. Future data-source connectors and summarization workflows belong behind service boundaries rather than in HTTP handlers.

No connector, inference, persistence, authentication, or background-processing behavior is included in the initial milestone.
