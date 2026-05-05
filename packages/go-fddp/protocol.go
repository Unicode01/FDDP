package fddp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrInvalidQuery = errors.New("fddp: invalid query")

type CommandEnvelope struct {
	Command         string          `json:"command"`
	Input           json.RawMessage `json:"input,omitempty"`
	IdempotencyKey  string          `json:"idempotencyKey,omitempty"`
	ExpectedVersion *int64          `json:"expectedVersion,omitempty"`
	Trace           bool            `json:"trace,omitempty"`
}

type ParseQueryLimits struct {
	MaxDepth int
	MaxNodes int
}

func ParseQueryEnvelope(body []byte) ([]string, bool, error) {
	plan, err := ParseQueryPlanEnvelope(body)
	if err != nil {
		return nil, false, err
	}
	return plan.Fields, plan.Trace, nil
}

func ParseQueryPlanEnvelope(body []byte) (QueryPlan, error) {
	return ParseQueryPlanEnvelopeWithLimits(body, ParseQueryLimits{})
}

func ParseQueryPlanEnvelopeWithLimits(body []byte, limits ParseQueryLimits) (QueryPlan, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return QueryPlan{}, err
	}

	trace := false
	queryRaw := body
	hasQueryEnvelope := false
	if raw, ok := root["trace"]; ok {
		_ = json.Unmarshal(raw, &trace)
	}
	if raw, ok := root["query"]; ok {
		queryRaw = raw
		hasQueryEnvelope = true
	}

	decoder := json.NewDecoder(bytes.NewReader(queryRaw))
	decoder.UseNumber()

	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return QueryPlan{}, err
	}
	if !hasQueryEnvelope {
		if object, ok := tree.(map[string]any); ok {
			delete(object, "trace")
		}
	}

	plan := QueryPlan{Trace: trace}
	state := queryParseState{limits: limits}
	if err := flattenQueryValue("", tree, &plan, &state, 0); err != nil {
		return QueryPlan{}, err
	}

	plan.Fields = uniqueSorted(plan.Fields)
	sort.SliceStable(plan.Resources, func(i, j int) bool {
		return plan.Resources[i].Path < plan.Resources[j].Path
	})
	return plan, nil
}

func ParseCommandEnvelope(body []byte) (CommandEnvelope, error) {
	var envelope CommandEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return CommandEnvelope{}, err
	}
	if strings.TrimSpace(envelope.Command) == "" {
		return CommandEnvelope{}, errors.New("fddp: command is required")
	}
	return envelope, nil
}

type queryParseState struct {
	limits ParseQueryLimits
	nodes  int
}

func (state *queryParseState) visit(depth int) error {
	state.nodes++
	if state.limits.MaxNodes >= 0 && state.limits.MaxNodes > 0 && state.nodes > state.limits.MaxNodes {
		return fmt.Errorf("%w: query node count %d exceeds limit %d", ErrQueryLimitExceeded, state.nodes, state.limits.MaxNodes)
	}
	if state.limits.MaxDepth >= 0 && state.limits.MaxDepth > 0 && depth > state.limits.MaxDepth {
		return fmt.Errorf("%w: query depth %d exceeds limit %d", ErrQueryLimitExceeded, depth, state.limits.MaxDepth)
	}
	return nil
}

func flattenQueryValue(prefix string, value any, plan *QueryPlan, state *queryParseState, depth int) error {
	if state == nil {
		state = &queryParseState{}
	}
	if err := state.visit(depth); err != nil {
		return err
	}
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed["$type"]; ok {
			resource, err := parseResourceQuery(prefix, typed, state, depth)
			if err != nil {
				return err
			}
			plan.Resources = append(plan.Resources, resource)
			return nil
		}

		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			if err := flattenQueryValue(next, typed[key], plan, state, depth+1); err != nil {
				return err
			}
		}
	case []any:
		if prefix == "" {
			return fmt.Errorf("%w: leaf array needs a prefix", ErrInvalidQuery)
		}
		for _, item := range typed {
			leaf, ok := item.(string)
			if !ok || leaf == "" {
				return fmt.Errorf("%w: leaf fields must be strings", ErrInvalidQuery)
			}
			plan.Fields = append(plan.Fields, prefix+"."+leaf)
		}
	case string:
		if prefix == "" {
			plan.Fields = append(plan.Fields, typed)
		} else {
			plan.Fields = append(plan.Fields, prefix+"."+typed)
		}
	default:
		return fmt.Errorf("%w: unsupported node type %T", ErrInvalidQuery, value)
	}
	return nil
}

func parseResourceQuery(path string, value map[string]any, state *queryParseState, depth int) (ResourceQuery, error) {
	if path == "" {
		return ResourceQuery{}, fmt.Errorf("%w: resource descriptor needs a prefix", ErrInvalidQuery)
	}

	marker, ok := value["$type"].(string)
	if !ok || marker == "" {
		return ResourceQuery{}, fmt.Errorf("%w: resource descriptor needs $type", ErrInvalidQuery)
	}

	query := ResourceQuery{Path: path, Type: ResourceQueryType(marker)}
	if raw, ok := value["selection"]; ok {
		selection, err := parseSelection(raw, state, depth+1)
		if err != nil {
			return ResourceQuery{}, err
		}
		query.Selection = selection
	}

	switch query.Type {
	case ResourceQueryCollection:
		if raw, ok := value["args"]; ok {
			args, err := decodeJSONValue[CollectionArgs](raw)
			if err != nil {
				return ResourceQuery{}, fmt.Errorf("%w: invalid collection args: %v", ErrInvalidQuery, err)
			}
			query.Collection = args
		}
	case ResourceQueryAggregate:
		name, _ := value["name"].(string)
		if name == "" {
			return ResourceQuery{}, fmt.Errorf("%w: aggregate descriptor needs name", ErrInvalidQuery)
		}
		query.Name = name
		if args, ok := value["args"].(map[string]any); ok {
			query.Args = args
		}
	default:
		return ResourceQuery{}, fmt.Errorf("%w: unsupported resource type %q", ErrInvalidQuery, marker)
	}

	return query, nil
}

func parseSelection(value any, state *queryParseState, depth int) (Selection, error) {
	if state == nil {
		state = &queryParseState{}
	}
	if err := state.visit(depth); err != nil {
		return Selection{}, err
	}
	switch typed := value.(type) {
	case nil:
		return Selection{}, nil
	case []any:
		fields, err := stringSlice(typed)
		if err != nil {
			return Selection{}, err
		}
		return Selection{Fields: fields}, nil
	case map[string]any:
		var selection Selection
		if rawFields, ok := typed["fields"]; ok {
			fields, ok := rawFields.([]any)
			if !ok {
				return Selection{}, fmt.Errorf("%w: selection fields must be strings", ErrInvalidQuery)
			}
			parsed, err := stringSlice(fields)
			if err != nil {
				return Selection{}, err
			}
			selection.Fields = parsed
		}
		if rawExpand, ok := typed["expand"]; ok {
			expand, ok := rawExpand.(map[string]any)
			if !ok {
				return Selection{}, fmt.Errorf("%w: selection expand must be an object", ErrInvalidQuery)
			}
			selection.Expand = make(map[string]Selection, len(expand))
			for name, child := range expand {
				parsed, err := parseSelection(child, state, depth+1)
				if err != nil {
					return Selection{}, err
				}
				selection.Expand[name] = parsed
			}
		}
		return selection, nil
	default:
		return Selection{}, fmt.Errorf("%w: unsupported selection type %T", ErrInvalidQuery, value)
	}
}

func stringSlice(values []any) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok || item == "" {
			return nil, fmt.Errorf("%w: selection fields must be strings", ErrInvalidQuery)
		}
		out = append(out, item)
	}
	return out, nil
}

func decodeJSONValue[T any](value any) (T, error) {
	var out T
	body, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(body, &out)
	return out, err
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func setNested(data map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
}
