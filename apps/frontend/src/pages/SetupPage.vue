<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed } from "vue";
import { useRouter, useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api";
import { validatePassword } from "@/lib/validators";
import { Eye, EyeOff } from "@lucide/vue";
import Button from "@/components/ui/Button.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import AuthLayout from "@/components/ui/AuthLayout.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";

defineOptions({ name: "SetupPage" });

const router = useRouter();
const route = useRoute();
const auth = useAuthStore();

const fullName = ref("");
const email = ref("");
const password = ref("");
const confirmPassword = ref("");
const error = ref("");
const loading = ref(false);
const showPassword = ref(false);
const showConfirmPassword = ref(false);
const touched = ref({ fullName: false, email: false, password: false, confirmPassword: false });

const passwordValidation = computed(() => validatePassword(password.value));
const passwordsMatch = computed(
  () => password.value === confirmPassword.value && confirmPassword.value.length > 0,
);

const emailError = computed(() => {
  if (!touched.value.email) return "";
  if (!email.value.trim()) return "Email is required";
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim())) return "Invalid email address";
  return "";
});

const fullNameError = computed(() => {
  if (!touched.value.fullName) return "";
  if (!fullName.value.trim()) return "Full name is required";
  return "";
});

const passwordError = computed(() => {
  if (!touched.value.password) return "";
  if (!password.value) return "Password is required";
  if (!passwordValidation.value.valid) return passwordValidation.value.error;
  return "";
});

const confirmPasswordError = computed(() => {
  if (!touched.value.confirmPassword) return "";
  if (!confirmPassword.value) return "Confirm password is required";
  if (!passwordsMatch.value) return "Passwords do not match";
  return "";
});

const formValid = computed(() => {
  return (
    fullName.value.trim() &&
    email.value.trim() &&
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim()) &&
    passwordValidation.value.valid &&
    passwordsMatch.value
  );
});

function handleBlur(field: keyof typeof touched.value) {
  touched.value[field] = true;
}

async function handleSetup() {
  touched.value = { fullName: true, email: true, password: true, confirmPassword: true };
  if (!formValid.value) return;

  error.value = "";
  loading.value = true;
  try {
    const res = await api.setup({
      email: email.value.trim(),
      password: password.value,
      full_name: fullName.value.trim(),
    });
    if (res.csrf_token) {
      api.setCSRFToken(res.csrf_token);
    }
    // Setup is complete; fetch real user object (with permissions) from /auth/me
    auth.needsSetup = false;
    await auth.fetchCurrentUser();
    await auth.refreshOnboardingStatus();
    router.push(safeRedirectTarget(route.query.redirect));
  } catch (err) {
    error.value = getErrorMessage(err, "Setup failed");
  } finally {
    loading.value = false;
  }
}

function safeRedirectTarget(redirect: unknown): string {
  if (typeof redirect === "string" && redirect.startsWith("/")) {
    return redirect;
  }
  return "/";
}
</script>

<template>
  <AuthLayout max-width="max-w-md">
    <div class="mb-6 text-center">
      <div
        class="mx-auto mb-4 flex h-11 w-11 items-center justify-center rounded-xl bg-[var(--accent)]"
      >
        <svg
          class="h-6 w-6 text-white"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" />
          <path d="m9 12 2 2 4-4" />
        </svg>
      </div>
      <h1 class="text-xl font-semibold">Welcome to Alga</h1>
      <p class="mt-1 text-sm text-[var(--text-secondary)]">
        Create your admin account to get started
      </p>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <form class="space-y-4" @submit.prevent="handleSetup">
      <div>
        <FormLabel for="setup-full-name" required>Full Name</FormLabel>
        <Input
          id="setup-full-name"
          v-model="fullName"
          type="text"
          autocomplete="name"
          required
          placeholder="Your name"
          :error="fullNameError"
          @blur="handleBlur('fullName')"
        />
      </div>
      <div>
        <FormLabel for="setup-email" required>Email</FormLabel>
        <Input
          id="setup-email"
          v-model="email"
          type="email"
          autocomplete="email"
          required
          placeholder="you@company.com"
          :error="emailError"
          @blur="handleBlur('email')"
        />
      </div>
      <div>
        <FormLabel for="setup-password" required>Password</FormLabel>
        <div class="relative">
          <Input
            id="setup-password"
            v-model="password"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="new-password"
            required
            :error="passwordError"
            class="pr-10"
            @blur="handleBlur('password')"
          />
          <button
            type="button"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
            :aria-label="showPassword ? 'Hide password' : 'Show password'"
            @click="showPassword = !showPassword"
          >
            <EyeOff v-if="showPassword" class="h-4 w-4" />
            <Eye v-else class="h-4 w-4" />
          </button>
        </div>
        <p v-if="passwordError" class="mt-1 text-xs text-[var(--text-error)]">
          {{ passwordError }}
        </p>
        <ul class="mt-1 space-y-0.5 text-xs text-[var(--text-muted)]">
          <li :class="passwordValidation.checks.length ? 'text-emerald-500' : ''">
            At least 8 characters
          </li>
          <li :class="passwordValidation.checks.uppercase ? 'text-emerald-500' : ''">
            One uppercase letter
          </li>
          <li :class="passwordValidation.checks.lowercase ? 'text-emerald-500' : ''">
            One lowercase letter
          </li>
          <li :class="passwordValidation.checks.digit ? 'text-emerald-500' : ''">One digit</li>
          <li :class="passwordValidation.checks.special ? 'text-emerald-500' : ''">
            One special character
          </li>
        </ul>
      </div>
      <div>
        <FormLabel for="setup-confirm-password" required>Confirm Password</FormLabel>
        <div class="relative">
          <Input
            id="setup-confirm-password"
            v-model="confirmPassword"
            :type="showConfirmPassword ? 'text' : 'password'"
            autocomplete="new-password"
            required
            :error="confirmPasswordError"
            class="pr-10"
            @blur="handleBlur('confirmPassword')"
          />
          <button
            type="button"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
            :aria-label="showConfirmPassword ? 'Hide password' : 'Show password'"
            @click="showConfirmPassword = !showConfirmPassword"
          >
            <EyeOff v-if="showConfirmPassword" class="h-4 w-4" />
            <Eye v-else class="h-4 w-4" />
          </button>
        </div>
        <p v-if="confirmPasswordError" class="mt-1 text-xs text-[var(--text-error)]">
          {{ confirmPasswordError }}
        </p>
      </div>
      <Button type="submit" class="w-full" :loading="loading" :disabled="!formValid">
        Create Admin Account
      </Button>
    </form>
  </AuthLayout>
</template>
