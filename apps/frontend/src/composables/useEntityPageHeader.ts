import { reactive, type Ref, type VNode } from "vue";
import { type HeaderBadge } from "@/lib/pageHeader";
import { usePageHeader } from "@/composables/usePageHeader";
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

  usePageHeader(() => {
    const row = options.source.value;
    if (!row) return null;
    const badges = options.buildBadges?.(row);
    const actions = options.buildActions?.(row, refs);
    const extra = options.extraOptions?.(row);
    return {
      title: options.buildTitle(row),
      badges,
      options: {
        ...extra,
        ...(actions ? { actions } : undefined),
      },
    };
  });

  return refs;
}
