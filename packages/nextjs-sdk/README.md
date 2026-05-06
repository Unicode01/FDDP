# @fddp/next-sdk

First-version TypeScript / Next.js SDK for FDDP.

FDDP is backend-owned application data access governance. The backend publishes a governed contract for page reads and simple data commands, while enforcing authorization, tenant boundaries, query limits, storage mapping, and command boundaries. This SDK consumes that backend contract and keeps application code short.

It is meant to reduce scattered page-data REST endpoints, not replace REST for business-process APIs such as login, payment, upload/download, webhooks, long-running jobs, or externally stable public APIs.

New to FDDP? If you are using a small Go/GORM backend, start with [FDDP Lite Getting Started](../../docs/lite/getting-started.md), then come back here for TypeScript SDK details.

This package intentionally starts with the v0.1 alpha surface:

- `createFddpClient()` posts Query Plane requests to `/data/query`.
- Query responses are normalized to the V9 `data/errors/meta` envelope.
- `collection()`, `aggregate()`, `definePageData()`, `defineFragment()`, and `defineMethodData()` provide the low-level SDK vocabulary.
- `generateFddpTypes()` turns a contract into field/resource/command constants, resource callers, typed command inputs, and a response data type.
- Optional memory cache is disabled unless you pass a cache and a cache scope.
- React hooks are provided in `@fddp/next-sdk/react`.
- Next server helpers are provided in `@fddp/next-sdk/next`.

## Install

The package is not published to npm yet. Current GitHub install commands and release tag rules are maintained in [Install FDDP](../../docs/install.md).

## Starter project

For a small project, generate a minimal backend/frontend starter:

```bash
npx fddp new my-fddp-app
cd my-fddp-app/backend
go mod tidy
go run .

cd ../frontend
npm install
npm run codegen
npm run typecheck
```

Templates:

```bash
npx fddp new my-api --template go
npx fddp new my-web --template next
npx fddp new my-app --template fullstack
```

For source development, build this package and run the local CLI directly:

```bash
cd packages/nextjs-sdk
npm install
npm run build
node dist/cli.js new ../../my-fddp-app
```

## Lite client

```ts
import { createFddpClient, createMemoryFddpCache } from "@fddp/next-sdk";

export const fddp = createFddpClient({
  baseUrl: process.env.NEXT_PUBLIC_FDDP_URL,
  cache: createMemoryFddpCache(),
  cacheScope: async () => "tenant_abc:user_123:perm_v17:policy_v9:contract_v12",
  getAccessToken: async () => localStorage.getItem("access_token"),
  trace: process.env.NODE_ENV === "development"
});

const result = await fddp.query({
  me: {
    profile: ["name", "avatar"]
  },
  global: {
    config: ["appName"]
  }
});

console.log(result.data, result.errors, result.meta.traceId);
```

## Generated API

For resources published by `GET /contract`, the generated API binds your client once and keeps page code short:

```ts
import { fddp } from "./fddp-client";
import { createFddpApi, fields } from "./fddp.generated";

const api = createFddpApi(fddp);

const response = await api.load({
  fields: [fields.me.profile.name],
  projectList: {
    first: 20,
    filter: { status: { eq: "active" } },
    orderBy: [{ field: "updatedAt", direction: "desc" }],
    fields: ["id", "name", "updatedAt"],
    expand: {
      owner: ["id", "name"]
    }
  }
});

console.log(response.data.project.list.items[0]?.owner?.name);
```

When the backend publishes resource field metadata, `fields`, `filter`, `orderBy`, and `expand` keys are type-checked from the contract. Lower-level `queryCallers` and `resourceCallers` are still generated for library code and advanced composition.

## Low-Level Resource Descriptors

```ts
import { collection, definePageData } from "@fddp/next-sdk";

export const dashboardData = definePageData({
  critical: {
    me: { profile: ["name", "avatar"] },
    tenant: { current: ["name"] }
  },
  lazy: {
    project: {
      list: collection({
        first: 20,
        filter: {
          status: { eq: "active" },
          name: { contains: "alpha" }
        },
        orderBy: [{ field: "updatedAt", direction: "desc" }]
      }).select({
        fields: ["id", "name", "updatedAt"],
        expand: {
          owner: ["id", "name", "avatar"]
        }
      })
    }
  }
}, {
  scope: "page",
  ssr: true
});
```

## Contract type generation

From a published contract JSON file:

```bash
npx fddp init --contract fddp.contract.json --output src/fddp.generated.ts
npx fddp codegen
```

From a running FDDP runtime:

```bash
npx fddp init --contract http://localhost:8080/contract --output src/fddp.generated.ts
npx fddp codegen
```

The legacy one-shot command remains available:

```bash
npx fddp-codegen --input http://localhost:8080/contract --output src/fddp.generated.ts
```

Validate a contract without generating code:

```bash
npx fddp check --contract http://localhost:8080/contract
```

Compare two contracts and fail on breaking changes:

```bash
npx fddp diff --from contracts/contract_v1.json --to contracts/contract_v2.json
```

Breaking changes include removed fields/resources/commands, type changes, nullable-to-non-nullable changes, removed resource field/relation surface, removed filter/order capability, newly required command inputs, and newly required idempotency keys.

Some teams intentionally roll out tighter contracts behind a version gate, migration window, or coordinated frontend release. Keep the default strict in CI, and opt in only for known migrations:

```bash
npx fddp diff --from contracts/contract_v1.json --to contracts/contract_v2.json \
  --allow-required-input-add \
  --allow-input-required-tighten \
  --allow-idempotency-tighten \
  --allow-max-page-size-decrease
```

From code, pass the same policy explicitly:

```ts
const diff = diffFddpContracts(previousContract, nextContract, {
  allowRequiredCommandInputAdd: true,
  allowIdempotencyTighten: true
});
```

Or from code:

```ts
import { diffFddpContracts, generateFddpTypes } from "@fddp/next-sdk";

const source = generateFddpTypes({
  contractVersion: "contract_v12",
  fields: [
    { field: "me.profile.name", type: "string" },
    { field: "me.profile.avatar", type: "string", nullable: true },
    { field: "tenant.current.name", type: "string" }
  ]
});

console.log(source);

const diff = diffFddpContracts(previousContract, nextContract);
if (diff.breaking.length) {
  throw new Error("FDDP contract has breaking changes");
}
```

Generated output includes nested `fields`, `resources`, and `commands` constants for query authoring, a bound `createFddpApi()` helper, generated resource callers, generated query callers, typed resource result types, typed command inputs, generated command callers, and a `FddpGeneratedData` type for scalar field responses.

```ts
import { queryFromFields } from "@fddp/next-sdk";
import { createFddpApi, fields, queryCallers } from "./fddp.generated";

const response = await queryCallers.query(fddp, queryFromFields([fields.me.profile.name]));

const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: { first: 20, fields: ["id", "name"] }
});

await api.command.user.profile.update({ displayName: "Tom" });
```

## React / Next client components

```tsx
"use client";

import { FddpClientProvider, useFddpQuery } from "@fddp/next-sdk/react";
import { fddp } from "./fddp-client";

export function AppProviders({ children }: { children: React.ReactNode }) {
  return <FddpClientProvider client={fddp}>{children}</FddpClientProvider>;
}

export function HeaderProfile() {
  const { data, loading, errors, traceId } = useFddpQuery<{
    me: { profile: { name: string; avatar: string | null } };
  }>({
    me: {
      profile: ["name", "avatar"]
    }
  });

  if (loading) return null;
  if (errors.length) return <span data-trace-id={traceId}>partial data</span>;

  return <span>{data?.me.profile.name}</span>;
}
```

## Server-side Next usage

```ts
import { createNextFddpClient } from "@fddp/next-sdk/next";
import { cookies } from "next/headers";

export async function loadDashboard() {
  const cookieStore = await cookies();
  const client = createNextFddpClient({
    baseUrl: process.env.FDDP_INTERNAL_URL,
    accessToken: () => cookieStore.get("access_token")?.value,
    revalidate: 30,
    tags: ["dashboard"]
  });

  return client.query({
    me: { profile: ["name", "avatar"] },
    tenant: { current: ["name"] }
  });
}
```

## Cache safety

The SDK does not enable caching by default. If you pass a cache, also pass a cache scope that includes the effective user, tenant, permission version, policy version, and contract version when available.

```text
tenant_abc:user_123:perm_v17:policy_v9:contract_v12
```

Public-only queries may use a public scope such as `public:global:contract_v12`.

## Trace diagnostics

Enable `trace` only for development or controlled diagnostics:

```ts
const response = await api.load({
  fields: [fields.me.profile.name],
  projectList: { first: 20, fields: ["id", "name"], expand: { owner: ["id", "name"] } }
}, { trace: true });

console.log(response.meta.trace?.calls);
```

The Go runtime reports scalar field resolver calls, field group resolver calls, resource resolver calls, and request-scoped batch loader misses. This is useful for checking that grouped fields and batch loaders are preventing repeated database work.
