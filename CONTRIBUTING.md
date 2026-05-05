# Contributing

FDDP is in alpha. Contributions should keep the core direction intact: backend-owned data access governance with a low-friction Lite path for small teams.

## Local Checks

Run the Go runtime tests:

```bash
cd packages/go-fddp
go test ./...
```

Run the TypeScript SDK checks:

```bash
cd packages/nextjs-sdk
npm install
npm run test:codegen
npm run test:cli
npm run typecheck
npm run build
```

Run the end-to-end demo smoke test:

```bash
cd examples/demo/frontend
npm install

cd ../../../packages/nextjs-sdk
npm install
npm run build

cd ../../examples/demo
node smoke-test.mjs
```

## Design Rules

- Keep FDDP complementary to REST. REST should still handle business-process APIs.
- Keep Lite simple enough for small GORM-backed projects.
- Keep security decisions on the backend. Generated frontend code is convenience, not authority.
- Do not expose raw SQL identifiers, arbitrary joins, or unmapped fields to clients.
- Add tests when changing query planning, contract diff behavior, guards, adapters, or generated SDK output.

## Pull Requests

For now, prefer focused pull requests:

- one runtime behavior change
- one adapter change
- one SDK/codegen change
- one documentation change

Include the commands you ran and any remaining risk in the PR description.
