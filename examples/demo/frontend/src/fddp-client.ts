import { createFddpClient, createMemoryFddpCache } from "@fddp/next-sdk";

export const fddp = createFddpClient({
  baseUrl: env("NEXT_PUBLIC_FDDP_URL") ?? "http://localhost:8080",
  cache: createMemoryFddpCache(),
  cacheScope: "tenant_abc:user_123:perm_v17:policy_v9:contract_v12",
  headers: {
    "X-DDP-Subject": "user_123",
    "X-DDP-Tenant": "tenant_abc",
    "X-DDP-Permission-Version": "perm_v17"
  },
  trace: env("NODE_ENV") !== "production"
});

function env(name: string): string | undefined {
  const value = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.[name];
  return value || undefined;
}
