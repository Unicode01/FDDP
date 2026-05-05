package gormadapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testProfile struct {
	ID          string
	UserID      string
	Name        string
	Description string
	Secret      string
}

type testProject struct {
	ID        string
	Name      string
	OwnerID   string
	Owner     testUser `gorm:"foreignKey:OwnerID"`
	Status    string
	TenantID  string
	UpdatedAt string
}

type testUser struct {
	ID     string
	Name   string
	Secret string
}

func TestGormFieldGroupSelectsOnlyRequestedMappedColumns(t *testing.T) {
	db := openTestDB(t)
	must(t, db.AutoMigrate(&testProfile{}))
	must(t, db.Create(&testProfile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: "Demo", Secret: "hidden"}).Error)

	engine := fddp.NewEngine()
	err := RegisterFieldGroup[testProfile](engine, FieldGroup[testProfile]{
		Path: "me.profile",
		DB:   db,
		Fields: map[string]FieldMapping{
			"id":   {Type: "string", Column: "id", StructField: "ID"},
			"name": {Type: "string", Column: "name", StructField: "Name"},
			"desc": {Type: "string", Column: "description", StructField: "Description", Nullable: true},
		},
		Scope: func(tx *gorm.DB, req fddp.FieldGroupRequest) *gorm.DB {
			return tx.Where("user_id = ?", req.Identity.Subject)
		},
		Options: []fddp.FieldGroupOption{fddp.FieldGroupPermission("self")},
	})
	must(t, err)

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"me":{"profile":["id","name"]}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if result.Status != 200 || len(result.Response.Errors) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	profile := result.Response.Data["me"].(map[string]any)["profile"].(map[string]any)
	if profile["id"] != "profile_1" || profile["name"] != "Tom" {
		t.Fatalf("unexpected projected profile: %#v", profile)
	}
	if _, ok := profile["desc"]; ok {
		t.Fatalf("unexpected unrequested desc field: %#v", profile)
	}
}

func TestBuilderRegistersFieldGroupWithDefaults(t *testing.T) {
	db := openTestDB(t)
	must(t, db.AutoMigrate(&testProfile{}))
	must(t, db.Create(&testProfile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: "Demo", Secret: "hidden"}).Error)

	engine := fddp.NewEngine()
	err := NewFieldGroup[testProfile]("me.profile", db).
		String("id", "ID").
		String("name", "Name").
		NullableString("desc", "Description", Column("description")).
		Scope(func(tx *gorm.DB, req fddp.FieldGroupRequest) *gorm.DB {
			return tx.Where("user_id = ?", req.Identity.Subject)
		}).
		Permission("self").
		Register(engine)
	must(t, err)

	contract := engine.Contract()
	if len(contract.Fields) != 3 {
		t.Fatalf("expected 3 contract fields, got %#v", contract.Fields)
	}

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"me":{"profile":["name","desc"]}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if result.Status != 200 || len(result.Response.Errors) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	profile := result.Response.Data["me"].(map[string]any)["profile"].(map[string]any)
	if profile["name"] != "Tom" || profile["desc"] != "Demo" {
		t.Fatalf("unexpected projected profile: %#v", profile)
	}
}

func TestGormCollectionRejectsUnmappedFields(t *testing.T) {
	db := openTestDB(t)
	engine := fddp.NewEngine()
	must(t, RegisterCollection[testProject](engine, Collection[testProject]{
		Path: "project.list",
		DB:   db,
		Fields: map[string]FieldMapping{
			"id": {Type: "string", Column: "id", StructField: "ID"},
		},
		Options: []fddp.ResourceOption{fddp.ResourcePermission("tenant")},
	}))

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"first":10,"filter":{"name;drop table users":"x"}}}}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(result.Response.Errors) != 1 || result.Response.Errors[0].Code != string(ErrorFieldNotMapped) {
		t.Fatalf("expected unmapped filter to fail safely, got %#v", result.Response.Errors)
	}
	if !strings.Contains(result.Response.Errors[0].Reason, "not mapped") {
		t.Fatalf("expected safe mapping error, got %#v", result.Response.Errors[0])
	}
}

func TestBuilderCollectionFiltersCursorCountAndRelation(t *testing.T) {
	db := openTestDB(t)
	must(t, db.AutoMigrate(&testUser{}, &testProject{}))
	must(t, db.Create(&testUser{ID: "u1", Name: "Tom", Secret: "hidden"}).Error)
	must(t, db.Create(&testUser{ID: "u2", Name: "Ann", Secret: "hidden"}).Error)
	must(t, db.Create(&testProject{ID: "p1", Name: "Alpha", OwnerID: "u1", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-01T00:00:00Z"}).Error)
	must(t, db.Create(&testProject{ID: "p2", Name: "Beta", OwnerID: "u2", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-02T00:00:00Z"}).Error)
	must(t, db.Create(&testProject{ID: "p3", Name: "Delta", OwnerID: "u1", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-03T00:00:00Z"}).Error)
	must(t, db.Create(&testProject{ID: "p4", Name: "Other", OwnerID: "u1", Status: "active", TenantID: "other_tenant", UpdatedAt: "2026-01-04T00:00:00Z"}).Error)

	engine := fddp.NewEngine()
	err := NewCollection[testProject]("project.list", db).
		String("id", "ID").
		String("name", "Name").
		String("ownerId", "OwnerID", Column("owner_id")).
		String("status", "Status").
		String("updatedAt", "UpdatedAt", Column("updated_at")).
		Relation("owner", "Owner", func(owner *RelationBuilder) {
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
		TotalCount().
		Permission("tenant").
		Register(engine)
	must(t, err)

	contract := engine.Contract()
	if len(contract.Resources) != 1 {
		t.Fatalf("expected collection resource in contract, got %#v", contract.Resources)
	}
	resource := contract.Resources[0]
	if len(resource.Fields) != 5 || resource.Fields[0].Field != "id" || !resource.Fields[0].Filterable || !resource.Fields[0].Orderable {
		t.Fatalf("expected selectable resource fields in contract, got %#v", resource.Fields)
	}
	if len(resource.Relations) != 1 || resource.Relations[0].Name != "owner" || len(resource.Relations[0].Fields) != 2 {
		t.Fatalf("expected relation fields in contract, got %#v", resource.Relations)
	}

	firstPage := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body: []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"first":2,"filter":{"status":{"eq":"active"},"name":{"contains":"a"}}},"selection":{"fields":["id","name","updatedAt"],"expand":{"owner":["id","name"]}}}}}}`),
		Identity: fddp.IdentityContext{
			Subject:  "user_123",
			TenantID: "tenant_abc",
		},
	})
	if len(firstPage.Response.Errors) != 0 {
		t.Fatalf("unexpected first page errors: %#v", firstPage.Response.Errors)
	}
	list := firstPage.Response.Data["project"].(map[string]any)["list"].(fddp.CollectionResult)
	if list.TotalCount == nil || *list.TotalCount != 3 {
		t.Fatalf("expected totalCount 3, got %#v", list.TotalCount)
	}
	if !list.PageInfo.HasNextPage || list.PageInfo.EndCursor != "2026-01-02T00:00:00Z" {
		t.Fatalf("unexpected first page info: %#v", list.PageInfo)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items, got %#v", list.Items)
	}
	first := list.Items[0].(map[string]any)
	owner := first["owner"].(map[string]any)
	if owner["id"] != "u1" || owner["name"] != "Tom" {
		t.Fatalf("unexpected owner projection: %#v", owner)
	}
	if _, ok := first["ownerId"]; ok {
		t.Fatalf("hidden parent field leaked into result: %#v", first)
	}
	if _, ok := owner["secret"]; ok {
		t.Fatalf("hidden owner field leaked into result: %#v", owner)
	}

	secondPageBody := []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"first":2,"after":"2026-01-02T00:00:00Z","filter":{"status":{"eq":"active"},"name":{"contains":"a"}}},"selection":{"fields":["id","name","updatedAt"],"expand":{"owner":["id","name"]}}}}}}`)
	secondPage := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body: secondPageBody,
		Identity: fddp.IdentityContext{
			Subject:  "user_123",
			TenantID: "tenant_abc",
		},
	})
	if len(secondPage.Response.Errors) != 0 {
		t.Fatalf("unexpected second page errors: %#v", secondPage.Response.Errors)
	}
	list = secondPage.Response.Data["project"].(map[string]any)["list"].(fddp.CollectionResult)
	if len(list.Items) != 1 || list.PageInfo.HasNextPage || !list.PageInfo.HasPreviousPage {
		t.Fatalf("unexpected second page result: %#v", list)
	}
	item := list.Items[0].(map[string]any)
	if item["id"] != "p3" {
		t.Fatalf("unexpected second page item: %#v", item)
	}
}

func TestGormCollectionRejectsUnsafeOperatorsAndNestedExpand(t *testing.T) {
	db := openTestDB(t)
	engine := fddp.NewEngine(fddp.WithoutQueryLimits())
	must(t, NewCollection[testProject]("project.list", db).
		String("id", "ID").
		String("name", "Name").
		Register(engine))

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"filter":{"name":{"raw":"name = name; drop table users"}}},"selection":{"fields":["id"]}}}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(result.Response.Errors) != 1 || result.Response.Errors[0].Code != string(ErrorUnsupportedFilter) {
		t.Fatalf("expected unsupported operator error, got %#v", result.Response.Errors)
	}

	result = engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","selection":{"fields":["id"],"expand":{"owner":{"fields":["id"],"expand":{"profile":["id"]}}}}}}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(result.Response.Errors) != 1 || result.Response.Errors[0].Code != string(ErrorUnsupportedExpand) {
		t.Fatalf("expected nested expand error, got %#v", result.Response.Errors)
	}
}

func TestGormCollectionProjectsRelationWithWhitelistedPreload(t *testing.T) {
	db := openTestDB(t)
	must(t, db.AutoMigrate(&testUser{}, &testProject{}))
	must(t, db.Create(&testUser{ID: "u1", Name: "Tom", Secret: "hidden"}).Error)
	must(t, db.Create(&testProject{ID: "p1", Name: "Alpha", OwnerID: "u1"}).Error)

	engine := fddp.NewEngine()
	must(t, RegisterCollection[testProject](engine, Collection[testProject]{
		Path: "project.list",
		DB:   db,
		Fields: map[string]FieldMapping{
			"id":      {Type: "string", Column: "id", StructField: "ID"},
			"name":    {Type: "string", Column: "name", StructField: "Name"},
			"ownerId": {Type: "string", Column: "owner_id", StructField: "OwnerID"},
		},
		Relations: map[string]RelationMapping{
			"owner": {
				Name: "Owner",
				Fields: map[string]FieldMapping{
					"id":   {Type: "string", Column: "id", StructField: "ID"},
					"name": {Type: "string", Column: "name", StructField: "Name"},
				},
			},
		},
		Options: []fddp.ResourceOption{fddp.ResourcePermission("tenant")},
	}))

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"first":10,"orderBy":[{"field":"name","direction":"asc"}]},"selection":{"fields":["id","name","ownerId"],"expand":{"owner":["id","name"]}}}}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(result.Response.Errors) != 0 {
		t.Fatalf("unexpected collection errors: %#v", result.Response.Errors)
	}
	list := result.Response.Data["project"].(map[string]any)["list"].(fddp.CollectionResult)
	items := list.Items
	item := items[0].(map[string]any)
	owner := item["owner"].(map[string]any)
	if owner["id"] != "u1" || owner["name"] != "Tom" {
		t.Fatalf("unexpected owner projection: %#v", owner)
	}
	if _, ok := owner["secret"]; ok {
		t.Fatalf("unexpected secret projection: %#v", owner)
	}
}

func TestRejectsUnsafeConfiguredColumn(t *testing.T) {
	err := RegisterFieldGroup[testProfile](fddp.NewEngine(), FieldGroup[testProfile]{
		Path: "me.profile",
		DB:   openTestDB(t),
		Fields: map[string]FieldMapping{
			"name": {Type: "string", Column: "name;drop table users", StructField: "Name"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe column") {
		t.Fatalf("expected unsafe column error, got %v", err)
	}
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Code != ErrorUnsafeIdentifier {
		t.Fatalf("expected unsafe identifier code, got %#v", err)
	}
	if adapterErr.Hint == "" {
		t.Fatalf("expected developer hint, got %#v", adapterErr)
	}
}

func TestAdapterErrorsExposeStableCodes(t *testing.T) {
	db := openTestDB(t)
	engine := fddp.NewEngine()
	must(t, NewCollection[testProject]("project.list", db).
		String("id", "ID").
		Register(engine))

	result := engine.ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","selection":{"fields":["missing"]}}}}}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(result.Response.Errors) != 1 {
		t.Fatalf("expected one error, got %#v", result.Response.Errors)
	}
	if !strings.Contains(result.Response.Errors[0].Reason, string(ErrorFieldNotMapped)) && !strings.Contains(result.Response.Errors[0].Reason, "not mapped") {
		t.Fatalf("expected stable mapping error detail, got %#v", result.Response.Errors[0])
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
