package fddplite

import (
	"context"
	"testing"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type liteProfile struct {
	ID          string
	UserID      string
	Name        string
	Description *string
}

type liteUser struct {
	ID   string
	Name string
}

type liteProject struct {
	ID        string
	Name      string
	OwnerID   string
	Owner     liteUser `gorm:"foreignKey:OwnerID"`
	Status    string
	TenantID  string
	UpdatedAt string
}

type liteUpdateInput struct {
	DisplayName string `json:"displayName"`
}

type liteUpdateProfileInput struct {
	DisplayName string  `json:"displayName"`
	Description *string `json:"description,omitempty"`
}

type liteUpdateProjectInput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type liteCreateProjectInput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type liteDeleteProjectInput struct {
	ID string `json:"id"`
}

func TestLiteRegistersFieldGroupCollectionAndCommand(t *testing.T) {
	db := openLiteDB(t)
	desc := "Demo user"
	must(t, db.AutoMigrate(&liteProfile{}, &liteUser{}, &liteProject{}))
	must(t, db.Create(&liteProfile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: &desc}).Error)
	must(t, db.Create(&liteUser{ID: "user_123", Name: "Tom"}).Error)
	must(t, db.Create(&liteProject{ID: "project_1", Name: "Alpha", OwnerID: "user_123", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-02T00:00:00Z"}).Error)
	must(t, db.Create(&liteProject{ID: "project_2", Name: "Other", OwnerID: "user_123", Status: "active", TenantID: "other", UpdatedAt: "2026-01-03T00:00:00Z"}).Error)

	app := NewApp(db)
	must(t, FieldGroup[liteProfile](app, "me.profile").
		Fields("ID", "Name", "Description").
		Self("UserID").
		Register())
	must(t, Collection[liteProject](app, "project.list").
		Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
		Tenant("TenantID").
		DescCursor("UpdatedAt").
		TotalCount().
		Relation("owner", "Owner", "ID", "Name").
		Register())
	must(t, Command[liteUpdateInput](app, "user.profile.update").
		Self().
		Idempotent().
		Register(func(ctx context.Context, req fddp.CommandExecutionRequest, input liteUpdateInput) (fddp.CommandExecutionResult, error) {
			return fddp.CommandExecutionResult{Result: map[string]any{"displayName": input.DisplayName}}, nil
		}))

	query := app.Engine().ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body: []byte(`{"query":{"me":{"profile":["id","name","description"]},"project":{"list":{"$type":"collection","args":{"first":10,"filter":{"status":{"eq":"active"}},"orderBy":[{"field":"updatedAt","direction":"desc"}]},"selection":{"fields":["id","name","updatedAt"],"expand":{"owner":["id","name"]}}}}}}`),
		Identity: fddp.IdentityContext{
			Subject:  "user_123",
			TenantID: "tenant_abc",
		},
	})
	if len(query.Response.Errors) != 0 {
		t.Fatalf("unexpected query errors: %#v", query.Response.Errors)
	}
	profile := query.Response.Data["me"].(map[string]any)["profile"].(map[string]any)
	if profile["name"] != "Tom" || profile["description"] != "Demo user" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	list := query.Response.Data["project"].(map[string]any)["list"].(fddp.CollectionResult)
	if len(list.Items) != 1 || list.TotalCount == nil || *list.TotalCount != 1 {
		t.Fatalf("unexpected collection result: %#v", list)
	}
	item := list.Items[0].(map[string]any)
	owner := item["owner"].(map[string]any)
	if owner["name"] != "Tom" {
		t.Fatalf("unexpected owner: %#v", owner)
	}

	command := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"user.profile.update","input":{"displayName":"Demo Tom"},"idempotencyKey":"cmd_1"}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(command.Response.Errors) != 0 || command.Response.Data.Status != "completed" {
		t.Fatalf("unexpected command response: %#v", command.Response)
	}

	contract := app.Engine().Contract()
	if len(contract.Commands) != 1 || len(contract.Commands[0].Input) != 1 || contract.Commands[0].Input[0].Field != "displayName" {
		t.Fatalf("expected command input schema, got %#v", contract.Commands)
	}
}

func TestLiteDevAppAddsDeveloperStores(t *testing.T) {
	db := openLiteDB(t)
	desc := "Demo user"
	must(t, db.AutoMigrate(&liteProfile{}))
	must(t, db.Create(&liteProfile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: &desc}).Error)

	app := NewDevApp(db)
	must(t, UpdateCommand[liteProfile, liteUpdateProfileInput](app, "user.profile.update").
		Self("UserID").
		Idempotent().
		Set("Name", "DisplayName").
		Register())

	body := []byte(`{"command":"user.profile.update","input":{"displayName":"Demo Tom"},"idempotencyKey":"cmd_1","trace":true}`)
	first := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     body,
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(first.Response.Errors) != 0 || first.Response.Data.Status != "completed" {
		t.Fatalf("unexpected first command response: %#v", first.Response)
	}

	second := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     body,
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(second.Response.Errors) != 0 || second.Response.Meta.Trace == nil || !second.Response.Meta.Trace.IdempotencyHit {
		t.Fatalf("expected dev app idempotency hit, got %#v", second.Response)
	}
}

func TestLiteProductionAppAppliesSafeLimits(t *testing.T) {
	db := openLiteDB(t)
	app := NewProductionApp(db)
	must(t, Collection[liteProject](app, "project.list").
		Fields("ID", "Name").
		Tenant("TenantID").
		Register())

	response := app.Engine().ExecuteQueryBody(context.Background(), fddp.QueryEndpointRequest{
		Body:     []byte(`{"query":{"project":{"list":{"$type":"collection","args":{"first":1000},"selection":{"fields":["id","name"]}}}},"trace":true}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(response.Response.Errors) == 0 || response.Response.Errors[0].Code != "QUERY_LIMIT_EXCEEDED" {
		t.Fatalf("expected production query limit rejection, got %#v", response.Response)
	}
}

func TestLiteUpdateCommandUpdatesScopedModel(t *testing.T) {
	db := openLiteDB(t)
	desc := "Demo user"
	otherDesc := "Other user"
	must(t, db.AutoMigrate(&liteProfile{}))
	must(t, db.Create(&liteProfile{ID: "profile_1", UserID: "user_123", Name: "Tom", Description: &desc}).Error)
	must(t, db.Create(&liteProfile{ID: "profile_2", UserID: "user_456", Name: "Ann", Description: &otherDesc}).Error)

	app := NewApp(db)
	must(t, UpdateCommand[liteProfile, liteUpdateProfileInput](app, "user.profile.update").
		Self("UserID").
		Idempotent().
		Set("Name", "DisplayName").
		Set("Description", "Description").
		Invalidates("me.profile.*").
		Register())

	command := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"user.profile.update","input":{"displayName":"Demo Tom","description":"Updated user"},"idempotencyKey":"cmd_1"}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(command.Response.Errors) != 0 || command.Response.Data.Status != "completed" {
		t.Fatalf("unexpected command response: %#v", command.Response)
	}
	result, ok := command.Response.Data.Result.(map[string]any)
	if !ok || result["updated"] != true || result["rowsAffected"] != int64(1) {
		t.Fatalf("unexpected update result: %#v", command.Response.Data.Result)
	}
	if len(command.Response.Data.Invalidates) != 1 || command.Response.Data.Invalidates[0] != "me.profile.*" {
		t.Fatalf("unexpected invalidates: %#v", command.Response.Data.Invalidates)
	}

	var profile liteProfile
	must(t, db.First(&profile, "id = ?", "profile_1").Error)
	if profile.Name != "Demo Tom" || profile.Description == nil || *profile.Description != "Updated user" {
		t.Fatalf("profile was not updated: %#v", profile)
	}
	var other liteProfile
	must(t, db.First(&other, "id = ?", "profile_2").Error)
	if other.Name != "Ann" || other.Description == nil || *other.Description != "Other user" {
		t.Fatalf("other profile should not be updated: %#v", other)
	}

	empty := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"user.profile.update","input":{"description":"Only desc"},"idempotencyKey":"cmd_2"}`),
		Identity: fddp.IdentityContext{Subject: "user_123"},
	})
	if len(empty.Response.Errors) != 0 {
		t.Fatalf("unexpected partial update errors: %#v", empty.Response.Errors)
	}
	must(t, db.First(&profile, "id = ?", "profile_1").Error)
	if profile.Name != "Demo Tom" || profile.Description == nil || *profile.Description != "Only desc" {
		t.Fatalf("zero-value input should be skipped: %#v", profile)
	}
}

func TestLiteUpdateCommandSupportsTenantScopedWhere(t *testing.T) {
	db := openLiteDB(t)
	must(t, db.AutoMigrate(&liteProject{}))
	must(t, db.Create(&liteProject{ID: "p1", Name: "Alpha", Status: "draft", TenantID: "tenant_abc"}).Error)
	must(t, db.Create(&liteProject{ID: "p2", Name: "Beta", Status: "draft", TenantID: "tenant_abc"}).Error)
	must(t, db.Create(&liteProject{ID: "p3", Name: "Other", Status: "draft", TenantID: "other_tenant"}).Error)

	app := NewApp(db)
	must(t, UpdateCommand[liteProject, liteUpdateProjectInput](app, "project.status.update").
		Tenant("TenantID").
		Where("ID", "ID").
		Set("Status", "Status").
		Register())

	command := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"project.status.update","input":{"id":"p1","status":"active"}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(command.Response.Errors) != 0 || command.Response.Data.Status != "completed" {
		t.Fatalf("unexpected command response: %#v", command.Response)
	}

	var projects []liteProject
	must(t, db.Order("id asc").Find(&projects).Error)
	status := map[string]string{}
	for _, project := range projects {
		status[project.ID] = project.Status
	}
	if status["p1"] != "active" || status["p2"] != "draft" || status["p3"] != "draft" {
		t.Fatalf("update should be limited by tenant and where, got %#v", status)
	}
}

func TestLiteUpdateCommandRequiresScopeOrWhere(t *testing.T) {
	db := openLiteDB(t)
	must(t, db.AutoMigrate(&liteProject{}))
	app := NewApp(db)

	err := UpdateCommand[liteProject, liteUpdateProjectInput](app, "project.status.update").
		Set("Status", "Status").
		Register()
	if err == nil {
		t.Fatalf("expected unsafe update command registration to fail")
	}
}

func TestLiteCreateAndDeleteCommands(t *testing.T) {
	db := openLiteDB(t)
	must(t, db.AutoMigrate(&liteProject{}))
	must(t, db.Create(&liteProject{ID: "other", Name: "Other", TenantID: "other_tenant"}).Error)

	app := NewApp(db)
	must(t, CreateCommand[liteProject, liteCreateProjectInput](app, "project.create").
		Tenant("TenantID").
		Set("ID", "ID").
		Set("Name", "Name").
		Set("Status", "Status").
		Invalidates("project.list").
		Register())
	must(t, DeleteCommand[liteProject, liteDeleteProjectInput](app, "project.delete").
		Tenant("TenantID").
		Where("ID", "ID").
		Invalidates("project.list").
		Register())

	create := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"project.create","input":{"id":"p1","name":"Alpha","status":"active"}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(create.Response.Errors) != 0 || create.Response.Data.Status != "completed" {
		t.Fatalf("unexpected create response: %#v", create.Response)
	}
	var project liteProject
	must(t, db.First(&project, "id = ?", "p1").Error)
	if project.Name != "Alpha" || project.TenantID != "tenant_abc" || project.Status != "active" {
		t.Fatalf("unexpected created project: %#v", project)
	}

	wrongTenantDelete := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"project.delete","input":{"id":"other"}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(wrongTenantDelete.Response.Errors) != 0 {
		t.Fatalf("unexpected delete response: %#v", wrongTenantDelete.Response)
	}
	var count int64
	must(t, db.Model(&liteProject{}).Where("id = ?", "other").Count(&count).Error)
	if count != 1 {
		t.Fatalf("delete crossed tenant boundary")
	}

	delete := app.Engine().ExecuteCommandBody(context.Background(), fddp.CommandEndpointRequest{
		Body:     []byte(`{"command":"project.delete","input":{"id":"p1"}}`),
		Identity: fddp.IdentityContext{Subject: "user_123", TenantID: "tenant_abc"},
	})
	if len(delete.Response.Errors) != 0 || delete.Response.Data.Status != "completed" {
		t.Fatalf("unexpected delete response: %#v", delete.Response)
	}
	must(t, db.Model(&liteProject{}).Where("id = ?", "p1").Count(&count).Error)
	if count != 0 {
		t.Fatalf("expected project to be deleted, count=%d", count)
	}
}

func openLiteDB(t *testing.T) *gorm.DB {
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
