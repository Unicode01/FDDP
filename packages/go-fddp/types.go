package fddp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type CacheScope string

const (
	CacheScopeNone    CacheScope = "none"
	CacheScopePublic  CacheScope = "public"
	CacheScopePrivate CacheScope = "private"
	CacheScopeTenant  CacheScope = "tenant"
)

type IdentityContext struct {
	Subject           string            `json:"subject,omitempty"`
	TenantID          string            `json:"tenantId,omitempty"`
	Roles             []string          `json:"roles,omitempty"`
	Scopes            []string          `json:"scopes,omitempty"`
	PermissionVersion string            `json:"permissionVersion,omitempty"`
	PolicyVersion     string            `json:"policyVersion,omitempty"`
	MFA               bool              `json:"mfa,omitempty"`
	SessionLevel      string            `json:"sessionLevel,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

type TokenClaims struct {
	Subject           string
	TenantID          string
	Roles             []string
	Scopes            []string
	PermissionVersion string
	PolicyVersion     string
	MFA               bool
	SessionLevel      string
	Attributes        map[string]string
}

type TokenVerifier interface {
	VerifyToken(ctx context.Context, token string) (TokenClaims, error)
}

type TokenVerifierFunc func(ctx context.Context, token string) (TokenClaims, error)

func (fn TokenVerifierFunc) VerifyToken(ctx context.Context, token string) (TokenClaims, error) {
	return fn(ctx, token)
}

type CachePolicy struct {
	Scope CacheScope    `json:"scope"`
	TTL   time.Duration `json:"-"`
}

type FieldDefinition struct {
	Path        string
	Type        string
	Owner       string
	Permission  string
	Cache       CachePolicy
	Nullable    bool
	Deprecated  bool
	Sensitivity string
	Resolve     FieldResolver
}

type FieldGroupDefinition struct {
	Path       string
	Owner      string
	Permission string
	Cache      CachePolicy
	Deprecated bool
	Fields     []FieldDefinition
	Resolve    FieldGroupResolver
}

type FieldRequest struct {
	Identity    IdentityContext
	Path        string
	HTTPRequest *http.Request
	Batcher     *RequestBatcher
}

type FieldResolver func(ctx context.Context, req FieldRequest) (any, error)

type FieldGroupRequest struct {
	Identity    IdentityContext
	Path        string
	Fields      []string
	HTTPRequest *http.Request
	Batcher     *RequestBatcher
}

type FieldGroupResolver func(ctx context.Context, req FieldGroupRequest) (map[string]any, error)

func (req FieldRequest) Load(ctx context.Context, group string, key string) (any, error) {
	return req.Batcher.LoadRegistered(ctx, group, key)
}

func (req FieldRequest) LoadMany(ctx context.Context, group string, keys []string) (map[string]any, error) {
	return req.Batcher.LoadManyRegistered(ctx, group, keys)
}

type ResourceQueryType string

const (
	ResourceQueryCollection ResourceQueryType = "collection"
	ResourceQueryAggregate  ResourceQueryType = "aggregate"
)

type OrderBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

type CollectionArgs struct {
	First   int            `json:"first,omitempty"`
	After   *string        `json:"after,omitempty"`
	Filter  map[string]any `json:"filter,omitempty"`
	OrderBy []OrderBy      `json:"orderBy,omitempty"`
}

type Selection struct {
	Fields []string             `json:"fields,omitempty"`
	Expand map[string]Selection `json:"expand,omitempty"`
}

type ResourceQuery struct {
	Path       string            `json:"path"`
	Type       ResourceQueryType `json:"type"`
	Name       string            `json:"name,omitempty"`
	Collection CollectionArgs    `json:"collection,omitempty"`
	Args       map[string]any    `json:"args,omitempty"`
	Selection  Selection         `json:"selection,omitempty"`
}

type QueryPlan struct {
	Fields    []string        `json:"fields,omitempty"`
	Resources []ResourceQuery `json:"resources,omitempty"`
	Trace     bool            `json:"trace,omitempty"`
}

type ResourceDefinition struct {
	Path        string
	Owner       string
	Permission  string
	MaxPageSize int
	Deprecated  bool
	Fields      []ContractResourceField
	Relations   []ContractResourceRelation
	Resolve     ResourceResolver
}

type ResourceRequest struct {
	Identity    IdentityContext
	Path        string
	Type        ResourceQueryType
	Name        string
	Collection  CollectionArgs
	Args        map[string]any
	Selection   Selection
	HTTPRequest *http.Request
	Batcher     *RequestBatcher
}

type ResourceResolver func(ctx context.Context, req ResourceRequest) (any, error)

type CodedError interface {
	error
	ErrorCode() string
	ErrorHint() string
}

func (req ResourceRequest) Load(ctx context.Context, group string, key string) (any, error) {
	return req.Batcher.LoadRegistered(ctx, group, key)
}

func (req ResourceRequest) LoadMany(ctx context.Context, group string, keys []string) (map[string]any, error) {
	return req.Batcher.LoadManyRegistered(ctx, group, keys)
}

type PageInfo struct {
	HasNextPage     bool   `json:"hasNextPage"`
	HasPreviousPage bool   `json:"hasPreviousPage,omitempty"`
	StartCursor     string `json:"startCursor,omitempty"`
	EndCursor       string `json:"endCursor,omitempty"`
}

type CollectionResult struct {
	Items      []any     `json:"items"`
	PageInfo   *PageInfo `json:"pageInfo,omitempty"`
	TotalCount *int      `json:"totalCount,omitempty"`
}

type ContractField struct {
	Field       string     `json:"field"`
	Type        string     `json:"type"`
	Owner       string     `json:"owner,omitempty"`
	Permission  string     `json:"permission,omitempty"`
	Nullable    bool       `json:"nullable,omitempty"`
	Deprecated  bool       `json:"deprecated,omitempty"`
	Sensitivity string     `json:"sensitivity,omitempty"`
	CacheScope  CacheScope `json:"cacheScope,omitempty"`
}

type ContractResource struct {
	Path        string                     `json:"path"`
	Owner       string                     `json:"owner,omitempty"`
	Permission  string                     `json:"permission,omitempty"`
	Types       []string                   `json:"types,omitempty"`
	MaxPageSize int                        `json:"maxPageSize,omitempty"`
	Deprecated  bool                       `json:"deprecated,omitempty"`
	Fields      []ContractResourceField    `json:"fields,omitempty"`
	Relations   []ContractResourceRelation `json:"relations,omitempty"`
}

type ContractResourceField struct {
	Field      string `json:"field"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable,omitempty"`
	Filterable bool   `json:"filterable,omitempty"`
	Orderable  bool   `json:"orderable,omitempty"`
}

type ContractResourceRelation struct {
	Name   string                  `json:"name"`
	Fields []ContractResourceField `json:"fields,omitempty"`
}

type ContractCommand struct {
	Name                string               `json:"name"`
	Owner               string               `json:"owner,omitempty"`
	Permission          string               `json:"permission,omitempty"`
	IdempotencyRequired bool                 `json:"idempotencyRequired,omitempty"`
	AuditLevel          string               `json:"auditLevel,omitempty"`
	Input               []ContractInputField `json:"input,omitempty"`
}

type ContractSchema struct {
	ProtocolVersion string             `json:"protocolVersion,omitempty"`
	ContractVersion string             `json:"contractVersion,omitempty"`
	Fields          []ContractField    `json:"fields"`
	Resources       []ContractResource `json:"resources,omitempty"`
	Commands        []ContractCommand  `json:"commands,omitempty"`
}

type CommandDefinition struct {
	Name                string
	Owner               string
	Permission          string
	IdempotencyRequired bool
	AuditLevel          string
	Input               []ContractInputField
	Execute             CommandExecutor
}

type ContractInputField struct {
	Field    string `json:"field"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Nullable bool   `json:"nullable,omitempty"`
}

type CommandExecutionRequest struct {
	Identity        IdentityContext
	Command         string
	Input           json.RawMessage
	IdempotencyKey  string
	ExpectedVersion *int64
	HTTPRequest     *http.Request
}

type CommandExecutionResult struct {
	Status      string        `json:"status,omitempty"`
	CommandID   string        `json:"commandId,omitempty"`
	OperationID string        `json:"operationId,omitempty"`
	Result      any           `json:"result,omitempty"`
	Invalidates []string      `json:"invalidates,omitempty"`
	Events      []DomainEvent `json:"events,omitempty"`
}

type CommandExecutor func(ctx context.Context, req CommandExecutionRequest) (CommandExecutionResult, error)

type DomainEvent struct {
	Type        string         `json:"type"`
	AggregateID string         `json:"aggregateId,omitempty"`
	Version     int64          `json:"version,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type DDPError struct {
	Path        string `json:"path,omitempty"`
	Code        string `json:"code"`
	Domain      string `json:"domain,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Retryable   bool   `json:"retryable,omitempty"`
	StaleUsed   bool   `json:"staleUsed,omitempty"`
	SafeMessage string `json:"safeMessage,omitempty"`
	DecisionID  string `json:"decisionId,omitempty"`

	// Legacy fields kept during the protocol transition.
	Field    string `json:"field,omitempty"`
	Command  string `json:"command,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Message  string `json:"message,omitempty"`
	Fallback string `json:"fallback,omitempty"`
}

type QueryTrace struct {
	RequestID   string     `json:"requestId,omitempty"`
	Fields      []string   `json:"fields,omitempty"`
	Resources   []string   `json:"resources,omitempty"`
	Cost        int        `json:"cost,omitempty"`
	Calls       TraceCalls `json:"calls,omitempty"`
	CacheHit    []string   `json:"cacheHit,omitempty"`
	CacheMiss   []string   `json:"cacheMiss,omitempty"`
	Denied      []string   `json:"denied,omitempty"`
	Resolved    []string   `json:"resolved,omitempty"`
	TotalTimeMS int64      `json:"totalTimeMs,omitempty"`
}

type TraceCalls struct {
	Fields      int            `json:"fields,omitempty"`
	FieldGroups int            `json:"fieldGroups,omitempty"`
	Resources   int            `json:"resources,omitempty"`
	BatchLoads  int            `json:"batchLoads,omitempty"`
	BatchKeys   int            `json:"batchKeys,omitempty"`
	ByBatcher   map[string]int `json:"byBatcher,omitempty"`
}

type CommandTrace struct {
	RequestID      string   `json:"requestId,omitempty"`
	Command        string   `json:"command,omitempty"`
	IdempotencyHit bool     `json:"idempotencyHit,omitempty"`
	Permission     string   `json:"permission,omitempty"`
	Invalidates    []string `json:"invalidates,omitempty"`
	TotalTimeMS    int64    `json:"totalTimeMs,omitempty"`
}

type QueryCacheMeta struct {
	Hit    []string `json:"hit"`
	Miss   []string `json:"miss"`
	Stale  []string `json:"stale"`
	Unsafe []string `json:"unsafe"`
}

type QueryMeta struct {
	RequestID       string         `json:"requestId,omitempty"`
	TraceID         string         `json:"traceId,omitempty"`
	Partial         bool           `json:"partial"`
	ElapsedMS       int64          `json:"elapsedMs,omitempty"`
	Cache           QueryCacheMeta `json:"cache"`
	Cost            *QueryCost     `json:"cost,omitempty"`
	ContractVersion string         `json:"contractVersion,omitempty"`
	PolicyVersion   string         `json:"policyVersion,omitempty"`
	Trace           *QueryTrace    `json:"trace,omitempty"`
}

type QueryResponse struct {
	Data   map[string]any `json:"data"`
	Errors []DDPError     `json:"errors"`
	Meta   QueryMeta      `json:"meta"`
}

type CommandData struct {
	Status      string        `json:"status"`
	CommandID   string        `json:"commandId,omitempty"`
	OperationID string        `json:"operationId,omitempty"`
	Result      any           `json:"result,omitempty"`
	Invalidates []string      `json:"invalidates,omitempty"`
	Events      []DomainEvent `json:"events,omitempty"`
}

type CommandMeta struct {
	RequestID string        `json:"requestId,omitempty"`
	TraceID   string        `json:"traceId,omitempty"`
	Partial   bool          `json:"partial"`
	ElapsedMS int64         `json:"elapsedMs,omitempty"`
	Trace     *CommandTrace `json:"trace,omitempty"`
}

type CommandResponse struct {
	Data   CommandData `json:"data,omitempty"`
	Errors []DDPError  `json:"errors"`
	Meta   CommandMeta `json:"meta"`
}

type QueryEndpointRequest struct {
	Body        []byte
	Identity    IdentityContext
	HTTPRequest *http.Request
}

type QueryEndpointResult struct {
	Status   int
	Response QueryResponse
}

type CommandEndpointRequest struct {
	Body        []byte
	Identity    IdentityContext
	HTTPRequest *http.Request
}

type CommandEndpointResult struct {
	Status   int
	Response CommandResponse
}
