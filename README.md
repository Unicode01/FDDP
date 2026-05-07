# FDDP

[中文说明](README.zh-CN.md)

FDDP is an experimental application data-access governance layer.

It lets a backend publish a governed data contract for page reads and simple data commands, then generates a typed TypeScript SDK for application code. The backend remains the authority for permissions, tenant boundaries, query cost, field-to-storage mapping, batching, cache scope, and contract evolution.

Status: alpha. The core path works, but APIs are still expected to evolve. Current install commands are kept in [Install FDDP](docs/install.md).

## Start Here

If you are trying FDDP in a small Go/GORM project, start with [FDDP Lite Getting Started](docs/lite/getting-started.md).

That guide is the shortest path from ordinary GORM models to typed frontend `api.load(...)` and `api.command...` calls. The rest of this README explains the broader architecture and package layout.

## Positioning

FDDP is not trying to replace every REST API.

The intended split is:

- REST keeps business-process APIs: login, payment, upload/download, webhooks, long-running jobs, third-party callbacks, and externally stable public APIs.
- FDDP absorbs application data-access APIs: page data, profile fragments, dashboards, filtered lists, relation expand, and simple model-adjacent commands.

In that role, FDDP behaves less like a frontend query language and more like a runtime data contract governance layer for business applications.

Small and mid-sized web apps often accumulate many thin page-data endpoints:

- profile fragments
- dashboard data
- filtered lists
- page-specific joins
- simple update commands

FDDP moves that surface into a backend-owned contract:

```ts
const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: {
    first: 20,
    filter: { status: { eq: "active" } },
    fields: ["id", "name"],
    expand: { owner: ["id", "name"] }
  }
});
```

The backend still decides which fields exist, who can read them, how they map to storage, and how expensive a query may be.

## What It Governs

- Data contract: fields, resources, commands, generated frontend types.
- Access control: subject, tenant, roles/scopes, policy version, protected fields.
- Query safety: max fields/resources/depth/nodes, collection size, expand depth, filter/order count, weighted cost, timeout.
- Storage mapping: client field names are mapped to explicit backend fields/columns, not trusted as raw SQL.
- N+1 control: field groups and request-scoped batch loaders expose resolver/batch call counts in trace output.
- Evolution: contract check/diff treats breaking changes as a governed backend API surface.

## Packages

- `packages/go-fddp`: Go runtime, FDDP Lite, GORM adapter, query/command guards, auth identity hooks, trace, cache, idempotency, and contract publication.
- `packages/nextjs-sdk`: TypeScript SDK, generated `createFddpApi`, React/Next helpers, contract codegen/check/diff, and `fddp new` starter scaffolding.
- `docs/lite/getting-started.md`: Lite-first guide for small Go/GORM projects.
- `examples/demo`: end-to-end GORM + Lite + TypeScript SDK demo with smoke test.
- `examples/gin`: Gin mounting example under `/api/fddp/*` with bearer-token identity resolution.

## Quick Start

Use [Install FDDP](docs/install.md) for the current TypeScript SDK and Go runtime install commands. While the TypeScript package is not published to npm, install it from the current GitHub release tag, then create a starter:

```bash
npx fddp new my-fddp-app
```

For source development, build the TypeScript SDK first, then run the local CLI:

```bash
git clone https://github.com/Unicode01/FDDP.git
cd FDDP
cd packages/nextjs-sdk
npm install
npm run build
node dist/cli.js new ../../my-fddp-app
```

Run the generated backend:

```bash
cd my-fddp-app/backend
go mod tidy
go run .
```

Generate and type-check the frontend contract:

```bash
cd ../frontend
npm install
npm run codegen
npm run typecheck
```

The generated backend uses `fddplite.NewDevApp(db)` with SQLite/GORM models, a field group, a collection, and an update command.

## Lite Backend Example

```go
app := fddplite.NewDevApp(db)

_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()

_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  Relation("owner", "Owner", "ID", "Name").
  Register()

_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Idempotent().
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()
```

When a domain outgrows Lite, keep the frontend contract shape and move only that backend registration down to `gormadapter` or full `app.Engine().RegisterResource(...)`.

## Contract Governance

```bash
fddp check --contract http://localhost:8080/contract
fddp diff --from contracts/old.json --to contracts/new.json
```

The diff command treats removed fields/resources/commands, type changes, tighter nullability, removed filter/order capability, newly required command inputs, and newly required idempotency keys as breaking by default.

## Demo Smoke Test

```bash
cd packages/nextjs-sdk
npm install
npm run build

cd ../../examples/demo
node smoke-test.mjs
```

The smoke test builds the Go demo backend, starts it on a free port, pulls `/contract`, regenerates frontend types, runs TypeScript typecheck, executes a query, executes a command, and verifies unsafe filters are rejected.

## v0.1 Boundary

Useful now:

- GORM-backed small projects using Lite.
- Typed page-data reads through generated `api.load(...)`.
- Simple command writes through generated `api.command...`.
- Safe field mapping, filters, order, pagination, and one-level relation expand.
- Query/command guards for forged or expensive requests.
- Contract codegen, validation, and diff checks.

Not production-complete yet:

- No Redis/distributed cache adapter in this repository.
- No persistent idempotency store adapter yet.
- Production auth requires wiring `BearerTokenIdentityResolver` to your JWT/session verifier.
- Public API stability is not guaranteed before later releases.

## License

MIT
