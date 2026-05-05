import type { CacheSetOptions, FddpCache } from "./types";

type CacheRecord<T> = {
  value: T;
  expiresAt?: number;
  fields: string[];
};

export class MemoryFddpCache implements FddpCache {
  private readonly records = new Map<string, CacheRecord<unknown>>();

  get<T>(key: string): T | undefined {
    const record = this.records.get(key);

    if (!record) {
      return undefined;
    }

    if (record.expiresAt !== undefined && record.expiresAt <= Date.now()) {
      this.records.delete(key);
      return undefined;
    }

    return record.value as T;
  }

  set<T>(key: string, value: T, options: CacheSetOptions = {}): void {
    this.records.set(key, {
      value,
      expiresAt: options.ttlMs ? Date.now() + options.ttlMs : undefined,
      fields: options.fields ?? []
    });
  }

  delete(key: string): void {
    this.records.delete(key);
  }

  clear(): void {
    this.records.clear();
  }

  invalidateFields(fields: readonly string[]): void {
    if (fields.length === 0) {
      return;
    }

    const invalidated = new Set(fields);

    for (const [key, record] of this.records) {
      if (record.fields.some((field) => invalidated.has(field) || fields.some((item) => matchesInvalidation(item, field)))) {
        this.records.delete(key);
      }
    }
  }
}

export function createMemoryFddpCache(): MemoryFddpCache {
  return new MemoryFddpCache();
}

export class MemoryDomainCache extends MemoryFddpCache {}

export function createMemoryDomainCache(): MemoryDomainCache {
  return new MemoryDomainCache();
}

function matchesInvalidation(pattern: string, field: string): boolean {
  if (pattern.endsWith(".*")) {
    return field.startsWith(pattern.slice(0, -1));
  }

  return pattern === field;
}
