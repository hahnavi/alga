import type { Component, Ref, VNode } from "vue";
import {
  h,
  isRef,
  isVNode,
  nextTick,
  onActivated,
  onBeforeUnmount,
  onDeactivated,
  ref,
  toValue,
} from "vue";
import { Plus, Search, SlidersHorizontal, X } from "@lucide/vue";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { headerInlineSearchExpanded } from "@/lib/pageHeader";
import { usePageHeader } from "@/composables/usePageHeader";

type UsePageHeaderActionsOptions = {
  /** Page title shown in the shell header. */
  title: string;
  /** Optional Lucide icon component (or pre-built VNode) shown before the title. */
  titleIcon?: Component | VNode;
  /** Page-owned search input ref bound to the inline header search field. Optional — pages that don't use the inline search may omit it. */
  searchInput?: Ref<string>;
  /** Placeholder for the inline search input. */
  searchPlaceholder?: string;
  /** Called on every input change in the search field. Also called on close (X). */
  onSearchInput?: () => void;
  /** Show the filter toggle button (defaults to false). */
  showFilters?: boolean;
  /** When true, renders a small dot indicator on the filter button. */
  hasNonDefaultFilters?: Ref<boolean>;
  /** Hook fired after the filter toggle is clicked. */
  onToggleFilters?: () => void;
  /** Show the add/create button (defaults to true). Accepts a ref for live gating. */
  showAdd?: Ref<boolean> | boolean;
  /** aria-label and title for the add button. */
  addLabel?: string;
  /** Click handler for the add button. */
  onAdd?: () => void;
};

type UsePageHeaderActionsReturn = {
  /** Whether the inline search bar is expanded in the shell header. */
  showSearch: Ref<boolean>;
  /** Whether the filter card is open. Use in the page template to gate the filter card. */
  showFilters: Ref<boolean>;
};

const SEARCH_INPUT_CLASS = "field h-9 pl-9 pr-10";

const SEARCH_CLEAR_BTN_CLASS =
  "absolute right-2 top-1/2 flex h-5 w-5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full text-[var(--text-muted)] transition-colors hover:bg-[var(--hover-neutral)] hover:text-[var(--text-primary)]";

const SEARCH_ESC_HINT_CLASS =
  "pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] px-1.5 py-0.5 font-mono text-[10px] leading-none text-[var(--text-muted)]";

// Tracks which usePageHeaderActions instance expanded the header search.
// Under KeepAlive the outgoing page's deactivate hook can fire AFTER the
// incoming page's activate hook, so only the instance that expanded the
// search may collapse the shared header state (same ownership pattern as
// headerOwner in usePageHeader).
let searchExpandedOwner: symbol | null = null;

export function usePageHeaderActions(
  options: UsePageHeaderActionsOptions,
): UsePageHeaderActionsReturn {
  const showSearch = ref(false);
  const showFilters = ref(false);
  const filtersEnabled = options.showFilters === true;
  const addRef = isRef(options.showAdd) ? options.showAdd : null;
  const searchInput = options.searchInput;
  const ownerId = Symbol("pageHeaderActionsOwner");

  function focusSearchInput() {
    document.querySelector<HTMLInputElement>("[data-page-header-search]")?.focus();
  }

  function expandSearch() {
    showSearch.value = true;
    searchExpandedOwner = ownerId;
    headerInlineSearchExpanded.value = true;
    nextTick(focusSearchInput);
  }

  function releaseSearchExpansion() {
    if (searchExpandedOwner !== ownerId) return;
    searchExpandedOwner = null;
    headerInlineSearchExpanded.value = false;
  }

  function closeSearch() {
    if (!showSearch.value) return;
    showSearch.value = false;
    releaseSearchExpansion();
    if (searchInput) {
      searchInput.value = "";
      options.onSearchInput?.();
    }
    nextTick(() => {
      document.querySelector<HTMLElement>("[data-page-header-search-toggle]")?.focus();
    });
  }

  onActivated(() => {
    if (!showSearch.value || !searchInput) return;
    searchExpandedOwner = ownerId;
    headerInlineSearchExpanded.value = true;
  });
  onDeactivated(releaseSearchExpansion);
  onBeforeUnmount(releaseSearchExpansion);

  function buildSearchBar(): VNode {
    const query = searchInput!.value;
    return h("div", { class: "page-header-search-bar flex min-w-0 flex-1 items-center gap-1.5" }, [
      h("div", { class: "relative min-w-0 flex-1" }, [
        h(Search, {
          class:
            "pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]",
          "aria-hidden": "true",
        }),
        h("input", {
          type: "search",
          value: query,
          "data-page-header-search": "",
          "aria-label": options.searchPlaceholder ?? "Search",
          placeholder: options.searchPlaceholder ?? "Search...",
          class: SEARCH_INPUT_CLASS,
          onInput: (e: Event) => {
            searchInput!.value = (e.target as HTMLInputElement).value;
            options.onSearchInput?.();
          },
          onKeydown: (e: KeyboardEvent) => {
            if (e.key === "Escape") closeSearch();
          },
        }),
        query
          ? h(
              "button",
              {
                type: "button",
                class: SEARCH_CLEAR_BTN_CLASS,
                "aria-label": "Clear search",
                title: "Clear search",
                onClick: () => {
                  searchInput!.value = "";
                  options.onSearchInput?.();
                  focusSearchInput();
                },
              },
              [h(X, { class: "h-3.5 w-3.5", "aria-hidden": "true" })],
            )
          : h("kbd", { class: SEARCH_ESC_HINT_CLASS, "aria-hidden": "true" }, "Esc"),
      ]),
      h(
        "button",
        {
          type: "button",
          class: HEADER_ICON_BTN_CLASS,
          "aria-label": "Close search",
          title: "Close search (Esc)",
          onClick: closeSearch,
        },
        [h(X, { class: "h-4 w-4", "aria-hidden": "true" })],
      ),
    ]);
  }

  function buildActions(): VNode[] {
    if (showSearch.value && searchInput) {
      return [buildSearchBar()];
    }

    const actions: VNode[] = [];

    if (searchInput) {
      actions.push(
        h(
          "button",
          {
            type: "button",
            class: HEADER_ICON_BTN_CLASS,
            "data-page-header-search-toggle": "",
            "aria-label": "Search",
            title: "Search",
            onClick: expandSearch,
          },
          [h(Search, { class: "h-4 w-4", "aria-hidden": "true" })],
        ),
      );
    }

    if (filtersEnabled) {
      actions.push(
        h(
          "button",
          {
            type: "button",
            class: HEADER_ICON_BTN_CLASS,
            "aria-label": "Toggle filters",
            title: "Filters",
            onClick: () => {
              showFilters.value = !showFilters.value;
              options.onToggleFilters?.();
            },
          },
          [
            h(SlidersHorizontal, { class: "h-4 w-4", "aria-hidden": "true" }),
            ...(options.hasNonDefaultFilters?.value
              ? [
                  h("span", {
                    class:
                      "flex h-4 w-4 items-center justify-center rounded-full bg-[var(--focus-ring)] text-[10px] font-semibold text-white",
                  }),
                ]
              : []),
          ],
        ),
      );
    }

    const addVisible = addRef ? addRef.value : (toValue(options.showAdd) ?? true);
    if (addVisible) {
      actions.push(
        h(
          "button",
          {
            type: "button",
            class: HEADER_ICON_BTN_CLASS,
            "aria-label": options.addLabel ?? "Create",
            title: options.addLabel ?? "Create",
            onClick: () => options.onAdd?.(),
          },
          [h(Plus, { class: "h-4 w-4", "aria-hidden": "true" })],
        ),
      );
    }

    return actions;
  }

  usePageHeader(() => {
    let titleIcon: VNode | undefined;
    if (options.titleIcon) {
      titleIcon = isVNode(options.titleIcon)
        ? options.titleIcon
        : h(options.titleIcon, {
            class: "h-5 w-5 shrink-0 text-[var(--text-muted)]",
            "aria-hidden": "true",
          });
    }

    return {
      title: options.title,
      options: {
        actions: buildActions(),
        titleIcon,
      },
    };
  });

  return { showSearch, showFilters };
}
