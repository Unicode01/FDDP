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
	)

	must(engine.RegisterFieldGroup(
		"me.profile",
		[]fddp.GroupField{
			{Name: "id", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "desc", Type: "string", Nullable: true},
		},
		func(ctx context.Context, req fddp.FieldGroupRequest) (map[string]any, error) {
			return map[string]any{
				"id":   "user_123",
				"name": "Tom",
				"desc": "Demo user",
			}, nil
		},
		fddp.FieldGroupOwner("user-domain"),
		fddp.FieldGroupPermission("self"),
		fddp.FieldGroupPrivateCache(10*time.Minute),
	))

	must(engine.RegisterStaticField(
		"global.config.appName",
		"FDDP Demo",
		fddp.FieldOwner("config-domain"),
		fddp.FieldPublicCache(time.Hour),
	))

	must(engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req fddp.ResourceRequest) (fddp.CollectionResult, error) {
			owners, err := req.Batcher.LoadMany(ctx, "users.byID", []string{"user_123"}, func(ctx context.Context, keys []string) (map[string]any, error) {
				return map[string]any{
					"user_123": map[string]any{"id": "user_123", "name": "Tom"},
				}, nil
			})
			if err != nil {
				return fddp.CollectionResult{}, err
			}

			return fddp.CollectionResult{
				Items: []any{
					map[string]any{"id": "project_1", "name": "Alpha", "owner": owners["user_123"]},
					map[string]any{"id": "project_2", "name": "Beta", "owner": owners["user_123"]},
				},
				PageInfo: &fddp.PageInfo{HasNextPage: false},
			}, nil
		},
		fddp.ResourceOwner("project-domain"),
		fddp.ResourcePermission("tenant"),
		fddp.ResourceMaxPageSize(50),
	))

	must(engine.RegisterCommand(fddp.CommandDefinition{
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
	}))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", engine.Handler()))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
