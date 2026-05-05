import type { FddpFieldNode, FddpQuery, HeaderFactory } from "./types";
import { isFddpResourceDescriptor } from "./query";

export function joinUrl(baseUrl: string | undefined, path: string): string {
  if (/^https?:\/\//i.test(path)) {
    return path;
  }

  const base = (baseUrl ?? "").replace(/\/+$/, "");
  const suffix = path.startsWith("/") ? path : `/${path}`;
  return `${base}${suffix}`;
}

export function stableStringify(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }

  if (Array.isArray(value)) {
    return `[${value.map((item) => stableStringify(item)).join(",")}]`;
  }

  const entries = Object.entries(value as Record<string, unknown>).sort(([a], [b]) => a.localeCompare(b));
  return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${stableStringify(item)}`).join(",")}}`;
}

export function flattenFddpQuery(query: FddpQuery): string[] {
  const fields: string[] = [];

  const walk = (prefix: string, node: FddpFieldNode): void => {
    if (Array.isArray(node)) {
      for (const leaf of node) {
        fields.push(prefix ? `${prefix}.${leaf}` : leaf);
      }
      return;
    }

    if (isFddpResourceDescriptor(node)) {
      fields.push(prefix);
      if (node.$type === "collection") {
        for (const field of node.selection?.fields ?? []) {
          fields.push(`${prefix}.${field}`);
        }
        for (const [relation, selection] of Object.entries(node.selection?.expand ?? {})) {
          if (Array.isArray(selection)) {
            for (const field of selection) {
              fields.push(`${prefix}.expand.${relation}.${field}`);
            }
          } else if (isSelectionObject(selection)) {
            for (const field of selection.fields ?? []) {
              fields.push(`${prefix}.expand.${relation}.${field}`);
            }
          }
        }
      }
      return;
    }

    for (const [key, child] of Object.entries(node)) {
      walk(prefix ? `${prefix}.${key}` : key, child as FddpFieldNode);
    }
  };

  for (const [domain, node] of Object.entries(query)) {
    walk(domain, node);
  }

  return Array.from(new Set(fields)).sort();
}

function isSelectionObject(value: unknown): value is { readonly fields?: readonly string[] } {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

export const flattenDomainQuery = flattenFddpQuery;

type FieldPathTree = {
  leaves: Set<string>;
  children: Map<string, FieldPathTree>;
};

export function queryFromFields(fields: readonly string[]): FddpQuery {
  const root = createFieldPathTree();

  for (const field of fields) {
    const parts = field.split(".").filter(Boolean);
    if (parts.length < 2) {
      throw new Error(`FDDP field path must include at least a domain and field: ${field}`);
    }

    const leaf = parts[parts.length - 1];
    if (!leaf) {
      throw new Error(`FDDP field path has an invalid leaf: ${field}`);
    }

    let current = root;
    for (const part of parts.slice(0, -1)) {
      let child = current.children.get(part);
      if (!child) {
        child = createFieldPathTree();
        current.children.set(part, child);
      }
      current = child;
    }
    current.leaves.add(leaf);
  }

  return renderFieldPathTree(root) as FddpQuery;
}

function createFieldPathTree(): FieldPathTree {
  return {
    leaves: new Set(),
    children: new Map()
  };
}

function renderFieldPathTree(tree: FieldPathTree): FddpFieldNode {
  if (tree.leaves.size > 0 && tree.children.size > 0) {
    throw new Error("FDDP query cannot mix direct leaf fields and nested child fields at the same path.");
  }

  if (tree.children.size === 0) {
    return Array.from(tree.leaves).sort();
  }

  const out: Record<string, FddpFieldNode> = {};
  for (const [key, child] of Array.from(tree.children.entries()).sort(([a], [b]) => a.localeCompare(b))) {
    out[key] = renderFieldPathTree(child);
  }
  return out;
}

export async function resolveHeaders(input?: HeadersInit | HeaderFactory): Promise<HeadersInit | undefined> {
  if (!input) {
    return undefined;
  }

  if (typeof input === "function") {
    return input();
  }

  return input;
}

export async function resolveString(input?: string | (() => string | null | undefined | Promise<string | null | undefined>)) {
  if (typeof input === "function") {
    return input();
  }

  return input;
}

export function mergeHeaders(...items: Array<HeadersInit | undefined>): Headers {
  const headers = new Headers();

  for (const item of items) {
    if (!item) {
      continue;
    }

    new Headers(item).forEach((value, key) => {
      headers.set(key, value);
    });
  }

  return headers;
}

export function makeQueryCacheKey(scope: string, query: FddpQuery): string {
  return `query:${scope}:${stableStringify(query)}`;
}
