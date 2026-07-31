<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { api } from "@/lib/api";
import { Mail } from "@lucide/vue";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
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

onMounted(() => {
  document.getElementById("forgot-email")?.focus();
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
  <div>
    <h1 class="mb-6 text-center text-xl font-semibold">Reset Password</h1>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="submitted" class="space-y-4">
      <p class="text-center text-sm text-[var(--text-secondary)]">
        If an account exists with that email, a reset link has been sent.
      </p>
      <Button
        variant="primary"
        class="h-11 w-full text-sm font-semibold"
        @click="router.push('/login')"
      >
        Back to sign in
      </Button>
    </div>

    <form v-else class="space-y-4" @submit.prevent="handleSubmit">
      <div>
        <FormLabel for="forgot-email" required>Email</FormLabel>
        <div class="group relative">
          <Mail
            class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)] transition-colors group-focus-within:text-[var(--accent)]"
          />
          <Input
            id="forgot-email"
            v-model="email"
            type="email"
            autocomplete="email"
            required
            placeholder="you@company.com"
            :error="emailError"
            class="h-11 pl-9"
            @blur="touched = true"
          />
        </div>
      </div>
      <Button
        type="submit"
        variant="primary"
        class="h-11 w-full text-sm font-semibold"
        :loading="loading"
      >
        Send reset link
      </Button>
      <p class="text-center text-sm">
        <router-link to="/login" class="text-[var(--color-primary)] hover:underline">
          Back to sign in
        </router-link>
      </p>
    </form>
  </div>
</template>
