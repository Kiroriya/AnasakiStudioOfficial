import { ApiToken, ApiTokenCreate } from '../core/models';

/** Pure helpers of the "API tokens" section (unit tested in api-tokens-utils.spec.ts). */

/** Expiry choices in days (0 = never), in menu order; 90 is the default. */
export const TOKEN_EXPIRY_DAYS = [30, 90, 365, 0] as const;
export type TokenExpiry = (typeof TOKEN_EXPIRY_DAYS)[number];
export const DEFAULT_TOKEN_EXPIRY: TokenExpiry = 90;
/** Scopes a token can have (the CLI needs `code`). */
export const TOKEN_SCOPES = ['code'] as const;
export const TOKEN_NAME_MAX = 100;

/** Message key of what is wrong with the form, or '' when it can be sent. */
export function tokenFormError(name: string, scopes: readonly string[]): string {
  const n = name.trim();
  if (!n) return 'apitok.err.name';
  if (n.length > TOKEN_NAME_MAX) return 'apitok.err.nameLong';
  if (!scopes.length) return 'apitok.err.scope';
  return '';
}

/** Body of POST /auth/tokens. */
export function tokenCreateBody(name: string, expiresInDays: number, scopes: readonly string[]): ApiTokenCreate {
  const days = (TOKEN_EXPIRY_DAYS as readonly number[]).includes(expiresInDays) ? expiresInDays : DEFAULT_TOKEN_EXPIRY;
  return { name: name.trim(), scopes: TOKEN_SCOPES.filter(s => scopes.includes(s)), expiresInDays: days };
}

/** The command that signs the CLI in with a new token. */
export function cliLoginCommand(origin: string, token: string): string {
  return `npx anasaki-code login ${origin.replace(/\/+$/, '')} --token ${token}`;
}

/** GET /auth/tokens answers `{items: [...]}` (a bare array is accepted too); newest first. */
export function normalizeTokens(res: { items?: ApiToken[] | null } | ApiToken[] | null | undefined): ApiToken[] {
  const list = Array.isArray(res) ? res : res?.items ?? [];
  return [...list].sort((a, b) => (b.createdAt ?? '').localeCompare(a.createdAt ?? '') || b.id - a.id);
}

/** Has the token expired at `now`? (no expiry = never) */
export function tokenExpired(t: Pick<ApiToken, 'expiresAt'>, now = Date.now()): boolean {
  if (!t.expiresAt) return false;
  const at = Date.parse(t.expiresAt);
  return !Number.isNaN(at) && at <= now;
}
