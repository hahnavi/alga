import { ref, watch } from "vue";
import { type LocationQuery, type Router, type RouteLocationNormalizedLoaded } from "vue-router";

type FilterSyncOptions = {
  route: RouteLocationNormalizedLoaded;
  router: Router;
  path: string;
  buildQuery: () => Record<string, string>;
  parseQuery: (q: LocationQuery) => void;
  onReload: () => void;
  /**
   * If true, automatically re-apply URL → state when the route query changes
   * externally (e.g. browser back/forward navigation). Defaults to false to
   * preserve the legacy opt-in call site behaviour; pages that benefit from
   * bidirectional sync should enable this and call `parseQuery` idempotently.
   */
  watchExternalChanges?: boolean;
};

export function useFilterSync(opts: FilterSyncOptions) {
  const syncingFromRoute = ref(false);

  function normalizedRouteQuery(): Record<string, string> {
    const o: Record<string, string> = {};
    for (const [k, v] of Object.entries(opts.route.query)) {
      if (typeof v === "string" && v.length > 0) o[k] = v;
      else if (Array.isArray(v) && typeof v[0] === "string" && v[0].length > 0) o[k] = v[0];
    }
    return o;
  }

  function syncFiltersToUrl(): void {
    const next = opts.buildQuery();
    const a = JSON.stringify(next);
    const b = JSON.stringify(normalizedRouteQuery());
    if (a === b) return;
    opts.router.replace({ path: opts.path, query: next });
  }

  function applyFromUrl(): void {
    syncingFromRoute.value = true;
    opts.parseQuery(opts.route.query);
    syncingFromRoute.value = false;
  }

  function clearFilters(resetRefs: () => void): void {
    resetRefs();
    syncFiltersToUrl();
    opts.onReload();
  }

  if (opts.watchExternalChanges) {
    watch(
      () => JSON.stringify(normalizedRouteQuery()),
      () => {
        if (syncingFromRoute.value) return;
        applyFromUrl();
      },
    );
  }

  return {
    syncingFromRoute,
    syncFiltersToUrl,
    applyFromUrl,
    clearFilters,
  };
}
