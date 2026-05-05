package fddp

import (
	"context"
	"time"
)

type FieldOption func(*FieldDefinition)

func FieldType(fieldType string) FieldOption {
	return func(field *FieldDefinition) {
		field.Type = fieldType
	}
}

func FieldOwner(owner string) FieldOption {
	return func(field *FieldDefinition) {
		field.Owner = owner
	}
}

func FieldPermission(permission string) FieldOption {
	return func(field *FieldDefinition) {
		field.Permission = permission
	}
}

func FieldCache(cache CachePolicy) FieldOption {
	return func(field *FieldDefinition) {
		field.Cache = cache
	}
}

func FieldPublicCache(ttl time.Duration) FieldOption {
	return FieldCache(CachePolicy{Scope: CacheScopePublic, TTL: ttl})
}

func FieldPrivateCache(ttl time.Duration) FieldOption {
	return FieldCache(CachePolicy{Scope: CacheScopePrivate, TTL: ttl})
}

func FieldTenantCache(ttl time.Duration) FieldOption {
	return FieldCache(CachePolicy{Scope: CacheScopeTenant, TTL: ttl})
}

func FieldNullable() FieldOption {
	return func(field *FieldDefinition) {
		field.Nullable = true
	}
}

func FieldDeprecated() FieldOption {
	return func(field *FieldDefinition) {
		field.Deprecated = true
	}
}

func FieldSensitivity(sensitivity string) FieldOption {
	return func(field *FieldDefinition) {
		field.Sensitivity = sensitivity
	}
}

func (e *Engine) RegisterFieldResolver(path string, resolve FieldResolver, options ...FieldOption) error {
	field := FieldDefinition{
		Path:       path,
		Type:       "unknown",
		Permission: "login",
		Resolve:    resolve,
	}
	applyFieldOptions(&field, options...)
	return e.RegisterField(field)
}

func (e *Engine) RegisterStringField(path string, resolve func(context.Context, FieldRequest) (string, error), options ...FieldOption) error {
	field := FieldDefinition{
		Path:       path,
		Type:       "string",
		Permission: "login",
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			return resolve(ctx, req)
		},
	}
	applyFieldOptions(&field, options...)
	return e.RegisterField(field)
}

func (e *Engine) RegisterNumberField(path string, resolve func(context.Context, FieldRequest) (float64, error), options ...FieldOption) error {
	field := FieldDefinition{
		Path:       path,
		Type:       "number",
		Permission: "login",
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			return resolve(ctx, req)
		},
	}
	applyFieldOptions(&field, options...)
	return e.RegisterField(field)
}

func (e *Engine) RegisterBoolField(path string, resolve func(context.Context, FieldRequest) (bool, error), options ...FieldOption) error {
	field := FieldDefinition{
		Path:       path,
		Type:       "boolean",
		Permission: "login",
		Resolve: func(ctx context.Context, req FieldRequest) (any, error) {
			return resolve(ctx, req)
		},
	}
	applyFieldOptions(&field, options...)
	return e.RegisterField(field)
}

func (e *Engine) RegisterStaticField(path string, value any, options ...FieldOption) error {
	field := FieldDefinition{
		Path:       path,
		Type:       inferContractType(value),
		Permission: "public",
		Resolve: func(context.Context, FieldRequest) (any, error) {
			return value, nil
		},
	}
	applyFieldOptions(&field, options...)
	return e.RegisterField(field)
}

type GroupField struct {
	Name        string
	Type        string
	Nullable    bool
	Deprecated  bool
	Sensitivity string
}

type FieldGroupOption func(*FieldGroupDefinition)

func FieldGroupOwner(owner string) FieldGroupOption {
	return func(group *FieldGroupDefinition) {
		group.Owner = owner
	}
}

func FieldGroupPermission(permission string) FieldGroupOption {
	return func(group *FieldGroupDefinition) {
		group.Permission = permission
	}
}

func FieldGroupCache(cache CachePolicy) FieldGroupOption {
	return func(group *FieldGroupDefinition) {
		group.Cache = cache
	}
}

func FieldGroupPrivateCache(ttl time.Duration) FieldGroupOption {
	return FieldGroupCache(CachePolicy{Scope: CacheScopePrivate, TTL: ttl})
}

func FieldGroupPublicCache(ttl time.Duration) FieldGroupOption {
	return FieldGroupCache(CachePolicy{Scope: CacheScopePublic, TTL: ttl})
}

func FieldGroupTenantCache(ttl time.Duration) FieldGroupOption {
	return FieldGroupCache(CachePolicy{Scope: CacheScopeTenant, TTL: ttl})
}

func (e *Engine) RegisterFieldGroup(path string, fields []GroupField, resolve FieldGroupResolver, options ...FieldGroupOption) error {
	group := FieldGroupDefinition{
		Path:       path,
		Permission: "login",
		Fields:     make([]FieldDefinition, 0, len(fields)),
		Resolve:    resolve,
	}
	for _, field := range fields {
		fieldType := field.Type
		if fieldType == "" {
			fieldType = "unknown"
		}
		group.Fields = append(group.Fields, FieldDefinition{
			Path:        path + "." + field.Name,
			Type:        fieldType,
			Nullable:    field.Nullable,
			Deprecated:  field.Deprecated,
			Sensitivity: field.Sensitivity,
		})
	}
	applyFieldGroupOptions(&group, options...)
	return e.registry.RegisterFieldGroup(group)
}

type ResourceOption func(*ResourceDefinition)

func ResourceOwner(owner string) ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.Owner = owner
	}
}

func ResourcePermission(permission string) ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.Permission = permission
	}
}

func ResourceMaxPageSize(size int) ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.MaxPageSize = size
	}
}

func ResourceFields(fields ...ContractResourceField) ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.Fields = append(resource.Fields, fields...)
	}
}

func ResourceRelations(relations ...ContractResourceRelation) ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.Relations = append(resource.Relations, relations...)
	}
}

func ResourceDeprecated() ResourceOption {
	return func(resource *ResourceDefinition) {
		resource.Deprecated = true
	}
}

type CollectionResolver func(context.Context, ResourceRequest) (CollectionResult, error)

func (e *Engine) RegisterCollection(path string, resolve CollectionResolver, options ...ResourceOption) error {
	resource := ResourceDefinition{
		Path:       path,
		Permission: "login",
		Resolve: func(ctx context.Context, req ResourceRequest) (any, error) {
			return resolve(ctx, req)
		},
	}
	applyResourceOptions(&resource, options...)
	return e.RegisterResource(resource)
}

func applyFieldOptions(field *FieldDefinition, options ...FieldOption) {
	for _, option := range options {
		if option != nil {
			option(field)
		}
	}
}

func applyFieldGroupOptions(group *FieldGroupDefinition, options ...FieldGroupOption) {
	for _, option := range options {
		if option != nil {
			option(group)
		}
	}
}

func applyResourceOptions(resource *ResourceDefinition, options ...ResourceOption) {
	for _, option := range options {
		if option != nil {
			option(resource)
		}
	}
}

func inferContractType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return "number"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}
