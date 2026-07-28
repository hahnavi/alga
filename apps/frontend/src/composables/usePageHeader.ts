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
export function usePageHeader(getSpec: () => PageHeaderSpec | null): void {
  let active = true;

  function sync() {
    if (!active) return;
    const spec = getSpec();
    if (spec) {
      setPageHeader(spec.title, spec.badges, spec.options);
    } else {
      clearPageHeader();
    }
  }

  watchEffect(sync);

  onActivated(() => {
    active = true;
    sync();
  });

  onDeactivated(() => {
    active = false;
    clearPageHeader();
  });

  onBeforeUnmount(() => {
    active = false;
    clearPageHeader();
  });
}
