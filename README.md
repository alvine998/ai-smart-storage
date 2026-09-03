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

For an existing database, apply `migrations/002_businesses.sql` and `migrations/004_billing.sql`. If the database previously used package quotas, apply `migrations/003_remove_packages.sql` before `migrations/004_billing.sql` to remove the old package tables.

The API listens on `http://localhost:8080`.

CORS is currently open to all origins, methods, and headers for frontend development. Restrict the middleware configuration before production deployment.

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
- `POST /v1/chat/stream` with `{"messages":[{"role":"user","content":"Hello"}]}`
- `GET /webhooks/whatsapp` for Meta webhook verification
- `POST /webhooks/whatsapp` for incoming messages

`POST /v1/chat/stream` is wired for Server-Sent Events. The WhatsApp handler stores inbound messages, loads the latest 20 messages, streams a MiMo reply, stores it, and sends it through the Graph API.

## Notes

- The MiMo URL, model, and credentials are environment variables because provider endpoint/model names can vary by account and region.
- Put the webhook behind HTTPS in production and set `WHATSAPP_APP_SECRET` so Meta signatures are verified.
- Create an R2 API token with Object Read & Write permissions, then set its access key ID and secret in `.env`.
- `R2_PUBLIC_URL` is optional. Set it to a configured R2 custom domain or public bucket URL if upload responses should include a public URL; otherwise the API returns an empty URL and objects remain private.

Plan names are `Starter`, `Business`, or `Enterprise`. Subscription statuses are `active`, `past_due`, or `canceled`. Invoice statuses are `paid`, `pending`, or `failed`.
