package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"github.com/gin-gonic/gin"
)

const maxFDDPBodyBytes = 1 << 20

func main() {
	identityResolver := fddp.BearerTokenIdentityResolver(demoTokenVerifier())
	engine := fddp.NewEngine(
		fddp.WithIdentityResolver(identityResolver),
		fddp.WithCache(fddp.NewMemoryCache()),
		fddp.WithIdempotencyStore(fddp.NewMemoryIdempotencyStore()),
		fddp.WithContractVersion("contract_gin_demo_v1"),
		fddp.WithQueryLimits(fddp.QueryLimits{
			MaxFields:          20,
			MaxResources:       3,
			MaxCollectionFirst: 50,
			MaxSelectionFields: 20,
			MaxExpandDepth:     1,
			MaxFilterFields:    5,
			MaxOrderBy:         2,
			MaxCost:            120,
			MaxBodyBytes:       maxFDDPBodyBytes,
			MaxQueryDepth:      12,
			MaxQueryNodes:      80,
			Timeout:            2 * time.Second,
		}),
		fddp.WithCommandLimits(fddp.CommandLimits{
			MaxBodyBytes:  maxFDDPBodyBytes,
			MaxInputBytes: 64 << 10,
			MaxInputDepth: 8,
			MaxInputNodes: 80,
			Timeout:       2 * time.Second,
		}),
	)
	must(registerDemoContract(engine))

	router := gin.Default()
	mountFDDP(router, engine, identityResolver)

	port := env("PORT", "8080")
	log.Printf("Gin FDDP example listening on http://localhost:%s/api/fddp", port)
	log.Printf("Use Authorization: Bearer demo-token")
	log.Fatal(router.Run(":" + port))
}

func mountFDDP(router *gin.Engine, engine *fddp.Engine, identityResolver fddp.IdentityResolver) {
	group := router.Group("/api/fddp")

	group.GET("/contract", func(c *gin.Context) {
		c.Header("cache-control", "no-store")
		c.JSON(http.StatusOK, engine.Contract())
	})

	group.POST("/data/query", func(c *gin.Context) {
		body, ok := readFDDPBody(c)
		if !ok {
			return
		}

		result := engine.ExecuteQueryBody(c.Request.Context(), fddp.QueryEndpointRequest{
			Body:        body,
			Identity:    identityResolver(c.Request),
			HTTPRequest: c.Request,
		})
		c.JSON(result.Status, result.Response)
	})

	group.POST("/command/execute", func(c *gin.Context) {
		body, ok := readFDDPBody(c)
		if !ok {
			return
		}

		result := engine.ExecuteCommandBody(c.Request.Context(), fddp.CommandEndpointRequest{
			Body:        body,
			Identity:    identityResolver(c.Request),
			HTTPRequest: c.Request,
		})
		c.JSON(result.Status, result.Response)
	})
}

func readFDDPBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxFDDPBodyBytes)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"errors": []fddp.DDPError{{
				Code:        "BAD_REQUEST",
				SafeMessage: "request body is invalid or too large",
			}},
		})
		return nil, false
	}
	return body, true
}

func demoTokenVerifier() fddp.TokenVerifier {
	return fddp.BearerTokenVerifierFunc(func(_ context.Context, token string) (fddp.TokenClaims, error) {
		if token != "demo-token" {
			return fddp.TokenClaims{}, errors.New("invalid demo token")
		}
		return fddp.TokenClaims{
			Subject:           "user_123",
			TenantID:          "tenant_abc",
			Roles:             []string{"tenant_admin"},
			PermissionVersion: "perm_demo_v1",
			PolicyVersion:     "policy_demo_v1",
		}, nil
	})
}

func registerDemoContract(engine *fddp.Engine) error {
	if err := engine.RegisterFieldGroup(
		"me.profile",
		[]fddp.GroupField{
			{Name: "id", Type: "string"},
			{Name: "name", Type: "string"},
			{Name: "desc", Type: "string", Nullable: true},
		},
		func(ctx context.Context, req fddp.FieldGroupRequest) (map[string]any, error) {
			return map[string]any{
				"id":   req.Identity.Subject,
				"name": "Tom",
				"desc": "Gin integration demo user",
			}, nil
		},
		fddp.FieldGroupOwner("user-domain"),
		fddp.FieldGroupPermission("self"),
		fddp.FieldGroupPrivateCache(time.Minute),
	); err != nil {
		return err
	}

	if err := engine.RegisterStaticField(
		"global.config.appName",
		"FDDP Gin Demo",
		fddp.FieldOwner("config-domain"),
		fddp.FieldPermission("public"),
		fddp.FieldPublicCache(0),
	); err != nil {
		return err
	}

	return engine.RegisterCommand(fddp.CommandDefinition{
		Name:                "user.profile.update",
		Owner:               "user-domain",
		Permission:          "self",
		IdempotencyRequired: true,
		Input: []fddp.ContractInputField{
			{Field: "displayName", Type: "string"},
		},
		Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
			var input struct {
				DisplayName string `json:"displayName"`
			}
			if len(req.Input) > 0 {
				if err := json.Unmarshal(req.Input, &input); err != nil {
					return fddp.CommandExecutionResult{}, err
				}
			}
			if input.DisplayName == "" {
				input.DisplayName = "Tom"
			}
			return fddp.CommandExecutionResult{
				Status:      "ok",
				Result:      map[string]any{"displayName": input.DisplayName},
				Invalidates: []string{"me.profile.*"},
			}, nil
		},
	})
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
