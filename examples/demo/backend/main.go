package main

import (
	"log"
	"net/http"
	"os"
	"time"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"github.com/Unicode01/FDDP/packages/go-fddp/fddplite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Profile struct {
	ID          string `gorm:"primaryKey"`
	UserID      string `gorm:"index"`
	Name        string
	Description string
}

type User struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

type Project struct {
	ID        string `gorm:"primaryKey"`
	Name      string
	OwnerID   string
	Owner     User `gorm:"foreignKey:OwnerID"`
	Status    string
	TenantID  string `gorm:"index"`
	UpdatedAt string `gorm:"index"`
}

type UpdateProfileInput struct {
	DisplayName string `json:"displayName"`
}

func main() {
	db := openDemoDB()
	seedDemoDB(db)

	app := fddplite.NewProductionApp(db,
		fddp.WithCache(fddp.NewMemoryCache()),
		fddp.WithIdempotencyStore(fddp.NewMemoryIdempotencyStore()),
		fddp.WithContractVersion("contract_demo_gorm_v1"),
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
		fddp.WithCommandLimits(fddp.CommandLimits{
			MaxBodyBytes:  128 << 10,
			MaxInputBytes: 64 << 10,
			MaxInputDepth: 8,
			MaxInputNodes: 100,
			Timeout:       2 * time.Second,
		}),
	)

	must(registerProfile(app))
	must(registerProjects(app))
	must(registerCommands(app))
	must(app.Engine().RegisterStaticField(
		"global.config.appName",
		"FDDP Demo",
		fddp.FieldOwner("config-domain"),
		fddp.FieldPermission("public"),
		fddp.FieldPublicCache(0),
	))

	port := env("PORT", "8080")
	log.Printf("FDDP demo backend listening on http://localhost:%s", port)
	log.Printf("contract: http://localhost:%s/contract", port)
	log.Fatal(http.ListenAndServe(":"+port, app.Handler()))
}

func registerProfile(app *fddplite.App) error {
	return fddplite.FieldGroup[Profile](app, "me.profile").
		Fields("ID", "Name", "Description").
		Self("UserID").
		Owner("user-domain").
		Register()
}

func registerProjects(app *fddplite.App) error {
	return fddplite.Collection[Project](app, "project.list").
		Fields("ID", "Name", "OwnerID", "Status", "UpdatedAt").
		Tenant("TenantID").
		DescCursor("UpdatedAt").
		TotalCount().
		DefaultLimit(20).
		MaxPageSize(50).
		Relation("owner", "Owner", "ID", "Name").
		Owner("project-domain").
		Register()
}

func registerCommands(app *fddplite.App) error {
	return fddplite.UpdateCommand[Profile, UpdateProfileInput](app, "user.profile.update").
		Owner("user-domain").
		Self("UserID").
		Idempotent().
		SetValue("Name", func(input UpdateProfileInput) (any, bool) {
			if input.DisplayName == "" {
				input.DisplayName = "Tom"
			}
			return input.DisplayName, true
		}).
		Invalidates("me.profile.*").
		Register()
}

func openDemoDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:fddp_demo?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal(err)
	}
	must(db.AutoMigrate(&Profile{}, &User{}, &Project{}))
	return db
}

func seedDemoDB(db *gorm.DB) {
	must(db.Create(&Profile{
		ID:          "profile_1",
		UserID:      "user_123",
		Name:        "Tom",
		Description: "Demo user",
	}).Error)

	must(db.Create(&User{ID: "user_123", Name: "Tom"}).Error)
	must(db.Create(&User{ID: "user_456", Name: "Ann"}).Error)

	projects := []Project{
		{ID: "project_1", Name: "Alpha", OwnerID: "user_123", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-03T00:00:00Z"},
		{ID: "project_2", Name: "Beta", OwnerID: "user_456", Status: "active", TenantID: "tenant_abc", UpdatedAt: "2026-01-02T00:00:00Z"},
		{ID: "project_3", Name: "Archived", OwnerID: "user_123", Status: "archived", TenantID: "tenant_abc", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "project_4", Name: "Other Tenant", OwnerID: "user_123", Status: "active", TenantID: "tenant_other", UpdatedAt: "2026-01-04T00:00:00Z"},
	}
	for _, project := range projects {
		must(db.Create(&project).Error)
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func env(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
