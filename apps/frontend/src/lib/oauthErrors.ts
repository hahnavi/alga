/**
 * Maps backend OAuth callback error codes to user-friendly messages.
 * The keys are bounded by the OIDC/Slack/Google error catalog on the backend.
 * Anything else arriving in ?error= is treated as unknown and silently dropped
 * (the user gets a generic message and a console hint) so we never reflect
 * untrusted input into the UI.
 */

const MAX_ERROR_KEY_LENGTH = 64;

const GOOGLE_ERROR_MESSAGES: Record<string, string> = {
  google_no_account: "No Alga account found for this Google email. Contact your administrator.",
  google_email_not_verified: "Your Google email is not verified. Verify it with Google first.",
  google_account_locked: "Your account is locked. Contact your administrator.",
  google_invalid_state: "Authentication session expired. Please try again.",
  google_auth_failed: "Google authentication failed. Please try again.",
  google_not_configured: "Google Sign-In is not configured.",
};

const SLACK_ERROR_MESSAGES: Record<string, string> = {
  slack_no_account: "No Alga account found for this Slack user. Contact your administrator.",
  slack_account_locked: "Your account is locked. Contact your administrator.",
  slack_invalid_state: "Authentication session expired. Please try again.",
  slack_auth_failed: "Slack authentication failed. Please try again.",
  slack_not_configured: "Slack Sign-In is not configured.",
};

const OIDC_ERROR_MESSAGES: Record<string, string> = {
  oidc_no_account: "No Alga account found for this email. Contact your administrator.",
  oidc_account_locked: "Your account is locked. Contact your administrator.",
  oidc_no_email: "The identity provider did not return an email address.",
  oidc_invalid_state: "Authentication session expired. Please try again.",
  oidc_auth_failed: "SSO authentication failed. Please try again.",
  oidc_not_configured: "SSO is not configured.",
  oidc_token_exchange_failed: "SSO token exchange failed. Please try again.",
  oidc_userinfo_failed: "Failed to retrieve user info from the identity provider.",
  oidc_discovery_failed: "Failed to contact the identity provider.",
  oidc_provider_not_found: "SSO provider not found or disabled.",
};

const ALL_MESSAGES: Record<string, string> = {
  ...GOOGLE_ERROR_MESSAGES,
  ...SLACK_ERROR_MESSAGES,
  ...OIDC_ERROR_MESSAGES,
};

const GENERIC_FALLBACK = "Sign-in failed. Please try again.";

/**
 * Resolve a `?error=` query value from an OAuth callback. Returns the
 * user-visible message, or `null` if the value is empty / malformed / unknown.
 * Caps the input length defensively and never reflects arbitrary strings.
 */
export function resolveOAuthErrorMessage(raw: unknown): string | null {
  if (typeof raw !== "string") return null;
  if (raw.length === 0 || raw.length > MAX_ERROR_KEY_LENGTH) return null;
  if (!/^[a-z_][a-z0-9_]*$/i.test(raw)) return null;
  if (raw in ALL_MESSAGES) return ALL_MESSAGES[raw];
  if (raw.startsWith("oidc_")) return "SSO authentication failed. Please try again.";
  return null;
}

/** Returns a generic message when the error key was rejected. */
export function unknownOAuthErrorMessage(): string {
  return GENERIC_FALLBACK;
}
