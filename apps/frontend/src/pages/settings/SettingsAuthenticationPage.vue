<script setup lang="ts">
import { computed, onMounted } from "vue";
import { ShieldCheck } from "@lucide/vue";
import Card from "@/components/ui/Card.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import Input from "@/components/ui/Input.vue";
import Switch from "@/components/ui/Switch.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import SkeletonRows from "@/components/ui/SkeletonRows.vue";
import SystemFormFooter from "@/components/system/SystemFormFooter.vue";
import { useSystemConfigForm, type SystemForm } from "@/composables/useSystemConfigForm";

defineOptions({ name: "SettingsAuthenticationPage" });

const FIELDS: ReadonlyArray<keyof SystemForm> = [
  "google_oauth_enabled",
  "google_client_id",
  "google_client_secret",
  "google_oauth_redirect_url",
];

const {
  form,
  original,
  loading,
  error,
  loadConfig,
  saving,
  saveError,
  isDirty,
  changedFields,
  discard,
  save,
} = useSystemConfigForm();

const dirty = computed(() => isDirty(FIELDS));

onMounted(() => {
  void loadConfig();
});
</script>

<template>
  <section class="px-4 py-4 md:px-6 md:py-6">
    <div class="mx-auto max-w-2xl space-y-4 md:space-y-6">
      <ErrorBanner :message="error" />

      <SkeletonRows v-if="loading" :count="4" />

      <template v-else-if="original">
        <p class="text-sm text-[var(--text-muted)]">
          Sign-in methods for your workspace. Secrets are stored encrypted and never displayed.
        </p>

        <ErrorBanner :message="saveError" />

        <Card class="space-y-4">
          <header class="flex items-start gap-3">
            <span
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] text-[var(--text-muted)]"
            >
              <ShieldCheck class="h-4 w-4" />
            </span>
            <div class="min-w-0">
              <h3 class="text-sm font-semibold text-[var(--text-primary)]">Google OAuth</h3>
              <p class="text-xs text-[var(--text-muted)]">
                Allow users to sign in with their Google account.
              </p>
            </div>
          </header>

          <label
            class="flex items-center justify-between gap-3 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2.5"
          >
            <span class="min-w-0">
              <span class="block text-sm font-medium text-[var(--text-primary)]">
                Enable Google Sign-In
              </span>
              <span class="block text-xs text-[var(--text-muted)]">
                Show the Google login option on the sign-in page.
              </span>
            </span>
            <Switch v-model="form.google_oauth_enabled" />
          </label>

          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <FormLabel for="settings-google-client-id">Client ID</FormLabel>
              <Input
                id="settings-google-client-id"
                v-model="form.google_client_id"
                placeholder="xxxxxxxx.apps.googleusercontent.com"
              />
              <p class="mt-1 text-xs text-[var(--text-muted)]">
                Google OAuth 2.0 client ID from the Google Cloud console.
              </p>
            </div>
            <div>
              <FormLabel for="settings-google-redirect">Redirect URL</FormLabel>
              <Input
                id="settings-google-redirect"
                v-model="form.google_oauth_redirect_url"
                placeholder="https://alga.example.com/api/v1/auth/google/callback"
              />
              <p class="mt-1 text-xs text-[var(--text-muted)]">
                Must match the authorized redirect URI in Google Cloud.
              </p>
            </div>
            <div>
              <FormLabel for="settings-google-secret">Client Secret</FormLabel>
              <Input
                id="settings-google-secret"
                v-model="form.google_client_secret"
                type="password"
                :placeholder="
                  form.google_client_secret_set ? '•••••••• (configured)' : 'Enter client secret'
                "
              />
              <p class="mt-1 text-xs text-[var(--text-muted)]">
                <span v-if="form.google_client_secret_set"
                  >A client secret is configured. Leave blank to keep it.</span
                >
                <span v-else>Stored encrypted; never displayed after saving.</span>
              </p>
            </div>
          </div>
        </Card>

        <SystemFormFooter
          :dirty="dirty"
          :saving="saving"
          @save="save(changedFields(FIELDS), 'Authentication')"
          @discard="discard(FIELDS)"
        />
      </template>
    </div>
  </section>
</template>
