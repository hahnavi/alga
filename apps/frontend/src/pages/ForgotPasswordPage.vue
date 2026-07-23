<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import { api } from "@/lib/api";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import AuthLayout from "@/components/ui/AuthLayout.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import FormLabel from "@/components/ui/FormLabel.vue";

defineOptions({ name: "ForgotPasswordPage" });

const router = useRouter();

const email = ref("");
const error = ref("");
const loading = ref(false);
const submitted = ref(false);
const touched = ref(false);

const emailError = computed(() => {
  if (!touched.value) return "";
  if (!email.value.trim()) return "Email is required";
  return "";
});

const formValid = computed(() => {
  return email.value.trim();
});

async function handleSubmit() {
  touched.value = true;
  if (!formValid.value) return;

  error.value = "";
  loading.value = true;
  try {
    await api.forgotPassword(email.value);
    submitted.value = true;
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to request password reset");
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <AuthLayout>
    <h1 class="mb-6 text-center text-xl font-semibold">Reset Password</h1>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="submitted" class="space-y-4">
      <p class="text-center text-sm text-[var(--text-secondary)]">
        If an account exists with that email, a reset link has been sent.
      </p>
      <Button class="w-full" @click="router.push('/login')">Back to sign in</Button>
    </div>

    <form v-else class="space-y-4" @submit.prevent="handleSubmit">
      <div>
        <FormLabel for="forgot-email" required>Email</FormLabel>
        <Input
          id="forgot-email"
          v-model="email"
          type="email"
          autocomplete="email"
          required
          :error="emailError"
          @blur="touched = true"
        />
      </div>
      <Button type="submit" class="w-full" :loading="loading" :disabled="!formValid">
        Send reset link
      </Button>
      <p class="text-center text-sm">
        <router-link to="/login" class="text-[var(--color-primary)] hover:underline">
          Back to sign in
        </router-link>
      </p>
    </form>
  </AuthLayout>
</template>
