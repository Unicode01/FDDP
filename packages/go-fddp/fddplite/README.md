# FDDP Lite

FDDP Lite is a low-boilerplate entry layer on top of FDDP Core and the GORM adapter.

Use it when a small project wants to start quickly, while keeping the same contract, query, command, permission, and GORM safety model underneath.

New to Lite? Start with the small-project guide first: [FDDP Lite Getting Started](../../../docs/lite/getting-started.md).

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

_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()

log.Fatal(http.ListenAndServe(":8080", app.Handler()))
```

Use `NewDevApp` for local development and small-project starters. It adds an in-memory cache, an in-memory idempotency store, and a `dev` contract version so the first run has fewer moving parts.

Lite derives:

- FDDP field names: `OwnerID` -> `ownerId`
- DB columns: `OwnerID` -> `owner_id`
- contract types from Go field types
- simple `self` and `tenant` scopes
- common relation preload keys
- common GORM create/update/delete commands

It does not trust client fields as SQL identifiers. All query planning still goes through `gormadapter`.

## App presets

Lite has three entry points:

```go
app := fddplite.NewDevApp(db)
```

Use this for local development, examples, tests, and MVPs that run as one process.

```go
app := fddplite.NewProductionApp(db,
  fddp.WithIdentityResolver(myTokenIdentityResolver),
  fddp.WithCache(myDistributedCache),
  fddp.WithIdempotencyStore(myPersistentIdempotencyStore),
  fddp.WithContractVersion("contract_v1"),
)
```

Use this when deploying. It applies a stricter default query/command budget, but you still provide production identity, cache, and idempotency infrastructure.

```go
app := fddplite.NewApp(db, options...)
```

Use this when a larger project wants full control over FDDP Core options. You can still keep Lite registrations for simple domains and drop individual complex domains down to `gormadapter` or `app.Engine().RegisterResource(...)`.

`ProductionQueryLimits()` and `ProductionCommandLimits()` are exported presets. Pass your own `fddp.WithQueryLimits(...)` or `fddp.WithCommandLimits(...)` after the preset when a service needs a different budget.

Lite also inherits FDDP Core Query Guard. By default, forged or overly large queries are rejected before GORM runs. Tune the budget at app creation:

```go
app := fddplite.NewProductionApp(db,
  fddp.WithQueryLimits(fddp.QueryLimits{
    MaxCollectionFirst: 50,
    MaxExpandDepth: 1,
    MaxCost: 180,
    MaxBodyBytes: 256 << 10,
    MaxQueryDepth: 12,
    MaxQueryNodes: 100,
    Timeout: 2 * time.Second,
  }),
)
```

## Small project tutorial

Start from ordinary GORM models:

```go
type Profile struct {
  ID          string
  UserID      string
  Name        string
  Description string
}

type Project struct {
  ID        string
  TenantID  string
  OwnerID   string
  Name      string
  Status    string
  UpdatedAt time.Time
  Owner     User
}
```

Expose the current user's profile as one grouped lookup:

```go
_ = fddplite.FieldGroup[Profile](app, "me.profile").
  Fields("ID", "Name", "Description").
  Self("UserID").
  Register()
```

Expose a tenant-scoped project list with mapped fields, filters, order, pagination, and one safe relation expand:

```go
_ = fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register()
```

Add a typed command when the page needs to write:

```go
type UpdateProfileInput struct {
  DisplayName string `json:"displayName"`
}

_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Self("UserID").
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()
```

Run the backend, pull `GET /contract`, and generate TypeScript:

```bash
npx fddp init --contract http://localhost:8080/contract --output src/fddp.generated.ts
npx fddp codegen
```

Frontend code can then use the generated binding:

```ts
const api = createFddpApi(fddp);

await api.load({
  fields: [fields.me.profile.name],
  projectList: {
    first: 20,
    filter: { status: { eq: "active" } },
    orderBy: [{ field: "updatedAt", direction: "desc" }],
    fields: ["id", "name", "updatedAt"],
    expand: { owner: ["id", "name"] }
  }
});

await api.command.user.profile.update({ displayName: "Tom" });
```

This is still FDDP Core underneath. When the project grows, keep the generated frontend calls and move individual registrations down to `gormadapter` or full `fddp.RegisterResource` only where custom behavior is needed.

## Growth path

Keep the small-project API stable while the backend evolves:

- start with `NewDevApp` and Lite builders
- switch to `NewProductionApp` when deploying
- add real token identity, distributed cache, and persistent idempotency through Core options
- move only the complex reads/writes to `gormadapter` or `app.Engine()`
- keep generated frontend `api.load(...)` and `api.command...` calls unchanged when the contract shape stays compatible

## Commands

Use `CreateCommand`, `UpdateCommand`, and `DeleteCommand` for common GORM-backed writes:

```go
type UpdateProfileInput struct {
  DisplayName string `json:"displayName"`
}

_ = fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
  Owner("user-domain").
  Self("UserID").
  Idempotent().
  Set("Name", "DisplayName").
  Invalidates("me.profile.*").
  Register()
```

For tenant-owned rows, add a server-side row condition with `Where`:

```go
_ = fddplite.UpdateCommand[Project, UpdateProjectInput](app, "project.status.update").
  Tenant("TenantID").
  Where("ID", "ID").
  Set("Status", "Status").
  Invalidates("project.list").
  Register()
```

```go
_ = fddplite.CreateCommand[Project, CreateProjectInput](app, "project.create").
  Tenant("TenantID").
  Set("ID", "ID").
  Set("Name", "Name").
  Invalidates("project.list").
  Register()

_ = fddplite.DeleteCommand[Project, DeleteProjectInput](app, "project.delete").
  Tenant("TenantID").
  Where("ID", "ID").
  Invalidates("project.list").
  Register()
```

`Set` maps server-known struct fields to server-known input fields. Empty non-pointer input values are skipped, so partial updates can omit fields. For custom validation or nonstandard writes, use the lower-level `Command` builder.
`UpdateCommand` requires at least one `Self`, `Tenant`, `Scope`, `Where`, or `WhereValue` boundary so an unsafe unscoped update cannot be registered by accident.

```go
_ = fddplite.Command[UpdateProfileInput](app, "user.profile.update.custom").
  Self().
  Register(func(ctx context.Context, req fddp.CommandExecutionRequest, input UpdateProfileInput) (fddp.CommandExecutionResult, error) {
    return fddp.CommandExecutionResult{Result: map[string]any{"ok": true}}, nil
  })
```

Move down to `gormadapter` or FDDP Core when you need custom column names, complex scopes, nonstandard relations, cache details, or hand-written resolvers.
