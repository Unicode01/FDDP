# FDDP Lite Getting Started

[FDDP Lite 中文入口](../../packages/go-fddp/fddplite/README.zh-CN.md)

FDDP Lite is the small-project entry point for FDDP.

Use it when you have a Go backend, GORM models, and frontend pages that need profile data, dashboard data, filtered lists, relation expand, or simple create/update/delete commands.

The short version:

- Backend registers a controlled data contract from GORM models.
- Frontend generates TypeScript helpers from `GET /contract`.
- Page code calls `api.load(...)` and `api.command...`.
- REST still handles login, payment, file upload, webhooks, long-running jobs, and public integration APIs.

Lite is not a database gateway. The backend still owns fields, filters, order keys, relations, tenant boundaries, identity, and query limits.

## 15-Minute Path

Install the SDK using the current instructions in [Install FDDP](../install.md), then create a starter app:

```bash
npx fddp new my-fddp-app
```

Start the backend:

```bash
cd my-fddp-app/backend
go mod tidy
go run .
```

In another terminal, generate and type-check the frontend contract:

```bash
cd my-fddp-app/frontend
npm install
npm run codegen
npm run typecheck
```

The generated backend publishes:

- `GET /contract`
- `POST /data/query`
- `POST /command/execute`

The generated frontend now has:

- `src/fddp-client.ts`: the client bound to the backend.
- `src/fddp.generated.ts`: typed fields, resources, commands, and `createFddpApi`.
- `src/dashboard-data.ts`: a page-data read example.
- `src/update-profile.ts`: a command example.

## Mental Model

Think of Lite as three backend registrations.

`FieldGroup` is for one row that exposes multiple fields:

```go
_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()
```

This lets the frontend request `me.profile.id`, `me.profile.name`, and `me.profile.description` while the backend can resolve them with one row lookup.

`Collection` is for a list:

```go
_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register()
```

This publishes a tenant-scoped list with explicit fields, filters, order keys, pagination, and one safe relation expand.

`Command` is for a write:

```go
type UpdateProfileInput struct {
  DisplayName string `json:"displayName"`
}

_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Idempotent().
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()
```

This exposes a typed update command with a self boundary, idempotency, and invalidation metadata.

## Small Backend

Start from ordinary GORM models:

```go
type Profile struct {
  ID          string `gorm:"primaryKey"`
  UserID      string `gorm:"index"`
  Name        string
  Description string
}

type User struct {
  ID   string `gorm:"primaryKey"`
  Name string
}

type Project struct {
  ID        string `gorm:"primaryKey"`
  Name      string
  OwnerID   string
  Owner     User `gorm:"foreignKey:OwnerID"`
  Status    string
  TenantID  string `gorm:"index"`
  UpdatedAt string `gorm:"index"`
}
```

Create the Lite app:

```go
app := fddplite.NewDevApp(db)
```

`NewDevApp` is for local development and MVPs. It gives you in-memory cache, in-memory idempotency, and a `dev` contract version so the first run has fewer moving parts.

Register page data:

```go
must(fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register())

must(fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register())
```

Register a write:

```go
must(fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Idempotent().
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register())
```

Serve it:

```go
log.Fatal(http.ListenAndServe(":8080", app.Handler()))
```

## Small Frontend

Create a client:

```ts
import { createFddpClient } from "@fddp/next-sdk";

export const fddp = createFddpClient({
  baseUrl: "http://localhost:8080",
  headers: {
    "X-DDP-Subject": "user_123",
    "X-DDP-Tenant": "tenant_abc"
  }
});
```

For local development, the starter uses headers to keep the sample small. Production services should use a bearer token on the frontend and `BearerTokenIdentityResolver` on the backend.

Generate the contract:

```bash
npx fddp init --contract http://localhost:8080/contract --output src/fddp.generated.ts
npx fddp codegen
```

Load page data:

```ts
import { fddp } from "./fddp-client";
import { createFddpApi, fields } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function loadDashboard() {
  return api.load({
    fields: [fields.me.profile.name, fields.global.config.appName],
    projectList: {
      first: 20,
      filter: { status: { eq: "active" } },
      orderBy: [{ field: "updatedAt", direction: "desc" }],
      fields: ["id", "name", "updatedAt"],
      expand: { owner: ["id", "name"] }
    }
  });
}
```

Run a command:

```ts
import { fddp } from "./fddp-client";
import { createFddpApi } from "./fddp.generated";

const api = createFddpApi(fddp);

export async function updateProfile(displayName: string) {
  return api.command.user.profile.update({ displayName }, {
    idempotencyKey: crypto.randomUUID()
  });
}
```

## What Lite Protects

Lite keeps the small-project API short, but it still goes through FDDP Core and the GORM adapter.

Important defaults:

- Client field names are never used as raw SQL identifiers.
- Only registered fields can be selected.
- Only registered fields can be filtered or ordered.
- Only registered relations can be expanded.
- `Self("UserID")` and `Tenant("TenantID")` add backend row boundaries.
- Query Guard rejects oversized, too deep, too broad, or too expensive queries before GORM runs.
- Command Guard rejects oversized or deeply nested command inputs.

If a forged request asks for an unknown field, unsupported filter, unregistered relation, or an over-budget query shape, the backend rejects it.

## Move To Production

Switch from `NewDevApp` to `NewProductionApp` and provide real identity infrastructure:

```go
identityResolver := fddp.BearerTokenIdentityResolver(
  fddp.BearerTokenVerifierFunc(func(ctx context.Context, token string) (fddp.TokenClaims, error) {
    // Verify a JWT or session token here.
    // Derive subject and tenant from trusted claims.
    return fddp.TokenClaims{
      Subject:  "user_123",
      TenantID: "tenant_abc",
      Roles:    []string{"tenant_admin"},
    }, nil
  }),
)

app := fddplite.NewProductionApp(db,
  fddp.WithIdentityResolver(identityResolver),
  fddp.WithContractVersion("contract_v1"),
)
```

Tune budgets only when you have a measured reason:

```go
app := fddplite.NewProductionApp(db,
  fddp.WithQueryLimits(fddp.QueryLimits{
    MaxCollectionFirst: 50,
    MaxExpandDepth: 1,
    MaxCost: 180,
    MaxBodyBytes: 256 << 10,
    Timeout: 2 * time.Second,
  }),
)
```

## Grow Beyond Lite

You do not need to rewrite the whole project when one area gets complicated.

Keep simple domains on Lite:

```go
fddplite.Collection[Project](app, "project.list")
```

Move only the complex domain down to `gormadapter` or FDDP Core:

```go
app.Engine().RegisterResource(...)
```

As long as the contract shape remains compatible, frontend calls like `api.load(...)` and `api.command.user.profile.update(...)` can stay the same.

For a concrete side-by-side example, see [Grow From Lite To Core](grow-to-core.md).

## Common Problems

`api.load(...)` returns permission errors:

Check that the request identity is present. In local development, the starter uses `X-DDP-Subject` and `X-DDP-Tenant`. In production, use bearer tokens.

Filter or order is rejected:

Only fields registered through `Fields(...)` can be used. Raw SQL and unknown fields are rejected.

`npx fddp codegen` fails:

Make sure the backend is running and `GET http://localhost:8080/contract` works.

The generated types are stale:

Rerun `npm run codegen` after changing backend registrations.

`go get` is slow or fails through a regional proxy:

See [Install FDDP](../install.md). Go submodule tags use `packages/go-fddp/<release-tag>`. If a Go proxy returns a timeout, retry later or use `GOPROXY=direct` for the install check.

## Next Steps

- Read the API reference: [FDDP Lite API](../../packages/go-fddp/fddplite/README.md)
- Grow beyond Lite: [Grow From Lite To Core](grow-to-core.md)
- Inspect the end-to-end demo: [examples/demo](../../examples/demo/README.md)
- Mount in Gin: [examples/gin](../../examples/gin/README.md)
- Use lower-level Go runtime APIs: [go-fddp](../../packages/go-fddp/README.md)
