import type {
  CacheScopeProvider,
  FddpClient,
  FddpClientOptions,
  FddpCommandOptions,
  FddpCommandResponse,
  FddpQuery,
  FddpQueryOptions,
  FddpQueryResponse,
  FddpRequestInit,
  HeaderFactory
} from "./types";
import { normalizeCommandResponse, normalizeQueryResponse } from "./protocol";
import { flattenFddpQuery, joinUrl, makeQueryCacheKey, mergeHeaders, resolveHeaders, resolveString } from "./utils";

export class FddpClientError extends Error {
  readonly status: number;
  readonly body: string;

  constructor(message: string, status: number, body: string) {
    super(message);
    this.name = "FddpClientError";
    this.status = status;
    this.body = body;
  }
}

export function createFddpClient(options: FddpClientOptions = {}): FddpClient {
  const queryPath = options.queryPath ?? "/data/query";
  const commandPath = options.commandPath ?? "/command/execute";

  const requestJson = async <TResponse>(
    path: string,
    body: unknown,
    requestOptions: {
      headers?: HeadersInit | HeaderFactory;
      signal?: AbortSignal;
      requestInit?: FddpRequestInit;
    } = {}
  ): Promise<TResponse> => {
    const fetchImpl = options.fetch ?? globalThis.fetch?.bind(globalThis);

    if (!fetchImpl) {
      throw new Error("No fetch implementation available. Pass fetch in createFddpClient().");
    }

    const token = await options.getAccessToken?.();
    const baseHeaders = await resolveHeaders(options.headers);
    const extraHeaders = await resolveHeaders(requestOptions.headers);
    const headers = mergeHeaders(
      { "content-type": "application/json", accept: "application/json" },
      baseHeaders,
      extraHeaders
    );

    if (token) {
      headers.set("authorization", `Bearer ${token}`);
    }

    const response = await fetchImpl(joinUrl(options.baseUrl, path), {
      ...options.requestInit,
      ...requestOptions.requestInit,
      method: "POST",
      credentials: options.credentials,
      headers,
      signal: requestOptions.signal,
      body: JSON.stringify(body)
    });

    if (!response.ok) {
      const text = await response.text();
      throw new FddpClientError(`FDDP request failed with HTTP ${response.status}`, response.status, text);
    }

    return response.json() as Promise<TResponse>;
  };

  const resolveCacheScope = async (scope?: string | CacheScopeProvider): Promise<string | undefined> => {
    const value = await resolveString(scope ?? options.cacheScope);
    return value ?? undefined;
  };

  const query = async <TData = unknown>(
    queryInput: FddpQuery,
    queryOptions: FddpQueryOptions = {}
  ): Promise<FddpQueryResponse<TData>> => {
    const fields = flattenFddpQuery(queryInput);
    const cacheScope = await resolveCacheScope(queryOptions.cacheScope);
    const cacheKey = queryOptions.cacheKey ?? (cacheScope ? makeQueryCacheKey(cacheScope, queryInput) : undefined);
    const shouldUseCache = queryOptions.cache !== false && Boolean(options.cache && cacheKey);

    if (shouldUseCache && cacheKey) {
      const cached = options.cache?.get<FddpQueryResponse<TData>>(cacheKey);
      if (cached) {
        return cached;
      }
    }

    const raw = await requestJson<unknown>(queryPath, {
      query: queryInput,
      trace: queryOptions.trace ?? options.trace
    }, {
      headers: queryOptions.headers,
      signal: queryOptions.signal,
      requestInit: queryOptions.requestInit
    });
    const response = normalizeQueryResponse<TData>(raw);

    if (options.onTrace) {
      options.onTrace(response.meta, response);
    }

    if (shouldUseCache && cacheKey && !response.meta.partial && response.errors.length === 0) {
      options.cache?.set(cacheKey, response, {
        ttlMs: queryOptions.cacheTtlMs ?? options.defaultCacheTtlMs,
        fields
      });
    }

    return response;
  };

  const command = async <TResult = unknown, TInput = unknown>(
    commandName: string,
    input?: TInput,
    commandOptions: FddpCommandOptions = {}
  ): Promise<FddpCommandResponse<TResult>> => {
    const raw = await requestJson<unknown>(commandPath, {
      command: commandName,
      input,
      idempotencyKey: commandOptions.idempotencyKey,
      expectedVersion: commandOptions.expectedVersion,
      trace: commandOptions.trace ?? options.trace
    }, {
      headers: commandOptions.headers,
      signal: commandOptions.signal,
      requestInit: commandOptions.requestInit
    });
    const response = normalizeCommandResponse<TResult>(raw);

    if (options.onTrace) {
      options.onTrace(response.meta, response);
    }

    const invalidates = response.data.invalidates ?? [];
    if (invalidates.length) {
      options.cache?.invalidateFields(invalidates);
      options.onInvalidate?.(invalidates);
    }

    return response;
  };

  return {
    query,
    command,
    invalidate(fields: readonly string[]) {
      options.cache?.invalidateFields(fields);
      options.onInvalidate?.(fields);
    },
    cache: options.cache
  };
}

export const createDomainClient = createFddpClient;
export const DomainClientError = FddpClientError;
