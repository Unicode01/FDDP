package gormadapter

import (
	"strings"
	"time"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"gorm.io/gorm"
)

type MappingOption func(*FieldMapping)

func Column(column string) MappingOption {
	return func(mapping *FieldMapping) {
		mapping.Column = column
	}
}

func Nullable() MappingOption {
	return func(mapping *FieldMapping) {
		mapping.Nullable = true
	}
}

type FieldGroupBuilder[T any] struct {
	cfg FieldGroup[T]
}

func NewFieldGroup[T any](path string, db *gorm.DB) *FieldGroupBuilder[T] {
	return &FieldGroupBuilder[T]{
		cfg: FieldGroup[T]{
			Path:   path,
			DB:     db,
			Fields: make(map[string]FieldMapping),
		},
	}
}

func (b *FieldGroupBuilder[T]) Field(name string, fieldType string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	b.cfg.Fields[name] = buildMapping(name, fieldType, structField, options...)
	return b
}

func (b *FieldGroupBuilder[T]) String(name string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	return b.Field(name, "string", structField, options...)
}

func (b *FieldGroupBuilder[T]) NullableString(name string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	return b.Field(name, "string", structField, append(options, Nullable())...)
}

func (b *FieldGroupBuilder[T]) Number(name string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	return b.Field(name, "number", structField, options...)
}

func (b *FieldGroupBuilder[T]) Bool(name string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	return b.Field(name, "boolean", structField, options...)
}

func (b *FieldGroupBuilder[T]) Object(name string, structField string, options ...MappingOption) *FieldGroupBuilder[T] {
	return b.Field(name, "object", structField, options...)
}

func (b *FieldGroupBuilder[T]) Scope(scope func(*gorm.DB, fddp.FieldGroupRequest) *gorm.DB) *FieldGroupBuilder[T] {
	b.cfg.Scope = scope
	return b
}

func (b *FieldGroupBuilder[T]) AfterQuery(after func(*T, fddp.FieldGroupRequest) (map[string]any, error)) *FieldGroupBuilder[T] {
	b.cfg.AfterQuery = after
	return b
}

func (b *FieldGroupBuilder[T]) Options(options ...fddp.FieldGroupOption) *FieldGroupBuilder[T] {
	b.cfg.Options = append(b.cfg.Options, options...)
	return b
}

func (b *FieldGroupBuilder[T]) Owner(owner string) *FieldGroupBuilder[T] {
	return b.Options(fddp.FieldGroupOwner(owner))
}

func (b *FieldGroupBuilder[T]) Permission(permission string) *FieldGroupBuilder[T] {
	return b.Options(fddp.FieldGroupPermission(permission))
}

func (b *FieldGroupBuilder[T]) PrivateCache(ttl time.Duration) *FieldGroupBuilder[T] {
	return b.Options(fddp.FieldGroupPrivateCache(ttl))
}

func (b *FieldGroupBuilder[T]) PublicCache(ttl time.Duration) *FieldGroupBuilder[T] {
	return b.Options(fddp.FieldGroupPublicCache(ttl))
}

func (b *FieldGroupBuilder[T]) TenantCache(ttl time.Duration) *FieldGroupBuilder[T] {
	return b.Options(fddp.FieldGroupTenantCache(ttl))
}

func (b *FieldGroupBuilder[T]) Register(engine *fddp.Engine) error {
	return RegisterFieldGroup[T](engine, b.cfg)
}

type CollectionBuilder[T any] struct {
	cfg Collection[T]
}

func NewCollection[T any](path string, db *gorm.DB) *CollectionBuilder[T] {
	return &CollectionBuilder[T]{
		cfg: Collection[T]{
			Path:      path,
			DB:        db,
			Fields:    make(map[string]FieldMapping),
			Relations: make(map[string]RelationMapping),
		},
	}
}

func (b *CollectionBuilder[T]) Field(name string, fieldType string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	b.cfg.Fields[name] = buildMapping(name, fieldType, structField, options...)
	return b
}

func (b *CollectionBuilder[T]) String(name string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	return b.Field(name, "string", structField, options...)
}

func (b *CollectionBuilder[T]) NullableString(name string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	return b.Field(name, "string", structField, append(options, Nullable())...)
}

func (b *CollectionBuilder[T]) Number(name string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	return b.Field(name, "number", structField, options...)
}

func (b *CollectionBuilder[T]) Bool(name string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	return b.Field(name, "boolean", structField, options...)
}

func (b *CollectionBuilder[T]) Object(name string, structField string, options ...MappingOption) *CollectionBuilder[T] {
	return b.Field(name, "object", structField, options...)
}

func (b *CollectionBuilder[T]) Relation(name string, structField string, configure func(*RelationBuilder)) *CollectionBuilder[T] {
	builder := &RelationBuilder{mapping: RelationMapping{
		Name:   structField,
		Fields: make(map[string]FieldMapping),
	}}
	if configure != nil {
		configure(builder)
	}
	b.cfg.Relations[name] = builder.mapping
	return b
}

func (b *CollectionBuilder[T]) Scope(scope func(*gorm.DB, fddp.ResourceRequest) *gorm.DB) *CollectionBuilder[T] {
	b.cfg.Scope = scope
	return b
}

func (b *CollectionBuilder[T]) DefaultLimit(limit int) *CollectionBuilder[T] {
	b.cfg.DefaultLimit = limit
	return b
}

func (b *CollectionBuilder[T]) Cursor(field string) *CollectionBuilder[T] {
	b.cfg.CursorField = field
	b.cfg.CursorDesc = false
	return b
}

func (b *CollectionBuilder[T]) DescCursor(field string) *CollectionBuilder[T] {
	b.cfg.CursorField = field
	b.cfg.CursorDesc = true
	return b
}

func (b *CollectionBuilder[T]) TotalCount() *CollectionBuilder[T] {
	b.cfg.IncludeTotalCount = true
	return b
}

func (b *CollectionBuilder[T]) Options(options ...fddp.ResourceOption) *CollectionBuilder[T] {
	b.cfg.Options = append(b.cfg.Options, options...)
	return b
}

func (b *CollectionBuilder[T]) Owner(owner string) *CollectionBuilder[T] {
	return b.Options(fddp.ResourceOwner(owner))
}

func (b *CollectionBuilder[T]) Permission(permission string) *CollectionBuilder[T] {
	return b.Options(fddp.ResourcePermission(permission))
}

func (b *CollectionBuilder[T]) MaxPageSize(size int) *CollectionBuilder[T] {
	return b.Options(fddp.ResourceMaxPageSize(size))
}

func (b *CollectionBuilder[T]) Deprecated() *CollectionBuilder[T] {
	return b.Options(fddp.ResourceDeprecated())
}

func (b *CollectionBuilder[T]) Register(engine *fddp.Engine) error {
	return RegisterCollection[T](engine, b.cfg)
}

type RelationBuilder struct {
	mapping RelationMapping
}

func (b *RelationBuilder) Field(name string, fieldType string, structField string, options ...MappingOption) *RelationBuilder {
	b.mapping.Fields[name] = buildMapping(name, fieldType, structField, options...)
	return b
}

func (b *RelationBuilder) String(name string, structField string, options ...MappingOption) *RelationBuilder {
	return b.Field(name, "string", structField, options...)
}

func (b *RelationBuilder) NullableString(name string, structField string, options ...MappingOption) *RelationBuilder {
	return b.Field(name, "string", structField, append(options, Nullable())...)
}

func (b *RelationBuilder) Number(name string, structField string, options ...MappingOption) *RelationBuilder {
	return b.Field(name, "number", structField, options...)
}

func (b *RelationBuilder) Bool(name string, structField string, options ...MappingOption) *RelationBuilder {
	return b.Field(name, "boolean", structField, options...)
}

func (b *RelationBuilder) Object(name string, structField string, options ...MappingOption) *RelationBuilder {
	return b.Field(name, "object", structField, options...)
}

func (b *RelationBuilder) ParentFields(fields ...string) *RelationBuilder {
	b.mapping.ParentFields = append(b.mapping.ParentFields, fields...)
	return b
}

func (b *RelationBuilder) RequiredFields(fields ...string) *RelationBuilder {
	b.mapping.RequiredFields = append(b.mapping.RequiredFields, fields...)
	return b
}

func buildMapping(name string, fieldType string, structField string, options ...MappingOption) FieldMapping {
	mapping := FieldMapping{
		Type:        fieldType,
		Column:      snakeCase(name),
		StructField: structField,
	}
	for _, option := range options {
		if option != nil {
			option(&mapping)
		}
	}
	return mapping
}

func snakeCase(value string) string {
	if value == "" {
		return value
	}
	var out strings.Builder
	for index, r := range value {
		if r >= 'A' && r <= 'Z' {
			if index > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(r + ('a' - 'A'))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
