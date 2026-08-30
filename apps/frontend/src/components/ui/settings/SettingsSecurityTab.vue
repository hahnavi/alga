<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { Laptop } from "@lucide/vue";
import { api, type SessionRow } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { validatePassword } from "@/lib/validators";
import { formatTimeAgo } from "@/lib/time";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";

const { push } = useToast();

const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");

const error = ref("");
const submitting = ref(false);

const sessions = ref<SessionRow[]>([]);
const sessionsLoading = ref(false);
const sessionsError = ref("");
const revokingId = ref("");
const revokingAll = ref(false);

async function loadSessions() {
  sessionsError.value = "";
  sessionsLoading.value = true;
  try {
    const res = await api.getSessions();
    sessions.value = res.items;
  } catch (err) {
    sessionsError.value = getErrorMessage(err, "Failed to load sessions");
  } finally {
    sessionsLoading.value = false;
  }
}

onMounted(() => {
  void loadSessions();
});

async function revokeSession(id: string) {
  revokingId.value = id;
  try {
    await api.revokeSession(id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
    push("Session revoked.", "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to revoke session"), "error");
  } finally {
    revokingId.value = "";
  }
}

async function revokeOtherSessions() {
  revokingAll.value = true;
  try {
    const res = await api.revokeOtherSessions();
    sessions.value = sessions.value.filter((s) => s.current);
    push(`Signed out of ${res.revoked} other session${res.revoked === 1 ? "" : "s"}.`, "success");
  } catch (err) {
    push(getErrorMessage(err, "Failed to sign out of other sessions"), "error");
  } finally {
    revokingAll.value = false;
  }
}

function deviceSummary(userAgent: string): string {
  const ua = userAgent.toLowerCase();
  const os = ua.includes("windows")
    ? "Windows"
    : ua.includes("mac") || ua.includes("iphone") || ua.includes("ipad")
      ? "macOS / iOS"
      : ua.includes("android")
        ? "Android"
        : ua.includes("linux")
          ? "Linux"
          : "Unknown OS";
  const browser = ua.includes("edg/")
    ? "Edge"
    : ua.includes("chrome") && !ua.includes("chromium")
      ? "Chrome"
      : ua.includes("safari") && !ua.includes("chrome")
        ? "Safari"
        : ua.includes("firefox")
          ? "Firefox"
          : "Browser";
  return `${browser} · ${os}`;
}

// Client-side preview using the shared password policy so users get the same
// feedback the server will eventually enforce. The server is still the
// source of truth — the policy may evolve after this build.
const passwordCheck = computed(() => validatePassword(newPassword.value));
const passwordError = computed(() => {
  if (!newPassword.value) return "";
  if (passwordCheck.value.valid) return "";
  return passwordCheck.value.error;
});
const confirmError = computed(() => {
  if (!confirmPassword.value) return "";
  if (newPassword.value !== confirmPassword.value) {
    return "New password confirmation does not match.";
  }
  return "";
});

async function changePassword() {
  error.value = "";
  if (!currentPassword.value || !newPassword.value || !confirmPassword.value) {
    error.value = "All password fields are required.";
    return;
  }
  if (passwordError.value) {
    error.value = passwordError.value;
    return;
  }
  if (confirmError.value) {
    error.value = confirmError.value;
    return;
  }

  submitting.value = true;
  try {
    await api.changePassword(currentPassword.value, newPassword.value);
    push("Password updated successfully.", "success");
    currentPassword.value = "";
    newPassword.value = "";
    confirmPassword.value = "";
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to change password");
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Change password</h3>
      <p class="text-xs text-[var(--text-muted)]">
        Changing your password signs you out everywhere, including this session.
      </p>
    </header>
    <div class="space-y-1.5">
      <FormLabel for="current-password">Current password</FormLabel>
      <Input
        id="current-password"
        v-model="currentPassword"
        type="password"
        autocomplete="current-password"
      />
    </div>
    <div class="space-y-1.5">
      <FormLabel for="new-password">New password</FormLabel>
      <Input
        id="new-password"
        v-model="newPassword"
        type="password"
        autocomplete="new-password"
        :error="passwordError"
      />
    </div>
    <div class="space-y-1.5">
      <FormLabel for="confirm-password">Confirm new password</FormLabel>
      <Input
        id="confirm-password"
        v-model="confirmPassword"
        type="password"
        autocomplete="new-password"
        :error="confirmError"
      />
    </div>
    <p v-if="error" class="text-xs text-[var(--text-error)]" role="alert">{{ error }}</p>
    <div class="flex justify-end">
      <Button :loading="submitting" @click="changePassword">Update password</Button>
    </div>
  </Card>

  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Active sessions</h3>
      <p class="text-xs text-[var(--text-muted)]">
        Devices currently signed in to your account. Revoking a session signs it out immediately.
      </p>
    </header>

    <LoadingSpinner v-if="sessionsLoading" centered />

    <p v-else-if="sessionsError" class="text-xs text-[var(--text-error)]" role="alert">
      {{ sessionsError }}
    </p>

    <template v-else>
      <ul class="divide-y divide-[var(--border-primary)]">
        <li
          v-for="sess in sessions"
          :key="sess.id"
          class="flex flex-wrap items-center justify-between gap-3 py-3 first:pt-0 last:pb-0"
        >
          <div class="flex min-w-0 items-start gap-3">
            <span
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[rgb(148_163_184/0.1)]"
            >
              <Laptop class="h-4 w-4 text-[var(--text-muted)]" />
            </span>
            <div class="min-w-0">
              <p class="text-sm text-[var(--text-primary)]">
                {{ deviceSummary(sess.user_agent) }}
                <span v-if="sess.current" class="badge-green ml-2">This device</span>
              </p>
              <p class="text-xs text-[var(--text-muted)]">
                {{ sess.ip }} · active {{ formatTimeAgo(sess.last_used_at) }} · signed in
                {{ formatTimeAgo(sess.created_at) }}
              </p>
            </div>
          </div>
          <Button
            v-if="!sess.current"
            variant="outline"
            size="sm"
            :loading="revokingId === sess.id"
            @click="revokeSession(sess.id)"
          >
            Revoke
          </Button>
        </li>
      </ul>

      <div
        v-if="sessions.some((s) => !s.current)"
        class="flex justify-end border-t border-[var(--border-primary)] pt-4"
      >
        <Button variant="outline" :loading="revokingAll" @click="revokeOtherSessions">
          Sign out of other sessions
        </Button>
      </div>
    </template>
  </Card>
</template>
