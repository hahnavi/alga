<script setup lang="ts">
import { ref, computed } from "vue";
import { api } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { validatePassword } from "@/lib/validators";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");

const message = ref("");
const error = ref("");
const submitting = ref(false);

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
  message.value = "";
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
    message.value = "Password updated successfully.";
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
  <h2 class="text-sm font-semibold text-[var(--text-primary)]">Change password</h2>
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
  <p v-if="message" class="text-xs text-[var(--text-success)]">{{ message }}</p>
  <Button :loading="submitting" @click="changePassword">Update password</Button>
</template>
