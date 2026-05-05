# Roadmap

FDDP is currently `v0.1.x-alpha`. The priority is to keep the small-project entry simple while preserving a path toward production data access governance.

## v0.1 Alpha

- Go runtime with query/command endpoints and contract publication.
- FDDP Lite for GORM-backed field groups, collections, and simple commands.
- GORM adapter with explicit safe field/relation mappings.
- TypeScript SDK with generated `createFddpApi`, `api.load(...)`, and `api.command...`.
- Contract `check` and `diff` commands.
- Query/command guards, trace call counts, request-scoped batch loaders, and demo smoke test.

## v0.2

- Redis-backed cache adapter.
- Redis or database-backed idempotency store.
- Production auth example using `BearerTokenIdentityResolver`.
- Production deployment guide for tenant boundaries, cache scope, and command idempotency.

## v0.3

- Contract governance CI examples.
- More migration examples from scattered REST endpoints to FDDP Lite.
- Better starter templates for production and framework integrations such as Gin.
- More adapter tests around relation expansion, filters, and cost limits.

## v0.4

- Observability examples for trace collection and N+1 regression checks.
- Additional database adapter investigation.
- More explicit compatibility policy for generated TypeScript contracts.

## v1.0 Direction

- Stable Go runtime and TypeScript SDK APIs.
- Production-ready cache/idempotency integrations.
- Clear contract evolution policy.
- Documented extension points for teams growing beyond Lite.
