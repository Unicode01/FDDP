# go-fddp gormadapter

GORM adapter for FDDP query planning.

The adapter is intentionally explicit. It does not trust client field names as SQL identifiers.

## Security rules

- Every FDDP field must be mapped to a server-side column.
- Unmapped `selection.fields`, `filter`, `orderBy`, and `expand` entries are rejected.
- Configured columns and relation names are validated as identifiers.
- User values are passed through GORM parameter binding.
- Tenant, subject, visibility, and soft-delete rules should be added in `Scope`.
- Adapter errors include stable codes such as `FIELD_NOT_MAPPED`, `UNSAFE_IDENTIFIER`, and `UNSUPPORTED_FILTER` with short developer hints.

## Field group

Use a field group when several FDDP fields come from one row.

```go
type Profile struct {
  ID string
  UserID string
  Name string
  Description string
}

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

A request for `me.profile.id`, `me.profile.name`, and `me.profile.desc` becomes one `SELECT id,name,description ...` query.

## Collection

```go
_ = gormadapter.NewCollection[Project]("project.list", db).
  String("id", "ID").
  String("name", "Name").
  String("ownerId", "OwnerID", gormadapter.Column("owner_id")).
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
  Cursor("updatedAt").
  DefaultLimit(20).
  MaxPageSize(50).
  Permission("tenant").
  Register(engine)
```

The same adapter accepts safe filter operators from the client, for example:

```json
{
  "first": 20,
  "after": "2026-01-02T00:00:00Z",
  "filter": {
    "status": { "eq": "active" },
    "name": { "contains": "alpha" },
    "updatedAt": { "range": { "from": "2026-01-01T00:00:00Z" } }
  },
  "orderBy": [{ "field": "updatedAt", "direction": "asc" }]
}
```

Supported operators are `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `notIn`, `like`, `contains`, `range`, `between`, and `isNull`. Field names and operators are checked before GORM sees the query; values still go through parameter binding.

The lower-level `RegisterFieldGroup` and `RegisterCollection` structs remain available when you need dynamic configuration.
