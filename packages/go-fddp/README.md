# go-fddp

First-version Go backend package for the federated domain data platform.

FDDP is backend-defined first. This package owns the data contract, authorization boundary, safe storage mapping, query/command budgets, cache safety, and resolver execution. TypeScript codegen exists to make that backend contract pleasant to consume; it is not the security boundary.

It provides a small runtime for the MVP protocol:

- `POST /data/query` for Query Plane reads.
- `POST /command/execute` for Command Plane writes.
- Field registry with owner, permission, cache policy, and resolver.
- Resource registry for collection and aggregate descriptors.
- Command registry with permission, idempotency requirement, and executor.
- Request-scoped batch loading helpers for resolvers.
- Query Guard limits for forged or overly expensive read requests.
- `GET /contract` for SDK code generation.
- Header-based identity resolver for local integration.
- Bearer token identity resolver template for production integration.
- In-memory cache and idempotency store for development or tests.

## Quick registration helpers

Small services can start with helper methods instead of building full definitions by hand:

```go
_ = engine.RegisterStringField(
  "me.profile.name",
  func(ctx context.Context, req fddp.FieldRequest) (string, error) {
    return "Tom", nil
  },
  fddp.FieldOwner("user-domain"),
  fddp.FieldPermission("self"),
  fddp.FieldPrivateCache(10*time.Minute),
)

_ = engine.RegisterStaticField(
  "global.config.appName",
  "FDDP Demo",
  fddp.FieldOwner("config-domain"),
  fddp.FieldPublicCache(time.Hour),
)

_ = engine.RegisterCollection(
  "project.list",
  func(ctx context.Context, req fddp.ResourceRequest) (fddp.CollectionResult, error) {
    return fddp.CollectionResult{
      Items: []any{map[string]any{"id": "project_1", "name": "Alpha"}},
      PageInfo: &fddp.PageInfo{HasNextPage: false},
    }, nil
  },
  fddp.ResourceOwner("project-domain"),
  fddp.ResourcePermission("tenant"),
  fddp.ResourceMaxPageSize(50),
)
```

Use field groups when several leaf fields come from the same row/object. A request for `me.profile.id`, `me.profile.name`, and `me.profile.desc` will call the group resolver once.

```go
_ = engine.RegisterFieldGroup(
  "me.profile",
  []fddp.GroupField{
    {Name: "id", Type: "string"},
    {Name: "name", Type: "string"},
    {Name: "desc", Type: "string", Nullable: true},
  },
  func(ctx context.Context, req fddp.FieldGroupRequest) (map[string]any, error) {
    profile := loadProfileOnce(req.Identity.Subject)
    return map[string]any{
      "id": profile.ID,
      "name": profile.Name,
      "desc": profile.Desc,
    }, nil
  },
  fddp.FieldGroupPermission("self"),
  fddp.FieldGroupPrivateCache(10*time.Minute),
)
```

## Batch loaders

Register batch loaders once on the engine. Field and resource resolvers can then share the same request-scoped cache through `req.Load` and `req.LoadMany`.

```go
engine.RegisterBatchLoader("users.byID", func(ctx context.Context, keys []string) (map[string]any, error) {
  users := make(map[string]any, len(keys))
  for _, key := range keys {
    users[key] = map[string]any{"id": key, "name": "Tom"}
  }
  return users, nil
})

_ = engine.RegisterCollection(
  "project.list",
  func(ctx context.Context, req fddp.ResourceRequest) (fddp.CollectionResult, error) {
    owners, err := req.LoadMany(ctx, "users.byID", []string{"user_123", "user_456"})
    if err != nil {
      return fddp.CollectionResult{}, err
    }

    return fddp.CollectionResult{
      Items: []any{
        map[string]any{"id": "project_1", "owner": owners["user_123"]},
        map[string]any{"id": "project_2", "owner": owners["user_456"]},
      },
    }, nil
  },
)
```

## GORM adapter

The optional `gormadapter` package can map FDDP fields to GORM models and build minimal safe queries from requested fields.

```go
_ = gormadapter.NewFieldGroup[Profile]("me.profile", db).
  String("id", "ID").
  String("name", "Name").
  NullableString("desc", "Description", gormadapter.Column("description")).
  Scope(func(tx *gorm.DB, req fddp.FieldGroupRequest) *gorm.DB {
    return tx.Where("user_id = ?", req.Identity.Subject)
  }).
  Permission("self").
  Register(engine)
```

Security boundary:

- client fields never become SQL identifiers directly
- every selectable/filterable/orderable field must be explicitly mapped
- tenant and subject constraints belong in `Scope`
- collection filters support only known operators such as `eq`, `in`, `contains`, `range`, and `between`
- adapter failures include stable error codes and hints for faster debugging

See `gormadapter/README.md` for collection and relation examples.

## Query Guard

FDDP Core rejects expensive query plans before any field/resource resolver runs. The default guard is enabled by `NewEngine()` and limits request body size, query tree depth, query tree node count, total field count, resource count, collection `first`, selection size, expand depth, filter count, order count, weighted query cost, and resolver time.

```go
engine := fddp.NewEngine(
  fddp.WithQueryLimits(fddp.QueryLimits{
    MaxFields:          20,
    MaxResources:       3,
    MaxCollectionFirst: 50,
    MaxSelectionFields: 20,
    MaxExpandDepth:     1,
    MaxExpandRelations: 3,
    MaxFilterFields:    5,
    MaxOrderBy:         2,
    MaxCost:            180,
    MaxBodyBytes:       256 << 10,
    MaxQueryDepth:      12,
    MaxQueryNodes:      100,
    Timeout:            2 * time.Second,
  }),
)
```

When a request exceeds the budget, the response contains `QUERY_LIMIT_EXCEEDED` and `meta.cost`; traced requests also include `trace.cost`. Resolver timeouts return `QUERY_TIMEOUT`. Use `fddp.WithoutQueryLimits()` only for trusted internal tests or migration tools.

## Production identity

The default header resolver is only for demos, local tools, and integration tests. Production services should verify an access token, derive the subject and tenant server-side, and let policy checks use that identity.

```go
engine := fddp.NewEngine(
  fddp.WithIdentityResolver(fddp.BearerTokenIdentityResolver(
    fddp.BearerTokenVerifierFunc(func(ctx context.Context, token string) (fddp.TokenClaims, error) {
      // Verify a JWT or session token here. Do not trust client-provided user or tenant headers.
      return fddp.TokenClaims{
        Subject:           "user_123",
        TenantID:          "tenant_abc",
        Roles:             []string{"tenant_admin"},
        PermissionVersion: "perm_v17",
        PolicyVersion:     "policy_v9",
      }, nil
    }),
  )),
)
```

If the bearer token is missing, malformed, or rejected by the verifier, FDDP uses an empty identity. Protected fields/resources/commands are then denied by policy. This keeps auth failures conservative, but your gateway may still choose to return `401` before the request reaches FDDP.

## Trace call counts

Add `"trace": true` while developing or in controlled diagnostics to inspect resolver behavior:

```json
{
  "trace": {
    "calls": {
      "fields": 1,
      "fieldGroups": 1,
      "resources": 1,
      "batchLoads": 2,
      "batchKeys": 2,
      "byBatcher": {
        "users.byID": 2
      }
    }
  }
}
```

`fields`, `fieldGroups`, and `resources` count resolver invocations. `batchLoads` and `batchKeys` count actual request-scoped batch loader misses, not every requested key. If a request asks for the same owner twice and the batcher already has it cached, the repeated key should not increase `batchKeys`.

## Command Guard

Command requests have a separate guard so write-side budgets can be tighter than read-side budgets.

```go
engine := fddp.NewEngine(
  fddp.WithCommandLimits(fddp.CommandLimits{
    MaxBodyBytes:  128 << 10,
    MaxInputBytes: 64 << 10,
    MaxInputDepth: 8,
    MaxInputNodes: 100,
    Timeout:       2 * time.Second,
  }),
)
```

When a command exceeds the budget, the response contains `COMMAND_LIMIT_EXCEEDED`. Executor timeouts return `COMMAND_TIMEOUT`.

## FDDP Lite

For small GORM-backed projects, `fddplite` gives a lower-boilerplate entry point while still using FDDP Core and `gormadapter` underneath.

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
```

Lite derives field names and columns such as `OwnerID -> ownerId -> owner_id`. Drop down to `gormadapter` when you need custom mappings.

Use `fddplite.NewProductionApp(db, options...)` for deployment defaults and `fddplite.NewApp(db, options...)` when a larger service wants full control over FDDP Core options. Lite registrations and full Core registrations can coexist on the same engine, so the project does not need a rewrite when one domain outgrows Lite.

## Install

```bash
go get github.com/Unicode01/FDDP/packages/go-fddp
```

## Minimal server

```go
package main

import (
  "context"
  "log"
  "net/http"
  "time"

  "github.com/Unicode01/FDDP/packages/go-fddp"
)

func main() {
  engine := fddp.NewEngine(
    fddp.WithCache(fddp.NewMemoryCache()),
    fddp.WithIdempotencyStore(fddp.NewMemoryIdempotencyStore()),
    fddp.WithContractVersion("contract_v12"),
    fddp.WithQueryLimits(fddp.QueryLimits{MaxCollectionFirst: 50, Timeout: 2*time.Second}),
  )

  _ = engine.RegisterField(fddp.FieldDefinition{
    Path:       "me.profile.name",
    Type:       "string",
    Owner:      "user-domain",
    Permission: "self",
    Cache:      fddp.CachePolicy{Scope: fddp.CacheScopePrivate, TTL: 10 * time.Minute},
    Resolve: func(ctx context.Context, req fddp.FieldRequest) (any, error) {
      return "Tom", nil
    },
  })

  _ = engine.RegisterResource(fddp.ResourceDefinition{
    Path:        "project.list",
    Owner:       "project-domain",
    Permission:  "tenant",
    MaxPageSize: 50,
    Resolve: func(ctx context.Context, req fddp.ResourceRequest) (any, error) {
      ownerIDs := []string{"user_123"}
      owners, err := req.Batcher.LoadMany(ctx, "users.byID", ownerIDs, func(ctx context.Context, keys []string) (map[string]any, error) {
        return map[string]any{"user_123": map[string]any{"id": "user_123", "name": "Tom"}}, nil
      })
      if err != nil {
        return nil, err
      }

      return fddp.CollectionResult{
        Items: []any{
          map[string]any{"id": "project_1", "name": "Alpha", "owner": owners["user_123"]},
        },
        PageInfo: &fddp.PageInfo{HasNextPage: false},
      }, nil
    },
  })

  _ = engine.RegisterCommand(fddp.CommandDefinition{
    Name:                "user.profile.update",
    Owner:               "user-domain",
    Permission:          "self",
    IdempotencyRequired: true,
    Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
      return fddp.CommandExecutionResult{
        Result:      map[string]any{"updated": true},
        Invalidates: []string{"me.profile.*"},
      }, nil
    },
  })

  log.Fatal(http.ListenAndServe(":8080", engine.Handler()))
}
```

## Query request

```bash
curl -X POST http://localhost:8080/data/query \
  -H 'content-type: application/json' \
  -H 'X-DDP-Subject: user_123' \
  -H 'X-DDP-Tenant: tenant_abc' \
  -H 'X-DDP-Permission-Version: perm_v17' \
  -d '{"query":{"me":{"profile":["name"]}},"trace":true}'
```

## Contract request

```bash
curl http://localhost:8080/contract
```

Use the returned JSON with the TypeScript SDK:

```bash
npx fddp init --contract http://localhost:8080/contract --output src/fddp.generated.ts
npx fddp codegen
```

Use contract checks before rolling a backend change:

```bash
npx fddp check --contract http://localhost:8080/contract
npx fddp diff --from contracts/contract_v1.json --to contracts/contract_v2.json
```

The diff command exits non-zero when the new contract removes registered surface, changes types, tightens nullability, removes resource filter/order capability, or adds a required command input.

## Gin integration

`go-fddp` does not require its built-in `net/http` handler. Frameworks can read the request body, build `IdentityContext`, and call the framework-neutral execution methods.

```go
router.POST("/data/query", func(c *gin.Context) {
  body, err := io.ReadAll(c.Request.Body)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"errors": []fddp.DDPError{{Code: "BAD_REQUEST", Reason: err.Error()}}})
    return
  }

  result := engine.ExecuteQueryBody(c.Request.Context(), fddp.QueryEndpointRequest{
    Body: body,
    Identity: fddp.IdentityContext{
      Subject: c.GetHeader("X-DDP-Subject"),
      TenantID: c.GetHeader("X-DDP-Tenant"),
      PermissionVersion: c.GetHeader("X-DDP-Permission-Version"),
    },
    HTTPRequest: c.Request,
  })
  c.JSON(result.Status, result.Response)
})

router.POST("/command/execute", func(c *gin.Context) {
  body, err := io.ReadAll(c.Request.Body)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"errors": []fddp.DDPError{{Code: "BAD_REQUEST", Reason: err.Error()}}})
    return
  }

  result := engine.ExecuteCommandBody(c.Request.Context(), fddp.CommandEndpointRequest{
    Body: body,
    Identity: fddp.IdentityContext{
      Subject: c.GetHeader("X-DDP-Subject"),
      TenantID: c.GetHeader("X-DDP-Tenant"),
    },
    HTTPRequest: c.Request,
  })
  c.JSON(result.Status, result.Response)
})
```

## Resource query request

```bash
curl -X POST http://localhost:8080/data/query \
  -H 'content-type: application/json' \
  -H 'X-DDP-Subject: user_123' \
  -H 'X-DDP-Tenant: tenant_abc' \
  -d '{"query":{"project":{"list":{"$type":"collection","args":{"first":20},"selection":{"fields":["id","name"],"expand":{"owner":["id","name"]}}}}},"trace":true}'
```

## Command request

```bash
curl -X POST http://localhost:8080/command/execute \
  -H 'content-type: application/json' \
  -H 'X-DDP-Subject: user_123' \
  -H 'X-DDP-Tenant: tenant_abc' \
  -d '{"command":"user.profile.update","input":{"displayName":"Tom"},"idempotencyKey":"cmd_1","trace":true}'
```

## Notes

This package is intentionally MVP-sized. Production integrations should replace:

- `HeaderIdentityResolver` with token verification and tenant boundary checks.
- `DefaultPolicy` with your policy engine if rules exceed the built-in public/login/self/tenant/admin modes.
- `MemoryCache` and `MemoryIdempotencyStore` with Redis or your service infrastructure.
- Inline command side effects with local transactions plus Transactional Outbox.
