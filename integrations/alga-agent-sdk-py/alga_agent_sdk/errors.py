from __future__ import annotations

from datetime import timedelta


class AlgaError(Exception):
    pass


class AlgaAuthError(AlgaError):
    status_code: int
    message: str

    def __init__(self, status_code: int, message: str = ""):
        self.status_code = status_code
        self.message = message
        super().__init__(f"auth error {status_code}: {message}")


class AlgaAPIError(AlgaError):
    status_code: int
    message: str
    retry_after: timedelta

    def __init__(
        self,
        status_code: int,
        message: str = "",
        retry_after: timedelta = timedelta(0),
    ):
        self.status_code = status_code
        self.message = message
        self.retry_after = retry_after
        if retry_after > timedelta(0):
            super().__init__(
                f"api error {status_code}: {message} (retry after {retry_after})"
            )
        else:
            super().__init__(f"api error {status_code}: {message}")

    def is_retryable(self) -> bool:
        return self.status_code in (429, 500, 502, 503, 504)


class AlgaConnectionError(AlgaError):
    message: str

    def __init__(self, message: str = ""):
        self.message = message
        super().__init__(message)

    def is_retryable(self) -> bool:
        return True


def is_auth_error(err: object) -> bool:
    return isinstance(err, AlgaAuthError)


def is_retryable_error(err: object) -> bool:
    if isinstance(err, AlgaAPIError):
        return err.is_retryable()
    if isinstance(err, AlgaConnectionError):
        return err.is_retryable()
    return False
