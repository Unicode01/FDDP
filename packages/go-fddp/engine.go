package fddp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type IdentityResolver func(r *http.Request) IdentityContext

type Engine struct {
	registry         *Registry
	policy           PolicyChecker
	cache            Cache
	idempotency      IdempotencyStore
	identityResolver IdentityResolver
	queryLimits      QueryLimits
	commandLimits    CommandLimits
	now              func() time.Time
	newTraceID       func() string
	contractVersion  string
	batchLoaders     map[string]BatchLoadFunc
	batchMu          sync.RWMutex
}

type Option func(*Engine)

func NewEngine(options ...Option) *Engine {
	engine := &Engine{
		registry:         NewRegistry(),
		policy:           DefaultPolicy{},
		identityResolver: HeaderIdentityResolver,
		queryLimits:      DefaultQueryLimits(),
		commandLimits:    DefaultCommandLimits(),
		now:              time.Now,
		newTraceID:       NewTraceID,
		contractVersion:  "dev",
		batchLoaders:     make(map[string]BatchLoadFunc),
	}

	for _, option := range options {
		option(engine)
	}

	return engine
}

func WithRegistry(registry *Registry) Option {
	return func(engine *Engine) {
		if registry != nil {
			engine.registry = registry
		}
	}
}

func WithPolicy(policy PolicyChecker) Option {
	return func(engine *Engine) {
		if policy != nil {
			engine.policy = policy
		}
	}
}

func WithCache(cache Cache) Option {
	return func(engine *Engine) {
		engine.cache = cache
	}
}

func WithIdempotencyStore(store IdempotencyStore) Option {
	return func(engine *Engine) {
		engine.idempotency = store
	}
}

func WithIdentityResolver(resolver IdentityResolver) Option {
	return func(engine *Engine) {
		if resolver != nil {
			engine.identityResolver = resolver
		}
	}
}

func WithTraceIDFunc(fn func() string) Option {
	return func(engine *Engine) {
		if fn != nil {
			engine.newTraceID = fn
		}
	}
}

func WithContractVersion(version string) Option {
	return func(engine *Engine) {
		if strings.TrimSpace(version) != "" {
			engine.contractVersion = version
		}
	}
}

func WithQueryLimits(limits QueryLimits) Option {
	return func(engine *Engine) {
		engine.queryLimits = limits
	}
}

func WithoutQueryLimits() Option {
	return WithQueryLimits(NoQueryLimits())
}

func WithCommandLimits(limits CommandLimits) Option {
	return func(engine *Engine) {
		engine.commandLimits = limits
	}
}

func WithoutCommandLimits() Option {
	return WithCommandLimits(NoCommandLimits())
}

func (e *Engine) RegisterField(field FieldDefinition) error {
	return e.registry.RegisterField(field)
}

func (e *Engine) RegisterResource(resource ResourceDefinition) error {
	return e.registry.RegisterResource(resource)
}

func (e *Engine) RegisterCommand(command CommandDefinition) error {
	return e.registry.RegisterCommand(command)
}

func (e *Engine) RegisterBatchLoader(group string, load BatchLoadFunc) {
	if group == "" || load == nil {
		return
	}
	e.batchMu.Lock()
	defer e.batchMu.Unlock()
	e.batchLoaders[group] = load
}

func (e *Engine) Contract() ContractSchema {
	return e.registry.Contract(e.contractVersion)
}

func (e *Engine) ExecuteQueryBody(ctx context.Context, req QueryEndpointRequest) QueryEndpointResult {
	if err := e.validateQueryBodyLimits(req.Body); err != nil {
		return QueryEndpointResult{
			Status: http.StatusOK,
			Response: queryLimitExceededResponse(
				e.newTraceID(),
				e.contractVersion,
				req.Identity,
				nil,
				QueryCost{},
				e.now(),
				e.now,
				err,
			),
		}
	}

	plan, err := ParseQueryPlanEnvelopeWithLimits(req.Body, e.parseQueryLimits())
	if err != nil {
		if errors.Is(err, ErrQueryLimitExceeded) {
			return QueryEndpointResult{
				Status: http.StatusOK,
				Response: queryLimitExceededResponse(
					e.newTraceID(),
					e.contractVersion,
					req.Identity,
					nil,
					QueryCost{},
					e.now(),
					e.now,
					err,
				),
			}
		}
		return QueryEndpointResult{
			Status: http.StatusBadRequest,
			Response: QueryResponse{
				Data:   map[string]any{},
				Errors: []DDPError{{Code: "INVALID_QUERY", Severity: "validation", Reason: err.Error(), SafeMessage: "Query is invalid."}},
				Meta: QueryMeta{
					Partial: true,
					Cache:   QueryCacheMeta{Hit: []string{}, Miss: []string{}, Stale: []string{}, Unsafe: []string{}},
				},
			},
		}
	}

	return QueryEndpointResult{
		Status:   http.StatusOK,
		Response: e.ExecuteQueryPlan(ctx, plan, req.Identity, req.HTTPRequest),
	}
}

func (e *Engine) validateQueryBodyLimits(body []byte) error {
	if e.queryLimits.disabled() {
		return nil
	}
	limits := e.queryLimits.withDefaults()
	if limitExceeded(len(body), limits.MaxBodyBytes) {
		return fmt.Errorf("query body size %d exceeds limit %d", len(body), limits.MaxBodyBytes)
	}
	return nil
}

func (e *Engine) parseQueryLimits() ParseQueryLimits {
	if e.queryLimits.disabled() {
		return ParseQueryLimits{MaxDepth: -1, MaxNodes: -1}
	}
	limits := e.queryLimits.withDefaults()
	return ParseQueryLimits{MaxDepth: limits.MaxQueryDepth, MaxNodes: limits.MaxQueryNodes}
}

func (e *Engine) ExecuteCommandBody(ctx context.Context, req CommandEndpointRequest) CommandEndpointResult {
	if err := validateCommandBodyLimits(req.Body, e.commandLimits); err != nil {
		return CommandEndpointResult{
			Status:   http.StatusOK,
			Response: commandLimitExceededResponse(e.newTraceID(), "", err),
		}
	}

	envelope, err := ParseCommandEnvelope(req.Body)
	if err != nil {
		return CommandEndpointResult{
			Status: http.StatusBadRequest,
			Response: CommandResponse{
				Data:   CommandData{Status: "failed"},
				Errors: []DDPError{{Code: "INVALID_COMMAND", Severity: "validation", Reason: err.Error(), SafeMessage: "Command is invalid."}},
				Meta:   CommandMeta{Partial: true},
			},
		}
	}
	if err := validateCommandEnvelopeLimits(envelope, e.commandLimits); err != nil {
		if errors.Is(err, ErrLimitExceeded) {
			return CommandEndpointResult{
				Status:   http.StatusOK,
				Response: commandLimitExceededResponse(e.newTraceID(), envelope.Command, err),
			}
		}
		return CommandEndpointResult{
			Status: http.StatusBadRequest,
			Response: CommandResponse{
				Data:   CommandData{Status: "failed"},
				Errors: []DDPError{{Path: envelope.Command, Code: "INVALID_COMMAND", Command: envelope.Command, Severity: "validation", Reason: err.Error(), SafeMessage: "Command is invalid."}},
				Meta:   CommandMeta{Partial: true},
			},
		}
	}

	return CommandEndpointResult{
		Status:   http.StatusOK,
		Response: e.ExecuteCommand(ctx, envelope, req.Identity, req.HTTPRequest),
	}
}

func (e *Engine) ExecuteQuery(ctx context.Context, fields []string, identity IdentityContext, r *http.Request, traceEnabled bool) QueryResponse {
	return e.ExecuteQueryPlan(ctx, QueryPlan{Fields: fields, Trace: traceEnabled}, identity, r)
}

func (e *Engine) ExecuteQueryPlan(ctx context.Context, plan QueryPlan, identity IdentityContext, r *http.Request) QueryResponse {
	start := e.now()
	traceID := e.newTraceID()
	trace := &QueryTrace{
		RequestID: traceID,
		Fields:    append([]string(nil), plan.Fields...),
		Resources: resourcePaths(plan.Resources),
	}
	cost, err := validateQueryPlanLimits(plan, e.queryLimits)
	trace.Cost = cost.TotalCost
	if err != nil {
		trace.TotalTimeMS = e.now().Sub(start).Milliseconds()
		if !plan.Trace {
			trace = nil
		}
		return queryLimitExceededResponse(traceID, e.contractVersion, identity, trace, cost, start, e.now, err)
	}
	if timeout := e.queryLimitsTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	batcher := e.newRequestBatcher()
	data := make(map[string]any)
	var errs []DDPError
	groupedFields := make(map[string]struct{})

	for _, group := range e.registry.FieldGroupsForPaths(plan.Fields) {
		requested := groupRequestedFields(group, plan.Fields)
		if len(requested) == 0 {
			continue
		}

		allowed := make([]string, 0, len(requested))
		for _, path := range requested {
			field, ok := e.registry.Field(path)
			if !ok {
				continue
			}
			decision, err := e.policy.CanRead(ctx, identity, field)
			if err != nil {
				errs = append(errs, DDPError{
					Path:        path,
					Code:        "FIELD_POLICY_ERROR",
					Domain:      field.Owner,
					Severity:    "fatal",
					Field:       path,
					Reason:      err.Error(),
					Message:     "field policy failed",
					SafeMessage: "Field is not available.",
				})
				continue
			}
			if !decision.Allow {
				trace.Denied = append(trace.Denied, path)
				errs = append(errs, DDPError{
					Path:        path,
					Code:        "FIELD_DENIED",
					Domain:      field.Owner,
					Severity:    "denied",
					Field:       path,
					Reason:      decision.Reason,
					Message:     "field access denied",
					SafeMessage: "Field is not available.",
				})
				continue
			}
			allowed = append(allowed, path)
		}
		if len(allowed) == 0 {
			continue
		}

		values, err := group.Resolve(ctx, FieldGroupRequest{
			Identity:    identity,
			Path:        group.Path,
			Fields:      append([]string(nil), allowed...),
			HTTPRequest: r,
			Batcher:     batcher,
		})
		trace.Calls.FieldGroups++
		if err != nil {
			for _, path := range allowed {
				field, _ := e.registry.Field(path)
				code := "FIELD_GROUP_RESOLVE_FAILED"
				safeMessage := "Field is temporarily unavailable."
				retryable := true
				if contextError := ctx.Err(); contextError != nil {
					code = "QUERY_TIMEOUT"
					err = contextError
					safeMessage = "Query timed out."
					retryable = false
				}
				errs = append(errs, DDPError{
					Path:        path,
					Code:        code,
					Domain:      field.Owner,
					Severity:    "degraded",
					Field:       path,
					Reason:      err.Error(),
					Message:     "field group resolver failed",
					SafeMessage: safeMessage,
					Retryable:   retryable,
				})
			}
			continue
		}

		for _, path := range allowed {
			leaf := strings.TrimPrefix(path, group.Path+".")
			value, ok := values[leaf]
			if !ok {
				value, ok = values[path]
			}
			if !ok {
				if contextError := ctx.Err(); contextError != nil {
					errs = append(errs, queryTimeoutError(path, contextError))
					continue
				}
				errs = append(errs, DDPError{
					Path:        path,
					Code:        "FIELD_GROUP_VALUE_MISSING",
					Severity:    "degraded",
					Field:       path,
					Reason:      "missing_value",
					Message:     "field group resolver did not return a requested field",
					SafeMessage: "Field is temporarily unavailable.",
					Retryable:   true,
				})
				continue
			}
			groupedFields[path] = struct{}{}
			trace.Resolved = append(trace.Resolved, path)
			setNested(data, path, value)
		}
	}

	for _, path := range plan.Fields {
		if _, grouped := groupedFields[path]; grouped {
			continue
		}
		field, ok := e.registry.Field(path)
		if !ok {
			errs = append(errs, DDPError{
				Path:        path,
				Code:        "FIELD_NOT_REGISTERED",
				Severity:    "validation",
				Field:       path,
				Reason:      "not_registered",
				Message:     "field is not registered",
				SafeMessage: "Field is not available.",
			})
			continue
		}

		decision, err := e.policy.CanRead(ctx, identity, field)
		if err != nil {
			errs = append(errs, DDPError{
				Path:        path,
				Code:        "FIELD_POLICY_ERROR",
				Domain:      field.Owner,
				Severity:    "fatal",
				Field:       path,
				Reason:      err.Error(),
				Message:     "field policy failed",
				SafeMessage: "Field is not available.",
			})
			continue
		}
		if !decision.Allow {
			trace.Denied = append(trace.Denied, path)
			errs = append(errs, DDPError{
				Path:        path,
				Code:        "FIELD_DENIED",
				Domain:      field.Owner,
				Severity:    "denied",
				Field:       path,
				Reason:      decision.Reason,
				Message:     "field access denied",
				SafeMessage: "Field is not available.",
			})
			continue
		}

		if key, ok := e.cacheKey(field, identity); ok && e.cache != nil {
			if entry, hit := e.cache.Get(ctx, key); hit {
				trace.CacheHit = append(trace.CacheHit, path)
				setNested(data, path, entry.Value)
				continue
			}
			trace.CacheMiss = append(trace.CacheMiss, path)
		}

		value, err := field.Resolve(ctx, FieldRequest{Identity: identity, Path: path, HTTPRequest: r, Batcher: batcher})
		trace.Calls.Fields++
		if err != nil {
			code := "FIELD_RESOLVE_FAILED"
			safeMessage := "Field is temporarily unavailable."
			retryable := true
			if contextError := ctx.Err(); contextError != nil {
				code = "QUERY_TIMEOUT"
				err = contextError
				safeMessage = "Query timed out."
				retryable = false
			}
			errs = append(errs, DDPError{
				Path:        path,
				Code:        code,
				Domain:      field.Owner,
				Severity:    "degraded",
				Field:       path,
				Reason:      err.Error(),
				Message:     "field resolver failed",
				SafeMessage: safeMessage,
				Retryable:   retryable,
			})
			continue
		}

		trace.Resolved = append(trace.Resolved, path)
		setNested(data, path, value)

		if key, ok := e.cacheKey(field, identity); ok && e.cache != nil {
			entry := CacheEntry{Value: value, Fields: []string{path}}
			if field.Cache.TTL > 0 {
				entry.ExpiresAt = e.now().Add(field.Cache.TTL)
			}
			e.cache.Set(ctx, key, entry)
		}
	}

	for _, query := range plan.Resources {
		resource, ok := e.registry.Resource(query.Path)
		if !ok {
			errs = append(errs, DDPError{
				Path:        query.Path,
				Code:        "RESOURCE_NOT_REGISTERED",
				Severity:    "validation",
				Field:       query.Path,
				Reason:      "not_registered",
				Message:     "resource is not registered",
				SafeMessage: "Resource is not available.",
			})
			continue
		}

		decision, err := e.canReadResource(ctx, identity, resource)
		if err != nil {
			errs = append(errs, DDPError{
				Path:        query.Path,
				Code:        "RESOURCE_POLICY_ERROR",
				Domain:      resource.Owner,
				Severity:    "fatal",
				Field:       query.Path,
				Reason:      err.Error(),
				Message:     "resource policy failed",
				SafeMessage: "Resource is not available.",
			})
			continue
		}
		if !decision.Allow {
			trace.Denied = append(trace.Denied, query.Path)
			errs = append(errs, DDPError{
				Path:        query.Path,
				Code:        "RESOURCE_DENIED",
				Domain:      resource.Owner,
				Severity:    "denied",
				Field:       query.Path,
				Reason:      decision.Reason,
				Message:     "resource access denied",
				SafeMessage: "Resource is not available.",
			})
			continue
		}

		if query.Type == ResourceQueryCollection && resource.MaxPageSize > 0 && query.Collection.First > resource.MaxPageSize {
			query.Collection.First = resource.MaxPageSize
		}

		value, err := resource.Resolve(ctx, ResourceRequest{
			Identity:    identity,
			Path:        query.Path,
			Type:        query.Type,
			Name:        query.Name,
			Collection:  query.Collection,
			Args:        query.Args,
			Selection:   query.Selection,
			HTTPRequest: r,
			Batcher:     batcher,
		})
		trace.Calls.Resources++
		if err != nil {
			code := "RESOURCE_RESOLVE_FAILED"
			severity := "degraded"
			safeMessage := "Resource is temporarily unavailable."
			message := "resource resolver failed"
			retryable := true
			if contextError := ctx.Err(); contextError != nil {
				code = "QUERY_TIMEOUT"
				err = contextError
				safeMessage = "Query timed out."
				retryable = false
			} else if coded := codedError(err); coded != nil {
				code = coded.ErrorCode()
				severity = "validation"
				safeMessage = "Resource query is invalid."
				message = coded.ErrorHint()
				retryable = false
			}
			errs = append(errs, DDPError{
				Path:        query.Path,
				Code:        code,
				Domain:      resource.Owner,
				Severity:    severity,
				Field:       query.Path,
				Reason:      err.Error(),
				Message:     message,
				SafeMessage: safeMessage,
				Retryable:   retryable,
			})
			continue
		}

		trace.Resolved = append(trace.Resolved, query.Path)
		setNested(data, query.Path, value)
	}

	trace.TotalTimeMS = e.now().Sub(start).Milliseconds()
	mergeTraceCalls(&trace.Calls, batcher.TraceCalls())
	elapsedMS := trace.TotalTimeMS
	cacheHit := stringsOrEmpty(trace.CacheHit)
	cacheMiss := stringsOrEmpty(trace.CacheMiss)
	if !plan.Trace {
		trace = nil
	}

	return QueryResponse{
		Data:   data,
		Errors: errorsOrEmpty(errs),
		Meta: QueryMeta{
			RequestID: traceID,
			TraceID:   traceID,
			Partial:   len(errs) > 0,
			ElapsedMS: elapsedMS,
			Cache: QueryCacheMeta{
				Hit:    cacheHit,
				Miss:   cacheMiss,
				Stale:  []string{},
				Unsafe: []string{},
			},
			Cost:            &cost,
			PolicyVersion:   identity.PolicyVersion,
			ContractVersion: e.contractVersion,
			Trace:           trace,
		},
	}
}

func (e *Engine) queryLimitsTimeout() time.Duration {
	if e.queryLimits.disabled() {
		return 0
	}
	return e.queryLimits.withDefaults().Timeout
}

func queryLimitExceededResponse(traceID string, contractVersion string, identity IdentityContext, trace *QueryTrace, cost QueryCost, start time.Time, now func() time.Time, err error) QueryResponse {
	elapsedMS := now().Sub(start).Milliseconds()
	return QueryResponse{
		Data: map[string]any{},
		Errors: []DDPError{{
			Code:        "QUERY_LIMIT_EXCEEDED",
			Severity:    "validation",
			Reason:      err.Error(),
			Message:     "query limit exceeded",
			SafeMessage: "Query is too expensive.",
		}},
		Meta: QueryMeta{
			RequestID: traceID,
			TraceID:   traceID,
			Partial:   true,
			ElapsedMS: elapsedMS,
			Cache: QueryCacheMeta{
				Hit:    []string{},
				Miss:   []string{},
				Stale:  []string{},
				Unsafe: []string{},
			},
			Cost:            &cost,
			PolicyVersion:   identity.PolicyVersion,
			ContractVersion: contractVersion,
			Trace:           trace,
		},
	}
}

func queryTimeoutError(path string, err error) DDPError {
	return DDPError{
		Path:        path,
		Code:        "QUERY_TIMEOUT",
		Severity:    "degraded",
		Field:       path,
		Reason:      err.Error(),
		Message:     "query timed out",
		SafeMessage: "Query timed out.",
	}
}

func codedError(err error) CodedError {
	var coded CodedError
	if errors.As(err, &coded) && coded.ErrorCode() != "" {
		return coded
	}
	return nil
}

func mergeTraceCalls(target *TraceCalls, source TraceCalls) {
	if target == nil {
		return
	}
	target.BatchLoads += source.BatchLoads
	target.BatchKeys += source.BatchKeys
	if len(source.ByBatcher) > 0 {
		if target.ByBatcher == nil {
			target.ByBatcher = make(map[string]int, len(source.ByBatcher))
		}
		for key, value := range source.ByBatcher {
			target.ByBatcher[key] += value
		}
	}
}

func errorsOrEmpty(errs []DDPError) []DDPError {
	if errs == nil {
		return []DDPError{}
	}
	return errs
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func groupRequestedFields(group FieldGroupDefinition, paths []string) []string {
	registered := make(map[string]struct{}, len(group.Fields))
	for _, field := range group.Fields {
		registered[field.Path] = struct{}{}
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := registered[path]; ok {
			out = append(out, path)
		}
	}
	return out
}

func (e *Engine) newRequestBatcher() *RequestBatcher {
	batcher := NewRequestBatcher()
	e.batchMu.RLock()
	defer e.batchMu.RUnlock()
	for group, load := range e.batchLoaders {
		batcher.Register(group, load)
	}
	return batcher
}

func resourcePaths(resources []ResourceQuery) []string {
	if len(resources) == 0 {
		return nil
	}
	out := make([]string, 0, len(resources))
	for _, resource := range resources {
		out = append(out, resource.Path)
	}
	return out
}

func (e *Engine) canReadResource(ctx context.Context, identity IdentityContext, resource ResourceDefinition) (Decision, error) {
	if checker, ok := e.policy.(ResourcePolicyChecker); ok {
		return checker.CanReadResource(ctx, identity, resource)
	}
	return e.policy.CanRead(ctx, identity, FieldDefinition{
		Path:       resource.Path,
		Owner:      resource.Owner,
		Permission: resource.Permission,
	})
}

func (e *Engine) ExecuteCommand(ctx context.Context, envelope CommandEnvelope, identity IdentityContext, r *http.Request) CommandResponse {
	start := e.now()
	traceID := e.newTraceID()
	trace := &CommandTrace{RequestID: traceID, Command: envelope.Command}
	if timeout := e.commandLimitsTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	command, ok := e.registry.Command(envelope.Command)
	if !ok {
		return CommandResponse{
			Data:   CommandData{Status: "failed"},
			Errors: []DDPError{{Path: envelope.Command, Code: "COMMAND_NOT_FOUND", Command: envelope.Command, Severity: "validation", Reason: "not_registered", Message: "command is not registered", SafeMessage: "Command is not available."}},
			Meta:   commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, true),
		}
	}

	if command.IdempotencyRequired && envelope.IdempotencyKey == "" {
		return CommandResponse{
			Data:   CommandData{Status: "failed"},
			Errors: []DDPError{{Path: envelope.Command, Code: "IDEMPOTENCY_KEY_REQUIRED", Command: envelope.Command, Severity: "validation", Reason: "missing_idempotency_key", Message: "idempotency key is required", SafeMessage: "Command cannot be executed."}},
			Meta:   commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, true),
		}
	}

	idempotencyKey := e.commandIdempotencyKey(identity, envelope)
	if idempotencyKey != "" && e.idempotency != nil {
		if cached, hit := e.idempotency.Get(ctx, idempotencyKey); hit {
			cached.Meta.TraceID = traceID
			cached.Meta.RequestID = traceID
			if envelope.Trace {
				trace.IdempotencyHit = true
				trace.Permission = "cached"
				trace.Invalidates = cached.Data.Invalidates
				trace.TotalTimeMS = e.now().Sub(start).Milliseconds()
				cached.Meta.Trace = trace
				cached.Meta.ElapsedMS = trace.TotalTimeMS
			} else {
				cached.Meta.Trace = nil
			}
			return cached
		}
	}

	decision, err := e.policy.CanExecute(ctx, identity, command)
	if err != nil {
		return CommandResponse{Data: CommandData{Status: "failed"}, Errors: []DDPError{{Path: envelope.Command, Code: "COMMAND_POLICY_ERROR", Command: envelope.Command, Severity: "fatal", Reason: err.Error(), Message: "command policy failed", SafeMessage: "Command cannot be executed."}}, Meta: commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, true)}
	}
	trace.Permission = decision.Reason
	if !decision.Allow {
		return CommandResponse{Data: CommandData{Status: "failed"}, Errors: []DDPError{{Path: envelope.Command, Code: "COMMAND_DENIED", Command: envelope.Command, Severity: "denied", Reason: decision.Reason, Message: "command access denied", SafeMessage: "Command cannot be executed."}}, Meta: commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, true)}
	}

	result, err := command.Execute(ctx, CommandExecutionRequest{
		Identity:        identity,
		Command:         envelope.Command,
		Input:           envelope.Input,
		IdempotencyKey:  envelope.IdempotencyKey,
		ExpectedVersion: envelope.ExpectedVersion,
		HTTPRequest:     r,
	})
	if err != nil {
		code := "COMMAND_EXECUTE_FAILED"
		safeMessage := "Command failed."
		retryable := true
		if contextError := ctx.Err(); contextError != nil {
			code = "COMMAND_TIMEOUT"
			err = contextError
			safeMessage = "Command timed out."
			retryable = false
		}
		response := CommandResponse{Data: CommandData{Status: "failed"}, Errors: []DDPError{{Path: envelope.Command, Code: code, Command: envelope.Command, Severity: "degraded", Reason: err.Error(), Message: "command executor failed", SafeMessage: safeMessage, Retryable: retryable}}, Meta: commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, true)}
		if idempotencyKey != "" && e.idempotency != nil {
			e.idempotency.Set(ctx, idempotencyKey, response)
		}
		return response
	}

	status := result.Status
	if status == "" {
		status = "completed"
	}
	trace.Invalidates = result.Invalidates
	response := CommandResponse{
		Data: CommandData{
			Status:      status,
			CommandID:   result.CommandID,
			OperationID: result.OperationID,
			Result:      result.Result,
			Invalidates: result.Invalidates,
			Events:      result.Events,
		},
		Errors: []DDPError{},
		Meta:   commandMeta(traceID, maybeCommandTrace(trace, envelope.Trace, start, e.now), start, e.now, false),
	}

	if len(result.Invalidates) > 0 && e.cache != nil {
		e.cache.InvalidateFields(ctx, result.Invalidates)
	}
	if idempotencyKey != "" && e.idempotency != nil {
		e.idempotency.Set(ctx, idempotencyKey, response)
	}
	return response
}

func (e *Engine) commandLimitsTimeout() time.Duration {
	if e.commandLimits.disabled() {
		return 0
	}
	return e.commandLimits.withDefaults().Timeout
}

func commandLimitExceededResponse(traceID string, command string, err error) CommandResponse {
	return CommandResponse{
		Data: CommandData{Status: "failed"},
		Errors: []DDPError{{
			Path:        command,
			Code:        "COMMAND_LIMIT_EXCEEDED",
			Command:     command,
			Severity:    "validation",
			Reason:      err.Error(),
			Message:     "command limit exceeded",
			SafeMessage: "Command is too expensive.",
		}},
		Meta: CommandMeta{
			RequestID: traceID,
			TraceID:   traceID,
			Partial:   true,
			ElapsedMS: 0,
		},
	}
}

func (e *Engine) cacheKey(field FieldDefinition, identity IdentityContext) (string, bool) {
	if field.Cache.Scope == "" || field.Cache.Scope == CacheScopeNone {
		return "", false
	}

	subject := identity.Subject
	tenant := identity.TenantID
	permissionVersion := identity.PermissionVersion

	switch field.Cache.Scope {
	case CacheScopePublic:
		subject = ""
		tenant = ""
	case CacheScopeTenant:
		subject = ""
	case CacheScopePrivate:
		// Keep subject, tenant, and permission version in the cache boundary.
	default:
		return "", false
	}

	return fmt.Sprintf("field:%s:scope=%s:tenant=%s:subject=%s:perm=%s", field.Path, field.Cache.Scope, tenant, subject, permissionVersion), true
}

func (e *Engine) commandIdempotencyKey(identity IdentityContext, envelope CommandEnvelope) string {
	if envelope.IdempotencyKey == "" {
		return ""
	}
	return strings.Join([]string{identity.Subject, identity.TenantID, envelope.Command, envelope.IdempotencyKey}, "|")
}

func maybeCommandTrace(trace *CommandTrace, enabled bool, start time.Time, now func() time.Time) *CommandTrace {
	if !enabled {
		return nil
	}
	trace.TotalTimeMS = now().Sub(start).Milliseconds()
	return trace
}

func commandMeta(traceID string, trace *CommandTrace, start time.Time, now func() time.Time, partial bool) CommandMeta {
	elapsedMS := now().Sub(start).Milliseconds()
	if trace != nil && trace.TotalTimeMS > 0 {
		elapsedMS = trace.TotalTimeMS
	}
	return CommandMeta{
		RequestID: traceID,
		TraceID:   traceID,
		Partial:   partial,
		ElapsedMS: elapsedMS,
		Trace:     trace,
	}
}

func NewTraceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "trace_" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("trace_%d", time.Now().UnixNano())
}

func DecodeInput[T any](raw json.RawMessage) (T, error) {
	var value T
	if len(raw) == 0 || string(raw) == "null" {
		return value, nil
	}
	return value, json.Unmarshal(raw, &value)
}
