package gormadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	fddp "github.com/Unicode01/FDDP/packages/go-fddp"
	"gorm.io/gorm"
)

type RelationMapping struct {
	Name           string
	Fields         map[string]FieldMapping
	ParentFields   []string
	RequiredFields []string
}

type Collection[T any] struct {
	Path              string
	DB                *gorm.DB
	Fields            map[string]FieldMapping
	Relations         map[string]RelationMapping
	Scope             func(*gorm.DB, fddp.ResourceRequest) *gorm.DB
	Options           []fddp.ResourceOption
	DefaultLimit      int
	CursorField       string
	CursorDesc        bool
	IncludeTotalCount bool
}

func RegisterCollection[T any](engine *fddp.Engine, cfg Collection[T]) error {
	if engine == nil {
		return adapterError(ErrorInvalidConfig, "engine is required", "pass a non-nil *fddp.Engine")
	}
	if cfg.DB == nil {
		return adapterError(ErrorInvalidConfig, "db is required", "pass a non-nil *gorm.DB")
	}
	if cfg.Path == "" {
		return adapterError(ErrorInvalidConfig, "path is required", "set the FDDP path, for example project.list")
	}
	if len(cfg.Fields) == 0 {
		return adapterError(ErrorInvalidConfig, "fields are required", "map at least one selectable field")
	}
	if err := validateMappings(cfg.Fields); err != nil {
		return err
	}
	if cfg.CursorField != "" {
		if _, ok := cfg.Fields[cfg.CursorField]; !ok {
			return adapterError(ErrorInvalidCursor, fmt.Sprintf("cursor field %q is not mapped", cfg.CursorField), "cursor fields must also be mapped fields")
		}
	}
	for name, relation := range cfg.Relations {
		if name == "" || !identifierPattern.MatchString(name) {
			return adapterError(ErrorUnsafeIdentifier, fmt.Sprintf("unsafe relation %q", name), "relation keys must be server-side identifiers")
		}
		if relation.Name == "" {
			relation.Name = name
		}
		if !identifierPattern.MatchString(relation.Name) {
			return adapterError(ErrorUnsafeIdentifier, fmt.Sprintf("unsafe preload relation %q", relation.Name), "GORM preload relation names must be configured identifiers")
		}
		if err := validateMappings(relation.Fields); err != nil {
			return err
		}
		if err := validateFieldNames(cfg.Fields, relation.ParentFields, "relation parent field"); err != nil {
			return err
		}
		if err := validateFieldNames(relation.Fields, relation.RequiredFields, "relation required field"); err != nil {
			return err
		}
		cfg.Relations[name] = relation
	}

	return engine.RegisterCollection(cfg.Path, func(ctx context.Context, req fddp.ResourceRequest) (fddp.CollectionResult, error) {
		fields, err := collectionFields(cfg.Fields, req.Selection.Fields)
		if err != nil {
			return fddp.CollectionResult{}, err
		}
		if err := rejectNestedExpand(req.Selection.Expand); err != nil {
			return fddp.CollectionResult{}, err
		}
		queryFields, err := collectionQueryFields(fields, cfg.Fields, cfg.Relations, req.Selection.Expand)
		if err != nil {
			return fddp.CollectionResult{}, err
		}

		db := cfg.DB.WithContext(ctx).Model(new(T))
		if cfg.Scope != nil {
			db = cfg.Scope(db, req)
		}

		db, err = applyFilters(db, cfg.Fields, req.Collection.Filter)
		if err != nil {
			return fddp.CollectionResult{}, err
		}
		totalCount, err := countCollection(db, cfg.IncludeTotalCount)
		if err != nil {
			return fddp.CollectionResult{}, err
		}
		db, err = applyCursor(db, cfg.Fields, cfg.CursorField, cfg.CursorDesc, req.Collection.After)
		if err != nil {
			return fddp.CollectionResult{}, err
		}
		if err := validateCursorOrder(cfg.CursorField, cfg.CursorDesc, req.Collection.After, req.Collection.OrderBy); err != nil {
			return fddp.CollectionResult{}, err
		}
		db, err = applyOrder(db, cfg.Fields, req.Collection.OrderBy)
		if err != nil {
			return fddp.CollectionResult{}, err
		}
		db = applyDefaultCursorOrder(db, cfg.Fields, cfg.CursorField, cfg.CursorDesc, req.Collection.OrderBy)
		db, err = applyPreloads(db, cfg.Relations, req.Selection.Expand)
		if err != nil {
			return fddp.CollectionResult{}, err
		}

		limit := req.Collection.First
		if limit <= 0 {
			limit = cfg.DefaultLimit
		}
		if limit <= 0 {
			limit = 50
		}
		queryLimit := limit + 1
		db = db.Select(uniqueStrings(mappingColumns(queryFields))).Limit(queryLimit)

		var rows []T
		if err := db.Find(&rows).Error; err != nil {
			return fddp.CollectionResult{}, err
		}
		hasNextPage := len(rows) > limit
		if hasNextPage {
			rows = rows[:limit]
		}

		items := make([]any, 0, len(rows))
		for _, row := range rows {
			item, err := projectRow(row, fields, cfg.Relations, req.Selection.Expand)
			if err != nil {
				return fddp.CollectionResult{}, err
			}
			items = append(items, item)
		}

		return fddp.CollectionResult{
			Items:      items,
			PageInfo:   pageInfo(rows, cfg.Fields, cfg.CursorField, hasNextPage, req.Collection.After),
			TotalCount: totalCount,
		}, nil
	}, append(cfg.Options, fddp.ResourceFields(contractResourceFields(cfg.Fields)...), fddp.ResourceRelations(contractResourceRelations(cfg.Relations)...))...)
}

func contractResourceFields(fields map[string]FieldMapping) []fddp.ContractResourceField {
	out := make([]fddp.ContractResourceField, 0, len(fields))
	for name, mapping := range fields {
		fieldType := mapping.Type
		if fieldType == "" {
			fieldType = "unknown"
		}
		out = append(out, fddp.ContractResourceField{
			Field:      name,
			Type:       fieldType,
			Nullable:   mapping.Nullable,
			Filterable: true,
			Orderable:  true,
		})
	}
	sortContractResourceFields(out)
	return out
}

func contractResourceRelations(relations map[string]RelationMapping) []fddp.ContractResourceRelation {
	out := make([]fddp.ContractResourceRelation, 0, len(relations))
	for name, relation := range relations {
		out = append(out, fddp.ContractResourceRelation{
			Name:   name,
			Fields: contractResourceFields(relation.Fields),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortContractResourceFields(fields []fddp.ContractResourceField) {
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Field < fields[j].Field
	})
}

func collectionFields(mappings map[string]FieldMapping, requested []string) (map[string]FieldMapping, error) {
	if len(requested) == 0 {
		return mappings, nil
	}
	out := make(map[string]FieldMapping, len(requested))
	for _, field := range requested {
		mapping, ok := mappings[field]
		if !ok {
			return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("field %q is not mapped", field), "add the field to the collection mapping or remove it from selection.fields")
		}
		out[field] = mapping
	}
	return out, nil
}

func collectionQueryFields(selected map[string]FieldMapping, all map[string]FieldMapping, relations map[string]RelationMapping, expand map[string]fddp.Selection) (map[string]FieldMapping, error) {
	out := make(map[string]FieldMapping, len(selected))
	for name, mapping := range selected {
		out[name] = mapping
	}
	for name := range expand {
		relation, ok := relations[name]
		if !ok {
			return nil, adapterError(ErrorRelationNotMapped, fmt.Sprintf("relation %q is not mapped", name), "add an explicit Relation mapping before exposing it to clients")
		}
		for _, field := range relation.ParentFields {
			mapping, ok := all[field]
			if !ok {
				return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("relation parent field %q is not mapped", field), "map parent key fields needed by GORM preloads")
			}
			out[field] = mapping
		}
	}
	return out, nil
}

func applyFilters(db *gorm.DB, mappings map[string]FieldMapping, filters map[string]any) (*gorm.DB, error) {
	for field, value := range filters {
		mapping, ok := mappings[field]
		if !ok {
			return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("filter field %q is not mapped", field), "filters can only use explicitly mapped fields")
		}
		next, err := applyFilterValue(db, mapping.Column, value)
		if err != nil {
			return nil, wrapAdapterError(ErrorUnsupportedFilter, fmt.Sprintf("filter field %q is invalid", field), "use supported operators: eq, ne, gt, gte, lt, lte, in, notIn, like, contains, range, between, isNull", err)
		}
		db = next
	}
	return db, nil
}

func applyFilterValue(db *gorm.DB, column string, value any) (*gorm.DB, error) {
	if value == nil {
		return db.Where(column + " IS NULL"), nil
	}
	if operators, ok := value.(map[string]any); ok {
		for op, raw := range operators {
			next, err := applyFilterOperator(db, column, strings.ToLower(op), raw)
			if err != nil {
				return nil, err
			}
			db = next
		}
		return db, nil
	}
	return db.Where(column+" = ?", value), nil
}

func applyFilterOperator(db *gorm.DB, column string, op string, value any) (*gorm.DB, error) {
	switch op {
	case "eq":
		if value == nil {
			return db.Where(column + " IS NULL"), nil
		}
		return db.Where(column+" = ?", value), nil
	case "ne", "neq":
		if value == nil {
			return db.Where(column + " IS NOT NULL"), nil
		}
		return db.Where(column+" <> ?", value), nil
	case "gt":
		return db.Where(column+" > ?", value), nil
	case "gte":
		return db.Where(column+" >= ?", value), nil
	case "lt":
		return db.Where(column+" < ?", value), nil
	case "lte":
		return db.Where(column+" <= ?", value), nil
	case "in":
		values, ok := toSlice(value)
		if !ok || len(values) == 0 {
			return nil, errors.New("needs a non-empty array")
		}
		return db.Where(column+" IN ?", values), nil
	case "notin", "not_in":
		values, ok := toSlice(value)
		if !ok || len(values) == 0 {
			return nil, errors.New("needs a non-empty array")
		}
		return db.Where(column+" NOT IN ?", values), nil
	case "like":
		return db.Where(column+" LIKE ?", value), nil
	case "contains":
		return db.Where(column+" LIKE ? ESCAPE '\\'", "%"+escapeLike(fmt.Sprint(value))+"%"), nil
	case "range":
		return applyRangeFilter(db, column, value)
	case "between":
		values, ok := toSlice(value)
		if !ok || len(values) != 2 {
			return nil, errors.New("needs a two-item array")
		}
		return db.Where(column+" BETWEEN ? AND ?", values[0], values[1]), nil
	case "isnull", "is_null":
		isNull, ok := value.(bool)
		if !ok {
			return nil, errors.New("needs a boolean")
		}
		if isNull {
			return db.Where(column + " IS NULL"), nil
		}
		return db.Where(column + " IS NOT NULL"), nil
	default:
		return nil, adapterError(ErrorUnsupportedFilter, fmt.Sprintf("uses unsupported operator %q", op), "do not pass raw SQL; use a supported filter operator")
	}
}

func applyRangeFilter(db *gorm.DB, column string, value any) (*gorm.DB, error) {
	bounds, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("range needs an object")
	}
	if from, ok := firstPresent(bounds, "from", "min", "start"); ok {
		db = db.Where(column+" >= ?", from)
	}
	if to, ok := firstPresent(bounds, "to", "max", "end"); ok {
		db = db.Where(column+" <= ?", to)
	}
	return db, nil
}

func applyCursor(db *gorm.DB, mappings map[string]FieldMapping, cursorField string, desc bool, after *string) (*gorm.DB, error) {
	if after == nil || *after == "" {
		return db, nil
	}
	if cursorField == "" {
		return nil, adapterError(ErrorInvalidCursor, "cursor field is required when after is used", "configure Cursor or DescCursor on the collection")
	}
	mapping, ok := mappings[cursorField]
	if !ok {
		return nil, adapterError(ErrorInvalidCursor, fmt.Sprintf("cursor field %q is not mapped", cursorField), "cursor fields must also be mapped fields")
	}
	operator := ">"
	if desc {
		operator = "<"
	}
	return db.Where(mapping.Column+" "+operator+" ?", *after), nil
}

func applyOrder(db *gorm.DB, mappings map[string]FieldMapping, order []fddp.OrderBy) (*gorm.DB, error) {
	for _, item := range order {
		mapping, ok := mappings[item.Field]
		if !ok {
			return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("order field %q is not mapped", item.Field), "orderBy can only use explicitly mapped fields")
		}
		direction := strings.ToLower(item.Direction)
		if direction == "" {
			direction = "asc"
		}
		if direction != "asc" && direction != "desc" {
			return nil, adapterError(ErrorUnsafeIdentifier, fmt.Sprintf("unsafe order direction %q", item.Direction), "order direction must be asc or desc")
		}
		db = db.Order(mapping.Column + " " + direction)
	}
	return db, nil
}

func validateCursorOrder(cursorField string, cursorDesc bool, after *string, order []fddp.OrderBy) error {
	if after == nil || *after == "" || cursorField == "" || len(order) == 0 {
		return nil
	}
	first := order[0]
	if first.Field != cursorField {
		return adapterError(ErrorInvalidCursor, fmt.Sprintf("cursor field %q must be the first order field", cursorField), "make orderBy start with the configured cursor field")
	}
	direction := strings.ToLower(first.Direction)
	if direction == "" {
		direction = "asc"
	}
	expected := "asc"
	if cursorDesc {
		expected = "desc"
	}
	if direction != expected {
		return adapterError(ErrorInvalidCursor, fmt.Sprintf("cursor field %q must use %s order", cursorField, expected), "match order direction with Cursor or DescCursor")
	}
	return nil
}

func applyDefaultCursorOrder(db *gorm.DB, mappings map[string]FieldMapping, cursorField string, desc bool, order []fddp.OrderBy) *gorm.DB {
	if cursorField == "" || len(order) > 0 {
		return db
	}
	mapping, ok := mappings[cursorField]
	if !ok {
		return db
	}
	direction := "asc"
	if desc {
		direction = "desc"
	}
	return db.Order(mapping.Column + " " + direction)
}

func applyPreloads(db *gorm.DB, relations map[string]RelationMapping, expand map[string]fddp.Selection) (*gorm.DB, error) {
	for name, selection := range expand {
		relation, ok := relations[name]
		if !ok {
			return nil, adapterError(ErrorRelationNotMapped, fmt.Sprintf("relation %q is not mapped", name), "add an explicit Relation mapping before exposing it to clients")
		}
		fields, err := collectionFields(relation.Fields, selection.Fields)
		if err != nil {
			return nil, err
		}
		queryFields, err := relationQueryFields(fields, relation)
		if err != nil {
			return nil, err
		}
		columns := uniqueStrings(mappingColumns(queryFields))
		db = db.Preload(relation.Name, func(tx *gorm.DB) *gorm.DB {
			return tx.Select(columns)
		})
	}
	return db, nil
}

func relationQueryFields(selected map[string]FieldMapping, relation RelationMapping) (map[string]FieldMapping, error) {
	out := make(map[string]FieldMapping, len(selected)+len(relation.RequiredFields))
	for name, mapping := range selected {
		out[name] = mapping
	}
	for _, field := range relation.RequiredFields {
		mapping, ok := relation.Fields[field]
		if !ok {
			return nil, adapterError(ErrorFieldNotMapped, fmt.Sprintf("relation required field %q is not mapped", field), "required preload fields must be mapped on the relation")
		}
		out[field] = mapping
	}
	return out, nil
}

func projectRow(row any, fields map[string]FieldMapping, relations map[string]RelationMapping, expand map[string]fddp.Selection) (map[string]any, error) {
	item := make(map[string]any, len(fields)+len(expand))
	for name, mapping := range fields {
		value, err := structFieldValue(row, mapping.StructField)
		if err != nil {
			return nil, wrapAdapterError(ErrorProjectionFailed, fmt.Sprintf("struct field %q cannot be projected", mapping.StructField), "check the StructField name in the collection mapping", err)
		}
		item[name] = value
	}
	for name, selection := range expand {
		relation, ok := relations[name]
		if !ok {
			return nil, adapterError(ErrorRelationNotMapped, fmt.Sprintf("relation %q is not mapped", name), "add an explicit Relation mapping before exposing it to clients")
		}
		value, err := structFieldValue(row, relation.Name)
		if err != nil {
			return nil, wrapAdapterError(ErrorProjectionFailed, fmt.Sprintf("relation %q cannot be projected", relation.Name), "check the GORM relation field name", err)
		}
		projected, err := projectAnyRelation(value, relation.Fields, selection.Fields)
		if err != nil {
			return nil, err
		}
		item[name] = projected
	}
	return item, nil
}

func projectAnyRelation(value any, fields map[string]FieldMapping, requested []string) (any, error) {
	selected, err := collectionFields(fields, requested)
	if err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, nil
	}
	if rv.Kind() == reflect.Slice {
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item, err := projectStructValue(rv.Index(i).Interface(), selected)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	}
	return projectStructValue(value, selected)
}

func projectStructValue(value any, fields map[string]FieldMapping) (map[string]any, error) {
	out := make(map[string]any, len(fields))
	for name, mapping := range fields {
		fieldValue, err := structFieldValue(value, mapping.StructField)
		if err != nil {
			return nil, wrapAdapterError(ErrorProjectionFailed, fmt.Sprintf("struct field %q cannot be projected", mapping.StructField), "check relation StructField mappings", err)
		}
		out[name] = fieldValue
	}
	return out, nil
}

func mappingColumns(fields map[string]FieldMapping) []string {
	columns := make([]string, 0, len(fields))
	for _, mapping := range fields {
		columns = append(columns, mapping.Column)
	}
	return columns
}

func validateFieldNames(mappings map[string]FieldMapping, fields []string, label string) error {
	for _, field := range fields {
		if _, ok := mappings[field]; !ok {
			return adapterError(ErrorFieldNotMapped, fmt.Sprintf("%s %q is not mapped", label, field), "map every configured helper field")
		}
	}
	return nil
}

func rejectNestedExpand(expand map[string]fddp.Selection) error {
	for name, selection := range expand {
		if len(selection.Expand) > 0 {
			return adapterError(ErrorUnsupportedExpand, fmt.Sprintf("nested expand for relation %q is not supported", name), "register a dedicated resource for deeper graphs")
		}
	}
	return nil
}

func countCollection(db *gorm.DB, enabled bool) (*int, error) {
	if !enabled {
		return nil, nil
	}
	var count int64
	if err := db.Session(&gorm.Session{}).Count(&count).Error; err != nil {
		return nil, err
	}
	value := int(count)
	return &value, nil
}

func pageInfo[T any](rows []T, mappings map[string]FieldMapping, cursorField string, hasNext bool, after *string) *fddp.PageInfo {
	info := &fddp.PageInfo{HasNextPage: hasNext}
	if after != nil && *after != "" {
		info.HasPreviousPage = true
	}
	if cursorField == "" || len(rows) == 0 {
		return info
	}
	mapping, ok := mappings[cursorField]
	if !ok {
		return info
	}
	start, err := structFieldValue(rows[0], mapping.StructField)
	if err == nil && start != nil {
		info.StartCursor = fmt.Sprint(start)
	}
	end, err := structFieldValue(rows[len(rows)-1], mapping.StructField)
	if err == nil && end != nil {
		info.EndCursor = fmt.Sprint(end)
	}
	return info
}

func toSlice(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	if values, ok := value.([]any); ok {
		return values, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		values = append(values, rv.Index(i).Interface())
	}
	return values, true
}

func firstPresent(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}
