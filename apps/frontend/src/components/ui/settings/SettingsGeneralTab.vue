<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useAuthStore } from "@/stores/auth";
import { getErrorMessage } from "@/lib/error";
import Button from "@/components/ui/Button.vue";
import Input from "@/components/ui/Input.vue";
import FormLabel from "@/components/ui/FormLabel.vue";
import PhoneInput from "@/components/ui/PhoneInput.vue";

const auth = useAuthStore();

const editFullName = ref("");
const editPhone = ref("");
const editPhoneCountry = ref("");
const editEmail = ref("");
const emailPassword = ref("");

const savingName = ref(false);
const savingPhone = ref(false);
const changingEmail = ref(false);

const nameMessage = ref("");
const nameError = ref("");
const phoneMessage = ref("");
const phoneError = ref("");
const emailMessage = ref("");
const emailError = ref("");

function syncFromUser() {
  editFullName.value = auth.user?.full_name ?? "";
  editPhone.value = auth.user?.phone ?? "";
  editPhoneCountry.value = auth.user?.phone_country ?? "";
  editEmail.value = auth.user?.email ?? "";
}

watch(
  () => auth.user,
  () => syncFromUser(),
  { immediate: true },
);

onMounted(syncFromUser);

async function saveName() {
  nameMessage.value = "";
  nameError.value = "";
  savingName.value = true;
  try {
    await auth.updateProfile(editFullName.value.trim(), editPhone.value, editPhoneCountry.value);
    nameMessage.value = "Name updated successfully.";
  } catch (err) {
    nameError.value = getErrorMessage(err, "Failed to update name");
  } finally {
    savingName.value = false;
  }
}

async function savePhone() {
  phoneMessage.value = "";
  phoneError.value = "";
  savingPhone.value = true;
  try {
    await auth.updateProfile(editFullName.value.trim(), editPhone.value, editPhoneCountry.value);
    phoneMessage.value = "Phone updated successfully.";
  } catch (err) {
    phoneError.value = getErrorMessage(err, "Failed to update phone");
  } finally {
    savingPhone.value = false;
  }
}

async function saveEmail() {
  emailMessage.value = "";
  emailError.value = "";
  if (!emailPassword.value) {
    emailError.value = "Password is required to change email.";
    return;
  }
  changingEmail.value = true;
  try {
    await auth.changeEmail(emailPassword.value, editEmail.value.trim());
    emailMessage.value = "Email updated successfully.";
    emailPassword.value = "";
  } catch (err) {
    emailError.value = getErrorMessage(err, "Failed to update email");
  } finally {
    changingEmail.value = false;
  }
}
</script>

<template>
  <p class="text-sm text-[var(--text-muted)]">Profile</p>
  <div class="space-y-1.5">
    <FormLabel>Full name</FormLabel>
    <Input v-model="editFullName" placeholder="Jane Doe" />
  </div>
  <p v-if="nameMessage" class="text-xs text-[var(--text-success)]">{{ nameMessage }}</p>
  <p v-if="nameError" class="text-xs text-[var(--text-error)]">{{ nameError }}</p>
  <Button :disabled="savingName" @click="saveName">
    {{ savingName ? "Saving..." : "Save name" }}
  </Button>

  <hr class="border-[var(--border-primary)]" />

  <p class="text-sm text-[var(--text-muted)]">Phone</p>
  <div class="space-y-1.5">
    <FormLabel>Phone number</FormLabel>
    <PhoneInput v-model="editPhone" v-model:country="editPhoneCountry" />
  </div>
  <p v-if="phoneMessage" class="text-xs text-[var(--text-success)]">{{ phoneMessage }}</p>
  <p v-if="phoneError" class="text-xs text-[var(--text-error)]">{{ phoneError }}</p>
  <Button :disabled="savingPhone" @click="savePhone">
    {{ savingPhone ? "Saving..." : "Save phone" }}
  </Button>

  <hr class="border-[var(--border-primary)]" />

  <p class="text-sm text-[var(--text-muted)]">Email</p>
  <div class="space-y-1.5">
    <FormLabel>Email address</FormLabel>
    <Input v-model="editEmail" type="email" placeholder="you@example.com" />
  </div>
  <div class="space-y-1.5">
    <FormLabel>Confirm password</FormLabel>
    <Input v-model="emailPassword" type="password" />
  </div>
  <p v-if="emailError" class="text-xs text-[var(--text-error)]">{{ emailError }}</p>
  <p v-if="emailMessage" class="text-xs text-[var(--text-success)]">{{ emailMessage }}</p>
  <Button :disabled="changingEmail" @click="saveEmail">
    {{ changingEmail ? "Updating..." : "Update email" }}
  </Button>
</template>
