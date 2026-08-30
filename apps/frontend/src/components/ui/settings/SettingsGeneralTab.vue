<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useAuthStore } from "@/stores/auth";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import PhoneInput from "@/components/ui/PhoneInput.vue";

const auth = useAuthStore();
const { push } = useToast();

const editFullName = ref("");
const editPhone = ref("");
const editPhoneCountry = ref("");
const editEmail = ref("");
const emailPassword = ref("");

const savingProfile = ref(false);
const changingEmail = ref(false);

const profileError = ref("");
const emailError = ref("");

function syncFromUser() {
  editFullName.value = auth.user?.full_name ?? "";
  editPhone.value = auth.user?.phone ?? "";
  editPhoneCountry.value = auth.user?.phone_country ?? "";
  editEmail.value = auth.user?.email ?? "";
}

// Refetches (OAuth link callbacks, re-login) replace the user object; only
// re-sync while no field has been edited so unsaved input is not clobbered.
const profileDirty = computed(
  () =>
    editFullName.value !== (auth.user?.full_name ?? "") ||
    editPhone.value !== (auth.user?.phone ?? "") ||
    editPhoneCountry.value !== (auth.user?.phone_country ?? ""),
);

watch(
  () => auth.user,
  () => {
    if (!profileDirty.value) syncFromUser();
  },
  { immediate: true },
);

onMounted(syncFromUser);

async function saveProfile() {
  profileError.value = "";
  savingProfile.value = true;
  try {
    await auth.updateProfile(editFullName.value.trim(), editPhone.value, editPhoneCountry.value);
    push("Profile updated successfully.", "success");
  } catch (err) {
    profileError.value = getErrorMessage(err, "Failed to update profile");
  } finally {
    savingProfile.value = false;
  }
}

async function saveEmail() {
  emailError.value = "";
  if (!emailPassword.value) {
    emailError.value = "Password is required to change email.";
    return;
  }
  changingEmail.value = true;
  try {
    await auth.changeEmail(emailPassword.value, editEmail.value.trim());
    push("Email updated successfully.", "success");
    emailPassword.value = "";
  } catch (err) {
    emailError.value = getErrorMessage(err, "Failed to update email");
  } finally {
    changingEmail.value = false;
  }
}
</script>

<template>
  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Profile</h3>
      <p class="text-xs text-[var(--text-muted)]">Your name and phone number.</p>
    </header>
    <div class="space-y-1.5">
      <FormLabel for="settings-full-name">Full name</FormLabel>
      <Input id="settings-full-name" v-model="editFullName" placeholder="Jane Doe" />
    </div>
    <div class="space-y-1.5">
      <FormLabel for="settings-phone">Phone number</FormLabel>
      <PhoneInput id="settings-phone" v-model="editPhone" v-model:country="editPhoneCountry" />
    </div>
    <p v-if="profileError" class="text-xs text-[var(--text-error)]" role="alert">
      {{ profileError }}
    </p>
    <div class="flex justify-end">
      <Button :loading="savingProfile" @click="saveProfile">Save profile</Button>
    </div>
  </Card>

  <Card class="space-y-4">
    <header>
      <h3 class="text-sm font-semibold text-[var(--text-primary)]">Email</h3>
      <p class="text-xs text-[var(--text-muted)]">
        Changing your email requires your current password.
      </p>
    </header>
    <div class="space-y-1.5">
      <FormLabel for="settings-email">Email address</FormLabel>
      <Input id="settings-email" v-model="editEmail" type="email" placeholder="you@example.com" />
    </div>
    <div class="space-y-1.5">
      <FormLabel for="settings-email-password">Confirm password</FormLabel>
      <Input id="settings-email-password" v-model="emailPassword" type="password" />
    </div>
    <p v-if="emailError" class="text-xs text-[var(--text-error)]" role="alert">
      {{ emailError }}
    </p>
    <div class="flex justify-end">
      <Button :loading="changingEmail" @click="saveEmail">Update email</Button>
    </div>
  </Card>
</template>
