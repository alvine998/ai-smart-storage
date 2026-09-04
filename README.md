# AI Smart Storage

Go Fiber template for a streaming MiMo V2.5 assistant with MySQL-backed message history, Cloudflare R2 storage, and a Meta WhatsApp webhook.

## Quick start

1. Copy `.env.example` to `.env` and set the MiMo and Meta credentials.
2. Start MySQL: `docker compose up -d mysql`.
3. Apply `schema.sql` to the `ai_storage` database.
4. Install dependencies and run:

```bash
go mod tidy
go run ./cmd/server
```

For an existing database, apply `migrations/002_businesses.sql`, `migrations/004_billing.sql`, `migrations/005_documents.sql`, `migrations/006_ai_processing_logs.sql`, `migrations/007_wa_conversations.sql`, and `migrations/008_usage_quota.sql`. If the database previously used package quotas, apply `migrations/003_remove_packages.sql` before the later migrations to remove the old package tables.

The API listens on `http://localhost:8080`.

CORS origins are controlled by `CORS_ALLOWED_ORIGINS` (default `*` for development; set a comma-separated list before production).

## Authentication

All `/v1` routes require a bearer token except user registration and login.

1. `POST /v1/users` to register.
2. `POST /v1/auth/login` with `email` and `password` to receive a token.
3. Send `Authorization: Bearer <token>` on every request.

Set `APP_JWT_SECRET` to a long random string to keep tokens valid across restarts; when it is empty the server generates an ephemeral secret and logs a warning. `JWT_TTL_HOURS` (default 24) controls token expiry. Chat message content is encrypted at rest with AES-256-GCM when `MESSAGE_ENCRYPTION_KEY` is set (recommended `openssl rand -hex 32`); rows written before the key was set stay readable, but removing or rotating the key makes already-encrypted messages unreadable. User-scoped resources enforce ownership server-side: the `user_id` in queries and bodies must match the authenticated user, `GET /v1/users` listing is admin-restricted, and subscription/invoice listing is admin-restricted. Login, chat streaming, and the WhatsApp webhook are rate limited. `/v1/storage` keys are scoped per user under `uploads/<user_id>/...`, and downloads force `Content-Disposition: attachment` with `X-Content-Type-Options: nosniff`.

## Testing

Run the unit tests, coverage summary, or static analysis with:

```bash
go test ./...
go test ./... -cover
go vet ./...
```

The tests use local HTTP fixtures and deterministic service inputs, so they do not require MySQL, R2, Meta, or MiMo credentials.

## HTTP structure

Feature handlers live in separate packages under `internal/http`:

- `health` - health check
- `users` - user CRUD
- `business` - optional user business profile CRUD
- `documents` - document metadata, tags, versions, and R2 file CRUD
- `ai_logs` - AI processing usage and cost logs
- `usagequota` - current subscription-period usage counters
- `plans` - plan CRUD and quota definitions
- `subscriptions` - subscription CRUD
- `invoices` - invoice CRUD
- `storage` - R2 upload and download
- `chat` - MiMo streaming chat
- `whatsapp` - Meta webhook and reply flow
- `middleware` - shared HTTP middleware such as open CORS

## Endpoints

- `GET /health`
- `POST /v1/users` to create a user with `name`, `email`, `password`, and optional `phone_number`
- `GET /v1/users` to list users
- `GET /v1/users/:id` to retrieve a user
- `PUT /v1/users/:id` to update user details and optionally the password
- `DELETE /v1/users/:id` to delete a user
- `POST /v1/users/:id/business` to create an optional business profile
- `GET /v1/users/:id/business` to retrieve a user's business profile
- `PUT /v1/users/:id/business` to update a business profile
- `DELETE /v1/users/:id/business` to remove a business profile
- `POST /v1/documents` to upload a document using multipart `file`, `user_id`, optional `category`, `summary`, `metadata`, and `uploaded_via`
- `GET /v1/documents?user_id=:id` to list active documents
- `GET /v1/documents/:id` to retrieve document metadata
- `PUT /v1/documents/:id` to update category, summary, or JSON metadata
- `DELETE /v1/documents/:id` to soft-delete a document and remove its R2 object
- `GET /v1/documents/:id/download` to download the document from R2
- `POST /v1/documents/:id/tags` to add a tag
- `GET /v1/documents/:id/tags` to list tags
- `DELETE /v1/documents/:id/tags/:tagID` to remove a tag
- `GET /v1/documents/:id/versions` to list immutable version records; version 1 is created on upload
- `GET /v1/ai-processing-logs?user_id=:id` to list per-user AI token and cost logs
- `GET /v1/usage-quota?user_id=:id` to read current storage, AI, and WhatsApp usage
- `GET /v1/wa-conversations?user_id=:id` to list WhatsApp conversation cost records
- `POST /v1/plans` to create a plan
- `GET /v1/plans` to list plans
- `GET /v1/plans/:id` to retrieve a plan
- `PUT /v1/plans/:id` to update a plan
- `DELETE /v1/plans/:id` to delete a plan
- `POST /v1/subscriptions` to create a subscription
- `GET /v1/subscriptions` to list subscriptions
- `GET /v1/subscriptions/:id` to retrieve a subscription
- `PUT /v1/subscriptions/:id` to update a subscription
- `DELETE /v1/subscriptions/:id` to delete a subscription
- `POST /v1/invoices` to create an invoice
- `GET /v1/invoices` to list invoices
- `GET /v1/invoices/:id` to retrieve an invoice
- `PUT /v1/invoices/:id` to update an invoice
- `DELETE /v1/invoices/:id` to delete an invoice
- `POST /v1/storage/upload` as multipart form data with a `file` field and optional `key` field
- `GET /v1/storage/{key}` to download an object from R2
- `POST /v1/chat/stream` with `{"user_id":1,"messages":[{"role":"user","content":"Hello"}]}`
- `GET /webhooks/whatsapp` for Meta webhook verification
- `POST /webhooks/whatsapp` for incoming messages

`POST /v1/chat/stream` is wired for Server-Sent Events. The WhatsApp handler stores inbound messages, loads the latest 20 messages, streams a MiMo reply, stores it, and sends it through the Graph API.

## Notes

- The MiMo URL, model, and credentials are environment variables because provider endpoint/model names can vary by account and region.
- Put the webhook behind HTTPS in production and set `WHATSAPP_APP_SECRET` so Meta signatures are verified.
- Create an R2 API token with Object Read & Write permissions, then set its access key ID and secret in `.env`.
- `R2_PUBLIC_URL` is optional. Set it to a configured R2 custom domain or public bucket URL if upload responses should include a public URL; otherwise the API returns an empty URL and objects remain private.
- Document objects are stored under `smart-storage/{user_id}/{uuid}/{file_name}` in R2. MySQL stores the metadata and supports soft deletion. MySQL `JSON` is used for flexible metadata, equivalent to PostgreSQL `JSONB`.
- AI chat logs actual provider token usage as `search_query` in `ai_processing_logs`. Set `MIMO_INPUT_COST_PER_1K` and `MIMO_OUTPUT_COST_PER_1K` to calculate provider cost; zero leaves cost at zero until rates are configured.
- AI processing actions support `categorize`, `summarize`, and `search_query`; `document_id` may be null for searches not tied to a document.
- Usage counters are stored in MySQL `usage_quota` and incremented after document uploads, AI queries, and WhatsApp messages. Redis is intentionally not required yet; it can cache `GET /v1/usage-quota` when traffic grows, while MySQL remains the source of truth.
- WhatsApp inbound messages open a 24-hour customer-service window in `wa_conversation_windows`. Outbound replies are logged in `wa_conversations` with `service` while the window is open, otherwise `utility`.
- Webhook user mapping uses `users.phone_number`; store the WhatsApp sender number in that column before expecting inbound messages to be logged or answered.

Plan names are `Starter`, `Business`, or `Enterprise`. Subscription statuses are `active`, `past_due`, or `canceled`. Invoice statuses are `paid`, `pending`, or `failed`.
