package fddp

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

var (
	ErrMissingPath             = errors.New("fddp: field path is required")
	ErrMissingResolver         = errors.New("fddp: field resolver is required")
	ErrMissingGroupPath        = errors.New("fddp: field group path is required")
	ErrMissingGroupResolver    = errors.New("fddp: field group resolver is required")
	ErrMissingCommand          = errors.New("fddp: command name is required")
	ErrMissingExecutor         = errors.New("fddp: command executor is required")
	ErrMissingResourcePath     = errors.New("fddp: resource path is required")
	ErrMissingResourceResolver = errors.New("fddp: resource resolver is required")
)

type Registry struct {
	mu        sync.RWMutex
	fields    map[string]FieldDefinition
	groups    map[string]FieldGroupDefinition
	resources map[string]ResourceDefinition
	commands  map[string]CommandDefinition
}

func NewRegistry() *Registry {
	return &Registry{
		fields:    make(map[string]FieldDefinition),
		groups:    make(map[string]FieldGroupDefinition),
		resources: make(map[string]ResourceDefinition),
		commands:  make(map[string]CommandDefinition),
	}
}

func (r *Registry) RegisterField(field FieldDefinition) error {
	if field.Path == "" {
		return ErrMissingPath
	}
	if field.Resolve == nil {
		return ErrMissingResolver
	}
	if field.Cache.Scope == "" {
		field.Cache.Scope = CacheScopeNone
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.fields[field.Path] = field
	return nil
}

func (r *Registry) RegisterFieldGroup(group FieldGroupDefinition) error {
	if group.Path == "" {
		return ErrMissingGroupPath
	}
	if group.Resolve == nil {
		return ErrMissingGroupResolver
	}
	if group.Cache.Scope == "" {
		group.Cache.Scope = CacheScopeNone
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[group.Path] = group
	for _, field := range group.Fields {
		if field.Path == "" {
			continue
		}
		if field.Owner == "" {
			field.Owner = group.Owner
		}
		if field.Permission == "" {
			field.Permission = group.Permission
		}
		if field.Cache.Scope == "" {
			field.Cache = group.Cache
		}
		if group.Deprecated {
			field.Deprecated = true
		}
		r.fields[field.Path] = field
	}
	return nil
}

func (r *Registry) RegisterResource(resource ResourceDefinition) error {
	if resource.Path == "" {
		return ErrMissingResourcePath
	}
	if resource.Resolve == nil {
		return ErrMissingResourceResolver
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[resource.Path] = resource
	return nil
}

func (r *Registry) FieldGroup(path string) (FieldGroupDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	group, ok := r.groups[path]
	return group, ok
}

func (r *Registry) FieldGroupsForPaths(paths []string) []FieldGroupDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{})
	groups := make([]FieldGroupDefinition, 0)
	for _, path := range paths {
		for prefix, group := range r.groups {
			if path == prefix || strings.HasPrefix(path, prefix+".") {
				if _, ok := seen[prefix]; ok {
					continue
				}
				seen[prefix] = struct{}{}
				groups = append(groups, group)
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Path < groups[j].Path
	})
	return groups
}

func (r *Registry) RegisterCommand(command CommandDefinition) error {
	if command.Name == "" {
		return ErrMissingCommand
	}
	if command.Execute == nil {
		return ErrMissingExecutor
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[command.Name] = command
	return nil
}

func (r *Registry) Field(path string) (FieldDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	field, ok := r.fields[path]
	return field, ok
}

func (r *Registry) Resource(path string) (ResourceDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	resource, ok := r.resources[path]
	return resource, ok
}

func (r *Registry) Command(name string) (CommandDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	command, ok := r.commands[name]
	return command, ok
}

func (r *Registry) Contract(version string) ContractSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fields := make([]ContractField, 0, len(r.fields))
	for _, field := range r.fields {
		fieldType := field.Type
		if fieldType == "" {
			fieldType = "unknown"
		}
		fields = append(fields, ContractField{
			Field:       field.Path,
			Type:        fieldType,
			Owner:       field.Owner,
			Permission:  field.Permission,
			Nullable:    field.Nullable,
			Deprecated:  field.Deprecated,
			Sensitivity: field.Sensitivity,
			CacheScope:  field.Cache.Scope,
		})
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Field < fields[j].Field
	})

	resources := make([]ContractResource, 0, len(r.resources))
	for _, resource := range r.resources {
		resources = append(resources, ContractResource{
			Path:        resource.Path,
			Owner:       resource.Owner,
			Permission:  resource.Permission,
			Types:       []string{string(ResourceQueryCollection), string(ResourceQueryAggregate)},
			MaxPageSize: resource.MaxPageSize,
			Deprecated:  resource.Deprecated,
			Fields:      append([]ContractResourceField(nil), resource.Fields...),
			Relations:   cloneContractResourceRelations(resource.Relations),
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Path < resources[j].Path
	})

	commands := make([]ContractCommand, 0, len(r.commands))
	for _, command := range r.commands {
		commands = append(commands, ContractCommand{
			Name:                command.Name,
			Owner:               command.Owner,
			Permission:          command.Permission,
			IdempotencyRequired: command.IdempotencyRequired,
			AuditLevel:          command.AuditLevel,
			Input:               append([]ContractInputField(nil), command.Input...),
		})
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return ContractSchema{
		ProtocolVersion: "v9",
		ContractVersion: version,
		Fields:          fields,
		Resources:       resources,
		Commands:        commands,
	}
}

func cloneContractResourceRelations(relations []ContractResourceRelation) []ContractResourceRelation {
	if len(relations) == 0 {
		return nil
	}
	out := make([]ContractResourceRelation, 0, len(relations))
	for _, relation := range relations {
		relation.Fields = append([]ContractResourceField(nil), relation.Fields...)
		out = append(out, relation)
	}
	return out
}
