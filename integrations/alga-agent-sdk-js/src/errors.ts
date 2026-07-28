// AlgaAuthError is thrown for 401/403 responses. Auth errors are never
// retryable — the token must be valid (not revoked, not expired) before
// retrying.
export class AlgaError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AlgaError";
  }
}

export class AlgaAuthError extends AlgaError {
  statusCode: number;

  constructor(statusCode: number, message: string) {
    super(message);
    this.name = "AlgaAuthError";
    this.statusCode = statusCode;
  }
}

// AlgaAPIError is thrown for any non-2xx, non-auth response. retryAfterMs is
// populated from the `Retry-After` header when present (notably for 429s).
export class AlgaAPIError extends AlgaError {
  statusCode: number;
  retryAfterMs: number;

  constructor(statusCode: number, message: string, retryAfterMs = 0) {
    super(message);
    this.name = "AlgaAPIError";
    this.statusCode = statusCode;
    this.retryAfterMs = retryAfterMs;
  }

  // isRetryable reports whether the error is worth retrying per the HTTP
  // status code (429, 500, 502, 503, 504). 4xx errors other than 429 are
  // considered permanent.
  isRetryable(): boolean {
    return (
      this.statusCode === 429 ||
      this.statusCode === 500 ||
      this.statusCode === 502 ||
      this.statusCode === 503 ||
      this.statusCode === 504
    );
  }
}

// AlgaConnectionError wraps a transport-level error (DNS, TCP, TLS, timeout).
export class AlgaConnectionError extends AlgaError {
  constructor(message: string) {
    super(message);
    this.name = "AlgaConnectionError";
  }

  isRetryable(): boolean {
    return true;
  }
}

// isAuthError reports whether err is an AlgaAuthError.
export function isAuthError(err: unknown): boolean {
  return err instanceof AlgaAuthError;
}

// isRetryableError reports whether err is a retryable SDK error.
export function isRetryableError(err: unknown): boolean {
  if (err instanceof AlgaAPIError) return err.isRetryable();
  if (err instanceof AlgaConnectionError) return err.isRetryable();
  return false;
}
