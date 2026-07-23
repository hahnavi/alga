import { onBeforeUnmount, reactive, watch, type Ref, type VNode } from "vue";
import { clearPageHeader, setPageHeader, type HeaderBadge } from "@/lib/pageHeader";
import { useDocumentTitle } from "@/composables/useDocumentTitle";

/** A bag of callbacks the page exposes to the `buildActions` builder (e.g. startEdit). */
export type EntityActionRefs = Record<string, (...args: unknown[]) => unknown>;

/** Build a HeaderBadge list for the current row. */
export type BuildBadges<TRow> = (row: TRow) => HeaderBadge[] | undefined;

/** Build the action VNodes (menu buttons, etc.) for the current row. */
export type BuildActions<TRow> = (row: TRow, refs: EntityActionRefs) => VNode[] | undefined;

/** Build the document title for the current row. */
export type BuildDocumentTitle<TRow> = (row: TRow | null) => string;

export type EntityPageHeaderExtra = {
  titleIcon?: VNode;
  titlePrefix?: string;
  leadingBadges?: HeaderBadge[];
  headerAgentBrand?: "hermes" | "openclaw" | "other";
};

export type UseEntityPageHeaderOptions<TRow> = {
  source: Ref<TRow | null>;
  buildTitle: (row: TRow) => string;
  buildBadges?: BuildBadges<TRow>;
  buildActions?: BuildActions<TRow>;
  documentTitle?: BuildDocumentTitle<TRow>;
  /** Additional page-header options to pass through (e.g. titlePrefix, headerAgentBrand). */
  extraOptions?: (row: TRow) => EntityPageHeaderExtra | undefined;
};

export function useEntityPageHeader<TRow>(
  options: UseEntityPageHeaderOptions<TRow>,
): EntityActionRefs {
  const refs = reactive<EntityActionRefs>({});

  if (options.documentTitle) {
    const getter = options.documentTitle;
    useDocumentTitle(() => getter(options.source.value));
  }

  function sync(row: TRow) {
    const badges = options.buildBadges?.(row);
    const actions = options.buildActions?.(row, refs);
    const extra = options.extraOptions?.(row);
    setPageHeader(options.buildTitle(row), badges, {
      ...extra,
      ...(actions ? { actions } : undefined),
    });
  }

  watch(
    options.source,
    (row) => {
      if (row) sync(row);
    },
    { immediate: true },
  );

  onBeforeUnmount(() => {
    clearPageHeader();
  });

  return refs;
}
