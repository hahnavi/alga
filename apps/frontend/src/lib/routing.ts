import type { LocationQuery, LocationQueryValue, Router } from "vue-router";
import router from "@/router";

export function isActiveRoute(currentPath: string, targetPath: string): boolean {
  if (targetPath === "/") return currentPath === "/";
  return currentPath === targetPath || currentPath.startsWith(targetPath + "/");
}

/**
 * Read the first string value for `key` from a Vue Router LocationQuery.
 * Returns "" for missing, null, or array values that don't start with a string.
 */
export function queryString(q: LocationQuery, key: string): string {
  const v: LocationQueryValue | LocationQueryValue[] | undefined = q[key];
  if (typeof v === "string") return v;
  if (Array.isArray(v) && typeof v[0] === "string") return v[0];
  return "";
}

const prefetchedPaths = new Set<string>();

type LazyImporter = () => Promise<unknown>;

function isLazyImporter(value: unknown): value is LazyImporter {
  return typeof value === "function";
}

/**
 * Preload the route chunks for a given path without navigating. Triggers
 * the lazy `() => import(...)` factories assigned to the matched route's
 * component slots, so the chunks land in the browser cache before the
 * user actually clicks. Safe to call repeatedly for the same path; only
 * the first call per path touches the importer.
 */
export function prefetchRoute(router: Router, path: string): void {
  if (prefetchedPaths.has(path)) return;
  const resolved = router.resolve(path);
  for (const record of resolved.matched) {
    for (const value of Object.values(record.components ?? {})) {
      if (isLazyImporter(value)) {
        prefetchedPaths.add(path);
        void value();
      }
    }
  }
}

export function prefetch(path: string): void {
  prefetchRoute(router, path);
}
