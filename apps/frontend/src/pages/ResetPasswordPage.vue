<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "@/lib/api";
import { validatePassword } from "@/lib/validators";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

defineOptions({ name: "ResetPasswordPage" });

const route = useRoute();
const router = useRouter();

const token = computed(() => (route.query.token as string) || "");
const newPassword = ref("");
const confirmPassword = ref("");
const error = ref("");
const loading = ref(false);
const succeeded = ref(false);
const touched = ref({ password: false, confirm: false });

const hasToken = computed(() => !!token.value);

const passwordError = computed(() => {
  if (!touched.value.password) return "";
  if (!newPassword.value) return "Password is required";
  return validatePassword(newPassword.value).error;
});

const confirmError = computed(() => {
  if (!touched.value.confirm) return "";
  if (!confirmPassword.value) return "Please confirm your password";
  if (confirmPassword.value !== newPassword.value) return "Passwords do not match";
  return "";
});

const formValid = computed(() => {
  if (!newPassword.value || !confirmPassword.value) return false;
  if (confirmPassword.value !== newPassword.value) return false;
  return validatePassword(newPassword.value).valid;
});

function handleBlur(field: "password" | "confirm") {
  touched.value[field] = true;
}

onMounted(() => {
  document.getElementById("reset-password")?.focus();
});

async function handleSubmit() {
  touched.value = { password: true, confirm: true };
  if (!formValid.value || !token.value) return;

  error.value = "";
  loading.value = true;
  try {
    await api.resetPassword(token.value, newPassword.value);
    succeeded.value = true;
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to reset password");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div>
    <h1 class="mb-6 text-center text-xl font-semibold">Set New Password</h1>

    <ErrorBanner v-if="!hasToken" message="Invalid or missing reset token." class="mb-4" />
    <ErrorBanner v-else :message="error" class="mb-4" />

    <div v-if="succeeded" class="space-y-4">
      <p class="text-center text-sm text-[var(--text-secondary)]">
        Your password has been reset successfully.
      </p>
      <Button
        variant="primary"
        class="h-11 w-full text-sm font-semibold"
        @click="router.push('/login')"
      >
        Sign in
      </Button>
    </div>

    <form v-else-if="hasToken" class="space-y-4" @submit.prevent="handleSubmit">
      <div>
        <FormLabel for="reset-password" required>New password</FormLabel>
        <Input
          id="reset-password"
          v-model="newPassword"
          type="password"
          autocomplete="new-password"
          required
          :error="passwordError"
          class="h-11"
          @blur="handleBlur('password')"
        />
      </div>
      <div>
        <FormLabel for="reset-confirm" required>Confirm password</FormLabel>
        <Input
          id="reset-confirm"
          v-model="confirmPassword"
          type="password"
          autocomplete="new-password"
          required
          :error="confirmError"
          class="h-11"
          @blur="handleBlur('confirm')"
        />
      </div>
      <p class="text-xs text-[var(--text-secondary)]">
        Must be at least 8 characters with uppercase, lowercase, digit, and special character.
      </p>
      <Button
        type="submit"
        variant="primary"
        class="h-11 w-full text-sm font-semibold"
        :loading="loading"
        :disabled="!formValid"
      >
        Reset password
      </Button>
    </form>

    <p v-if="!succeeded" class="mt-4 text-center text-sm">
      <router-link to="/login" class="text-[var(--color-primary)] hover:underline">
        Back to sign in
      </router-link>
    </p>
  </div>
</template>
