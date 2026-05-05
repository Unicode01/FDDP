# Changelog

## v0.1.1-alpha

- Clarifies FDDP positioning as backend-owned application data access governance.
- Documents the intended FDDP/REST split.
- Archives architecture prototype documents under `docs/prototypes`.
- Fixes the demo smoke test for GitHub Actions and local cross-platform execution.
- Adds CI, roadmap, contributing guide, and release-oriented repository metadata.

## v0.1.0-alpha

- Initial public alpha.
- Adds Go FDDP runtime with query/command endpoints, policy checks, query/command guards, trace, cache, idempotency, batch loading, and contract publication.
- Adds FDDP Lite for GORM-backed field groups, collections, and simple commands.
- Adds GORM adapter with explicit field/relation mappings.
- Adds TypeScript SDK with generated field/resource/command constants, `createFddpApi`, React/Next helpers, contract check/diff, and starter scaffolding.
- Adds end-to-end demo and smoke test.
