# FDDP Demo

Small end-to-end demo for the Go runtime, GORM adapter, contract codegen, and TypeScript SDK.

FDDP is backend-defined first: the Go runtime publishes the contract, enforces identity/tenant boundaries, maps client fields to safe storage fields, plans guarded queries, and rejects over-budget requests. The frontend SDK is intentionally just a typed access layer over that backend contract.

## 30-Minute Path

From the repository root:

```bash
cd packages/nextjs-sdk
npm install
npm run build

cd ../../examples/demo/backend
go mod tidy
go run .
```

In another terminal:

```bash
cd examples/demo/frontend
npm install
npm run codegen
npm run typecheck
```

The first application call should look like this:

```ts
const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: { first: 20, fields: ["id", "name"] }
});
```

For an automated check of the same path:

```bash
cd examples/demo
node smoke-test.mjs
```

## What It Shows

- `me.profile.id/name/description` is served by one GORM field group query.
- `project.list` supports `filter`, `orderBy`, cursor pagination, `totalCount`, and `owner` expand.
- Client field names never become SQL identifiers directly; fields and relations are explicitly mapped.
- Query Guard and Command Guard reject forged or overly expensive requests before GORM executes.
- The backend uses `fddplite` to keep the small-project setup short while still using FDDP Core underneath.
- The frontend gets `fields`, `resources`, `commands`, typed command inputs, and response types from `GET /contract`.
- `user.profile.update` shows the Command Plane and cache invalidation metadata.

This is the intended medium-complexity path for v0.1: dashboard scalar fields, a tenant list, one relation expand, a profile update command, generated frontend calls, contract diff checks, and a rejected unsafe query. It is more than a hello world, but still small enough to inspect end to end.

## Backend-First Shape

The backend defines the contract surface:

- `FieldGroup[Profile](app, "me.profile")` groups multiple frontend fields into one backend row lookup.
- `Collection[Project](app, "project.list")` exposes only mapped fields, filters, order keys, pagination, and relation expand.
- `UpdateCommand[Profile, UpdateProfileInput](...)` registers writes with typed inputs, idempotency, invalidation, and a self boundary.
- `WithQueryLimits(...)` and `WithCommandLimits(...)` reject forged or expensive shapes before business code runs.

The frontend generated API mirrors that contract:

```ts
const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: { first: 20, fields: ["id", "name"] }
});
```

This call is convenient, but it is not the authority. Unknown fields, unsafe filters, unregistered relations, tenant leaks, and oversized queries are rejected by the backend.

## Run Backend

```bash
cd examples/demo/backend
go mod tidy
go run .
```

The backend listens on `http://localhost:8080` by default. Use another port if needed:

```bash
$env:PORT = "18080"
go run .
```

Endpoints:

- `GET /contract`
- `POST /data/query`
- `POST /command/execute`

## Generate Frontend Types

```bash
cd examples/demo/frontend
npm install
npm run codegen
npm run typecheck
```

For contract governance:

```bash
fddp check --contract http://localhost:8080/contract
fddp diff --from contracts/old.json --to contracts/new.json
```

Important files:

- `src/dashboard-data.ts`: typed page query with scalar fields and `project.list`.
- `src/update-profile.ts`: typed command call using the generated bound API.
- `src/security-check.ts`: intentionally unsafe request showing adapter rejection.

## Query Example

```bash
curl -X POST http://localhost:8080/data/query ^
  -H "content-type: application/json" ^
  -H "X-DDP-Subject: user_123" ^
  -H "X-DDP-Tenant: tenant_abc" ^
  -d "{\"query\":{\"me\":{\"profile\":[\"id\",\"name\",\"description\"]},\"global\":{\"config\":[\"appName\"]},\"project\":{\"list\":{\"$type\":\"collection\",\"args\":{\"first\":2,\"filter\":{\"status\":{\"eq\":\"active\"}},\"orderBy\":[{\"field\":\"updatedAt\",\"direction\":\"desc\"}]},\"selection\":{\"fields\":[\"id\",\"name\",\"updatedAt\"],\"expand\":{\"owner\":[\"id\",\"name\"]}}}}},\"trace\":true}"
```

## Command Example

```bash
curl -X POST http://localhost:8080/command/execute ^
  -H "content-type: application/json" ^
  -H "X-DDP-Subject: user_123" ^
  -H "X-DDP-Tenant: tenant_abc" ^
  -d "{\"command\":\"user.profile.update\",\"input\":{\"displayName\":\"Demo Tom\"},\"idempotencyKey\":\"demo_1\"}"
```

## Lite Backend Shape

The backend setup intentionally starts from Lite:

```go
app := fddplite.NewDevApp(db)

_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()

_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register()
```

Lite derives `OwnerID -> ownerId -> owner_id`, but it still registers explicit mappings through the GORM adapter.

## Security Check

This request is rejected before raw SQL can be used:

```bash
curl -X POST http://localhost:8080/data/query ^
  -H "content-type: application/json" ^
  -H "X-DDP-Subject: user_123" ^
  -H "X-DDP-Tenant: tenant_abc" ^
  -d "{\"query\":{\"project\":{\"list\":{\"$type\":\"collection\",\"args\":{\"filter\":{\"name\":{\"raw\":\"name = name; drop table users\"}}},\"selection\":{\"fields\":[\"id\",\"name\"]}}}}}"
```

Expected error reason includes `UNSUPPORTED_FILTER`.

Query Guard also blocks expensive shapes before any resolver runs. For example, a forged request with a body above `MaxBodyBytes`, a query tree deeper than `MaxQueryDepth`, too many query nodes, `first` above `MaxCollectionFirst`, nested expand deeper than `MaxExpandDepth`, too many filters/order clauses, or a cost above `MaxCost` returns `QUERY_LIMIT_EXCEEDED`.

Command Guard blocks oversized or deeply nested command inputs and executor timeouts. Over-budget commands return `COMMAND_LIMIT_EXCEEDED`; timed out executors return `COMMAND_TIMEOUT`.

## Trace Diagnostics

Add `"trace": true` to the query example while developing. The response includes resolver and batch counts:

```json
{
  "calls": {
    "fieldGroups": 1,
    "resources": 1,
    "batchLoads": 1,
    "batchKeys": 2,
    "byBatcher": {
      "users.byID": 2
    }
  }
}
```

Use this to check that `me.profile.id/name/description` is one grouped resolver call and repeated owner loads go through the request-scoped batcher.

## v0.1 Boundary

Supported now:

- Scalar query fields and GORM-backed field groups.
- Collection resources with mapped fields, filters, order, cursor pagination, `totalCount`, and one-level relation expand.
- Command execution with typed generated inputs, idempotency key support, invalidation metadata, input size/depth limits, and timeout protection.
- Contract-driven TypeScript generation with `createFddpApi(fddp)` for simple app code.
- Query/command guards for forged, oversized, too deep, or too expensive requests.

Not in v0.1:

- Streaming responses and file transfer.
- A production auth/token verifier. The demo uses headers to keep the sample small.
- Arbitrary SQL, arbitrary client-side joins, or raw user-provided field names.
- Deep nested relation expansion beyond the configured guard limits.
- Distributed cache, persistent idempotency storage, or multi-service deployment wiring.
