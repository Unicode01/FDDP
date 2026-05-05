# FDDP SDK MVP

This workspace contains first-version SDK/runtime packages for the FDDP architecture.

Positioning: FDDP is backend-defined data contract infrastructure with frontend-friendly access. The backend owns the domain contract, authorization boundary, query planning, cost limits, and data mapping. The frontend consumes the generated contract; it does not decide what data is allowed or how storage is queried.

## 30-minute check

Build the TypeScript SDK, then run the demo smoke test:

```bash
cd packages/nextjs-sdk
npm install
npm run build

cd ../../examples/demo
node smoke-test.mjs
```

The smoke test starts the Go demo backend on a free local port, pulls `GET /contract`, regenerates frontend types, runs `npm run typecheck`, executes one query, executes one command, and confirms an unsafe filter is rejected.

## Packages

- `packages/nextjs-sdk`: TypeScript / Next.js SDK with Lite query client, resource query descriptors, contract type generation, V9 response envelope normalization, memory cache, React hooks, and server helper.
- `packages/go-fddp`: Go backend runtime with field registry, resource registry, command registry, HTTP handlers, request-scoped batch loading, policy checks, cache, trace, and idempotency support.

## Protocol endpoints

- `POST /data/query`
- `POST /command/execute`
- `GET /contract` publishes the registered field/resource/command contract for code generation.

Query responses use the V9 envelope:

```json
{
  "data": {},
  "errors": [],
  "meta": {
    "requestId": "trace_xxx",
    "traceId": "trace_xxx",
    "partial": false,
    "cache": {
      "hit": [],
      "miss": [],
      "stale": [],
      "unsafe": []
    }
  }
}
```

## Current MVP focus

1. Keep FDDP Lite simple enough for small GORM-backed projects.
2. Use Resource Query Descriptor for list/aggregate reads instead of overloading scalar fields.
3. Use Command Plane for common create/update/delete writes with idempotency, invalidation, and input limits.
4. Generate TypeScript field/resource/command constants and command input types from `GET /contract`.
5. Treat contract changes as a governed backend API surface with `fddp check` and `fddp diff`.
6. Replace the Go header identity resolver with real token and tenant checks before production use.
7. Add more cross-package contract tests before expanding advanced protocol behavior.

## Lite growth path

The Lite API should stay easy at the entrance without blocking larger services:

```go
app := fddplite.NewDevApp(db)
```

Use this for local development and MVPs.

```go
app := fddplite.NewProductionApp(db,
  fddp.WithIdentityResolver(myTokenIdentityResolver),
  fddp.WithCache(myDistributedCache),
  fddp.WithIdempotencyStore(myPersistentIdempotencyStore),
)
```

Use this for deployment defaults.

```go
app := fddplite.NewApp(db, options...)
```

Use this when a larger service wants direct control over FDDP Core. Lite builders, `gormadapter`, and hand-written `app.Engine()` registrations can coexist, so teams can move one domain at a time without changing frontend `api.load(...)` calls.

## Contract governance

Validate a contract before codegen:

```bash
fddp check --contract http://localhost:8080/contract
```

Compare two contracts and fail the command on breaking changes:

```bash
fddp diff --from contracts/contract_v1.json --to contracts/contract_v2.json
```

Breaking changes currently include removed fields/resources/commands, type changes, nullable-to-non-nullable changes, removed resource fields or relations, removed filter/order capability, newly required command inputs, and newly required idempotency keys.

For coordinated migrations, the diff command can mark selected tightenings as allowed instead of breaking:

```bash
fddp diff --from contracts/contract_v1.json --to contracts/contract_v2.json \
  --allow-required-input-add \
  --allow-input-required-tighten \
  --allow-idempotency-tighten \
  --allow-max-page-size-decrease
```

Use these flags as release policy, not as defaults. The strict behavior is still the safer CI gate.

## Production identity and diagnostics

Production services should prefer `BearerTokenIdentityResolver` with a real JWT/session verifier. Missing or rejected tokens become an empty identity, so protected data is denied by policy; an outer gateway can still return `401` earlier.

Traced query responses include `meta.trace.calls` with resolver counts and batch loader miss counts. Use it during development and load testing to prove field groups and batch loaders are avoiding repeated work.

## v0.1 boundary

Use this version for:

- Small GORM-backed projects that want typed frontend reads/writes without hand-writing REST endpoints for every page.
- Collection reads with mapped fields, filters, order, pagination, and one-level relation expand.
- Command writes with idempotency, invalidation metadata, and input guards.
- Contract-driven TypeScript generation through `createFddpApi(fddp)`.

Do not treat this version as complete production infrastructure yet:

- Replace header-based identity with real token verification and tenant checks.
- Keep query limits enabled and tuned per deployment.
- Do not add streaming or file transfer until the core request/contract surface has stabilized.
- Do not expose arbitrary SQL, raw field names, or unmapped relations.
- Add persistent idempotency/cache stores before multi-instance production use.

## Developer loop

Create a small starter:

```bash
cd packages/nextjs-sdk
npm run build
npx fddp new my-fddp-app
```

Run the existing demo loop:

```bash
cd examples/demo/backend
go run .

cd ../frontend
npx fddp init --contract http://localhost:8080/contract --output src/fddp.generated.ts --force
npx fddp codegen
```
