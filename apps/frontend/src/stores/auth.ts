import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { api, type UserInfo } from "@/lib/api";

export const useAuthStore = defineStore("auth", () => {
  const user = ref<UserInfo | null>(null);
  const loading = ref(true);
  const onboardingCompleted = ref<boolean | null>(null);
  const needsSetup = ref<boolean | null>(null);

  const isAdmin = computed(() => user.value?.role === "admin");
  const isAuthenticated = computed(() => user.value !== null);

  /**
   * Permissions come straight from the server's `/auth/me` response
   * (populated by `rbac.AllPermissions` on the backend). When the
   * field is missing — older backend, or a transient load failure —
   * the permission check returns an empty set, which causes every
   * `hasPermission(...)` call to deny. This is the safer default for a
   * new RBAC string: an unknown permission is denied until the backend
   * catches up.
   */
  const permissions = computed<readonly string[]>(() => user.value?.permissions ?? []);

  function hasPermission(...perms: string[]): boolean {
    if (permissions.value.includes("*")) return true;
    return perms.some((p) => permissions.value.includes(p));
  }

  /**
   * True if the user holds ALL of the given permissions. Prefer this over
   * `hasPermission` when you specifically need AND semantics, since
   * `hasPermission` is variadic-OR for backward compatibility.
   */
  function hasAllPermissions(...perms: string[]): boolean {
    if (permissions.value.includes("*")) return true;
    return perms.every((p) => permissions.value.includes(p));
  }

  /**
   * True if the user holds AT LEAST ONE of the given permissions. Prefer this
   * over `hasPermission` for explicit OR semantics with two or more perms.
   */
  function hasAnyPermission(...perms: string[]): boolean {
    if (permissions.value.includes("*")) return true;
    return perms.some((p) => permissions.value.includes(p));
  }

  async function refreshOnboardingStatus() {
    try {
      const status = await api.getOnboardingStatus();
      onboardingCompleted.value = status.completed;
    } catch {
      onboardingCompleted.value = null;
    }
  }

  async function checkSetupStatus() {
    if (needsSetup.value !== null) return;
    try {
      const status = await api.getSetupStatus();
      needsSetup.value = status.needs_setup;
    } catch {
      needsSetup.value = null;
    }
  }

  async function markOnboardingCompleted() {
    // Optimistic local update for snappy UX; refresh from server so any
    // server-side gating (router guards, onboarding banner) sees the truth.
    onboardingCompleted.value = true;
    await refreshOnboardingStatus();
  }

  async function login(email: string, password: string) {
    const res = await api.login(email, password);
    user.value = res;
    await refreshOnboardingStatus();
  }

  async function logout() {
    try {
      await api.logout();
    } catch {
      // best effort — clear local state regardless
    }
    user.value = null;
    api.setCSRFToken(null);
    onboardingCompleted.value = null;
    needsSetup.value = null;
    sessionChecked.value = false;
  }

  let pendingFetch: Promise<void> | null = null;
  const sessionChecked = ref(false);

  async function fetchCurrentUser() {
    if (pendingFetch) return pendingFetch;
    pendingFetch = (async () => {
      loading.value = true;
      try {
        user.value = await api.getCurrentUser();
        await refreshOnboardingStatus();
        sessionChecked.value = true;
      } catch (err) {
        // Only treat "Unauthorized" (raised by api.ts on 401 outside of safe
        // callback paths) as an auth failure. Network errors and 5xx leave the
        // previous user intact so a transient blip doesn't force re-login.
        if (err instanceof Error && err.message === "Unauthorized") {
          user.value = null;
          api.setCSRFToken(null);
          sessionChecked.value = true;
        }
      } finally {
        loading.value = false;
        pendingFetch = null;
      }
    })();
    return pendingFetch;
  }

  async function refreshSession() {
    try {
      await api.refreshSession();
      return true;
    } catch {
      return false;
    }
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    return api.changePassword(currentPassword, newPassword);
  }

  async function changeEmail(password: string, email: string) {
    const result = await api.changeEmail(password, email);
    if (user.value) {
      user.value = { ...user.value, email };
    }
    return result;
  }

  async function updateProfile(fullName: string, phone: string, phoneCountry: string) {
    const result = await api.updateProfile(fullName, phone, phoneCountry);
    if (user.value) {
      user.value = { ...user.value, full_name: fullName, phone, phone_country: phoneCountry };
    }
    return result;
  }

  return {
    user,
    loading,
    onboardingCompleted,
    needsSetup,
    sessionChecked,
    isAdmin,
    isAuthenticated,
    permissions,
    hasPermission,
    hasAllPermissions,
    hasAnyPermission,
    checkSetupStatus,
    refreshOnboardingStatus,
    login,
    logout,
    fetchCurrentUser,
    refreshSession,
    changePassword,
    changeEmail,
    updateProfile,
    markOnboardingCompleted,
  };
});
