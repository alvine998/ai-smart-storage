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

For an existing database, apply `migrations/001_package_quotas.sql` instead of recreating the schema.

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
- `packages` - package catalog CRUD
- `userpackages` - user package assignment CRUD
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
- `POST /v1/packages` to create a package with `name`, `price`, and optional `description`
- `GET /v1/packages` to list packages
- `GET /v1/packages/:id` to retrieve a package
- `PUT /v1/packages/:id` to update a package
- `DELETE /v1/packages/:id` to delete a package
- `POST /v1/users/:id/packages` to assign a package with `package_id`, optional `status`, and `expires_at`
- `GET /v1/users/:id/packages` to list a user's packages
- `PUT /v1/users/:id/packages/:assignmentID` to update an assignment
- `DELETE /v1/users/:id/packages/:assignmentID` to remove an assignment
- Package fields `storage_limit_bytes` and `ai_token_limit` define monthly quotas. A value of `0` means unlimited; active, non-expired packages are combined.
- Storage uploads and AI chat require the `X-User-ID` header. Uploads consume the file size; chat reserves an estimate of input tokens plus 2,048 output tokens.

`X-User-ID` is a temporary identity mechanism for this template. Protect these endpoints with authentication and derive the user ID from the authenticated principal before production use.

Example package payload:

```json
{
	"name": "Pro",
	"price": "29.00",
	"storage_limit_bytes": 10737418240,
	"ai_token_limit": 100000
}
```
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
