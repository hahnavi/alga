const PASSWORD_SPECIAL_REGEX = /[^A-Za-z0-9]/;

type PasswordPolicyResult = {
  valid: boolean;
  error: string;
  checks: {
    length: boolean;
    uppercase: boolean;
    lowercase: boolean;
    digit: boolean;
    special: boolean;
  };
};

export function validatePassword(password: string): PasswordPolicyResult {
  const checks = {
    length: password.length >= 8,
    uppercase: /[A-Z]/.test(password),
    lowercase: /[a-z]/.test(password),
    digit: /[0-9]/.test(password),
    special: PASSWORD_SPECIAL_REGEX.test(password),
  };

  const valid =
    checks.length && checks.uppercase && checks.lowercase && checks.digit && checks.special;

  let error = "";
  if (!checks.length) error = "At least 8 characters";
  else if (!checks.uppercase) error = "One uppercase letter required";
  else if (!checks.lowercase) error = "One lowercase letter required";
  else if (!checks.digit) error = "One digit required";
  else if (!checks.special) error = "One special character required";

  return { valid, error, checks };
}
