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

export class AlgaAPIError extends AlgaError {
  statusCode: number;

  constructor(statusCode: number, message: string) {
    super(message);
    this.name = "AlgaAPIError";
    this.statusCode = statusCode;
  }
}

export class AlgaConnectionError extends AlgaError {
  constructor(message: string) {
    super(message);
    this.name = "AlgaConnectionError";
  }
}
