import { createFddpClient } from "./client";
import type { FddpClient, FddpClientOptions, FddpRequestInit, TokenProvider } from "./types";

export type NextFddpClientOptions = Omit<FddpClientOptions, "getAccessToken" | "requestInit"> & {
  accessToken?: string | TokenProvider;
  getAccessToken?: TokenProvider;
  requestInit?: FddpRequestInit;
  fetchCache?: RequestCache;
  revalidate?: number | false;
  tags?: string[];
};

export function createNextFddpClient(options: NextFddpClientOptions = {}): FddpClient {
  const { accessToken, fetchCache, revalidate, tags, requestInit, ...rest } = options;
  const nextOptions = revalidate !== undefined || tags?.length ? { revalidate, tags } : undefined;

  return createFddpClient({
    ...rest,
    getAccessToken: rest.getAccessToken ?? normalizeTokenProvider(accessToken),
    requestInit: {
      ...requestInit,
      cache: fetchCache ?? requestInit?.cache,
      next: nextOptions ?? requestInit?.next
    }
  });
}

export const createNextDomainClient = createNextFddpClient;

export function readBearerToken(authorizationHeader: string | null | undefined): string | null {
  if (!authorizationHeader) {
    return null;
  }

  const match = authorizationHeader.match(/^Bearer\s+(.+)$/i);
  return match?.[1] ?? null;
}

function normalizeTokenProvider(accessToken?: string | TokenProvider): TokenProvider | undefined {
  if (!accessToken) {
    return undefined;
  }

  if (typeof accessToken === "function") {
    return accessToken;
  }

  return () => accessToken;
}
