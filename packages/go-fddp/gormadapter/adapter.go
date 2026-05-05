package gormadapter

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"gorm.io/gorm"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

type FieldMapping struct {
	Type        string
	Column      string
	StructField string
	Nullable    bool
}

type FieldGroup[T any] struct {
	Path       string
	DB         *gorm.DB
	Fields     map[string]FieldMapping
	Scope      func(*gorm.DB, fddp.FieldGroupRequest) *gorm.DB
	Options    []fddp.FieldGroupOption
	AfterQuery func(*T, fddp.FieldGroupRequest) (map[string]any, error)
}

func RegisterFieldGroup[T any](engine *fddp.Engine, cfg FieldGroup[T]) error {
	if engine == nil {
		return adapterError(ErrorInvalidConfig, "engine is required", "pass a non-nil *fddp.Engine")
	}
	if cfg.DB == nil {
		return adapterError(ErrorInvalidConfig, "db is required", "pass a non-nil *gorm.DB")
	}
	if cfg.Path == "" {
		return adapterError(ErrorInvalidConfig, "path is required", "set the FDDP path, for example me.profile")
	}
	if len(cfg.Fields) == 0 {
		return adapterError(ErrorInvalidConfig, "fields are required", "map at least one FDDP field")
	}
	if err := validateMappings(cfg.Fields); err != nil {
		return err
	}

	fields := make([]fddp.GroupField, 0, len(cfg.Fields))
	for name, mapping := range cfg.Fields {
		fieldType := mapping.Type
		if fieldType == "" {
			fieldType = "unknown"
		}
		fields = append(fields, fddp.GroupField{Name: name, Type: fieldType, Nullable: mapping.Nullable})
	}

	return engine.RegisterFieldGroup(cfg.Path, fields, func(ctx context.Context, req fddp.FieldGroupRequest) (map[string]any, error) {
		selected, err := selectedMappings(cfg.Path, cfg.Fields, req.Fields)
		if err != nil {
			return nil, err
		}

		columns := make([]string, 0, len(selected))
		for _, mapping := range selected {
			columns = append(columns, mapping.Column)
		}

		db := cfg.DB.WithContext(ctx).Model(new(T)).Select(uniqueStrings(columns))
		if cfg.Scope != nil {
			db = cfg.Scope(db, req)
		}

		var row T
		if err := db.Take(&row).Error; err != nil {
			return nil, err
		}

		values := make(map[string]any, len(selected))
		if cfg.AfterQuery != nil {
			extra, err := cfg.AfterQuery(&row, req)
			if err != nil {
				return nil, wrapAdapterError(ErrorProjectionFailed, "after query hook failed", "return only requested computed fields from AfterQuery", err)
			}
			for key, value := range extra {
				values[key] = value
			}
		}

		for name, mapping := range selected {
			if _, exists := values[name]; exists {
				continue
			}
			value, err := structFieldValue(row, mapping.StructField)
			if err != nil {
				return nil, wrapAdapterError(ErrorProjectionFailed, fmt.Sprintf("struct field %q cannot be projected", mapping.StructField), "check the StructField name in the GORM adapter mapping", err)
			}
			values[name] = value
		}

		return values, nil
	}, cfg.Options...)
}

func selectedMappings(path string, fields map[string]FieldMapping, requested []string) (map[string]FieldMapping, error) {
	selected := make(map[string]FieldMapping, len(requested))
	for _, fullPath := range requested {
		name := strings.TrimPrefix(fullPath, path+".")
		mapping, ok := fields[name]
		if !ok {
			return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("field %q is not mapped", fullPath), "add an explicit field mapping before exposing it to clients")
		}
		selected[name] = mapping
	}
	return selected, nil
}

func validateMappings(fields map[string]FieldMapping) error {
	for name, mapping := range fields {
		if name == "" || strings.Contains(name, ".") {
			return adapterError(ErrorInvalidConfig, fmt.Sprintf("invalid field name %q", name), "field names must be local leaf names without dots")
		}
		if mapping.Column == "" {
			return adapterError(ErrorInvalidConfig, fmt.Sprintf("field %q needs a column", name), "set Column or use a builder method that derives one")
		}
		if !identifierPattern.MatchString(mapping.Column) {
			return adapterError(ErrorUnsafeIdentifier, fmt.Sprintf("unsafe column %q", mapping.Column), "columns must be configured server-side identifiers, not client input")
		}
		if mapping.StructField == "" {
			return adapterError(ErrorInvalidConfig, fmt.Sprintf("field %q needs a struct field", name), "set StructField to the exported Go field name")
		}
		if !identifierPattern.MatchString(mapping.StructField) {
			return adapterError(ErrorUnsafeIdentifier, fmt.Sprintf("unsafe struct field %q", mapping.StructField), "struct fields must be configured exported Go identifiers")
		}
	}
	return nil
}

func structFieldValue(row any, field string) (any, error) {
	value := reflect.ValueOf(row)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, adapterError(ErrorProjectionFailed, "row is not a struct", "use a struct model type with the GORM adapter")
	}

	current := value
	for _, part := range strings.Split(field, ".") {
		current = current.FieldByName(part)
		if !current.IsValid() {
			return nil, adapterError(ErrorProjectionFailed, fmt.Sprintf("struct field %q is not found", field), "check the StructField mapping and exported Go field names")
		}
		if current.Kind() == reflect.Pointer {
			if current.IsNil() {
				return nil, nil
			}
			current = current.Elem()
		}
	}
	return current.Interface(), nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
