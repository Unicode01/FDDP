# Grow From Lite To Core

FDDP Lite is meant to be a starting point, not a dead end.

Keep simple domains on Lite. Move only the domain that needs more control down to `gormadapter` or FDDP Core. If the contract path and field names stay compatible, frontend calls such as `api.load(...)` can stay the same.

This guide uses the same `project.list` collection through all three levels.

## Level 1: Lite

Use Lite when your GORM model follows normal field and column names.

```go
app := fddplite.NewDevApp(db)

must(fddplite.Collection[Project](app, "project.list").
  Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
  Tenant("TenantID").
  DescCursor("UpdatedAt").
  Relation("owner", "Owner", "ID", "Name").
  Register())
```

Lite derives the contract fields:

- `ID` -> `id`
- `OwnerID` -> `ownerId`
- `UpdatedAt` -> `updatedAt`

It also derives common column names such as `OwnerID` -> `owner_id` and adds the tenant scope from `Tenant("TenantID")`.

Use this level for most small-project lists.

## Level 2: gormadapter

Move to `gormadapter` when one domain needs explicit mapping while still using GORM query planning.

Common reasons:

- The database column names do not follow the default convention.
- A field needs a different public contract name.
- A relation needs explicit parent or required keys.
- The query needs a custom GORM scope.
- You want cache, owner, permission, or max page size metadata closer to the mapped resource.

The frontend contract path can remain `project.list`:

```go
engine := app.Engine()

must(gormadapter.NewCollection[Project]("project.list", db).
  String("id", "ID").
  String("name", "Name").
  String("ownerId", "OwnerID", gormadapter.Column("owner_id")).
  String("status", "Status").
  String("updatedAt", "UpdatedAt", gormadapter.Column("updated_at")).
  Relation("owner", "Owner", func(owner *gormadapter.RelationBuilder) {
    owner.
      ParentFields("ownerId").
      RequiredFields("id").
      String("id", "ID").
      String("name", "Name")
  }).
  Scope(func(tx *gorm.DB, req fddp.ResourceRequest) *gorm.DB {
    return tx.Where("tenant_id = ?", req.Identity.TenantID)
  }).
  DescCursor("updatedAt").
  DefaultLimit(20).
  MaxPageSize(50).
  Permission("tenant").
  Register(engine))
```

At this level, client fields still never become SQL identifiers directly. Every selectable, filterable, orderable, and expandable field is mapped on the server.

## Level 3: Core

Move to Core when the data no longer fits a normal GORM collection.

Common reasons:

- The result comes from several services or databases.
- The query needs custom batching or request coalescing.
- The field-level result needs custom permission logic.
- The list is backed by search, cache, CQRS projections, or a hand-written SQL path.
- You need custom trace, fallback, or partial failure behavior.

Keep the same contract path and field names when possible:

```go
engine := app.Engine()

must(engine.RegisterCollection(
  "project.list",
  func(ctx context.Context, req fddp.ResourceRequest) (fddp.CollectionResult, error) {
    first := req.Collection.First
    if first == 0 {
      first = 20
    }

    projects, pageInfo, err := projectService.Search(ctx, ProjectSearch{
      TenantID: req.Identity.TenantID,
      First:    first,
      After:    req.Collection.After,
      Filter:   req.Collection.Filter,
      OrderBy:  req.Collection.OrderBy,
      Fields:   req.Selection.Fields,
      Expand:   req.Selection.Expand,
    })
    if err != nil {
      return fddp.CollectionResult{}, err
    }

    items := make([]any, 0, len(projects))
    for _, project := range projects {
      items = append(items, map[string]any{
        "id":        project.ID,
        "name":      project.Name,
        "ownerId":   project.OwnerID,
        "status":    project.Status,
        "updatedAt": project.UpdatedAt,
        "owner": map[string]any{
          "id":   project.Owner.ID,
          "name": project.Owner.Name,
        },
      })
    }

    return fddp.CollectionResult{
      Items:    items,
      PageInfo: pageInfo,
    }, nil
  },
  fddp.ResourcePermission("tenant"),
  fddp.ResourceMaxPageSize(50),
  fddp.ResourceFields(
    fddp.ContractResourceField{Field: "id", Type: "string", Filterable: true, Orderable: true},
    fddp.ContractResourceField{Field: "name", Type: "string", Filterable: true, Orderable: true},
    fddp.ContractResourceField{Field: "ownerId", Type: "string", Filterable: true, Orderable: true},
    fddp.ContractResourceField{Field: "status", Type: "string", Filterable: true, Orderable: true},
    fddp.ContractResourceField{Field: "updatedAt", Type: "string", Filterable: true, Orderable: true},
  ),
  fddp.ResourceRelations(
    fddp.ContractResourceRelation{
      Name: "owner",
      Fields: []fddp.ContractResourceField{
        {Field: "id", Type: "string"},
        {Field: "name", Type: "string"},
      },
    },
  ),
))
```

Core gives full control, but it also means you own safe query planning. Do not pass client field names directly to SQL or downstream services. Treat `req.Selection`, `req.Collection.Filter`, and `req.Collection.OrderBy` as contract-checked input that still needs server-side mapping.

## What Stays Stable

The frontend can stay stable when these contract details remain compatible:

- Resource path: `project.list`
- Public field names: `id`, `name`, `ownerId`, `status`, `updatedAt`
- Relation name: `owner`
- Relation fields: `id`, `name`
- Filter and order capability for fields already used by the frontend
- Page size and required input rules

The generated call can remain:

```ts
await api.load({
  projectList: {
    first: 20,
    filter: { status: { eq: "active" } },
    orderBy: [{ field: "updatedAt", direction: "desc" }],
    fields: ["id", "name", "updatedAt"],
    expand: { owner: ["id", "name"] }
  }
});
```

Run contract checks before replacing a Lite registration:

```bash
npx fddp check --contract http://localhost:8080/contract
npx fddp diff --from contracts/before.json --to contracts/after.json
```

## Practical Rule

Start with Lite.

Drop only the complicated domain to `gormadapter` when naming, mapping, or relation details need to be explicit.

Drop only the exceptional domain to Core when GORM is no longer the right execution model.

Do not migrate everything at once. Lite, `gormadapter`, and Core registrations can coexist on the same `app.Engine()`.
