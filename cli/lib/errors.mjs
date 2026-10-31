// CliError carries a user-facing message and an exit code
// (0 ok, 1 error, 2 conflict / validation).
export class CliError extends Error {
  constructor(message, code = 1) {
    super(message);
    this.exitCode = code;
  }
}

export const EXIT_OK = 0;
export const EXIT_ERROR = 1;
export const EXIT_CONFLICT = 2;
