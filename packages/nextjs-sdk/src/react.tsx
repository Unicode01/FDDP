"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode
} from "react";
import type {
  FddpClient,
  FddpCommandOptions,
  FddpCommandResponse,
  FddpError,
  FddpQuery,
  FddpQueryOptions,
  FddpQueryResponse,
  FddpResponseMeta
} from "./types";
import { stableStringify } from "./utils";

const FddpClientContext = createContext<FddpClient | null>(null);

export function FddpClientProvider(props: { client: FddpClient; children: ReactNode }) {
  return <FddpClientContext.Provider value={props.client}>{props.children}</FddpClientContext.Provider>;
}

export const DomainClientProvider = FddpClientProvider;

export function useFddpClient(override?: FddpClient): FddpClient {
  const client = useContext(FddpClientContext) ?? override;

  if (!client) {
    throw new Error("No FDDP client found. Wrap your app with FddpClientProvider or pass client in options.");
  }

  return client;
}

export const useDomainClient = useFddpClient;

export type UseFddpQueryOptions = FddpQueryOptions & {
  client?: FddpClient;
  enabled?: boolean;
};

export type UseFddpQueryState<TData> = {
  data?: TData;
  errors: FddpError[];
  meta?: FddpResponseMeta;
  traceId?: string;
  loading: boolean;
  error?: Error;
  refetch: () => Promise<FddpQueryResponse<TData>>;
};

export function useFddpQuery<TData = unknown>(
  query: FddpQuery,
  options: UseFddpQueryOptions = {}
): UseFddpQueryState<TData> {
  const client = useFddpClient(options.client);
  const key = useMemo(() => stableStringify(query), [query]);
  const latestQuery = useRef(query);
  const latestOptions = useRef(options);
  latestQuery.current = query;
  latestOptions.current = options;

  const [state, setState] = useState<Omit<UseFddpQueryState<TData>, "refetch">>({
    errors: [],
    loading: options.enabled !== false
  });

  const refetch = useCallback(async () => {
    setState((current) => ({ ...current, loading: true, error: undefined }));

    try {
      const response = await client.query<TData>(latestQuery.current, latestOptions.current);
      setState({
        data: response.data,
        errors: response.errors,
        meta: response.meta,
        traceId: response.meta.traceId,
        loading: false
      });
      return response;
    } catch (error) {
      const normalized = error instanceof Error ? error : new Error(String(error));
      setState((current) => ({ ...current, loading: false, error: normalized }));
      throw normalized;
    }
  }, [client]);

  useEffect(() => {
    if (options.enabled === false) {
      return;
    }

    const controller = new AbortController();
    let active = true;

    setState((current) => ({ ...current, loading: true, error: undefined }));

    client.query<TData>(query, { ...options, signal: options.signal ?? controller.signal })
      .then((response) => {
        if (!active) {
          return;
        }

        setState({
          data: response.data,
          errors: response.errors,
          meta: response.meta,
          traceId: response.meta.traceId,
          loading: false
        });
      })
      .catch((error) => {
        if (!active || controller.signal.aborted) {
          return;
        }

        setState((current) => ({
          ...current,
          loading: false,
          error: error instanceof Error ? error : new Error(String(error))
        }));
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [client, key, options.enabled]);

  return { ...state, refetch };
}

export const useDomainQuery = useFddpQuery;
export type UseDomainQueryOptions = UseFddpQueryOptions;
export type UseDomainQueryState<TData> = UseFddpQueryState<TData>;

export type UseFddpCommandOptions = {
  client?: FddpClient;
  onSuccess?: (response: FddpCommandResponse) => void;
  onError?: (error: Error) => void;
};

export type UseFddpCommandState<TResult, TInput> = {
  result?: FddpCommandResponse<TResult>;
  loading: boolean;
  error?: Error;
  execute: (input?: TInput, options?: FddpCommandOptions) => Promise<FddpCommandResponse<TResult>>;
};

export function useFddpCommand<TResult = unknown, TInput = unknown>(
  commandName: string,
  options: UseFddpCommandOptions = {}
): UseFddpCommandState<TResult, TInput> {
  const client = useFddpClient(options.client);
  const [result, setResult] = useState<FddpCommandResponse<TResult> | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | undefined>();

  const execute = useCallback(async (input?: TInput, commandOptions?: FddpCommandOptions) => {
    setLoading(true);
    setError(undefined);

    try {
      const response = await client.command<TResult, TInput>(commandName, input, commandOptions);
      setResult(response);
      setLoading(false);
      options.onSuccess?.(response);
      return response;
    } catch (err) {
      const normalized = err instanceof Error ? err : new Error(String(err));
      setError(normalized);
      setLoading(false);
      options.onError?.(normalized);
      throw normalized;
    }
  }, [client, commandName]);

  return { result, loading, error, execute };
}

export const useDomainCommand = useFddpCommand;
export type UseDomainCommandOptions = UseFddpCommandOptions;
export type UseDomainCommandState<TResult, TInput> = UseFddpCommandState<TResult, TInput>;
