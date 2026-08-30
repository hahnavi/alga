<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";
import { Hash, Mail } from "@lucide/vue";
import { useOAuthPopup } from "@/composables/useOAuthPopup";
import { useAuthStore } from "@/stores/auth";
import { api } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";

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
  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Slack</h3>
      <p class="text-xs text-[var(--text-muted)]">
        Connect your Slack account to be automatically invited to incident channels.
      </p>
    </header>
    <div v-if="!auth.user?.slack_linked" class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[rgb(148_163_184/0.1)]"
      >
        <Hash class="h-4 w-4 text-[var(--text-muted)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">Not connected.</p>
        <p v-if="slackError" class="mt-1 text-xs text-[var(--text-error)]" role="alert">
          {{ slackError }}
        </p>
        <Button
          class="mt-2.5"
          :loading="slackAuthorizing"
          :disabled="slackAuthorizing"
          @click="startUserSlackOAuth"
        >
          Connect Slack
        </Button>
      </div>
    </div>
    <div v-else class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[rgb(34_197_94/0.1)]"
      >
        <Hash class="h-4 w-4 text-[var(--text-success)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Connected as
          <span class="font-medium">@{{ auth.user.slack_display_name }}</span>
        </p>
        <p v-if="slackError" class="mt-1 text-xs text-[var(--text-error)]" role="alert">
          {{ slackError }}
        </p>
        <Button
          variant="outline"
          class="mt-2.5"
          :loading="slackDisconnecting"
          :disabled="slackDisconnecting"
          @click="disconnectUserSlack"
        >
          Disconnect
        </Button>
      </div>
    </div>
  </Card>

  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Google</h3>
      <p class="text-xs text-[var(--text-muted)]">
        Bind your Google account to sign in with Google using your email address.
      </p>
    </header>
    <div v-if="!auth.user?.google_linked" class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[rgb(148_163_184/0.1)]"
      >
        <Mail class="h-4 w-4 text-[var(--text-muted)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Not connected to
          <span class="font-medium">{{ auth.user?.email }}</span>
          .
        </p>
        <p v-if="googleError" class="mt-1 text-xs text-[var(--text-error)]" role="alert">
          {{ googleError }}
        </p>
        <Button
          class="mt-2.5"
          :loading="googleAuthorizing"
          :disabled="googleAuthorizing"
          @click="startUserGoogleOAuth"
        >
          Bind Google account
        </Button>
      </div>
    </div>
    <div v-else class="flex items-start gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[rgb(34_197_94/0.1)]"
      >
        <Mail class="h-4 w-4 text-[var(--text-success)]" />
      </div>
      <div class="flex-1">
        <p class="text-sm text-[var(--text-primary)]">
          Bound to <span class="font-medium">{{ auth.user.email }}</span>
        </p>
        <p class="text-xs text-[var(--text-muted)]">You can sign in with this Google account.</p>
        <p v-if="googleError" class="mt-1 text-xs text-[var(--text-error)]" role="alert">
          {{ googleError }}
        </p>
        <Button
          variant="outline"
          class="mt-2.5"
          :loading="googleDisconnecting"
          :disabled="googleDisconnecting"
          @click="disconnectUserGoogle"
        >
          Unbind
        </Button>
      </div>
    </div>
  </Card>
</template>
