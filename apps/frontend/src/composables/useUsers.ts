import { ref } from "vue";
import { api, type UserInfo } from "@/lib/api";
import { useAuthStore } from "@/stores/auth";

/**
 * Reactive list of users with an explicit `loadUsers` call. Gate the network
 * call with a permission check at the call site (`useUsersIfPermitted` does
 * this for you; `useUsers` does not).
 */
export function useUsers() {
  const users = ref<UserInfo[]>([]);

  async function loadUsers() {
    try {
      const list = await api.getUsers();
      users.value = Array.isArray(list) ? list : [];
    } catch {
      users.value = [];
    }
  }

  return { users, loadUsers };
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
