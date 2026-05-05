package fddp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryResolvesAndCachesField(t *testing.T) {
	engine := NewEngine(
		WithCache(NewMemoryCache()),
		WithTraceIDFunc(func() string { return "trace_test" }),
		WithContractVersion("contract_test"),
	)
	calls := 0

	must(t, engine.RegisterField(FieldDefinition{
		Path:       "me.profile.name",
		Permission: "self",
		Cache:      CachePolicy{Scope: CacheScopePrivate, TTL: time.Minute},
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			calls++
			return "Tom", nil
		},
	}))

	body := `{"query":{"me":{"profile":["name"]}},"trace":true}`
	first := performQuery(engine, body)
	if first.Meta.TraceID != "trace_test" {
		t.Fatalf("unexpected trace id: %s", first.Meta.TraceID)
	}
	if first.Meta.ContractVersion != "contract_test" {
		t.Fatalf("unexpected contract version: %s", first.Meta.ContractVersion)
	}
	if calls != 1 {
		t.Fatalf("expected resolver once, got %d", calls)
	}
	if got := nestedString(first.Data, "me", "profile", "name"); got != "Tom" {
		t.Fatalf("unexpected data: %q", got)
	}
	if len(first.Meta.Cache.Miss) != 1 || first.Meta.Cache.Miss[0] != "me.profile.name" {
		t.Fatalf("expected cache miss meta, got %#v", first.Meta.Cache.Miss)
	}
	if first.Meta.Trace == nil || len(first.Meta.Trace.CacheMiss) != 1 || first.Meta.Trace.CacheMiss[0] != "me.profile.name" {
		t.Fatalf("expected cache miss trace, got %#v", first.Meta.Trace)
	}

	second := performQuery(engine, body)
	if calls != 1 {
		t.Fatalf("expected cache hit without resolver, got %d calls", calls)
	}
	if len(second.Meta.Cache.Hit) != 1 || second.Meta.Cache.Hit[0] != "me.profile.name" {
		t.Fatalf("expected cache hit meta, got %#v", second.Meta.Cache.Hit)
	}
	if second.Meta.Trace == nil || len(second.Meta.Trace.CacheHit) != 1 || second.Meta.Trace.CacheHit[0] != "me.profile.name" {
		t.Fatalf("expected cache hit trace, got %#v", second.Meta.Trace)
	}
}

func TestQueryDeniedField(t *testing.T) {
	engine := NewEngine(WithTraceIDFunc(func() string { return "trace_denied" }))
	must(t, engine.RegisterField(FieldDefinition{
		Path:       "me.profile.email",
		Permission: "admin",
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			return "tom@example.com", nil
		},
	}))

	response := performQuery(engine, `{"query":{"me":{"profile":["email"]}},"trace":true}`)
	if len(response.Errors) != 1 || response.Errors[0].Code != "FIELD_DENIED" {
		t.Fatalf("expected denied error, got %#v", response.Errors)
	}
	if len(response.Meta.Trace.Denied) != 1 || response.Meta.Trace.Denied[0] != "me.profile.email" {
		t.Fatalf("expected denied trace, got %#v", response.Meta.Trace.Denied)
	}
}

func TestBearerTokenIdentityResolver(t *testing.T) {
	engine := NewEngine(WithIdentityResolver(BearerTokenIdentityResolver(BearerTokenVerifierFunc(func(ctx context.Context, token string) (TokenClaims, error) {
		if token != "token_123" {
			t.Fatalf("unexpected token: %q", token)
		}
		return TokenClaims{
			Subject:           "user_123",
			TenantID:          "tenant_abc",
			Roles:             []string{"tenant_admin"},
			Scopes:            []string{"profile.read"},
			PermissionVersion: "perm_v17",
			PolicyVersion:     "policy_v9",
			MFA:               true,
			SessionLevel:      "strong",
			Attributes:        map[string]string{"region": "ap"},
		}, nil
	}))))
	must(t, engine.RegisterStringField(
		"me.profile.name",
		func(ctx context.Context, req FieldRequest) (string, error) {
			if req.Identity.Subject != "user_123" || req.Identity.TenantID != "tenant_abc" || req.Identity.PermissionVersion != "perm_v17" {
				t.Fatalf("unexpected identity: %#v", req.Identity)
			}
			if !req.Identity.MFA || req.Identity.SessionLevel != "strong" || req.Identity.Attributes["region"] != "ap" {
				t.Fatalf("unexpected identity claims: %#v", req.Identity)
			}
			return "Tom", nil
		},
		FieldPermission("self"),
	))

	req := httptest.NewRequest(http.MethodPost, "/data/query", strings.NewReader(`{"query":{"me":{"profile":["name"]}}}`))
	req.Header.Set("Authorization", "Bearer token_123")
	rr := httptest.NewRecorder()
	engine.HandleQuery(rr, req)

	var response QueryResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &response)
	if len(response.Errors) != 0 || nestedString(response.Data, "me", "profile", "name") != "Tom" {
		t.Fatalf("unexpected bearer response: %#v", response)
	}
}

func TestParseQueryPlanEnvelopeWithResourceDescriptors(t *testing.T) {
	body := []byte(`{"query":{"me":{"profile":["name","avatar"]},"project":{"list":{"$type":"collection","args":{"first":20,"after":"cursor_1","filter":{"status":"active"},"orderBy":[{"field":"updatedAt","direction":"desc"}]},"selection":{"fields":["id","name"],"expand":{"owner":["id","name"]}}}},"report":{"summary":{"$type":"aggregate","name":"countByStatus","args":{"scope":"tenant"}}}},"trace":true}`)

	plan, err := ParseQueryPlanEnvelope(body)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Trace {
		t.Fatal("expected trace flag")
	}
	if strings.Join(plan.Fields, ",") != "me.profile.avatar,me.profile.name" {
		t.Fatalf("unexpected fields: %#v", plan.Fields)
	}
	if len(plan.Resources) != 2 {
		t.Fatalf("expected two resources, got %#v", plan.Resources)
	}

	collection := plan.Resources[0]
	if collection.Path != "project.list" || collection.Type != ResourceQueryCollection {
		t.Fatalf("unexpected collection descriptor: %#v", collection)
	}
	if collection.Collection.First != 20 || collection.Collection.After == nil || *collection.Collection.After != "cursor_1" {
		t.Fatalf("unexpected collection args: %#v", collection.Collection)
	}
	if collection.Selection.Expand["owner"].Fields[1] != "name" {
		t.Fatalf("unexpected expand selection: %#v", collection.Selection)
	}

	aggregate := plan.Resources[1]
	if aggregate.Path != "report.summary" || aggregate.Type != ResourceQueryAggregate || aggregate.Name != "countByStatus" {
		t.Fatalf("unexpected aggregate descriptor: %#v", aggregate)
	}
}

func TestContractEndpointPublishesRegisteredSurface(t *testing.T) {
	engine := NewEngine(WithContractVersion("contract_public_test"))
	must(t, engine.RegisterField(FieldDefinition{
		Path:        "me.profile.name",
		Type:        "string",
		Owner:       "user-domain",
		Permission:  "self",
		Cache:       CachePolicy{Scope: CacheScopePrivate},
		Sensitivity: "public",
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			return "Tom", nil
		},
	}))
	must(t, engine.RegisterResource(ResourceDefinition{
		Path:        "project.list",
		Owner:       "project-domain",
		Permission:  "tenant",
		MaxPageSize: 50,
		Resolve: func(ctx context.Context, req ResourceRequest) (any, error) {
			return CollectionResult{}, nil
		},
	}))
	must(t, engine.RegisterCommand(CommandDefinition{
		Name:                "user.profile.update",
		Owner:               "user-domain",
		Permission:          "self",
		IdempotencyRequired: true,
		Execute: func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error) {
			return CommandExecutionResult{}, nil
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/contract", nil)
	rr := httptest.NewRecorder()
	engine.HandleContract(rr, req)

	var contract ContractSchema
	if err := json.Unmarshal(rr.Body.Bytes(), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ProtocolVersion != "v9" || contract.ContractVersion != "contract_public_test" {
		t.Fatalf("unexpected contract versions: %#v", contract)
	}
	if len(contract.Fields) != 1 || contract.Fields[0].Field != "me.profile.name" || contract.Fields[0].Type != "string" {
		t.Fatalf("unexpected contract fields: %#v", contract.Fields)
	}
	if len(contract.Resources) != 1 || contract.Resources[0].Path != "project.list" || contract.Resources[0].MaxPageSize != 50 {
		t.Fatalf("unexpected contract resources: %#v", contract.Resources)
	}
	if len(contract.Commands) != 1 || contract.Commands[0].Name != "user.profile.update" || !contract.Commands[0].IdempotencyRequired {
		t.Fatalf("unexpected contract commands: %#v", contract.Commands)
	}
}

func TestHelperRegistrationPublishesContractAndResolves(t *testing.T) {
	engine := NewEngine(WithContractVersion("contract_helper_test"))

	must(t, engine.RegisterStringField(
		"me.profile.name",
		func(ctx context.Context, req FieldRequest) (string, error) {
			return "Tom", nil
		},
		FieldOwner("user-domain"),
		FieldPermission("self"),
		FieldPrivateCache(time.Minute),
	))
	must(t, engine.RegisterStaticField(
		"global.config.appName",
		"FDDP Demo",
		FieldOwner("config-domain"),
		FieldPublicCache(time.Hour),
	))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			if req.Collection.First != 2 {
				t.Fatalf("expected max page size clamp, got %d", req.Collection.First)
			}
			return CollectionResult{
				Items: []any{
					map[string]any{"id": "p1", "name": "Alpha"},
				},
				PageInfo: &PageInfo{HasNextPage: false},
			}, nil
		},
		ResourceOwner("project-domain"),
		ResourcePermission("tenant"),
		ResourceMaxPageSize(2),
	))

	contract := engine.Contract()
	if len(contract.Fields) != 2 {
		t.Fatalf("expected helper fields in contract, got %#v", contract.Fields)
	}
	if contract.Fields[0].Field != "global.config.appName" || contract.Fields[0].Type != "string" || contract.Fields[0].CacheScope != CacheScopePublic {
		t.Fatalf("unexpected static field contract: %#v", contract.Fields[0])
	}
	if contract.Fields[1].Field != "me.profile.name" || contract.Fields[1].Permission != "self" || contract.Fields[1].CacheScope != CacheScopePrivate {
		t.Fatalf("unexpected string field contract: %#v", contract.Fields[1])
	}

	response := performQuery(engine, `{"query":{"me":{"profile":["name"]},"project":{"list":{"$type":"collection","args":{"first":10},"selection":{"fields":["id","name"]}}}}}`)
	if len(response.Errors) != 0 {
		t.Fatalf("expected helper query without errors, got %#v", response.Errors)
	}
	if got := nestedString(response.Data, "me", "profile", "name"); got != "Tom" {
		t.Fatalf("unexpected helper field data: %q", got)
	}
}

func TestFieldGroupResolvesMultipleLeafFieldsOnce(t *testing.T) {
	engine := NewEngine()
	calls := 0

	must(t, engine.RegisterFieldGroup(
		"me.profile",
		[]GroupField{
			{Name: "id", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "desc", Type: "string", Nullable: true},
		},
		func(ctx context.Context, req FieldGroupRequest) (map[string]any, error) {
			calls++
			if strings.Join(req.Fields, ",") != "me.profile.desc,me.profile.id,me.profile.name" {
				t.Fatalf("unexpected grouped fields: %#v", req.Fields)
			}
			return map[string]any{
				"id":   "user_123",
				"name": "Tom",
				"desc": "Demo user",
			}, nil
		},
		FieldGroupOwner("user-domain"),
		FieldGroupPermission("self"),
		FieldGroupPrivateCache(time.Minute),
	))

	response := performQuery(engine, `{"query":{"me":{"profile":["name","desc","id"]}}}`)
	if len(response.Errors) != 0 {
		t.Fatalf("expected grouped fields without errors, got %#v", response.Errors)
	}
	if calls != 1 {
		t.Fatalf("expected group resolver once, got %d", calls)
	}
	if nestedString(response.Data, "me", "profile", "id") != "user_123" {
		t.Fatalf("unexpected grouped profile data: %#v", response.Data)
	}

	contract := engine.Contract()
	var fields []string
	for _, field := range contract.Fields {
		fields = append(fields, field.Field)
	}
	if strings.Join(fields, ",") != "me.profile.desc,me.profile.id,me.profile.name" {
		t.Fatalf("unexpected grouped contract fields: %#v", fields)
	}
}

func TestQueryPlanResolvesCollectionResource(t *testing.T) {
	engine := NewEngine(WithTraceIDFunc(func() string { return "trace_resource" }))

	must(t, engine.RegisterResource(ResourceDefinition{
		Path:        "project.list",
		Owner:       "project-domain",
		Permission:  "tenant",
		MaxPageSize: 2,
		Resolve: func(ctx context.Context, req ResourceRequest) (any, error) {
			if req.Collection.First != 2 {
				t.Fatalf("expected max page size to clamp to 2, got %d", req.Collection.First)
			}
			if len(req.Selection.Fields) != 2 || req.Selection.Fields[0] != "id" || req.Selection.Fields[1] != "name" {
				t.Fatalf("unexpected selection: %#v", req.Selection)
			}
			return CollectionResult{
				Items: []any{
					map[string]any{"id": "p1", "name": "Alpha"},
					map[string]any{"id": "p2", "name": "Beta"},
				},
				PageInfo: &PageInfo{HasNextPage: true, EndCursor: "cursor_2"},
			}, nil
		},
	}))

	body := `{"query":{"project":{"list":{"$type":"collection","args":{"first":10,"orderBy":[{"field":"updatedAt","direction":"desc"}]},"selection":{"fields":["id","name"]}}}},"trace":true}`
	response := performQuery(engine, body)
	if len(response.Errors) != 0 {
		t.Fatalf("expected no errors, got %#v", response.Errors)
	}
	if response.Meta.Trace == nil || len(response.Meta.Trace.Resources) != 1 || response.Meta.Trace.Resources[0] != "project.list" {
		t.Fatalf("expected resource trace, got %#v", response.Meta.Trace)
	}

	result, ok := nestedAny(response.Data, "project", "list").(map[string]any)
	if !ok {
		t.Fatalf("expected collection result map, got %#v", nestedAny(response.Data, "project", "list"))
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected two collection items, got %#v", result["items"])
	}
}

func TestRequestBatcherDeduplicatesKeysPerGroup(t *testing.T) {
	batcher := NewRequestBatcher()
	calls := 0
	var loadedKeys []string

	load := func(ctx context.Context, keys []string) (map[string]any, error) {
		calls++
		loadedKeys = append(loadedKeys, keys...)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = "value_" + key
		}
		return out, nil
	}

	first, err := batcher.LoadMany(context.Background(), "users", []string{"u1", "u1", "u2"}, load)
	if err != nil {
		t.Fatal(err)
	}
	second, err := batcher.LoadMany(context.Background(), "users", []string{"u2", "u3"}, load)
	if err != nil {
		t.Fatal(err)
	}

	if calls != 2 {
		t.Fatalf("expected two load calls, got %d", calls)
	}
	if strings.Join(loadedKeys, ",") != "u1,u2,u3" {
		t.Fatalf("expected deduped keys, got %#v", loadedKeys)
	}
	if first["u1"] != "value_u1" || second["u2"] != "value_u2" || second["u3"] != "value_u3" {
		t.Fatalf("unexpected batch results: %#v %#v", first, second)
	}
}

func TestRegisteredBatchLoaderSharedAcrossResolvers(t *testing.T) {
	engine := NewEngine()
	calls := 0
	var loadedKeys []string
	engine.RegisterBatchLoader("users.byID", func(ctx context.Context, keys []string) (map[string]any, error) {
		calls++
		loadedKeys = append(loadedKeys, keys...)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = map[string]any{"id": key, "name": "User " + key}
		}
		return out, nil
	})

	must(t, engine.RegisterFieldResolver(
		"me.owner.name",
		func(ctx context.Context, req FieldRequest) (any, error) {
			owner, err := req.Load(ctx, "users.byID", "u1")
			if err != nil {
				return nil, err
			}
			return owner.(map[string]any)["name"], nil
		},
		FieldPermission("self"),
	))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			owners, err := req.LoadMany(ctx, "users.byID", []string{"u1", "u2"})
			if err != nil {
				return CollectionResult{}, err
			}
			return CollectionResult{Items: []any{
				map[string]any{"id": "p1", "owner": owners["u1"]},
				map[string]any{"id": "p2", "owner": owners["u2"]},
			}}, nil
		},
		ResourcePermission("tenant"),
	))

	response := performQuery(engine, `{"query":{"me":{"owner":["name"]},"project":{"list":{"$type":"collection","args":{"first":2},"selection":{"fields":["id"]}}}}}`)
	if len(response.Errors) != 0 {
		t.Fatalf("expected no batch errors, got %#v", response.Errors)
	}
	if calls != 2 {
		t.Fatalf("expected registered loader to be called twice, got %d", calls)
	}
	if strings.Join(loadedKeys, ",") != "u1,u2" {
		t.Fatalf("expected cached u1 and loaded u2 only, got %#v", loadedKeys)
	}
}

func TestQueryTraceIncludesResolverAndBatchCallCounts(t *testing.T) {
	engine := NewEngine()
	engine.RegisterBatchLoader("users.byID", func(ctx context.Context, keys []string) (map[string]any, error) {
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = map[string]any{"id": key, "name": "User " + key}
		}
		return out, nil
	})
	must(t, engine.RegisterStringField(
		"me.owner.name",
		func(ctx context.Context, req FieldRequest) (string, error) {
			owner, err := req.Load(ctx, "users.byID", "u1")
			if err != nil {
				return "", err
			}
			return owner.(map[string]any)["name"].(string), nil
		},
		FieldPermission("self"),
	))
	must(t, engine.RegisterFieldGroup(
		"me.profile",
		[]GroupField{{Name: "name", Type: "string"}},
		func(ctx context.Context, req FieldGroupRequest) (map[string]any, error) {
			return map[string]any{"name": "Tom"}, nil
		},
		FieldGroupPermission("self"),
	))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			owners, err := req.LoadMany(ctx, "users.byID", []string{"u1", "u2"})
			if err != nil {
				return CollectionResult{}, err
			}
			return CollectionResult{Items: []any{owners["u1"], owners["u2"]}}, nil
		},
		ResourcePermission("tenant"),
	))

	response := performQuery(engine, `{"query":{"me":{"owner":["name"],"profile":["name"]},"project":{"list":{"$type":"collection","args":{"first":2},"selection":{"fields":["id"]}}}},"trace":true}`)
	if len(response.Errors) != 0 {
		t.Fatalf("unexpected trace errors: %#v", response.Errors)
	}
	calls := response.Meta.Trace.Calls
	if calls.Fields != 1 || calls.FieldGroups != 1 || calls.Resources != 1 {
		t.Fatalf("unexpected resolver call counts: %#v", calls)
	}
	if calls.BatchLoads != 2 || calls.BatchKeys != 2 || calls.ByBatcher["users.byID"] != 2 {
		t.Fatalf("unexpected batch call counts: %#v", calls)
	}
}

func TestQueryLimitsRejectTooManyFields(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxFields: 1, Timeout: time.Second}))
	must(t, engine.RegisterStaticField("global.config.appName", "FDDP", FieldPermission("public")))
	must(t, engine.RegisterStaticField("global.config.region", "local", FieldPermission("public")))

	response := performQuery(engine, `{"query":{"global":{"config":["appName","region"]}},"trace":true}`)
	assertQueryLimitExceeded(t, response, "field count")
	if response.Meta.Cost == nil || response.Meta.Cost.Fields != 2 {
		t.Fatalf("expected cost fields to be recorded, got %#v", response.Meta.Cost)
	}
	if response.Meta.Trace == nil || response.Meta.Trace.Cost == 0 {
		t.Fatalf("expected trace cost, got %#v", response.Meta.Trace)
	}
}

func TestQueryLimitsRejectTooLargeCollectionFirst(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxCollectionFirst: 2, Timeout: time.Second}))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			t.Fatal("resolver should not run for over-limit query")
			return CollectionResult{}, nil
		},
		ResourcePermission("tenant"),
		ResourceMaxPageSize(50),
	))

	response := performQuery(engine, `{"query":{"project":{"list":{"$type":"collection","args":{"first":10},"selection":{"fields":["id"]}}}}}`)
	assertQueryLimitExceeded(t, response, "collection first")
}

func TestQueryLimitsRejectNestedExpand(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxExpandDepth: 1, Timeout: time.Second}))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			t.Fatal("resolver should not run for over-limit query")
			return CollectionResult{}, nil
		},
		ResourcePermission("tenant"),
	))

	response := performQuery(engine, `{"query":{"project":{"list":{"$type":"collection","selection":{"fields":["id"],"expand":{"owner":{"fields":["id"],"expand":{"profile":["id"]}}}}}}}}`)
	assertQueryLimitExceeded(t, response, "expand depth")
}

func TestQueryLimitsRejectHighCostContains(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxCost: 20, ContainsCost: 100, Timeout: time.Second}))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			t.Fatal("resolver should not run for over-limit query")
			return CollectionResult{}, nil
		},
		ResourcePermission("tenant"),
	))

	response := performQuery(engine, `{"query":{"project":{"list":{"$type":"collection","args":{"first":1,"filter":{"name":{"Contains":"alpha"}}},"selection":{"fields":["id"]}}}}}`)
	assertQueryLimitExceeded(t, response, "query cost")
	if response.Meta.Cost == nil || response.Meta.Cost.TotalCost <= 20 {
		t.Fatalf("expected high query cost, got %#v", response.Meta.Cost)
	}
}

func TestQueryTimeoutCancelsResolver(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{Timeout: 10 * time.Millisecond}))
	must(t, engine.RegisterStringField(
		"me.profile.name",
		func(ctx context.Context, req FieldRequest) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		FieldPermission("self"),
	))

	response := performQuery(engine, `{"query":{"me":{"profile":["name"]}}}`)
	if len(response.Errors) != 1 || response.Errors[0].Code != "QUERY_TIMEOUT" {
		t.Fatalf("expected query timeout, got %#v", response.Errors)
	}
}

func TestQueryLimitsCanBeDisabled(t *testing.T) {
	engine := NewEngine(WithoutQueryLimits())
	calls := 0
	must(t, engine.RegisterStaticField("global.config.appName", "FDDP", FieldPermission("public")))
	must(t, engine.RegisterStaticField("global.config.region", "local", FieldPermission("public")))
	must(t, engine.RegisterCollection(
		"project.list",
		func(ctx context.Context, req ResourceRequest) (CollectionResult, error) {
			calls++
			return CollectionResult{Items: []any{}}, nil
		},
		ResourcePermission("tenant"),
	))

	response := performQuery(engine, `{"query":{"global":{"config":["appName","region"]},"project":{"list":{"$type":"collection","args":{"first":1000},"selection":{"fields":["id"],"expand":{"owner":{"fields":["id"],"expand":{"profile":["id"]}}}}}}}}`)
	if len(response.Errors) != 0 {
		t.Fatalf("expected disabled limits to allow query, got %#v", response.Errors)
	}
	if calls != 1 {
		t.Fatalf("expected collection resolver to run once, got %d", calls)
	}
}

func TestQueryLimitsRejectTooLargeBodyBeforeParse(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxBodyBytes: 10, Timeout: time.Second}))

	result := engine.ExecuteQueryBody(context.Background(), QueryEndpointRequest{
		Body: []byte(`{"query":{"global":{"config":["appName"]}}}`),
	})
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	assertQueryLimitExceeded(t, result.Response, "query body size")
}

func TestQueryLimitsRejectTooDeepQueryBeforeFlatten(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxQueryDepth: 2, Timeout: time.Second}))

	result := engine.ExecuteQueryBody(context.Background(), QueryEndpointRequest{
		Body: []byte(`{"query":{"a":{"b":{"c":["d"]}}}}`),
	})
	assertQueryLimitExceeded(t, result.Response, "query depth")
}

func TestQueryLimitsRejectTooManyQueryNodesBeforeFlatten(t *testing.T) {
	engine := NewEngine(WithQueryLimits(QueryLimits{MaxQueryNodes: 3, Timeout: time.Second}))

	result := engine.ExecuteQueryBody(context.Background(), QueryEndpointRequest{
		Body: []byte(`{"query":{"a":["one"],"b":["two"],"c":["three"]}}`),
	})
	assertQueryLimitExceeded(t, result.Response, "query node count")
}

func TestFrameworkNeutralQueryAndCommandExecution(t *testing.T) {
	engine := NewEngine(WithContractVersion("contract_framework_test"))
	must(t, engine.RegisterStringField(
		"me.profile.name",
		func(ctx context.Context, req FieldRequest) (string, error) {
			return "Tom", nil
		},
		FieldPermission("self"),
	))
	must(t, engine.RegisterCommand(CommandDefinition{
		Name:       "profile.touch",
		Permission: "self",
		Execute: func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error) {
			return CommandExecutionResult{Result: map[string]any{"ok": true}}, nil
		},
	}))

	identity := IdentityContext{Subject: "user_123", TenantID: "tenant_abc"}
	query := engine.ExecuteQueryBody(context.Background(), QueryEndpointRequest{
		Body:     []byte(`{"query":{"me":{"profile":["name"]}}}`),
		Identity: identity,
	})
	if query.Status != http.StatusOK || len(query.Response.Errors) != 0 {
		t.Fatalf("unexpected framework query result: %#v", query)
	}
	if got := nestedString(query.Response.Data, "me", "profile", "name"); got != "Tom" {
		t.Fatalf("unexpected framework query data: %q", got)
	}

	command := engine.ExecuteCommandBody(context.Background(), CommandEndpointRequest{
		Body:     []byte(`{"command":"profile.touch","input":{}}`),
		Identity: identity,
	})
	if command.Status != http.StatusOK || command.Response.Data.Status != "completed" || len(command.Response.Errors) != 0 {
		t.Fatalf("unexpected framework command result: %#v", command)
	}
}

func TestQueryHTTPResponseUsesEmptyArrays(t *testing.T) {
	engine := NewEngine()
	must(t, engine.RegisterStaticField("global.config.appName", "FDDP", FieldPermission("public")))

	req := httptest.NewRequest(http.MethodPost, "/data/query", strings.NewReader(`{"query":{"global":{"config":["appName"]}}}`))
	rr := httptest.NewRecorder()
	engine.HandleQuery(rr, req)

	var raw map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["errors"].([]any); !ok {
		t.Fatalf("expected errors to be a JSON array, got %s", rr.Body.String())
	}
	meta, ok := raw["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %s", rr.Body.String())
	}
	cache, ok := meta["cache"].(map[string]any)
	if !ok {
		t.Fatalf("expected cache object, got %s", rr.Body.String())
	}
	for _, key := range []string{"hit", "miss", "stale", "unsafe"} {
		if _, ok := cache[key].([]any); !ok {
			t.Fatalf("expected cache.%s to be a JSON array, got %s", key, rr.Body.String())
		}
	}
}

func TestCommandIdempotency(t *testing.T) {
	engine := NewEngine(
		WithIdempotencyStore(NewMemoryIdempotencyStore()),
		WithTraceIDFunc(func() string { return "trace_command" }),
	)
	calls := 0

	must(t, engine.RegisterCommand(CommandDefinition{
		Name:                "user.profile.update",
		Permission:          "self",
		IdempotencyRequired: true,
		Execute: func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error) {
			calls++
			return CommandExecutionResult{
				Result:      map[string]any{"ok": true},
				Invalidates: []string{"me.profile.*"},
			}, nil
		},
	}))

	body := `{"command":"user.profile.update","input":{"displayName":"Tom"},"idempotencyKey":"cmd_1","trace":true}`
	first := performCommand(engine, body)
	if first.Data.Status != "completed" || calls != 1 {
		t.Fatalf("unexpected first response: %#v calls=%d", first, calls)
	}
	second := performCommand(engine, body)
	if second.Data.Status != "completed" || calls != 1 {
		t.Fatalf("expected cached idempotent response: %#v calls=%d", second, calls)
	}
	if second.Meta.Trace == nil || !second.Meta.Trace.IdempotencyHit {
		t.Fatalf("expected idempotency trace, got %#v", second.Meta.Trace)
	}
}

func TestCommandLimitsRejectTooLargeBody(t *testing.T) {
	engine := NewEngine(WithCommandLimits(CommandLimits{MaxBodyBytes: 10, Timeout: time.Second}))

	result := engine.ExecuteCommandBody(context.Background(), CommandEndpointRequest{
		Body: []byte(`{"command":"profile.touch","input":{"name":"Tom"}}`),
	})
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	assertCommandLimitExceeded(t, result.Response, "command body size")
}

func TestCommandLimitsRejectTooLargeInput(t *testing.T) {
	engine := NewEngine(WithCommandLimits(CommandLimits{MaxInputBytes: 5, Timeout: time.Second}))

	result := engine.ExecuteCommandBody(context.Background(), CommandEndpointRequest{
		Body: []byte(`{"command":"profile.touch","input":{"name":"Tom"}}`),
	})
	assertCommandLimitExceeded(t, result.Response, "command input size")
}

func TestCommandLimitsRejectTooDeepInput(t *testing.T) {
	engine := NewEngine(WithCommandLimits(CommandLimits{MaxInputDepth: 2, Timeout: time.Second}))

	result := engine.ExecuteCommandBody(context.Background(), CommandEndpointRequest{
		Body: []byte(`{"command":"profile.touch","input":{"a":{"b":{"c":"d"}}}}`),
	})
	assertCommandLimitExceeded(t, result.Response, "command input depth")
}

func TestCommandLimitsRejectTooManyInputNodes(t *testing.T) {
	engine := NewEngine(WithCommandLimits(CommandLimits{MaxInputNodes: 3, Timeout: time.Second}))

	result := engine.ExecuteCommandBody(context.Background(), CommandEndpointRequest{
		Body: []byte(`{"command":"profile.touch","input":{"a":1,"b":2,"c":3}}`),
	})
	assertCommandLimitExceeded(t, result.Response, "command input node count")
}

func TestCommandTimeoutCancelsExecutor(t *testing.T) {
	engine := NewEngine(WithCommandLimits(CommandLimits{Timeout: 10 * time.Millisecond}))
	must(t, engine.RegisterCommand(CommandDefinition{
		Name:       "profile.touch",
		Permission: "self",
		Execute: func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error) {
			<-ctx.Done()
			return CommandExecutionResult{}, ctx.Err()
		},
	}))

	response := performCommand(engine, `{"command":"profile.touch","input":{}}`)
	if len(response.Errors) != 1 || response.Errors[0].Code != "COMMAND_TIMEOUT" {
		t.Fatalf("expected command timeout, got %#v", response.Errors)
	}
}

func TestCommandLimitsCanBeDisabled(t *testing.T) {
	engine := NewEngine(WithoutCommandLimits())
	must(t, engine.RegisterCommand(CommandDefinition{
		Name:       "profile.touch",
		Permission: "self",
		Execute: func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error) {
			return CommandExecutionResult{Result: map[string]any{"ok": true}}, nil
		},
	}))

	response := performCommand(engine, `{"command":"profile.touch","input":{"a":{"b":{"c":{"d":"e"}}}}}`)
	if len(response.Errors) != 0 || response.Data.Status != "completed" {
		t.Fatalf("expected disabled command limits to allow command, got %#v", response)
	}
}

func performQuery(engine *Engine, body string) QueryResponse {
	req := httptest.NewRequest(http.MethodPost, "/data/query", strings.NewReader(body))
	req.Header.Set("X-DDP-Subject", "user_123")
	req.Header.Set("X-DDP-Tenant", "tenant_abc")
	req.Header.Set("X-DDP-Permission-Version", "perm_v17")
	rr := httptest.NewRecorder()
	engine.HandleQuery(rr, req)
	var response QueryResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &response)
	return response
}

func performCommand(engine *Engine, body string) CommandResponse {
	req := httptest.NewRequest(http.MethodPost, "/command/execute", strings.NewReader(body))
	req.Header.Set("X-DDP-Subject", "user_123")
	req.Header.Set("X-DDP-Tenant", "tenant_abc")
	rr := httptest.NewRecorder()
	engine.HandleCommand(rr, req)
	var response CommandResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &response)
	return response
}

func nestedString(data map[string]any, parts ...string) string {
	value, _ := nestedAny(data, parts...).(string)
	return value
}

func nestedAny(data map[string]any, parts ...string) any {
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertQueryLimitExceeded(t *testing.T, response QueryResponse, reasonContains string) {
	t.Helper()
	if len(response.Errors) != 1 || response.Errors[0].Code != "QUERY_LIMIT_EXCEEDED" {
		t.Fatalf("expected query limit error, got %#v", response.Errors)
	}
	if !strings.Contains(response.Errors[0].Reason, reasonContains) {
		t.Fatalf("expected reason to contain %q, got %#v", reasonContains, response.Errors[0])
	}
	if !response.Meta.Partial {
		t.Fatalf("expected partial response")
	}
}

func assertCommandLimitExceeded(t *testing.T, response CommandResponse, reasonContains string) {
	t.Helper()
	if len(response.Errors) != 1 || response.Errors[0].Code != "COMMAND_LIMIT_EXCEEDED" {
		t.Fatalf("expected command limit error, got %#v", response.Errors)
	}
	if !strings.Contains(response.Errors[0].Reason, reasonContains) {
		t.Fatalf("expected reason to contain %q, got %#v", reasonContains, response.Errors[0])
	}
	if response.Data.Status != "failed" || !response.Meta.Partial {
		t.Fatalf("expected failed partial response, got %#v", response)
	}
}
