package fddp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/contract", e.HandleContract)
	mux.HandleFunc("/data/query", e.HandleQuery)
	mux.HandleFunc("/command/execute", e.HandleCommand)
	return mux
}

func (e *Engine) HandleContract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body := e.Contract()
	w.Header().Set("cache-control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (e *Engine) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []DDPError{{Code: "BAD_REQUEST", Reason: err.Error()}}})
		return
	}

	identity := e.identityResolver(r)
	result := e.ExecuteQueryBody(r.Context(), QueryEndpointRequest{Body: body, Identity: identity, HTTPRequest: r})
	writeJSON(w, result.Status, result.Response)
}

func (e *Engine) HandleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []DDPError{{Code: "BAD_REQUEST", Reason: err.Error()}}})
		return
	}

	identity := e.identityResolver(r)
	result := e.ExecuteCommandBody(r.Context(), CommandEndpointRequest{Body: body, Identity: identity, HTTPRequest: r})
	writeJSON(w, result.Status, result.Response)
}

func HeaderIdentityResolver(r *http.Request) IdentityContext {
	return IdentityContext{
		Subject:           firstHeader(r, "X-DDP-Subject", "X-Subject-Id"),
		TenantID:          firstHeader(r, "X-DDP-Tenant", "X-Tenant-Id"),
		Roles:             splitCSV(firstHeader(r, "X-DDP-Roles", "X-Roles")),
		Scopes:            splitCSV(firstHeader(r, "X-DDP-Scopes", "X-Scopes")),
		PermissionVersion: firstHeader(r, "X-DDP-Permission-Version", "X-Permission-Version"),
		MFA:               strings.EqualFold(firstHeader(r, "X-DDP-MFA", "X-MFA"), "true"),
		SessionLevel:      firstHeader(r, "X-DDP-Session-Level", "X-Session-Level"),
	}
}

func BearerTokenIdentityResolver(verifier TokenVerifier) IdentityResolver {
	return func(r *http.Request) IdentityContext {
		if verifier == nil {
			return IdentityContext{}
		}
		token := bearerToken(r)
		if token == "" {
			return IdentityContext{}
		}
		claims, err := verifier.VerifyToken(r.Context(), token)
		if err != nil {
			return IdentityContext{}
		}
		return IdentityContext{
			Subject:           claims.Subject,
			TenantID:          claims.TenantID,
			Roles:             append([]string(nil), claims.Roles...),
			Scopes:            append([]string(nil), claims.Scopes...),
			PermissionVersion: claims.PermissionVersion,
			PolicyVersion:     claims.PolicyVersion,
			MFA:               claims.MFA,
			SessionLevel:      claims.SessionLevel,
			Attributes:        cloneStringMap(claims.Attributes),
		}
	}
}

func BearerTokenVerifierFunc(fn func(ctx context.Context, token string) (TokenClaims, error)) TokenVerifier {
	if fn == nil {
		return TokenVerifierFunc(func(context.Context, string) (TokenClaims, error) {
			return TokenClaims{}, errors.New("missing token verifier")
		})
	}
	return TokenVerifierFunc(fn)
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if value == "" {
		return ""
	}
	prefix := "Bearer "
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}

func firstHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
