import { h, reactive, ref, type VNode } from "vue";
import { Search } from "@lucide/vue";
import { HEADER_ICON_BTN_CLASS } from "@/lib/uiClasses";
import type { AgentBrand } from "@/lib/agentAvatar";

export type HeaderBadge = {
  text: string;
  cssClass: string;
};

export type PageHeaderOptions = {
  /** Rendered immediately before `title` (e.g. page icon). */
  titleIcon?: VNode;
  /** Shown before `title` (e.g. alert `#123`). */
  titlePrefix?: string;
  /** Rendered before `titlePrefix` and `title` (e.g. severity). */
  leadingBadges?: HeaderBadge[];
  /** Agent DM: which 32px header avatar to show in the shell. */
  headerAgentBrand?: AgentBrand;
  /** Action VNodes rendered in the header (left of user menu). */
  actions?: VNode[];
};

type PageHeaderState = {
  title: string;
  titleIcon?: VNode;
  titlePrefix?: string;
  leadingBadges?: HeaderBadge[];
  badges?: HeaderBadge[];
  headerAgentBrand?: AgentBrand;
  actions?: VNode[];
};

export const pageHeader = ref<PageHeaderState | null>(null);

export function setPageHeader(title: string, badges?: HeaderBadge[], options?: PageHeaderOptions) {
  pageHeader.value = {
    title,
    badges,
    titleIcon: options?.titleIcon,
    titlePrefix: options?.titlePrefix,
    leadingBadges: options?.leadingBadges,
    headerAgentBrand: options?.headerAgentBrand,
    actions: options?.actions,
  };
}

export function clearPageHeader() {
  pageHeader.value = null;
}

export const headerSearchActive = ref(false);

/**
 * True while a page's inline search (see `usePageHeaderActions`) is expanded
 * to fill the shell header. The shell collapses the title areas while this is
 * set so the search field can take the full header width.
 */
export const headerInlineSearchExpanded = ref(false);

export function createSearchActionButton(onClick: () => void): VNode {
  return h(
    "button",
    {
      type: "button",
      class: HEADER_ICON_BTN_CLASS,
      "aria-label": "Search messages",
      title: "Search messages (Ctrl+F)",
      onClick,
    },
    [h(Search, { class: "h-4 w-4", "aria-hidden": "true" })],
  );
}

/**
 * Module-global UI state for the chat search bar. Pages that own a chat
 * thread push their query/matches here via `useChatSearch`, and `App.vue`
 * binds a single `<ChatSearchBar>` to these fields. The `onUpdateQuery` /
 * `onNext` / `onPrev` / `onClose` callbacks are owned by the most recently
 * activated `useChatSearch` instance (last-writer-wins); the active instance
 * is tracked in `headerSearchOwners` so multiple pages can coexist under
 * `<KeepAlive>` without trampling each other on unmount.
 */
type HeaderSearchOwner = {
  id: symbol;
  onUpdateQuery: (v: string) => void;
  onNext: () => void;
  onPrev: () => void;
  onClose: () => void;
};

export const headerSearchState = reactive({
  query: "",
  matchCount: 0,
  currentIndex: 0,
  onUpdateQuery: (_v: string) => {},
  onNext: () => {},
  onPrev: () => {},
  onClose: () => {},
});

const headerSearchOwners: HeaderSearchOwner[] = [];
const noopOwner: HeaderSearchOwner = {
  id: Symbol("noop"),
  onUpdateQuery: (_v: string) => {},
  onNext: () => {},
  onPrev: () => {},
  onClose: () => {},
};

function applyOwner(owner: HeaderSearchOwner) {
  headerSearchState.onUpdateQuery = owner.onUpdateQuery;
  headerSearchState.onNext = owner.onNext;
  headerSearchState.onPrev = owner.onPrev;
  headerSearchState.onClose = owner.onClose;
}

export function registerHeaderSearchOwner(owner: Omit<HeaderSearchOwner, "id">): symbol {
  const id = Symbol("headerSearchOwner");
  const entry = { id, ...owner };
  headerSearchOwners.push(entry);
  applyOwner(entry);
  return id;
}

/**
 * Promote an existing owner to the head of the stack (e.g. when the user
 * focuses a page whose chat thread should own the search bar).
 */
export function focusHeaderSearchOwner(id: symbol) {
  const idx = headerSearchOwners.findIndex((o) => o.id === id);
  if (idx < 0) return;
  const [entry] = headerSearchOwners.splice(idx, 1);
  headerSearchOwners.push(entry);
  applyOwner(entry);
}

export function unregisterHeaderSearchOwner(id: symbol) {
  const idx = headerSearchOwners.findIndex((o) => o.id === id);
  if (idx < 0) return;
  const wasHead = idx === headerSearchOwners.length - 1;
  headerSearchOwners.splice(idx, 1);
  if (wasHead) {
    const next = headerSearchOwners[headerSearchOwners.length - 1] ?? noopOwner;
    applyOwner(next);
  }
}
