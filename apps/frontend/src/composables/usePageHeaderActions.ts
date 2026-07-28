import type { Component, ComponentPublicInstance, Ref, VNode } from "vue";
import { h, isRef, isVNode, nextTick, ref, toValue } from "vue";
import { Plus, Search, SlidersHorizontal, X } from "@lucide/vue";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import { usePageHeader } from "@/composables/usePageHeader";

interface UsePageHeaderActionsOptions {
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
}

interface UsePageHeaderActionsReturn {
  /** Whether the inline search input is open. Use in the page template to gate the filter card. */
  showSearch: Ref<boolean>;
  /** Whether the filter card is open. Use in the page template to gate the filter card. */
  showFilters: Ref<boolean>;
}

const SEARCH_INPUT_CLASS =
  "h-9 w-full rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] pl-9 pr-3 text-sm text-[var(--text-primary)] outline-none transition-colors placeholder:text-[var(--text-muted)] focus:border-[var(--focus-ring)] focus:ring-1 focus:ring-[var(--focus-ring)]";

export function usePageHeaderActions(
  options: UsePageHeaderActionsOptions,
): UsePageHeaderActionsReturn {
  const showSearch = ref(false);
  const showFilters = ref(false);
  const filtersEnabled = options.showFilters === true;
  const addRef = isRef(options.showAdd) ? options.showAdd : null;
  const searchInput = options.searchInput;

  function buildActions(): VNode[] {
    const actions: VNode[] = [];

    if (showSearch.value && searchInput) {
      actions.push(
        h("div", { class: "relative min-w-48 flex-1" }, [
          h(Search, {
            class:
              "pointer-events-none absolute left-2.5 top-1/2 z-[1] h-4 w-4 -translate-y-1/2 text-[var(--text-muted)]",
          }),
          h("input", {
            type: "search",
            "data-page-header-search": "",
            placeholder: options.searchPlaceholder ?? "Search...",
            class: SEARCH_INPUT_CLASS,
            ref: (el: Element | ComponentPublicInstance | null) => {
              if (el instanceof HTMLInputElement && el.value !== searchInput!.value) {
                el.value = searchInput!.value;
              }
            },
            onInput: (e: Event) => {
              searchInput!.value = (e.target as HTMLInputElement).value;
              options.onSearchInput?.();
            },
          }),
        ]),
      );
    }

    if (searchInput) {
      actions.push(
        h(
          "button",
          {
            type: "button",
            class: HEADER_ICON_BTN_CLASS,
            "aria-label": showSearch.value ? "Close search" : "Search",
            title: "Search",
            onClick: () => {
              const wasOpen = showSearch.value;
              showSearch.value = !showSearch.value;
              if (wasOpen) {
                searchInput!.value = "";
                options.onSearchInput?.();
              }
              if (showSearch.value) {
                nextTick(() => {
                  document.querySelector<HTMLInputElement>("[data-page-header-search]")?.focus();
                });
              }
            },
          },
          [h(showSearch.value ? X : Search, { class: "h-4 w-4", "aria-hidden": "true" })],
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
