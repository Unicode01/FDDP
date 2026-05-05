import type {
  FddpAggregateDescriptor,
  FddpCollectionArgs,
  FddpCollectionDescriptor,
  FddpFragmentManifest,
  FddpMethodDataManifest,
  FddpMethodDataOptions,
  FddpPageDataManifest,
  FddpPageDataOptions,
  FddpQuery,
  FddpResourceDescriptor,
  FddpSelection
} from "./types";

export type FddpCollectionBuilder = {
  select(selection: FddpSelection): FddpCollectionDescriptor;
};

export function collection(args: FddpCollectionArgs): FddpCollectionBuilder {
  return {
    select(selection: FddpSelection): FddpCollectionDescriptor {
      return { $type: "collection", args, selection };
    }
  };
}

export function aggregate(name: string, args?: Record<string, unknown>): FddpAggregateDescriptor {
  return args === undefined ? { $type: "aggregate", name } : { $type: "aggregate", name, args };
}

export function definePageData<TQuery extends FddpQuery>(
  query: TQuery,
  options: FddpPageDataOptions = {}
): FddpPageDataManifest<TQuery> {
  return { $type: "pageData", query, options };
}

export function defineFragment<TQuery extends FddpQuery>(query: TQuery): FddpFragmentManifest<TQuery> {
  return { $type: "fragment", query };
}

export function defineMethodData<TQuery extends FddpQuery>(
  query: TQuery,
  options: FddpMethodDataOptions = {}
): FddpMethodDataManifest<TQuery> {
  return { $type: "methodData", query, options };
}

export function isFddpResourceDescriptor(value: unknown): value is FddpResourceDescriptor {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }

  const marker = (value as { $type?: unknown }).$type;
  return marker === "collection" || marker === "aggregate";
}
