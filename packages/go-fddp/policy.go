package fddp

import (
	"context"
	"strings"
)

type Decision struct {
	Allow  bool
	Reason string
}

type PolicyChecker interface {
	CanRead(ctx context.Context, identity IdentityContext, field FieldDefinition) (Decision, error)
	CanExecute(ctx context.Context, identity IdentityContext, command CommandDefinition) (Decision, error)
}

type ResourcePolicyChecker interface {
	CanReadResource(ctx context.Context, identity IdentityContext, resource ResourceDefinition) (Decision, error)
}

type DefaultPolicy struct{}

func (DefaultPolicy) CanRead(_ context.Context, identity IdentityContext, field FieldDefinition) (Decision, error) {
	return decide(identity, field.Permission, field.Path), nil
}

func (DefaultPolicy) CanExecute(_ context.Context, identity IdentityContext, command CommandDefinition) (Decision, error) {
	return decideCommand(identity, command.Permission), nil
}

func (DefaultPolicy) CanReadResource(_ context.Context, identity IdentityContext, resource ResourceDefinition) (Decision, error) {
	return decide(identity, resource.Permission, resource.Path), nil
}

func decide(identity IdentityContext, permission string, resource string) Decision {
	permission = strings.TrimSpace(permission)
	if permission == "" || permission == "none" {
		return Decision{Allow: false, Reason: "missing_permission"}
	}

	if hasString(identity.Scopes, permission) {
		return Decision{Allow: true, Reason: "scope"}
	}

	switch permission {
	case "public", "anonymous":
		return Decision{Allow: true, Reason: "public"}
	case "login", "logged_in", "authenticated":
		return Decision{Allow: identity.Subject != "", Reason: "login_required"}
	case "self":
		return Decision{Allow: identity.Subject != "" && strings.HasPrefix(resource, "me."), Reason: "self_required"}
	case "self_or_admin":
		return Decision{Allow: identity.Subject != "" && (strings.HasPrefix(resource, "me.") || hasRole(identity, "admin")), Reason: "self_or_admin_required"}
	case "tenant", "tenant_member", "tenant.member":
		return Decision{Allow: identity.Subject != "" && identity.TenantID != "", Reason: "tenant_required"}
	case "tenant_admin", "tenant.admin", "tenant.admin_or_owner":
		return Decision{Allow: identity.Subject != "" && identity.TenantID != "" && (hasRole(identity, "tenant_admin") || hasRole(identity, "owner") || hasRole(identity, "admin")), Reason: "tenant_admin_required"}
	case "admin":
		return Decision{Allow: hasRole(identity, "admin"), Reason: "admin_required"}
	default:
		return Decision{Allow: false, Reason: "unsupported_permission"}
	}
}

func decideCommand(identity IdentityContext, permission string) Decision {
	permission = strings.TrimSpace(permission)
	if permission == "" || permission == "none" {
		return Decision{Allow: false, Reason: "missing_permission"}
	}

	if hasString(identity.Scopes, permission) {
		return Decision{Allow: true, Reason: "scope"}
	}

	switch permission {
	case "public", "anonymous":
		return Decision{Allow: true, Reason: "public"}
	case "login", "logged_in", "authenticated", "self":
		return Decision{Allow: identity.Subject != "", Reason: "login_required"}
	case "self_or_admin":
		return Decision{Allow: identity.Subject != "" || hasRole(identity, "admin"), Reason: "self_or_admin_required"}
	case "tenant", "tenant_member", "tenant.member":
		return Decision{Allow: identity.Subject != "" && identity.TenantID != "", Reason: "tenant_required"}
	case "tenant_admin", "tenant.admin", "tenant.admin_or_owner":
		return Decision{Allow: identity.Subject != "" && identity.TenantID != "" && (hasRole(identity, "tenant_admin") || hasRole(identity, "owner") || hasRole(identity, "admin")), Reason: "tenant_admin_required"}
	case "admin":
		return Decision{Allow: hasRole(identity, "admin"), Reason: "admin_required"}
	default:
		return Decision{Allow: false, Reason: "unsupported_permission"}
	}
}

func hasRole(identity IdentityContext, role string) bool {
	return hasString(identity.Roles, role)
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}
