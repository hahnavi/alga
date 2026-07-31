<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed, onMounted } from "vue";
import { useRouter, useRoute } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { api, type OIDCProviderPublic } from "@/lib/api";
import { safeRedirectTarget } from "@/lib/redirect";
import { resolveOAuthErrorMessage, unknownOAuthErrorMessage } from "@/lib/oauthErrors";
import { Eye, EyeOff, Mail, Lock, ArrowRight } from "@lucide/vue";
import Button from "@/components/ui/Button.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";

defineOptions({ name: "LoginPage" });

const auth = useAuthStore();
const router = useRouter();
const route = useRoute();

const email = ref("");
const password = ref("");
const error = ref("");
const loading = ref(false);
const showPassword = ref(false);
const touched = ref({ email: false, password: false });
const googleEnabled = ref(false);
const slackEnabled = ref(false);
const oidcProviders = ref<OIDCProviderPublic[]>([]);

const emailError = computed(() => {
  if (!touched.value.email) return "";
  if (!email.value.trim()) return "Email is required";
  return "";
});

const passwordError = computed(() => {
  if (!touched.value.password) return "";
  if (!password.value) return "Password is required";
  return "";
});

const formValid = computed(() => {
  return email.value.trim() && password.value;
});

const showOAuthSection = computed(() => {
  return googleEnabled.value || slackEnabled.value || oidcProviders.value.length > 0;
});

function handleBlur(field: "email" | "password") {
  touched.value[field] = true;
}

async function handleLogin() {
  touched.value = { email: true, password: true };
  if (!formValid.value) return;

  error.value = "";
  loading.value = true;
  try {
    await auth.login(email.value, password.value);
    router.push(safeRedirectTarget(route.query.redirect));
  } catch (err) {
    error.value = getErrorMessage(err, "Login failed");
  } finally {
    loading.value = false;
  }
}

function handleGoogleSignIn() {
  window.location.href = "/api/v1/auth/google";
}

function handleSlackSignIn() {
  window.location.href = "/api/v1/auth/slack";
}

function handleOIDCSignIn(providerId: string) {
  window.location.href = "/api/v1/auth/oidc/" + providerId + "/authorize";
}

onMounted(async () => {
  document.getElementById("login-email")?.focus();

  const googleParam = route.query.google as string;
  const slackParam = route.query.slack as string;
  const errorParam = route.query.error;

  if (googleParam === "success" || slackParam === "success") {
    try {
      await auth.fetchCurrentUser();
      router.replace(safeRedirectTarget(route.query.redirect));
      return;
    } catch {
      error.value = `Failed to complete ${slackParam === "success" ? "Slack" : "Google"} Sign-In. Please try again.`;
    }
  }

  if (errorParam) {
    const msg = resolveOAuthErrorMessage(errorParam);
    if (msg) {
      error.value = msg;
    } else {
      console.warn("Ignored unrecognized OAuth error key:", errorParam);
      error.value = unknownOAuthErrorMessage();
    }
  }

  try {
    const res = await api.isGoogleAuthEnabled();
    googleEnabled.value = res.enabled;
  } catch {
    googleEnabled.value = false;
  }

  try {
    const res = await api.isSlackAuthEnabled();
    slackEnabled.value = res.enabled;
  } catch {
    slackEnabled.value = false;
  }

  try {
    oidcProviders.value = await api.listPublicOIDCProviders();
  } catch {
    oidcProviders.value = [];
  }

  if (googleParam || slackParam || errorParam) {
    await router.replace({ query: {} });
  }
});
</script>

<template>
  <div>
    <div class="auth-rise mb-8">
      <p class="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-[var(--accent)]">
        Sign in
      </p>
      <h1 class="mt-2 text-[28px] font-semibold leading-tight tracking-tight">Welcome back</h1>
      <p class="mt-1.5 text-sm text-[var(--text-secondary)]">Access the Alga Ops Console</p>
    </div>

    <div class="auth-rise [animation-delay:60ms]">
      <ErrorBanner :message="error" class="mb-4" />
    </div>

    <form
      class="auth-rise space-y-5 [animation-delay:120ms]"
      novalidate
      @submit.prevent="handleLogin"
    >
      <div>
        <FormLabel for="login-email" required>Email</FormLabel>
        <div class="group relative">
          <Mail
            class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)] transition-colors group-focus-within:text-[var(--accent)]"
          />
          <Input
            id="login-email"
            v-model="email"
            type="email"
            autocomplete="email"
            required
            placeholder="you@company.com"
            :error="emailError"
            :aria-describedby="emailError ? 'login-email-error' : undefined"
            class="h-11 pl-9"
            @blur="handleBlur('email')"
          />
        </div>
        <p v-if="emailError" id="login-email-error" class="mt-1 text-xs text-[var(--text-error)]">
          {{ emailError }}
        </p>
      </div>
      <div>
        <div class="flex items-baseline justify-between">
          <FormLabel for="login-password" required>Password</FormLabel>
          <router-link
            to="/forgot-password"
            class="text-xs font-medium text-[var(--color-primary)] transition-colors hover:text-[var(--accent-strong)] hover:underline"
          >
            Forgot password?
          </router-link>
        </div>
        <div class="group relative">
          <Lock
            class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--text-muted)] transition-colors group-focus-within:text-[var(--accent)]"
          />
          <Input
            id="login-password"
            v-model="password"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="current-password"
            required
            :error="passwordError"
            :aria-describedby="passwordError ? 'login-password-error' : undefined"
            class="h-11 pl-9 pr-10"
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
        <p
          v-if="passwordError"
          id="login-password-error"
          class="mt-1 text-xs text-[var(--text-error)]"
        >
          {{ passwordError }}
        </p>
      </div>
      <Button
        type="submit"
        variant="primary"
        class="group h-11 w-full text-sm font-semibold"
        :loading="loading"
      >
        Sign in
        <ArrowRight class="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
      </Button>
    </form>

    <div v-if="showOAuthSection" class="auth-rise [animation-delay:180ms]">
      <div class="relative my-6">
        <div class="absolute inset-0 flex items-center">
          <div class="w-full border-t border-[var(--border-primary)]"></div>
        </div>
        <div class="relative flex justify-center text-xs">
          <span class="bg-[var(--bg-primary)] px-2 text-[var(--text-muted)]">
            or continue with
          </span>
        </div>
      </div>

      <div class="space-y-2.5">
        <Button
          v-if="googleEnabled"
          variant="outline"
          class="h-10 w-full"
          @click="handleGoogleSignIn"
        >
          <svg class="mr-2 h-4 w-4" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path
              d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"
              fill="#4285F4"
            />
            <path
              d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
              fill="#34A853"
            />
            <path
              d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
              fill="#FBBC05"
            />
            <path
              d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
              fill="#EA4335"
            />
          </svg>
          Sign in with Google
        </Button>

        <Button
          v-if="slackEnabled"
          variant="outline"
          class="h-10 w-full"
          @click="handleSlackSignIn"
        >
          <svg class="mr-2 h-4 w-4" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path
              d="M5.042 15.165a2.528 2.528 0 0 1-2.52 2.523A2.528 2.528 0 0 1 0 15.165a2.527 2.527 0 0 1 2.522-2.52h2.52v2.52zm1.271 0a2.527 2.527 0 0 1 2.521-2.52 2.527 2.527 0 0 1 2.521 2.52v6.313A2.528 2.528 0 0 1 8.834 24a2.528 2.528 0 0 1-2.521-2.522v-6.313z"
              fill="#E01E5A"
            />
            <path
              d="M8.834 5.042a2.528 2.528 0 0 1-2.521-2.52A2.528 2.528 0 0 1 8.834 0a2.528 2.528 0 0 1 2.521 2.522v2.52H8.834zm0 1.271a2.527 2.527 0 0 1 2.521 2.521 2.527 2.527 0 0 1-2.521 2.521H2.522A2.528 2.528 0 0 1 0 8.834a2.527 2.527 0 0 1 2.522-2.521h6.312z"
              fill="#36C5F0"
            />
            <path
              d="M18.956 8.834a2.528 2.528 0 0 1 2.522-2.521A2.528 2.528 0 0 1 24 8.834a2.528 2.528 0 0 1-2.522 2.521h-2.522V8.834zm-1.27 0a2.527 2.527 0 0 1-2.523 2.521 2.527 2.527 0 0 1-2.52-2.521V2.522A2.527 2.527 0 0 1 15.163 0a2.528 2.528 0 0 1 2.522 2.522v6.312z"
              fill="#2EB67D"
            />
            <path
              d="M15.163 18.956a2.528 2.528 0 0 1 2.522 2.522A2.528 2.528 0 0 1 15.163 24a2.527 2.527 0 0 1-2.52-2.522v-2.522h2.52zm0-1.27a2.527 2.527 0 0 1-2.52-2.523 2.527 2.527 0 0 1 2.52-2.52h6.315A2.528 2.528 0 0 1 24 15.163a2.528 2.528 0 0 1-2.522 2.522h-6.315z"
              fill="#ECB22E"
            />
          </svg>
          Sign in with Slack
        </Button>

        <Button
          v-for="provider in oidcProviders"
          :key="provider.id"
          variant="outline"
          class="h-10 w-full"
          @click="handleOIDCSignIn(provider.id)"
        >
          <svg
            class="mr-2 h-4 w-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path
              d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"
            />
          </svg>
          Sign in with {{ provider.name }}
        </Button>
      </div>
    </div>
  </div>
</template>
