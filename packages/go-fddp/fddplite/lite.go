package fddplite

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"github.com/Unicode01/FDDP/packages/go-fddp/gormadapter"
	"gorm.io/gorm"
)

type App struct {
	engine *fddp.Engine
	db     *gorm.DB
}

func NewApp(db *gorm.DB, options ...fddp.Option) *App {
	return &App{engine: fddp.NewEngine(options...), db: db}
}

func NewDevApp(db *gorm.DB, options ...fddp.Option) *App {
	defaults := []fddp.Option{
		fddp.WithCache(fddp.NewMemoryCache()),
		fddp.WithIdempotencyStore(fddp.NewMemoryIdempotencyStore()),
		fddp.WithContractVersion("dev"),
	}
	return NewApp(db, append(defaults, options...)...)
}

func NewProductionApp(db *gorm.DB, options ...fddp.Option) *App {
	defaults := []fddp.Option{
		fddp.WithQueryLimits(ProductionQueryLimits()),
		fddp.WithCommandLimits(ProductionCommandLimits()),
	}
	return NewApp(db, append(defaults, options...)...)
}

func ProductionQueryLimits() fddp.QueryLimits {
	return fddp.QueryLimits{
		MaxFields:          30,
		MaxResources:       4,
		MaxCollectionFirst: 50,
		MaxSelectionFields: 30,
		MaxExpandDepth:     1,
		MaxExpandRelations: 3,
		MaxFilterFields:    6,
		MaxOrderBy:         3,
		MaxCost:            180,
		MaxBodyBytes:       256 << 10,
		MaxQueryDepth:      14,
		MaxQueryNodes:      160,
		Timeout:            2 * time.Second,
	}
}

func ProductionCommandLimits() fddp.CommandLimits {
	return fddp.CommandLimits{
		MaxBodyBytes:  128 << 10,
		MaxInputBytes: 64 << 10,
		MaxInputDepth: 8,
		MaxInputNodes: 120,
		Timeout:       2 * time.Second,
	}
}

func (app *App) Engine() *fddp.Engine {
	return app.engine
}

func (app *App) Handler() http.Handler {
	return app.engine.Handler()
}

type FieldGroupBuilder[T any] struct {
	app        *App
	path       string
	fields     []string
	scope      func(*gorm.DB, fddp.FieldGroupRequest) *gorm.DB
	options    []fddp.FieldGroupOption
	afterQuery func(*T, fddp.FieldGroupRequest) (map[string]any, error)
}

func FieldGroup[T any](app *App, path string) *FieldGroupBuilder[T] {
	return &FieldGroupBuilder[T]{app: app, path: path}
}

func (b *FieldGroupBuilder[T]) Fields(fields ...string) *FieldGroupBuilder[T] {
	b.fields = append(b.fields, fields...)
	return b
}

func (b *FieldGroupBuilder[T]) Self(subjectField string) *FieldGroupBuilder[T] {
	b.scope = func(tx *gorm.DB, req fddp.FieldGroupRequest) *gorm.DB {
		return tx.Where(columnName(subjectField)+" = ?", req.Identity.Subject)
	}
	b.options = append(b.options, fddp.FieldGroupPermission("self"))
	return b
}

func (b *FieldGroupBuilder[T]) Scope(scope func(*gorm.DB, fddp.FieldGroupRequest) *gorm.DB) *FieldGroupBuilder[T] {
	b.scope = scope
	return b
}

func (b *FieldGroupBuilder[T]) Permission(permission string) *FieldGroupBuilder[T] {
	b.options = append(b.options, fddp.FieldGroupPermission(permission))
	return b
}

func (b *FieldGroupBuilder[T]) Owner(owner string) *FieldGroupBuilder[T] {
	b.options = append(b.options, fddp.FieldGroupOwner(owner))
	return b
}

func (b *FieldGroupBuilder[T]) AfterQuery(after func(*T, fddp.FieldGroupRequest) (map[string]any, error)) *FieldGroupBuilder[T] {
	b.afterQuery = after
	return b
}

func (b *FieldGroupBuilder[T]) Register() error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	mappings, err := mappingsFor[T](b.fields)
	if err != nil {
		return err
	}
	return gormadapter.RegisterFieldGroup[T](b.app.engine, gormadapter.FieldGroup[T]{
		Path:       b.path,
		DB:         b.app.db,
		Fields:     mappings,
		Scope:      b.scope,
		Options:    b.options,
		AfterQuery: b.afterQuery,
	})
}

type CollectionBuilder[T any] struct {
	app               *App
	path              string
	fields            []string
	scope             func(*gorm.DB, fddp.ResourceRequest) *gorm.DB
	options           []fddp.ResourceOption
	defaultLimit      int
	cursorField       string
	cursorDesc        bool
	includeTotalCount bool
	relations         []relationConfig
}

type relationConfig struct {
	name           string
	structField    string
	fields         []string
	parentFields   []string
	requiredFields []string
}

func Collection[T any](app *App, path string) *CollectionBuilder[T] {
	return &CollectionBuilder[T]{app: app, path: path}
}

func (b *CollectionBuilder[T]) Fields(fields ...string) *CollectionBuilder[T] {
	b.fields = append(b.fields, fields...)
	return b
}

func (b *CollectionBuilder[T]) Tenant(tenantField string) *CollectionBuilder[T] {
	b.scope = func(tx *gorm.DB, req fddp.ResourceRequest) *gorm.DB {
		return tx.Where(columnName(tenantField)+" = ?", req.Identity.TenantID)
	}
	b.options = append(b.options, fddp.ResourcePermission("tenant"))
	return b
}

func (b *CollectionBuilder[T]) Scope(scope func(*gorm.DB, fddp.ResourceRequest) *gorm.DB) *CollectionBuilder[T] {
	b.scope = scope
	return b
}

func (b *CollectionBuilder[T]) Permission(permission string) *CollectionBuilder[T] {
	b.options = append(b.options, fddp.ResourcePermission(permission))
	return b
}

func (b *CollectionBuilder[T]) Owner(owner string) *CollectionBuilder[T] {
	b.options = append(b.options, fddp.ResourceOwner(owner))
	return b
}

func (b *CollectionBuilder[T]) MaxPageSize(size int) *CollectionBuilder[T] {
	b.options = append(b.options, fddp.ResourceMaxPageSize(size))
	return b
}

func (b *CollectionBuilder[T]) DefaultLimit(limit int) *CollectionBuilder[T] {
	b.defaultLimit = limit
	return b
}

func (b *CollectionBuilder[T]) Cursor(field string) *CollectionBuilder[T] {
	b.cursorField = fddpName(field)
	b.cursorDesc = false
	return b
}

func (b *CollectionBuilder[T]) DescCursor(field string) *CollectionBuilder[T] {
	b.cursorField = fddpName(field)
	b.cursorDesc = true
	return b
}

func (b *CollectionBuilder[T]) TotalCount() *CollectionBuilder[T] {
	b.includeTotalCount = true
	return b
}

func (b *CollectionBuilder[T]) Relation(name string, structField string, fields ...string) *CollectionBuilder[T] {
	b.relations = append(b.relations, relationConfig{name: name, structField: structField, fields: fields})
	return b
}

func (b *CollectionBuilder[T]) RelationWithKeys(name string, structField string, parentFields []string, requiredFields []string, fields ...string) *CollectionBuilder[T] {
	b.relations = append(b.relations, relationConfig{
		name:           name,
		structField:    structField,
		fields:         fields,
		parentFields:   namesToFDDP(parentFields),
		requiredFields: namesToFDDP(requiredFields),
	})
	return b
}

func (b *CollectionBuilder[T]) Register() error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	mappings, err := mappingsFor[T](b.fields)
	if err != nil {
		return err
	}
	relations := make(map[string]gormadapter.RelationMapping, len(b.relations))
	for _, relation := range b.relations {
		relationMappings, err := relationMappingsFor[T](relation.structField, relation.fields)
		if err != nil {
			return err
		}
		parentFields := relation.parentFields
		if len(parentFields) == 0 {
			parentFields = inferParentFields(mappings, relation.structField)
		}
		requiredFields := relation.requiredFields
		if len(requiredFields) == 0 {
			requiredFields = []string{"id"}
		}
		relations[relation.name] = gormadapter.RelationMapping{
			Name:           relation.structField,
			Fields:         relationMappings,
			ParentFields:   parentFields,
			RequiredFields: requiredFields,
		}
	}
	return gormadapter.RegisterCollection[T](b.app.engine, gormadapter.Collection[T]{
		Path:              b.path,
		DB:                b.app.db,
		Fields:            mappings,
		Relations:         relations,
		Scope:             b.scope,
		Options:           b.options,
		DefaultLimit:      b.defaultLimit,
		CursorField:       b.cursorField,
		CursorDesc:        b.cursorDesc,
		IncludeTotalCount: b.includeTotalCount,
	})
}

type CommandBuilder[TInput any] struct {
	app                 *App
	name                string
	owner               string
	permission          string
	idempotencyRequired bool
	auditLevel          string
}

func Command[TInput any](app *App, name string) *CommandBuilder[TInput] {
	return &CommandBuilder[TInput]{app: app, name: name}
}

func (b *CommandBuilder[TInput]) Owner(owner string) *CommandBuilder[TInput] {
	b.owner = owner
	return b
}

func (b *CommandBuilder[TInput]) Permission(permission string) *CommandBuilder[TInput] {
	b.permission = permission
	return b
}

func (b *CommandBuilder[TInput]) Self() *CommandBuilder[TInput] {
	b.permission = "self"
	return b
}

func (b *CommandBuilder[TInput]) Tenant() *CommandBuilder[TInput] {
	b.permission = "tenant"
	return b
}

func (b *CommandBuilder[TInput]) Idempotent() *CommandBuilder[TInput] {
	b.idempotencyRequired = true
	return b
}

func (b *CommandBuilder[TInput]) Audit(level string) *CommandBuilder[TInput] {
	b.auditLevel = level
	return b
}

func (b *CommandBuilder[TInput]) Register(execute func(context.Context, fddp.CommandExecutionRequest, TInput) (fddp.CommandExecutionResult, error)) error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	return b.app.engine.RegisterCommand(fddp.CommandDefinition{
		Name:                b.name,
		Owner:               b.owner,
		Permission:          b.permission,
		IdempotencyRequired: b.idempotencyRequired,
		AuditLevel:          b.auditLevel,
		Input:               inputSchemaFor[TInput](),
		Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
			input, err := fddp.DecodeInput[TInput](req.Input)
			if err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			return execute(ctx, req, input)
		},
	})
}

type UpdateCommandBuilder[TModel any, TInput any] struct {
	app                 *App
	name                string
	owner               string
	permission          string
	idempotencyRequired bool
	auditLevel          string
	scope               func(*gorm.DB, fddp.CommandExecutionRequest, TInput) *gorm.DB
	sets                []updateSet[TInput]
	wheres              []updateSet[TInput]
	invalidates         []string
	before              func(context.Context, fddp.CommandExecutionRequest, TInput) error
	after               func(context.Context, fddp.CommandExecutionRequest, TInput, int64) (map[string]any, error)
}

type updateSet[TInput any] struct {
	modelField string
	inputField string
	value      func(TInput) (any, bool)
}

func UpdateCommand[TModel any, TInput any](app *App, name string) *UpdateCommandBuilder[TModel, TInput] {
	return &UpdateCommandBuilder[TModel, TInput]{app: app, name: name}
}

func (b *UpdateCommandBuilder[TModel, TInput]) Owner(owner string) *UpdateCommandBuilder[TModel, TInput] {
	b.owner = owner
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Permission(permission string) *UpdateCommandBuilder[TModel, TInput] {
	b.permission = permission
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Self(subjectField string) *UpdateCommandBuilder[TModel, TInput] {
	b.permission = "self"
	column := columnName(subjectField)
	b.scope = func(tx *gorm.DB, req fddp.CommandExecutionRequest, input TInput) *gorm.DB {
		return tx.Where(column+" = ?", req.Identity.Subject)
	}
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Tenant(tenantField string) *UpdateCommandBuilder[TModel, TInput] {
	b.permission = "tenant"
	column := columnName(tenantField)
	b.scope = func(tx *gorm.DB, req fddp.CommandExecutionRequest, input TInput) *gorm.DB {
		return tx.Where(column+" = ?", req.Identity.TenantID)
	}
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Scope(scope func(*gorm.DB, fddp.CommandExecutionRequest, TInput) *gorm.DB) *UpdateCommandBuilder[TModel, TInput] {
	b.scope = scope
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Set(modelField string, inputField string) *UpdateCommandBuilder[TModel, TInput] {
	b.sets = append(b.sets, updateSet[TInput]{modelField: modelField, inputField: inputField})
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) SetValue(modelField string, value func(TInput) (any, bool)) *UpdateCommandBuilder[TModel, TInput] {
	b.sets = append(b.sets, updateSet[TInput]{modelField: modelField, value: value})
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Where(modelField string, inputField string) *UpdateCommandBuilder[TModel, TInput] {
	b.wheres = append(b.wheres, updateSet[TInput]{modelField: modelField, inputField: inputField})
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) WhereValue(modelField string, value func(TInput) (any, bool)) *UpdateCommandBuilder[TModel, TInput] {
	b.wheres = append(b.wheres, updateSet[TInput]{modelField: modelField, value: value})
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Idempotent() *UpdateCommandBuilder[TModel, TInput] {
	b.idempotencyRequired = true
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Audit(level string) *UpdateCommandBuilder[TModel, TInput] {
	b.auditLevel = level
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Invalidates(fields ...string) *UpdateCommandBuilder[TModel, TInput] {
	b.invalidates = append(b.invalidates, fields...)
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Before(before func(context.Context, fddp.CommandExecutionRequest, TInput) error) *UpdateCommandBuilder[TModel, TInput] {
	b.before = before
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) After(after func(context.Context, fddp.CommandExecutionRequest, TInput, int64) (map[string]any, error)) *UpdateCommandBuilder[TModel, TInput] {
	b.after = after
	return b
}

func (b *UpdateCommandBuilder[TModel, TInput]) Register() error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	if len(b.sets) == 0 {
		return fmt.Errorf("fddp lite: update command needs at least one Set or SetValue")
	}
	if b.scope == nil && len(b.wheres) == 0 {
		return fmt.Errorf("fddp lite: update command needs Self, Tenant, Scope, Where, or WhereValue")
	}
	updates, err := compileUpdateSets[TModel, TInput](b.sets)
	if err != nil {
		return err
	}
	wheres, err := compileUpdateSets[TModel, TInput](b.wheres)
	if err != nil {
		return err
	}
	return b.app.engine.RegisterCommand(fddp.CommandDefinition{
		Name:                b.name,
		Owner:               b.owner,
		Permission:          b.permission,
		IdempotencyRequired: b.idempotencyRequired,
		AuditLevel:          b.auditLevel,
		Input:               inputSchemaFor[TInput](),
		Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
			input, err := fddp.DecodeInput[TInput](req.Input)
			if err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			if b.before != nil {
				if err := b.before(ctx, req, input); err != nil {
					return fddp.CommandExecutionResult{}, err
				}
			}

			values := make(map[string]any, len(updates))
			for _, update := range updates {
				value, ok := update.value(input)
				if !ok {
					continue
				}
				values[update.column] = value
			}
			if len(values) == 0 {
				return fddp.CommandExecutionResult{}, fmt.Errorf("fddp lite: update command has no values")
			}

			tx := b.app.db.WithContext(ctx).Model(new(TModel))
			if b.scope != nil {
				tx = b.scope(tx, req, input)
			}
			appliedWhere := 0
			for _, where := range wheres {
				value, ok := where.value(input)
				if !ok {
					continue
				}
				tx = tx.Where(where.column+" = ?", value)
				appliedWhere++
			}
			if len(wheres) > 0 && appliedWhere == 0 {
				return fddp.CommandExecutionResult{}, fmt.Errorf("fddp lite: update command has no where values")
			}
			result := tx.Updates(values)
			if result.Error != nil {
				return fddp.CommandExecutionResult{}, result.Error
			}

			payload := map[string]any{"updated": result.RowsAffected > 0, "rowsAffected": result.RowsAffected}
			if b.after != nil {
				next, err := b.after(ctx, req, input, result.RowsAffected)
				if err != nil {
					return fddp.CommandExecutionResult{}, err
				}
				if next != nil {
					payload = next
				}
			}
			return fddp.CommandExecutionResult{
				Result:      payload,
				Invalidates: append([]string(nil), b.invalidates...),
			}, nil
		},
	})
}

type compiledUpdateSet[TInput any] struct {
	column string
	value  func(TInput) (any, bool)
}

func compileUpdateSets[TModel any, TInput any](sets []updateSet[TInput]) ([]compiledUpdateSet[TInput], error) {
	modelType := derefType(reflect.TypeOf((*TModel)(nil)).Elem())
	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("fddp lite: model must be a struct")
	}
	inputType := derefType(reflect.TypeOf((*TInput)(nil)).Elem())
	if inputType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("fddp lite: input must be a struct")
	}

	out := make([]compiledUpdateSet[TInput], 0, len(sets))
	for _, set := range sets {
		modelField, ok := modelType.FieldByName(set.modelField)
		if !ok {
			return nil, fmt.Errorf("fddp lite: struct field %q is not found on %s", set.modelField, modelType.Name())
		}
		column := columnName(modelField.Name)
		if set.value != nil {
			out = append(out, compiledUpdateSet[TInput]{column: column, value: set.value})
			continue
		}
		if set.inputField == "" {
			return nil, fmt.Errorf("fddp lite: input field is required for %s", set.modelField)
		}
		inputField, ok := inputType.FieldByName(set.inputField)
		if !ok {
			return nil, fmt.Errorf("fddp lite: input field %q is not found on %s", set.inputField, inputType.Name())
		}
		index := inputField.Index
		out = append(out, compiledUpdateSet[TInput]{
			column: column,
			value: func(input TInput) (any, bool) {
				value := reflect.ValueOf(input)
				if value.Kind() == reflect.Pointer {
					if value.IsNil() {
						return nil, false
					}
					value = value.Elem()
				}
				field := value.FieldByIndex(index)
				return updateFieldValue(field)
			},
		})
	}
	return out, nil
}

func updateFieldValue(value reflect.Value) (any, bool) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		return value.Elem().Interface(), true
	}
	if value.IsZero() {
		return nil, false
	}
	return value.Interface(), true
}

type CreateCommandBuilder[TModel any, TInput any] struct {
	app                 *App
	name                string
	owner               string
	permission          string
	idempotencyRequired bool
	auditLevel          string
	sets                []updateSet[TInput]
	identitySets        []identitySet[TInput]
	invalidates         []string
	before              func(context.Context, fddp.CommandExecutionRequest, TInput) error
	after               func(context.Context, fddp.CommandExecutionRequest, TInput, any) (map[string]any, error)
}

type identitySet[TInput any] struct {
	modelField string
	value      func(fddp.CommandExecutionRequest, TInput) any
}

func CreateCommand[TModel any, TInput any](app *App, name string) *CreateCommandBuilder[TModel, TInput] {
	return &CreateCommandBuilder[TModel, TInput]{app: app, name: name}
}

func (b *CreateCommandBuilder[TModel, TInput]) Owner(owner string) *CreateCommandBuilder[TModel, TInput] {
	b.owner = owner
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Permission(permission string) *CreateCommandBuilder[TModel, TInput] {
	b.permission = permission
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Self(subjectField string) *CreateCommandBuilder[TModel, TInput] {
	b.permission = "self"
	b.identitySets = append(b.identitySets, identitySet[TInput]{
		modelField: subjectField,
		value: func(req fddp.CommandExecutionRequest, input TInput) any {
			return req.Identity.Subject
		},
	})
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Tenant(tenantField string) *CreateCommandBuilder[TModel, TInput] {
	b.permission = "tenant"
	b.identitySets = append(b.identitySets, identitySet[TInput]{
		modelField: tenantField,
		value: func(req fddp.CommandExecutionRequest, input TInput) any {
			return req.Identity.TenantID
		},
	})
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Set(modelField string, inputField string) *CreateCommandBuilder[TModel, TInput] {
	b.sets = append(b.sets, updateSet[TInput]{modelField: modelField, inputField: inputField})
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) SetValue(modelField string, value func(TInput) (any, bool)) *CreateCommandBuilder[TModel, TInput] {
	b.sets = append(b.sets, updateSet[TInput]{modelField: modelField, value: value})
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Idempotent() *CreateCommandBuilder[TModel, TInput] {
	b.idempotencyRequired = true
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Audit(level string) *CreateCommandBuilder[TModel, TInput] {
	b.auditLevel = level
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Invalidates(fields ...string) *CreateCommandBuilder[TModel, TInput] {
	b.invalidates = append(b.invalidates, fields...)
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Before(before func(context.Context, fddp.CommandExecutionRequest, TInput) error) *CreateCommandBuilder[TModel, TInput] {
	b.before = before
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) After(after func(context.Context, fddp.CommandExecutionRequest, TInput, any) (map[string]any, error)) *CreateCommandBuilder[TModel, TInput] {
	b.after = after
	return b
}

func (b *CreateCommandBuilder[TModel, TInput]) Register() error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	if len(b.sets) == 0 && len(b.identitySets) == 0 {
		return fmt.Errorf("fddp lite: create command needs at least one Set, SetValue, Self, or Tenant")
	}
	sets, err := compileUpdateSets[TModel, TInput](b.sets)
	if err != nil {
		return err
	}
	identitySets, err := compileIdentitySets[TModel, TInput](b.identitySets)
	if err != nil {
		return err
	}
	return b.app.engine.RegisterCommand(fddp.CommandDefinition{
		Name:                b.name,
		Owner:               b.owner,
		Permission:          b.permission,
		IdempotencyRequired: b.idempotencyRequired,
		AuditLevel:          b.auditLevel,
		Input:               inputSchemaFor[TInput](),
		Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
			input, err := fddp.DecodeInput[TInput](req.Input)
			if err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			if b.before != nil {
				if err := b.before(ctx, req, input); err != nil {
					return fddp.CommandExecutionResult{}, err
				}
			}
			values := make(map[string]any, len(sets)+len(identitySets))
			for _, set := range sets {
				value, ok := set.value(input)
				if ok {
					values[set.column] = value
				}
			}
			for _, set := range identitySets {
				values[set.column] = set.value(req, input)
			}
			row, err := buildModelFromColumns[TModel](values)
			if err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			if err := b.app.db.WithContext(ctx).Create(row).Error; err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			payload := map[string]any{"created": true}
			if b.after != nil {
				next, err := b.after(ctx, req, input, row)
				if err != nil {
					return fddp.CommandExecutionResult{}, err
				}
				if next != nil {
					payload = next
				}
			}
			return fddp.CommandExecutionResult{Result: payload, Invalidates: append([]string(nil), b.invalidates...)}, nil
		},
	})
}

type DeleteCommandBuilder[TModel any, TInput any] struct {
	app                 *App
	name                string
	owner               string
	permission          string
	idempotencyRequired bool
	auditLevel          string
	scope               func(*gorm.DB, fddp.CommandExecutionRequest, TInput) *gorm.DB
	wheres              []updateSet[TInput]
	invalidates         []string
	before              func(context.Context, fddp.CommandExecutionRequest, TInput) error
	after               func(context.Context, fddp.CommandExecutionRequest, TInput, int64) (map[string]any, error)
}

func DeleteCommand[TModel any, TInput any](app *App, name string) *DeleteCommandBuilder[TModel, TInput] {
	return &DeleteCommandBuilder[TModel, TInput]{app: app, name: name}
}

func (b *DeleteCommandBuilder[TModel, TInput]) Owner(owner string) *DeleteCommandBuilder[TModel, TInput] {
	b.owner = owner
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Permission(permission string) *DeleteCommandBuilder[TModel, TInput] {
	b.permission = permission
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Self(subjectField string) *DeleteCommandBuilder[TModel, TInput] {
	b.permission = "self"
	column := columnName(subjectField)
	b.scope = func(tx *gorm.DB, req fddp.CommandExecutionRequest, input TInput) *gorm.DB {
		return tx.Where(column+" = ?", req.Identity.Subject)
	}
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Tenant(tenantField string) *DeleteCommandBuilder[TModel, TInput] {
	b.permission = "tenant"
	column := columnName(tenantField)
	b.scope = func(tx *gorm.DB, req fddp.CommandExecutionRequest, input TInput) *gorm.DB {
		return tx.Where(column+" = ?", req.Identity.TenantID)
	}
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Scope(scope func(*gorm.DB, fddp.CommandExecutionRequest, TInput) *gorm.DB) *DeleteCommandBuilder[TModel, TInput] {
	b.scope = scope
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Where(modelField string, inputField string) *DeleteCommandBuilder[TModel, TInput] {
	b.wheres = append(b.wheres, updateSet[TInput]{modelField: modelField, inputField: inputField})
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) WhereValue(modelField string, value func(TInput) (any, bool)) *DeleteCommandBuilder[TModel, TInput] {
	b.wheres = append(b.wheres, updateSet[TInput]{modelField: modelField, value: value})
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Idempotent() *DeleteCommandBuilder[TModel, TInput] {
	b.idempotencyRequired = true
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Audit(level string) *DeleteCommandBuilder[TModel, TInput] {
	b.auditLevel = level
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Invalidates(fields ...string) *DeleteCommandBuilder[TModel, TInput] {
	b.invalidates = append(b.invalidates, fields...)
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Before(before func(context.Context, fddp.CommandExecutionRequest, TInput) error) *DeleteCommandBuilder[TModel, TInput] {
	b.before = before
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) After(after func(context.Context, fddp.CommandExecutionRequest, TInput, int64) (map[string]any, error)) *DeleteCommandBuilder[TModel, TInput] {
	b.after = after
	return b
}

func (b *DeleteCommandBuilder[TModel, TInput]) Register() error {
	if b.app == nil {
		return fmt.Errorf("fddp lite: app is required")
	}
	if len(b.wheres) == 0 {
		return fmt.Errorf("fddp lite: delete command needs at least one Where or WhereValue")
	}
	wheres, err := compileUpdateSets[TModel, TInput](b.wheres)
	if err != nil {
		return err
	}
	return b.app.engine.RegisterCommand(fddp.CommandDefinition{
		Name:                b.name,
		Owner:               b.owner,
		Permission:          b.permission,
		IdempotencyRequired: b.idempotencyRequired,
		AuditLevel:          b.auditLevel,
		Input:               inputSchemaFor[TInput](),
		Execute: func(ctx context.Context, req fddp.CommandExecutionRequest) (fddp.CommandExecutionResult, error) {
			input, err := fddp.DecodeInput[TInput](req.Input)
			if err != nil {
				return fddp.CommandExecutionResult{}, err
			}
			if b.before != nil {
				if err := b.before(ctx, req, input); err != nil {
					return fddp.CommandExecutionResult{}, err
				}
			}
			tx := b.app.db.WithContext(ctx).Where("1 = 1")
			if b.scope != nil {
				tx = b.scope(tx, req, input)
			}
			applied := 0
			for _, where := range wheres {
				value, ok := where.value(input)
				if !ok {
					continue
				}
				tx = tx.Where(where.column+" = ?", value)
				applied++
			}
			if applied == 0 {
				return fddp.CommandExecutionResult{}, fmt.Errorf("fddp lite: delete command has no where values")
			}
			result := tx.Delete(new(TModel))
			if result.Error != nil {
				return fddp.CommandExecutionResult{}, result.Error
			}
			payload := map[string]any{"deleted": result.RowsAffected > 0, "rowsAffected": result.RowsAffected}
			if b.after != nil {
				next, err := b.after(ctx, req, input, result.RowsAffected)
				if err != nil {
					return fddp.CommandExecutionResult{}, err
				}
				if next != nil {
					payload = next
				}
			}
			return fddp.CommandExecutionResult{Result: payload, Invalidates: append([]string(nil), b.invalidates...)}, nil
		},
	})
}

type compiledIdentitySet[TInput any] struct {
	column string
	value  func(fddp.CommandExecutionRequest, TInput) any
}

func compileIdentitySets[TModel any, TInput any](sets []identitySet[TInput]) ([]compiledIdentitySet[TInput], error) {
	modelType := derefType(reflect.TypeOf((*TModel)(nil)).Elem())
	out := make([]compiledIdentitySet[TInput], 0, len(sets))
	for _, set := range sets {
		modelField, ok := modelType.FieldByName(set.modelField)
		if !ok {
			return nil, fmt.Errorf("fddp lite: struct field %q is not found on %s", set.modelField, modelType.Name())
		}
		out = append(out, compiledIdentitySet[TInput]{column: columnName(modelField.Name), value: set.value})
	}
	return out, nil
}

func buildModelFromColumns[TModel any](values map[string]any) (*TModel, error) {
	modelType := derefType(reflect.TypeOf((*TModel)(nil)).Elem())
	row := new(TModel)
	rv := reflect.ValueOf(row).Elem()
	for column, raw := range values {
		fieldIndex := -1
		for index := 0; index < modelType.NumField(); index++ {
			if columnName(modelType.Field(index).Name) == column {
				fieldIndex = index
				break
			}
		}
		if fieldIndex < 0 {
			return nil, fmt.Errorf("fddp lite: column %q cannot be mapped to model", column)
		}
		field := rv.Field(fieldIndex)
		if !field.CanSet() {
			return nil, fmt.Errorf("fddp lite: struct field %q cannot be set", modelType.Field(fieldIndex).Name)
		}
		if err := assignValue(field, raw); err != nil {
			return nil, fmt.Errorf("fddp lite: struct field %q cannot be set: %w", modelType.Field(fieldIndex).Name, err)
		}
	}
	return row, nil
}

func assignValue(field reflect.Value, raw any) error {
	if raw == nil {
		return nil
	}
	value := reflect.ValueOf(raw)
	if value.Type().AssignableTo(field.Type()) {
		field.Set(value)
		return nil
	}
	if value.Type().ConvertibleTo(field.Type()) {
		field.Set(value.Convert(field.Type()))
		return nil
	}
	if field.Kind() == reflect.Pointer {
		elemType := field.Type().Elem()
		if value.Type().AssignableTo(elemType) {
			ptr := reflect.New(elemType)
			ptr.Elem().Set(value)
			field.Set(ptr)
			return nil
		}
		if value.Type().ConvertibleTo(elemType) {
			ptr := reflect.New(elemType)
			ptr.Elem().Set(value.Convert(elemType))
			field.Set(ptr)
			return nil
		}
	}
	return fmt.Errorf("value type %s is not assignable to %s", value.Type(), field.Type())
}

func inputSchemaFor[TInput any]() []fddp.ContractInputField {
	inputType := derefType(reflect.TypeOf((*TInput)(nil)).Elem())
	if inputType.Kind() != reflect.Struct {
		return nil
	}
	fields := make([]fddp.ContractInputField, 0, inputType.NumField())
	for index := 0; index < inputType.NumField(); index++ {
		field := inputType.Field(index)
		if field.PkgPath != "" || field.Anonymous {
			continue
		}
		name := jsonFieldName(field)
		if name == "-" {
			continue
		}
		if name == "" {
			name = fddpName(field.Name)
		}
		fields = append(fields, fddp.ContractInputField{
			Field:    name,
			Type:     contractType(field.Type),
			Required: !isNullable(field.Type),
			Nullable: isNullable(field.Type),
		})
	}
	return fields
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return ""
	}
	name, _, _ := strings.Cut(tag, ",")
	return name
}

func mappingsFor[T any](fields []string) (map[string]gormadapter.FieldMapping, error) {
	modelType := derefType(reflect.TypeOf((*T)(nil)).Elem())
	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("fddp lite: model must be a struct")
	}
	if len(fields) == 0 {
		fields = exportedFieldNames(modelType)
	}
	mappings := make(map[string]gormadapter.FieldMapping, len(fields))
	for _, field := range fields {
		structField, ok := modelType.FieldByName(field)
		if !ok {
			return nil, fmt.Errorf("fddp lite: struct field %q is not found on %s", field, modelType.Name())
		}
		name := fddpName(field)
		mappings[name] = gormadapter.FieldMapping{
			Type:        contractType(structField.Type),
			Column:      columnName(field),
			StructField: field,
			Nullable:    isNullable(structField.Type),
		}
	}
	return mappings, nil
}

func relationMappingsFor[T any](relationField string, fields []string) (map[string]gormadapter.FieldMapping, error) {
	modelType := derefType(reflect.TypeOf((*T)(nil)).Elem())
	structField, ok := modelType.FieldByName(relationField)
	if !ok {
		return nil, fmt.Errorf("fddp lite: relation field %q is not found on %s", relationField, modelType.Name())
	}
	relationType := derefType(structField.Type)
	if relationType.Kind() == reflect.Slice || relationType.Kind() == reflect.Array {
		relationType = derefType(relationType.Elem())
	}
	if relationType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("fddp lite: relation field %q is not a struct relation", relationField)
	}
	if len(fields) == 0 {
		fields = exportedFieldNames(relationType)
	}
	mappings := make(map[string]gormadapter.FieldMapping, len(fields))
	for _, field := range fields {
		childField, ok := relationType.FieldByName(field)
		if !ok {
			return nil, fmt.Errorf("fddp lite: relation field %q is not found on %s", field, relationType.Name())
		}
		name := fddpName(field)
		mappings[name] = gormadapter.FieldMapping{
			Type:        contractType(childField.Type),
			Column:      columnName(field),
			StructField: field,
			Nullable:    isNullable(childField.Type),
		}
	}
	return mappings, nil
}

func exportedFieldNames(modelType reflect.Type) []string {
	fields := make([]string, 0, modelType.NumField())
	for index := 0; index < modelType.NumField(); index++ {
		field := modelType.Field(index)
		if field.PkgPath == "" && !field.Anonymous && isScalarType(field.Type) {
			fields = append(fields, field.Name)
		}
	}
	return fields
}

func inferParentFields(mappings map[string]gormadapter.FieldMapping, relationField string) []string {
	candidate := fddpName(relationField + "ID")
	if _, ok := mappings[candidate]; ok {
		return []string{candidate}
	}
	return nil
}

func namesToFDDP(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, fddpName(field))
	}
	return out
}

func derefType(value reflect.Type) reflect.Type {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func contractType(value reflect.Type) string {
	value = derefType(value)
	switch value.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Struct, reflect.Map:
		return "object"
	default:
		return "unknown"
	}
}

func isNullable(value reflect.Type) bool {
	return value.Kind() == reflect.Pointer
}

func isScalarType(value reflect.Type) bool {
	value = derefType(value)
	switch value.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func fddpName(value string) string {
	words := identifierWords(value)
	if len(words) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(strings.ToLower(words[0]))
	for _, word := range words[1:] {
		lower := strings.ToLower(word)
		runes := []rune(lower)
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		out.WriteString(string(runes))
	}
	return out.String()
}

func columnName(value string) string {
	words := identifierWords(value)
	for index, word := range words {
		words[index] = strings.ToLower(word)
	}
	return strings.Join(words, "_")
}

func identifierWords(value string) []string {
	if value == "" {
		return nil
	}
	runes := []rune(value)
	words := make([]string, 0, 4)
	start := 0
	for index := 1; index < len(runes); index++ {
		current := runes[index]
		previous := runes[index-1]
		var next rune
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		if current == '_' || current == '-' || current == ' ' {
			if start < index {
				words = append(words, string(runes[start:index]))
			}
			start = index + 1
			continue
		}
		if previous == '_' || previous == '-' || previous == ' ' {
			start = index
			continue
		}
		if unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			words = append(words, string(runes[start:index]))
			start = index
			continue
		}
		if unicode.IsUpper(current) && unicode.IsUpper(previous) && next != 0 && unicode.IsLower(next) {
			words = append(words, string(runes[start:index]))
			start = index
		}
	}
	if start < len(runes) {
		words = append(words, string(runes[start:]))
	}
	return words
}
