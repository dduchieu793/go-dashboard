# Slack text ingestion

The first Slack vertical slice synchronizes thread text locally and submits the current context to the existing controlled summary workflow. It does not download attachments or publish results back to Slack.

## Slack app configuration

Create or select a Slack app, install it to the target workspace, and configure:

- Bot token scopes appropriate to the conversations the app may read: `channels:history`, `groups:history`, `im:history`, and/or `mpim:history`.
- Event subscriptions for the same conversation types: `message.channels`, `message.groups`, `message.im`, and/or `message.mpim`.
- Request URL: `https://YOUR_PUBLIC_HOST/api/v1/slack/events`.

The bot must be a member of private channels it needs to synchronize. Local development requires an HTTPS tunnel that forwards to the backend port.

Set these backend variables without committing their values:

```sh
SLACK_SIGNING_SECRET=your-app-signing-secret
SLACK_BOT_TOKEN=xoxb-your-token
```

Optional controls:

```sh
SLACK_API_BASE_URL=https://slack.com/api
SLACK_REQUEST_TIMEOUT=15s
SLACK_MAX_CONTEXT_MESSAGES=200
SLACK_MAX_CONTEXT_CHARS=50000
ATTACHMENT_STORAGE_PATH=./data/attachments
```

Both credentials must be configured together. When absent, manual workflows remain available and the Slack event endpoint returns `503`.

## Processing behavior

For a previously unseen thread, the server calls `conversations.replies`, follows pagination, and persists the parent and replies in one local thread. Normal new replies, edits, and deletions update SQLite directly without fetching the whole thread again. Manual refresh is available for reconciliation.

Material changes increment the thread context version once per transaction. Repeated events and unchanged message payloads do not increment it. Deleted messages are soft-deleted and excluded from LLM context.

The context builder always retains the parent when available, selects the latest active replies within configured message/character bounds, restores chronological order, and includes stable Slack source references. Each thread/context version maps to a deterministic workflow request ID.

## Local APIs

```text
GET  /api/v1/threads
GET  /api/v1/threads/{id}
GET  /api/v1/threads/{id}/messages
POST /api/v1/threads/{id}/refresh
POST /api/v1/threads/{id}/analyze
```

Workflow run details include `request.sources`, which is the exact message subset used for that run. This allows the dashboard to display source messages even if the live Slack thread changes later.

Slack event and synchronization payloads also upsert file metadata. Attachments are associated with their source message and thread, start with pending download/extraction states, and remain traceable when Slack removes them. The dashboard shows these states, but no file is downloaded during this phase. The configured attachment storage directory is reserved for the secure-download phase and local paths are never returned by APIs.

## Deferred work

- Debouncing rapid reply bursts
- Slack user/profile name resolution
- Publishing summaries back to Slack
- Secure file download and extraction
- OAuth installation and multi-workspace token management

Remaining file work stays separate from thread synchronization and proceeds as secure download, CSV, XLSX, PDF text, then image OCR.
