class AlgaError(Exception):
    pass


class AlgaAuthError(AlgaError):
    def __init__(self, status_code: int, message: str = ""):
        self.status_code = status_code
        self.message = message
        super().__init__(f"Auth error {status_code}: {message}")


class AlgaAPIError(AlgaError):
    def __init__(self, status_code: int, message: str = ""):
        self.status_code = status_code
        self.message = message
        super().__init__(f"API error {status_code}: {message}")


class AlgaConnectionError(AlgaError):
    def __init__(self, message: str = ""):
        self.message = message
        super().__init__(message)
