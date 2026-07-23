import { computed } from "vue";
import { useAuthStore } from "@/stores/auth";

type EntityPermissionsOptions = {
  /** Map an action name to the actual permission string. Defaults to `${prefix}:${action}`. */
  actions?: Record<string, string>;
};

export function useEntityPermissions(prefix: string, options: EntityPermissionsOptions = {}) {
  const auth = useAuthStore();
  const resolve = (action: string) => options.actions?.[action] ?? `${prefix}:${action}`;

  const canRead = computed(() => auth.hasPermission(resolve("read")));
  const canWrite = computed(() => auth.hasPermission(resolve("write")));
  const canDelete = computed(() => auth.hasPermission(resolve("delete")));
  const canCommand = computed(() => auth.hasPermission(resolve("command")));

  function can(...perms: string[]) {
    return perms.every((p) => auth.hasPermission(resolve(p)));
  }

  return { canRead, canWrite, canDelete, canCommand, can };
}
