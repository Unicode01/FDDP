import type { FddpCommandResponse, FddpError, FddpQueryResponse, FddpResponseMeta } from "./types";

type AnyRecord = Record<string, any>;

const emptyCache = () => ({ hit: [], miss: [], stale: [], unsafe: [] });

export function normalizeQueryResponse<TData = unknown>(raw: unknown): FddpQueryResponse<TData> {
  const value = (raw ?? {}) as AnyRecord;
  const errors = normalizeErrors(value.errors);

  if (value.meta && typeof value.meta === "object") {
    return {
      data: (value.data ?? {}) as TData,
      errors,
      meta: normalizeMeta(value.meta, errors)
    };
  }

  return {
    data: (value.data ?? {}) as TData,
    errors,
    meta: normalizeMeta({
      requestId: value.trace?.requestId ?? value.traceId,
      traceId: value.traceId,
      partial: errors.length > 0,
      cache: {
        hit: value.trace?.cacheHit ?? [],
        miss: value.trace?.cacheMiss ?? [],
        stale: [],
        unsafe: []
      },
      trace: value.trace
    }, errors)
  };
}

export function normalizeCommandResponse<TResult = unknown>(raw: unknown): FddpCommandResponse<TResult> {
  const value = (raw ?? {}) as AnyRecord;
  const errors = normalizeErrors(value.errors);

  if (value.meta && typeof value.meta === "object") {
    return {
      data: value.data,
      errors,
      meta: normalizeMeta(value.meta, errors)
    };
  }

  return {
    data: {
      status: value.status ?? (errors.length ? "failed" : "completed"),
      commandId: value.commandId,
      operationId: value.operationId,
      result: value.result,
      invalidates: value.invalidates,
      events: value.events
    },
    errors,
    meta: normalizeMeta({
      requestId: value.trace?.requestId ?? value.traceId,
      traceId: value.traceId,
      partial: errors.length > 0,
      trace: value.trace
    }, errors)
  };
}

function normalizeMeta(input: AnyRecord, errors: FddpError[]): FddpResponseMeta {
  return {
    requestId: input.requestId,
    traceId: input.traceId,
    partial: Boolean(input.partial ?? errors.length > 0),
    elapsedMs: input.elapsedMs,
    cache: normalizeCache(input.cache),
    contractVersion: input.contractVersion ?? input.contracts?.contractVersion,
    policyVersion: input.policyVersion ?? input.contracts?.policyVersion,
    trace: input.trace,
    ...copyUnknownMeta(input)
  };
}

function copyUnknownMeta(input: AnyRecord): AnyRecord {
  const reserved = new Set([
    "requestId",
    "traceId",
    "partial",
    "elapsedMs",
    "cache",
    "contractVersion",
    "contracts",
    "policyVersion",
    "trace"
  ]);
  const output: AnyRecord = {};

  for (const [key, value] of Object.entries(input)) {
    if (!reserved.has(key)) {
      output[key] = value;
    }
  }

  return output;
}

function normalizeCache(input: unknown): FddpResponseMeta["cache"] {
  if (!input || typeof input !== "object") {
    return emptyCache();
  }

  const cache = input as AnyRecord;
  return {
    hit: Array.isArray(cache.hit) ? cache.hit : [],
    miss: Array.isArray(cache.miss) ? cache.miss : [],
    stale: Array.isArray(cache.stale) ? cache.stale : [],
    unsafe: Array.isArray(cache.unsafe) ? cache.unsafe : [],
    written: Array.isArray(cache.written) ? cache.written : undefined
  };
}

function normalizeErrors(input: unknown): FddpError[] {
  if (!Array.isArray(input)) {
    return [];
  }

  return input.map((error) => {
    const value = (error ?? {}) as AnyRecord;
    const path = value.path ?? value.field;
    return {
      path,
      code: String(value.code ?? "UNKNOWN_ERROR"),
      domain: value.domain,
      severity: value.severity,
      retryable: value.retryable,
      staleUsed: value.staleUsed,
      safeMessage: value.safeMessage ?? value.message,
      decisionId: value.decisionId,
      field: value.field,
      command: value.command,
      reason: value.reason,
      message: value.message,
      fallback: value.fallback
    };
  });
}
