<script setup lang="ts">
import { getErrorMessage } from "@/lib/error";
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle,
  Copy,
  Eye,
  EyeOff,
  Link,
  MessageSquare,
  Rocket,
  Shield,
  Bot,
} from "@lucide/vue";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api";
import { validatePassword } from "@/lib/validators";
import { useClipboard } from "@/composables/useClipboard";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import AuthLayout from "@/components/ui/AuthLayout.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import Tabs, { type Tab } from "@/components/ui/Tabs.vue";

defineOptions({ name: "OnboardingPage" });

const auth = useAuthStore();
const router = useRouter();
const { copyToClipboard } = useClipboard();
const { push } = useToast();

const step = ref(0);
const totalSteps = 4;
const loading = ref(false);
const error = ref("");

const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const showCurrentPassword = ref(false);
const showNewPassword = ref(false);
const showConfirmPassword = ref(false);
const passwordChanged = ref(false);

const connectTab = ref<"webhook" | "mattermost" | "slack" | "agent">("webhook");

const connectTabItems: Tab<typeof connectTab.value>[] = [
  { id: "webhook", label: "Webhook" },
  { id: "mattermost", label: "Mattermost" },
  { id: "slack", label: "Slack" },
  { id: "agent", label: "Agent" },
];

const webhookUrl = computed(() => {
  const base = window.location.origin;
  return `${base}/webhooks/alerts`;
});

const passwordPolicyMet = computed(() => validatePassword(newPassword.value).valid);

const passwordsMatch = computed(() => {
  return newPassword.value === confirmPassword.value && confirmPassword.value.length > 0;
});

const canChangePassword = computed(() => {
  return currentPassword.value && passwordPolicyMet.value && passwordsMatch.value;
});

const stepLabels = ["Welcome", "Password", "Connect", "Done"];
const stepIcons = [Rocket, Shield, Link, CheckCircle];

async function changePassword() {
  if (!canChangePassword.value) return;
  error.value = "";
  loading.value = true;
  try {
    await auth.changePassword(currentPassword.value, newPassword.value);
    passwordChanged.value = true;
    push("Password changed successfully", "success");
  } catch (err) {
    error.value = getErrorMessage(err, "Failed to change password");
  } finally {
    loading.value = false;
  }
}

async function completeOnboarding() {
  loading.value = true;
  try {
    await api.completeOnboarding();
    auth.markOnboardingCompleted();
    router.push("/");
  } catch {
    push("Failed to complete setup", "error");
  } finally {
    loading.value = false;
  }
}

function nextStep() {
  if (step.value < totalSteps - 1) step.value++;
}

function prevStep() {
  if (step.value > 0) step.value--;
}
</script>

<template>
  <AuthLayout max-width="max-w-lg">
    <div class="mb-6">
      <div class="flex items-center justify-between">
        <div
          v-for="i in totalSteps"
          :key="i"
          class="flex items-center"
          :class="i < totalSteps ? 'flex-1' : ''"
        >
          <div
            class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold transition-colors duration-200"
            :class="
              i - 1 <= step
                ? 'bg-[var(--color-primary)] text-white'
                : 'bg-[var(--bg-secondary)] text-[var(--text-muted)]'
            "
          >
            <component :is="stepIcons[i - 1]" class="h-4 w-4" v-if="i - 1 < step" />
            <span v-else>{{ i }}</span>
          </div>
          <div
            v-if="i < totalSteps"
            class="mx-1 h-0.5 flex-1 transition-colors duration-200"
            :class="i <= step ? 'bg-[var(--color-primary)]' : 'bg-[var(--bg-secondary)]'"
          />
        </div>
      </div>
      <div class="mt-2 flex justify-between">
        <span
          v-for="(label, i) in stepLabels"
          :key="i"
          class="text-xs"
          :class="
            i === step ? 'text-[var(--text-primary)] font-medium' : 'text-[var(--text-muted)]'
          "
          >{{ label }}</span
        >
      </div>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="step === 0">
      <div class="mb-6 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-[var(--color-primary)]/10"
        >
          <Rocket class="h-8 w-8 text-[var(--color-primary)]" />
        </div>
        <h1 class="mb-2 text-xl font-semibold">Welcome to Alga</h1>
        <p class="text-sm text-[var(--text-muted)]">
          Your operations console for alert management, SRE agent investigations, and incident
          response. Let's get you set up in a few quick steps.
        </p>
      </div>
    </div>

    <div v-else-if="step === 1">
      <div class="mb-4">
        <h2 class="mb-1 text-lg font-semibold">Change Default Password</h2>
        <p class="text-sm text-[var(--text-muted)]">
          For security, please set a new password to replace the default admin credentials.
        </p>
      </div>

      <div v-if="passwordChanged" class="py-8 text-center">
        <CheckCircle class="mx-auto mb-2 h-12 w-12 text-[var(--text-success)]" />
        <p class="font-medium">Password updated</p>
      </div>

      <form v-else class="space-y-3" @submit.prevent="changePassword">
        <div>
          <label class="mb-1 block text-sm font-medium">Current Password</label>
          <div class="relative">
            <Input
              v-model="currentPassword"
              :type="showCurrentPassword ? 'text' : 'password'"
              autocomplete="current-password"
              required
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              @click="showCurrentPassword = !showCurrentPassword"
            >
              <Eye v-if="!showCurrentPassword" class="h-4 w-4" />
              <EyeOff v-else class="h-4 w-4" />
            </button>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium">New Password</label>
          <div class="relative">
            <Input
              v-model="newPassword"
              :type="showNewPassword ? 'text' : 'password'"
              autocomplete="new-password"
              required
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              @click="showNewPassword = !showNewPassword"
            >
              <Eye v-if="!showNewPassword" class="h-4 w-4" />
              <EyeOff v-else class="h-4 w-4" />
            </button>
          </div>
        </div>
        <div class="space-y-1 text-xs text-[var(--text-muted)]">
          <p
            :class="validatePassword(newPassword).checks.length ? 'text-[var(--text-success)]' : ''"
          >
            At least 8 characters
          </p>
          <p
            :class="
              validatePassword(newPassword).checks.uppercase ? 'text-[var(--text-success)]' : ''
            "
          >
            One uppercase letter
          </p>
          <p
            :class="
              validatePassword(newPassword).checks.lowercase ? 'text-[var(--text-success)]' : ''
            "
          >
            One lowercase letter
          </p>
          <p
            :class="validatePassword(newPassword).checks.digit ? 'text-[var(--text-success)]' : ''"
          >
            One digit
          </p>
          <p
            :class="
              validatePassword(newPassword).checks.special ? 'text-[var(--text-success)]' : ''
            "
          >
            One special character
          </p>
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium">Confirm New Password</label>
          <div class="relative">
            <Input
              v-model="confirmPassword"
              :type="showConfirmPassword ? 'text' : 'password'"
              autocomplete="new-password"
              required
              :error="confirmPassword && !passwordsMatch ? 'Passwords do not match' : ''"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]"
              @click="showConfirmPassword = !showConfirmPassword"
            >
              <Eye v-if="!showConfirmPassword" class="h-4 w-4" />
              <EyeOff v-else class="h-4 w-4" />
            </button>
          </div>
        </div>
        <Button type="submit" class="w-full" :loading="loading" :disabled="!canChangePassword">
          Update Password
        </Button>
      </form>
    </div>

    <div v-else-if="step === 2">
      <div class="mb-4">
        <h2 class="mb-1 text-lg font-semibold">Connect Your Tools</h2>
        <p class="text-sm text-[var(--text-muted)]">
          Configure integrations to start receiving alerts and dispatching investigations. You can
          also set these up later in Settings.
        </p>
      </div>

      <Tabs
        v-model="connectTab"
        :tabs="connectTabItems"
        aria-label="Connect tools"
        id-prefix="onboarding-connect"
      >
        <template #panel-webhook>
          <div class="space-y-3">
            <div
              class="flex items-start gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3"
            >
              <Link class="mt-0.5 h-5 w-5 shrink-0 text-[var(--text-muted)]" />
              <div class="min-w-0 flex-1">
                <p class="mb-1 text-sm font-medium">Grafana Webhook URL</p>
                <p class="mb-2 text-xs text-[var(--text-muted)]">
                  Add this URL as a webhook endpoint in Grafana alerting rules. Use a webhook token
                  for authentication.
                </p>
                <div
                  class="flex items-center gap-2 rounded border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2"
                >
                  <code class="min-w-0 flex-1 truncate text-xs">{{ webhookUrl }}</code>
                  <button
                    class="shrink-0 cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    @click="copyToClipboard(webhookUrl, 'Webhook URL copied')"
                  >
                    <Copy class="h-4 w-4" />
                  </button>
                </div>
              </div>
            </div>
            <p class="text-xs text-[var(--text-muted)]">
              Create a webhook token in <strong>Settings &rarr; Agents</strong> to authenticate
              incoming alerts.
            </p>
          </div>
        </template>

        <template #panel-mattermost>
          <div class="space-y-3">
            <div
              class="flex items-start gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3"
            >
              <MessageSquare class="mt-0.5 h-5 w-5 shrink-0 text-[var(--text-muted)]" />
              <div>
                <p class="mb-1 text-sm font-medium">Mattermost Integration</p>
                <p class="text-xs text-[var(--text-muted)]">
                  Configure the Mattermost server URL, webhook secret, team, and default channel in
                  <strong>Settings &rarr; Integrations</strong>. Alga routes alerts to the
                  configured channels and syncs thread replies bidirectionally.
                </p>
              </div>
            </div>
            <p class="text-xs text-[var(--text-muted)]">
              Install the Alga Mattermost plugin for bidirectional investigation sync. See the
              <code>integrations/mattermost-plugin</code> directory for installation instructions.
            </p>
          </div>
        </template>

        <template #panel-slack>
          <div class="space-y-3">
            <div
              class="flex items-start gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3"
            >
              <MessageSquare class="mt-0.5 h-5 w-5 shrink-0 text-[var(--text-muted)]" />
              <div>
                <p class="mb-1 text-sm font-medium">Slack Integration</p>
                <p class="text-xs text-[var(--text-muted)]">
                  Provide a Slack bot token and signing secret in
                  <strong>Settings &rarr; Integrations</strong>. Alga posts alerts to Slack channels
                  and receives events via the Slack Events API.
                </p>
              </div>
            </div>
            <p class="text-xs text-[var(--text-muted)]">
              See <code>integrations/slack-app</code> for the Slack app manifest.
            </p>
          </div>
        </template>

        <template #panel-agent>
          <div class="space-y-3">
            <div
              class="flex items-start gap-3 rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-3"
            >
              <Bot class="mt-0.5 h-5 w-5 shrink-0 text-[var(--text-muted)]" />
              <div>
                <p class="mb-1 text-sm font-medium">SRE Agent</p>
                <p class="text-xs text-[var(--text-muted)]">
                  Create an agent token in <strong>Settings &rarr; Agents</strong> to connect a
                  Hermes or OpenClaw SRE agent. Agents connect via SSE and receive investigation
                  dispatches automatically.
                </p>
              </div>
            </div>
            <p class="text-xs text-[var(--text-muted)]">
              The agent uses the token to authenticate to the SSE stream at
              <code>/api/v1/agent/events</code> and the REST API at <code>/api/v1/agent/*</code>.
            </p>
          </div>
        </template>
      </Tabs>
    </div>

    <div v-else-if="step === 3">
      <div class="py-8 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-[var(--bg-badge-resolved)]"
        >
          <CheckCircle class="h-8 w-8 text-[var(--text-success)]" />
        </div>
        <h2 class="mb-2 text-xl font-semibold">You're all set!</h2>
        <p class="mb-6 text-sm text-[var(--text-muted)]">
          Alga is ready to receive alerts and dispatch investigations. Head to the dashboard to get
          started, or configure additional settings later.
        </p>
        <Button class="w-full" :loading="loading" @click="completeOnboarding">
          Go to Dashboard
        </Button>
      </div>
    </div>

    <div
      v-if="step < 3 && !(step === 1 && !passwordChanged)"
      class="mt-6 flex items-center justify-between"
    >
      <Button v-if="step > 0" variant="outline" @click="prevStep">
        <ArrowLeft class="h-4 w-4" />
        Back
      </Button>
      <div v-else />
      <Button v-if="step < 2" @click="nextStep">
        Next
        <ArrowRight class="h-4 w-4" />
      </Button>
      <Button v-else-if="step === 2" @click="nextStep">
        Finish
        <ArrowRight class="h-4 w-4" />
      </Button>
    </div>

    <div v-if="step === 1 && !passwordChanged" class="mt-6 flex items-center justify-between">
      <Button variant="outline" @click="prevStep">
        <ArrowLeft class="h-4 w-4" />
        Back
      </Button>
      <Button variant="outline" @click="nextStep">
        Skip for now
        <ArrowRight class="h-4 w-4" />
      </Button>
    </div>
  </AuthLayout>
</template>
