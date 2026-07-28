<script setup lang="ts">
import { ref } from "vue";
import { api } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");

const message = ref("");
const error = ref("");
const submitting = ref(false);

async function changePassword() {
  message.value = "";
  error.value = "";
  if (!currentPassword.value || !newPassword.value || !confirmPassword.value) {
    error.value = "All password fields are required.";
    return;
  }
  if (newPassword.value.length < 8) {
    error.value = "New password must be at least 8 characters.";
    return;
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = "New password confirmation does not match.";
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
  <p class="text-sm text-[var(--text-muted)]">Change password</p>
  <div class="space-y-1.5">
    <FormLabel>Current password</FormLabel>
    <Input v-model="currentPassword" type="password" />
  </div>
  <div class="space-y-1.5">
    <FormLabel>New password</FormLabel>
    <Input v-model="newPassword" type="password" />
  </div>
  <div class="space-y-1.5">
    <FormLabel>Confirm new password</FormLabel>
    <Input v-model="confirmPassword" type="password" />
  </div>
  <p v-if="error" class="text-xs text-[var(--text-error)]">{{ error }}</p>
  <p v-if="message" class="text-xs text-[var(--text-success)]">{{ message }}</p>
  <Button :disabled="submitting" @click="changePassword">
    {{ submitting ? "Updating..." : "Update password" }}
  </Button>
</template>
