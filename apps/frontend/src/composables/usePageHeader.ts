import { onActivated, onBeforeUnmount, onDeactivated, watchEffect } from "vue";
import {
  clearPageHeader,
  setPageHeader,
  type HeaderBadge,
  type PageHeaderOptions,
} from "@/lib/pageHeader";

export type PageHeaderSpec = {
  title: string;
  badges?: HeaderBadge[];
  options?: PageHeaderOptions;
};

/**
 * Manages the shell header lifecycle for the owning page component.
 *
 * Pass a getter that returns the desired header state (or `null` to clear).
 * The composable automatically:
 * - Sets the header on mount and KeepAlive re-activation
 * - Clears the header on KeepAlive deactivation and unmount
 * - Re-syncs whenever reactive dependencies accessed in the getter change
 * - Guards against writing the header while the page is deactivated
 *
 * This eliminates the stale-header bug class: pages no longer need to
 * manually call `clearPageHeader` in `onDeactivated`/`onBeforeUnmount`.
 */
// Tracks which usePageHeader instance last wrote the header. Under
// KeepAlive + Suspense the outgoing page's deactivate/unmount hooks can fire
// AFTER the incoming page's onActivated, so an unguarded clear would wipe the
// header the new page just set. Only the current owner may clear it.
let headerOwner: symbol | null = null;

export function usePageHeader(getSpec: () => PageHeaderSpec | null): void {
  const owner = Symbol("pageHeaderOwner");
  let active = true;

  function clearIfOwner() {
    if (headerOwner !== owner) return;
    headerOwner = null;
    clearPageHeader();
  }

  function sync() {
    if (!active) return;
    const spec = getSpec();
    if (spec) {
      headerOwner = owner;
      setPageHeader(spec.title, spec.badges, spec.options);
    } else {
      clearIfOwner();
    }
  }

  watchEffect(sync);

  onActivated(() => {
    active = true;
    sync();
  });

  onDeactivated(() => {
    active = false;
    clearIfOwner();
  });

  onBeforeUnmount(() => {
    active = false;
    clearIfOwner();
  });
}
