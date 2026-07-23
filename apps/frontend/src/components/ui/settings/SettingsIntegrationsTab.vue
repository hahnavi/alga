<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";
import { Hash, Mail } from "@lucide/vue";
import { useOAuthPopup } from "@/composables/useOAuthPopup";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import Button from "@/components/ui/Button.vue";

defineProps<{ onClose: () => void }>();

const auth = useAuthStore();

const slackAuthorizing = ref(false);
const slackDisconnecting = ref(false);
const slackError = ref("");
const googleAuthorizing = ref(false);
const googleDisconnecting = ref(false);
const googleError = ref("");

const userSlackOAuth = useOAuthPopup();
const userGoogleOAuth = useOAuthPopup();

async function startUserSlackOAuth() {
  slackError.value = "";
  try {
    const { url } = await api.initiateUserSlackOAuth();
    userSlackOAuth.open(
      url,
      () => {
        slackAuthorizing.value = false;
        void auth.fetchCurrentUser();
      },
      "slack-connect",
    );
    slackAuthorizing.value = true;
  } catch (err) {
    slackError.value = getErrorMessage(err, "Failed to start Slack connection");
  }
}

async function disconnectUserSlack() {
  slackError.value = "";
  slackDisconnecting.value = true;
  try {
    await api.disconnectUserSlack();
    await auth.fetchCurrentUser();
  } catch (err) {
    slackError.value = getErrorMessage(err, "Failed to disconnect Slack");
  } finally {
    slackDisconnecting.value = false;
  }
}

async function startUserGoogleOAuth() {
  googleError.value = "";
  try {
    const { url } = await api.initiateUserGoogleOAuth();
    userGoogleOAuth.open(
      url,
      () => {
        googleAuthorizing.value = false;
        void auth.fetchCurrentUser();
      },
      "google-connect",
    );
    googleAuthorizing.value = true;
  } catch (err) {
    googleError.value = getErrorMessage(err, "Failed to start Google connection");
  }
}

async function disconnectUserGoogle() {
  googleError.value = "";
  googleDisconnecting.value = true;
  try {
    await api.disconnectUserGoogle();
    await auth.fetchCurrentUser();
  } catch (err) {
    googleError.value = getErrorMessage(err, "Failed to disconnect Google account");
  } finally {
    googleDisconnecting.value = false;
  }
}

onBeforeUnmount(() => {
  userSlackOAuth.cleanup();
  userGoogleOAuth.cleanup();
});
</script>

<template>
  <p class="text-sm text-[var(--text-muted)]">Slack</p>
  <div v-if="!auth.user?.slack_linked" class="rounded-lg border border-[var(--border-primary)] p-4">
    <div class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-tertiary,rgb(148_163_184/0.1))]"
      >
        <Hash class="h-4 w-4 text-[var(--text-muted)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Connect your Slack account to be automatically invited to incident channels.
        </p>
        <p v-if="slackError" class="mt-1 text-xs text-[var(--text-error)]">{{ slackError }}</p>
        <Button class="mt-2.5" :disabled="slackAuthorizing" @click="startUserSlackOAuth">
          {{ slackAuthorizing ? "Opening Slack..." : "Connect Slack" }}
        </Button>
      </div>
    </div>
  </div>

  <div v-else class="rounded-lg border border-[var(--border-primary)] p-4">
    <div class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--text-success,rgb(34_197_94/0.1))]"
      >
        <Hash class="h-4 w-4 text-[var(--text-success)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Connected as
          <span class="font-medium">@{{ auth.user.slack_display_name }}</span>
        </p>
        <p v-if="slackError" class="mt-1 text-xs text-[var(--text-error)]">{{ slackError }}</p>
        <Button
          variant="outline"
          class="mt-2.5"
          :disabled="slackDisconnecting"
          @click="disconnectUserSlack"
        >
          {{ slackDisconnecting ? "Disconnecting..." : "Disconnect" }}
        </Button>
      </div>
    </div>
  </div>

  <hr class="border-[var(--border-primary)]" />

  <p class="text-sm text-[var(--text-muted)]">Google</p>
  <div
    v-if="!auth.user?.google_linked"
    class="rounded-lg border border-[var(--border-primary)] p-4"
  >
    <div class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--bg-tertiary,rgb(148_163_184/0.1))]"
      >
        <Mail class="h-4 w-4 text-[var(--text-muted)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Bind your Google account so you can sign in with Google using your
          <span class="font-medium">{{ auth.user?.email }}</span> address.
        </p>
        <p v-if="googleError" class="mt-1 text-xs text-[var(--text-error)]">{{ googleError }}</p>
        <Button class="mt-2.5" :disabled="googleAuthorizing" @click="startUserGoogleOAuth">
          {{ googleAuthorizing ? "Opening Google..." : "Bind Google account" }}
        </Button>
      </div>
    </div>
  </div>

  <div v-else class="rounded-lg border border-[var(--border-primary)] p-4">
    <div class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[var(--text-success,rgb(34_197_94/0.1))]"
      >
        <Mail class="h-4 w-4 text-[var(--text-success)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Bound to <span class="font-medium">{{ auth.user.email }}</span>
        </p>
        <p class="text-xs text-[var(--text-muted)]">You can sign in with this Google account.</p>
        <p v-if="googleError" class="mt-1 text-xs text-[var(--text-error)]">{{ googleError }}</p>
        <Button
          variant="outline"
          class="mt-2.5"
          :disabled="googleDisconnecting"
          @click="disconnectUserGoogle"
        >
          {{ googleDisconnecting ? "Unbinding..." : "Unbind" }}
        </Button>
      </div>
    </div>
  </div>
</template>
