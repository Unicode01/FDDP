export type FddpScalarLeaf = readonly string[];

export type FddpOrderBy = {
  readonly field: string;
  readonly direction?: "asc" | "desc";
};

export type FddpFilterValue =
  | string
  | number
  | boolean
  | null
  | readonly unknown[]
  | {
      readonly eq?: unknown;
      readonly ne?: unknown;
      readonly neq?: unknown;
      readonly gt?: unknown;
      readonly gte?: unknown;
      readonly lt?: unknown;
      readonly lte?: unknown;
      readonly in?: readonly unknown[];
      readonly notIn?: readonly unknown[];
      readonly not_in?: readonly unknown[];
      readonly like?: string;
      readonly contains?: string;
      readonly range?: {
        readonly from?: unknown;
        readonly to?: unknown;
        readonly min?: unknown;
        readonly max?: unknown;
        readonly start?: unknown;
        readonly end?: unknown;
      };
      readonly between?: readonly [unknown, unknown];
      readonly isNull?: boolean;
      readonly is_null?: boolean;
    };

export type FddpFilter = {
  readonly [field: string]: FddpFilterValue | undefined;
};

export type FddpCollectionArgs = {
  readonly first?: number;
  readonly after?: string | null;
  readonly filter?: FddpFilter;
  readonly orderBy?: readonly FddpOrderBy[];
};

export type FddpSelection = {
  readonly fields?: readonly string[];
  readonly expand?: Record<string, readonly string[] | FddpSelection>;
};

export type FddpCollectionDescriptor = {
  readonly $type: "collection";
  readonly args: FddpCollectionArgs;
  readonly selection?: FddpSelection;
};

export type FddpAggregateDescriptor = {
  readonly $type: "aggregate";
  readonly name: string;
  readonly args?: Record<string, unknown>;
};

export type FddpResourceDescriptor = FddpCollectionDescriptor | FddpAggregateDescriptor;

export type FddpFieldNode =
  | FddpScalarLeaf
  | FddpResourceDescriptor
  | { readonly [key: string]: FddpFieldNode };

export type FddpQuery = { readonly [domain: string]: FddpFieldNode };

export type FddpPageDataOptions = {
  readonly scope?: "global" | "session" | "page" | "component" | "method";
  readonly prefetch?: "none" | "on-route-hover" | "on-visible" | "manual";
  readonly retain?: "while-mounted" | "session" | "none";
  readonly ssr?: boolean;
};

export type FddpPageDataManifest<TQuery extends FddpQuery = FddpQuery> = {
  readonly $type: "pageData";
  readonly query: TQuery;
  readonly options: FddpPageDataOptions;
};

export type FddpFragmentManifest<TQuery extends FddpQuery = FddpQuery> = {
  readonly $type: "fragment";
  readonly query: TQuery;
};

export type FddpMethodDataOptions = FddpPageDataOptions & {
  readonly abortOnUnmount?: boolean;
  readonly cache?: "transient" | "page" | "session";
};

export type FddpMethodDataManifest<TQuery extends FddpQuery = FddpQuery> = {
  readonly $type: "methodData";
  readonly query: TQuery;
  readonly options: FddpMethodDataOptions;
};

export type HeaderFactory = () => HeadersInit | Promise<HeadersInit>;

export type TokenProvider = () => string | null | undefined | Promise<string | null | undefined>;

export type CacheScopeProvider = () => string | null | undefined | Promise<string | null | undefined>;

export type NextFetchOptions = {
  revalidate?: number | false;
  tags?: string[];
};

export type FddpRequestInit = RequestInit & {
  next?: NextFetchOptions;
};

export type FddpErrorSeverity = "fatal" | "degraded" | "denied" | "masked" | "validation" | string;

export type FddpError = {
  path?: string;
  code: string;
  domain?: string;
  severity?: FddpErrorSeverity;
  retryable?: boolean;
  staleUsed?: boolean;
  safeMessage?: string;
  decisionId?: string;
  // Legacy compatibility while older runtimes are upgraded.
  field?: string;
  command?: string;
  reason?: string;
  message?: string;
  fallback?: string;
};

export type FddpTrace = {
  requestId?: string;
  query?: string;
  command?: string;
  calls?: {
    fields?: number;
    fieldGroups?: number;
    resources?: number;
    batchLoads?: number;
    batchKeys?: number;
    byBatcher?: Record<string, number>;
  };
  cacheHit?: string[];
  cacheMiss?: string[];
  denied?: string[];
  serviceCalls?: Record<string, unknown>;
  domains?: Record<string, unknown>;
  totalTimeMs?: number;
  [key: string]: unknown;
};

export type FddpCacheSummary = {
  hit: string[];
  miss: string[];
  stale: string[];
  unsafe: string[];
  written?: string[];
};

export type FddpResponseMeta = {
  requestId?: string;
  traceId?: string;
  partial: boolean;
  elapsedMs?: number;
  cache?: FddpCacheSummary;
  contractVersion?: string;
  policyVersion?: string;
  trace?: FddpTrace;
};

export type FddpQueryResponse<TData = unknown> = {
  data: TData;
  errors: FddpError[];
  meta: FddpResponseMeta;
};

export type FddpCommandStatus = "completed" | "accepted" | "conflict" | "failed";

export type FddpEvent = {
  type: string;
  aggregateId?: string;
  version?: number;
  payload?: Record<string, unknown>;
  [key: string]: unknown;
};

export type FddpCommandData<TResult = unknown> = {
  status: FddpCommandStatus | string;
  commandId?: string;
  operationId?: string;
  result?: TResult;
  invalidates?: string[];
  events?: FddpEvent[];
};

export type FddpCommandResponse<TResult = unknown> = {
  data: FddpCommandData<TResult>;
  errors: FddpError[];
  meta: FddpResponseMeta;
};

export type CacheSetOptions = {
  ttlMs?: number;
  fields?: string[];
};

export type FddpCache = {
  get<T>(key: string): T | undefined;
  set<T>(key: string, value: T, options?: CacheSetOptions): void;
  delete(key: string): void;
  clear(): void;
  invalidateFields(fields: readonly string[]): void;
};

export type FddpQueryOptions = {
  trace?: boolean;
  signal?: AbortSignal;
  headers?: HeadersInit | HeaderFactory;
  requestInit?: FddpRequestInit;
  cache?: boolean;
  cacheKey?: string;
  /** Should include subject, tenant, permissionVersion, policyVersion, and contractVersion when available. */
  cacheScope?: string | CacheScopeProvider;
  cacheTtlMs?: number;
};

export type FddpCommandOptions = {
  trace?: boolean;
  signal?: AbortSignal;
  headers?: HeadersInit | HeaderFactory;
  requestInit?: FddpRequestInit;
  idempotencyKey?: string;
  expectedVersion?: number;
};

export type FddpClientOptions = {
  baseUrl?: string;
  queryPath?: string;
  commandPath?: string;
  fetch?: typeof fetch;
  getAccessToken?: TokenProvider;
  headers?: HeadersInit | HeaderFactory;
  requestInit?: FddpRequestInit;
  credentials?: RequestCredentials;
  cache?: FddpCache;
  cacheScope?: string | CacheScopeProvider;
  defaultCacheTtlMs?: number;
  trace?: boolean;
  onTrace?: (meta: FddpResponseMeta, response: FddpQueryResponse | FddpCommandResponse) => void;
  onInvalidate?: (fields: readonly string[]) => void;
};

export type FddpClient = {
  query<TData = unknown>(query: FddpQuery, options?: FddpQueryOptions): Promise<FddpQueryResponse<TData>>;
  command<TResult = unknown, TInput = unknown>(
    command: string,
    input?: TInput,
    options?: FddpCommandOptions
  ): Promise<FddpCommandResponse<TResult>>;
  invalidate(fields: readonly string[]): void;
  cache?: FddpCache;
};

// Backward-compatible aliases for the first spike package API.
export type DomainFieldLeaf = FddpScalarLeaf;
export type DomainFieldNode = FddpFieldNode;
export type DomainQuery = FddpQuery;
export type DomainPageDataOptions = FddpPageDataOptions;
export type DomainPageDataManifest<TQuery extends FddpQuery = FddpQuery> = FddpPageDataManifest<TQuery>;
export type DomainFragmentManifest<TQuery extends FddpQuery = FddpQuery> = FddpFragmentManifest<TQuery>;
export type DomainMethodDataOptions = FddpMethodDataOptions;
export type DomainMethodDataManifest<TQuery extends FddpQuery = FddpQuery> = FddpMethodDataManifest<TQuery>;
export type DomainRequestInit = FddpRequestInit;
export type DomainError = FddpError;
export type DomainTrace = FddpTrace;
export type DomainQueryResponse<TData = unknown> = FddpQueryResponse<TData>;
export type DomainCommandStatus = FddpCommandStatus;
export type DomainEvent = FddpEvent;
export type DomainCommandResponse<TResult = unknown> = FddpCommandResponse<TResult>;
export type DomainCache = FddpCache;
export type DomainQueryOptions = FddpQueryOptions;
export type DomainCommandOptions = FddpCommandOptions;
export type DomainClientOptions = FddpClientOptions;
export type DomainClient = FddpClient;
