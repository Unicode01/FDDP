# FDDP

FDDP is an experimental backend-defined data contract platform with a Go runtime, a GORM-backed Lite layer, and a TypeScript/Next.js SDK.

It is designed for teams that want to reduce scattered page-data REST endpoints without moving data authority into the frontend. The backend publishes a contract, enforces authorization and query budgets, maps fields to safe storage access, and generates typed frontend access.

Status: `v0.1.0-alpha`. The core path works, but APIs are still expected to evolve.

## Why

Small and mid-sized web apps often accumulate many thin endpoints:

- profile fragments
- dashboard data
- filtered lists
- page-specific joins
- simple update commands

FDDP keeps REST for business-process APIs, while moving page-data access into a governed contract:

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

## Packages

- `packages/go-fddp`: Go runtime, FDDP Lite, GORM adapter, query/command guards, auth identity hooks, trace, cache, idempotency, and contract publication.
- `packages/nextjs-sdk`: TypeScript SDK, generated `createFddpApi`, React/Next helpers, contract codegen/check/diff, and `fddp new` starter scaffolding.
- `examples/demo`: end-to-end GORM + Lite + TypeScript SDK demo with smoke test.

## Quick Start

Build the TypeScript SDK, then create a starter app:

```bash
cd packages/nextjs-sdk
npm install
npm run build
node dist/cli.js new my-fddp-app
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
