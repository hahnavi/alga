import { ref } from "vue";
import { api, type UserInfo } from "@/lib/api";
import { useAuthStore } from "@/stores/auth";
import { getErrorMessage } from "@/lib/error";

/**
 * Reactive list of users with an explicit `loadUsers` call. Gate the network
 * call with a permission check at the call site (`useUsersIfPermitted` does
 * this for you; `useUsers` does not).
 */
export function useUsers() {
  const users = ref<UserInfo[]>([]);
  const loading = ref(false);
  const error = ref("");

  async function loadUsers() {
    loading.value = true;
    error.value = "";
    try {
      const list = await api.getUsers();
      users.value = Array.isArray(list) ? list : [];
    } catch (err) {
      const msg = getErrorMessage(err, "Failed to load users");
      error.value = msg;
      // No toast here — callers may want to control the user-visible error.
      // The ref is exposed so they can render `error` in the page.
    } finally {
      loading.value = false;
    }
  }

  return { users, loading, error, loadUsers };
}

/**
 * Same as `useUsers` but skips the network call entirely when the current
 * user lacks the given permission. Use this for "lookup" user lists in pages
 * where `/api/v1/users` is only needed for display resolution (e.g. assignee
 * dropdowns).
 */
export function useUsersIfPermitted(permission: string) {
  const { users, loadUsers: loadUsersUnguarded } = useUsers();
  const auth = useAuthStore();

  async function loadUsers() {
    if (!auth.hasPermission(permission)) {
      users.value = [];
      return;
    }
    await loadUsersUnguarded();
  }

  return { users, loadUsers };
}
